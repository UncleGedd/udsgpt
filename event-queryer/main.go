package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type LokiResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func queryLoki(query string, start, end time.Time) (*LokiResponse, error) {
	lokiURL := "http://localhost:8080" // Replace with your Loki URL

	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", start.UnixNano()))
	params.Add("end", fmt.Sprintf("%d", end.UnixNano()))

	resp, err := http.Get(fmt.Sprintf("%s/loki/api/v1/query_range?%s", lokiURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error querying Loki: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var lokiResp LokiResponse
	err = json.Unmarshal(body, &lokiResp)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}

	return &lokiResp, nil
}

func main() {
	// Example usage
	end := time.Now()
	start := end.Add(-1 * time.Hour) // Last hour

	// Query for logs
	logsQuery := `{namespace="monitoring"}`
	logsResp, err := queryLoki(logsQuery, start, end)
	if err != nil {
		fmt.Printf("Error querying logs: %v\n", err)
	} else {
		fmt.Println("Logs:")
		printResults(logsResp, false)
	}

	// Query for events
	eventsQuery := `{job="kubernetes-events"}`
	eventsResp, err := queryLoki(eventsQuery, start, end)
	if err != nil {
		fmt.Printf("Error querying events: %v\n", err)
	} else {
		fmt.Println("\nKubernetes Events:")
		printResults(eventsResp, true)
	}
}

func printResults(resp *LokiResponse, isEvent bool) {
	for _, result := range resp.Data.Result {
		fmt.Printf("Stream: %v\n", result.Stream)
		for _, value := range result.Values {
			timestamp, err := parseUnixNano(value[0])
			if err != nil {
				fmt.Printf("Error parsing timestamp: %v\n", err)
				continue
			}

			if isEvent {
				printEvent(timestamp, value[1], result.Stream)
			} else {
				fmt.Printf("[%s] %s\n", timestamp.Format(time.RFC3339), value[1])
			}
		}
		fmt.Println()
	}
}

func parseUnixNano(ts string) (time.Time, error) {
	nanos, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("error parsing Unix nano timestamp: %v", err)
	}
	return time.Unix(0, nanos), nil
}

func printEvent(timestamp time.Time, message string, labels map[string]string) {
	fmt.Printf("Time: %s\n", timestamp.Format(time.RFC3339))
	fmt.Printf("Namespace: %s\n", labels["namespace"])
	fmt.Printf("Kind: %s\n", labels["kind"])
	fmt.Printf("Name: %s\n", labels["name"])
	fmt.Printf("Reason: %s\n", labels["reason"])

	// Extract event message from the log line
	parts := strings.SplitN(message, "Message: ", 2)
	if len(parts) == 2 {
		fmt.Printf("Message: %s\n", parts[1])
	} else {
		fmt.Printf("Message: %s\n", message)
	}
	fmt.Println()
}
