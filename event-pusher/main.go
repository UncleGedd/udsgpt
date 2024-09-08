package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type EventStore struct {
	mu     sync.Mutex
	events map[string]*v1.Event
}

func NewEventStore() *EventStore {
	return &EventStore{
		events: make(map[string]*v1.Event),
	}
}

func (es *EventStore) AddOrUpdate(event *v1.Event) {
	es.mu.Lock()
	defer es.mu.Unlock()
	key := fmt.Sprintf("%s/%s/%s", event.Namespace, event.InvolvedObject.Kind, event.InvolvedObject.Name)
	es.events[key] = event
	log.Printf("Event added/updated: %s", key)
}

func (es *EventStore) GetAndClear() []*v1.Event {
	es.mu.Lock()
	defer es.mu.Unlock()
	events := make([]*v1.Event, 0, len(es.events))
	for _, event := range es.events {
		events = append(events, event)
	}
	count := len(events)
	es.events = make(map[string]*v1.Event)
	log.Printf("Retrieved and cleared %d events from store", count)
	return events
}

type LokiClient struct {
	URL        string
	HTTPClient *http.Client
}

func NewLokiClient(url string) *LokiClient {
	return &LokiClient{
		URL: url,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("Starting Kubernetes Event Collector")

	// Set up Kubernetes client using local kubeconfig
	log.Println("Setting up Kubernetes client using local kubeconfig...")
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to get kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	log.Println("Kubernetes client set up successfully")

	lokiURL := "http://localhost:8080/loki/api/v1/push" // Adjust this URL as needed
	log.Printf("Setting up Loki client with URL: %s", lokiURL)
	lokiClient := NewLokiClient(lokiURL)
	eventStore := NewEventStore()

	// Start event watcher
	log.Println("Starting event watcher...")
	go watchEvents(clientset, eventStore)

	// Start continuous pusher
	log.Println("Starting continuous event pusher...")
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		pushEventsWithRetry(lokiClient, eventStore)
	}
}

func watchEvents(clientset *kubernetes.Clientset, eventStore *EventStore) {
	for {
		log.Println("Setting up event watcher...")
		watcher, err := clientset.CoreV1().Events("").Watch(context.Background(), metav1.ListOptions{})
		if err != nil {
			log.Printf("Failed to create event watcher: %v", err)
			log.Println("Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Event watcher started successfully")

		for event := range watcher.ResultChan() {
			if event.Type == watch.Error {
				log.Printf("Error watching events: %v", event.Object)
				continue
			}

			k8sEvent, ok := event.Object.(*v1.Event)
			if !ok {
				log.Printf("Unexpected event type: %T", event.Object)
				continue
			}

			log.Printf("Received event: %s/%s/%s", k8sEvent.Namespace, k8sEvent.InvolvedObject.Kind, k8sEvent.Name)
			eventStore.AddOrUpdate(k8sEvent)
		}

		log.Println("Event watcher closed. Restarting...")
	}
}

func pushEventsWithRetry(lokiClient *LokiClient, eventStore *EventStore) {
	events := eventStore.GetAndClear()
	if len(events) == 0 {
		log.Println("No events to push")
		return
	}

	log.Printf("Preparing to push %d events to Loki", len(events))

	maxRetries := 5
	baseDelay := time.Second

	for retries := 0; retries < maxRetries; retries++ {
		err := pushEvents(lokiClient, events)
		if err == nil {
			log.Printf("Successfully pushed %d events to Loki", len(events))
			return
		}

		delay := baseDelay * time.Duration(1<<uint(retries))
		log.Printf("Failed to push events to Loki (attempt %d/%d): %v. Retrying in %v...", retries+1, maxRetries, err, delay)
		time.Sleep(delay)
	}

	log.Printf("Failed to push events to Loki after %d attempts. Events will be lost.", maxRetries)
}

func pushEvents(lokiClient *LokiClient, events []*v1.Event) error {
	streams := []map[string]interface{}{}

	for _, event := range events {
		stream := map[string]interface{}{
			"stream": map[string]string{
				"job":       "kubernetes-events",
				"namespace": event.Namespace,
				"name":      event.Name,
				"kind":      event.InvolvedObject.Kind,
				"reason":    event.Reason,
			},
			"values": [][]string{
				{
					getTimestamp(event),
					fmt.Sprintf("Event: %s, Message: %s", event.Reason, event.Message),
				},
			},
		}
		streams = append(streams, stream)
	}

	payload := map[string]interface{}{
		"streams": streams,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	log.Printf("Sending request to Loki: %s", lokiClient.URL)
	resp, err := lokiClient.HTTPClient.Post(lokiClient.URL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to send events to Loki: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code from Loki: %d, Body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func getTimestamp(event *v1.Event) string {
	var timestamp time.Time
	if !event.EventTime.IsZero() {
		timestamp = event.EventTime.Time
	} else if !event.LastTimestamp.IsZero() {
		timestamp = event.LastTimestamp.Time
	} else {
		timestamp = time.Now()
	}
	return fmt.Sprintf("%d", timestamp.UnixNano())
}
