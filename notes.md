## Get logs from last 2 minutes
```
kubectl get pods --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace} {.metadata.name}{"\n"}{end}' | while read namespace pod; do
  echo "Logs for $namespace/$pod:"
  kubectl logs -n $namespace $pod --since=2m
  echo "-----------------------"
done
```

## Get events from last 2 minutes
```
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | awk 'BEGIN {print "NAMESPACE\tLAST SEEN\tTYPE\tREASON\tOBJECT\tMESSAGE"} {if ($2 ~ /[0-9]+s$/ || $2 ~ /[0-9]+m$/) print $0}'
```

# Took logs during a UDS Core deployment (before Authservice was deployed)
Total tokens: 2786507
Log file tokens: 2762061
Event file tokens: 24446

^ gonna need to vectorize log data for sure!

# RAG
- https://cloud.zilliz.com/login?redirect=/orgs/org-ksmsygiwjdtrsvgolppurr/projects
- check Downloads folder for creds

## Reminders For Next Time
- finished backend implementation in Go, check uds-runtime in branch "ai-things"; endpoint http://localhost:8080/api/v1/ai?namespace=bookstack
- think about making a "app portal" type of page
- you've done a good job getting UDS Package / Exemption data (frame this is as "mission app") data
- Next steps
  - remember you had good luck doing "multi-query" type models
  - introduce data hooks in UDS Operator (maybe as events, see next bullet)
  - save events in a sqllite db
  - need to get a consistent HTML response to render properly in frontend
- Things to think about
  - what are other common SRE tasks that need to be done? (node rotations, check if a node has gone down, etc)

### Commands to remember
- poetry run jupyter lab
- poetry add <dep>