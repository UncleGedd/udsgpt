#!/bin/bash

# Default values
LOKI_URL="http://localhost:3100"
QUERY='{job="kubernetes-events"}'
LIMIT=1000

# Function to display usage
usage() {
    echo "Usage: $0 -s START_TIME -e END_TIME [-u LOKI_URL] [-q QUERY] [-l LIMIT]"
    echo "  -s START_TIME : Start time in RFC3339 format (e.g., 2023-06-01T00:00:00Z)"
    echo "  -e END_TIME   : End time in RFC3339 format (e.g., 2023-06-02T00:00:00Z)"
    echo "  -u LOKI_URL   : Loki URL (default: http://localhost:3100)"
    echo "  -q QUERY      : LogQL query (default: {job=\"kubernetes-events\"})"
    echo "  -l LIMIT      : Maximum number of entries to return (default: 1000)"
    exit 1
}

# Parse command line arguments
while getopts "s:e:u:q:l:" opt; do
    case $opt in
        s) START_TIME="$OPTARG" ;;
        e) END_TIME="$OPTARG" ;;
        u) LOKI_URL="$OPTARG" ;;
        q) QUERY="$OPTARG" ;;
        l) LIMIT="$OPTARG" ;;
        *) usage ;;
    esac
done

# Check if required parameters are provided
if [ -z "$START_TIME" ] || [ -z "$END_TIME" ]; then
    echo "Error: Start time and end time are required."
    usage
fi

# Function to convert RFC3339 to nanoseconds since epoch
rfc3339_to_nanos() {
    if date -j &>/dev/null; then
        # macOS
        date -j -f "%Y-%m-%dT%H:%M:%SZ" "$1" "+%s000000000"
    else
        # Linux
        date -u -d "$1" +%s%N
    fi
}

# Convert times to nanoseconds since epoch
START_NANOS=$(rfc3339_to_nanos "$START_TIME")
END_NANOS=$(rfc3339_to_nanos "$END_TIME")

# URL encode the query
ENCODED_QUERY=$(printf '%s' "$QUERY" | jq -sRr @uri)

# Construct the curl command
CURL_CMD="curl -G -s \"$LOKI_URL/loki/api/v1/query_range\" \
    --data-urlencode \"query=$ENCODED_QUERY\" \
    --data-urlencode \"start=$START_NANOS\" \
    --data-urlencode \"end=$END_NANOS\" \
    --data-urlencode \"limit=$LIMIT\""

# Execute the curl command and process the output
eval "$CURL_CMD" | jq -r '.data.result[] | "\n--- Stream: \(.stream | to_entries | map("\(.key)=\(.value)") | join(", ")) ---\n" + (.values[] | "\(.[0] | strptime("%Y-%m-%dT%H:%M:%S.%fZ") | strftime("%Y-%m-%d %H:%M:%S")) \(.[1])")'

echo "Query completed."