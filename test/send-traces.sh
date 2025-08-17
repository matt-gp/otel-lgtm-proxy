#!/bin/bash

# OTLP Traces Test Client
# Sends traces to the OpenTelemetry Collector via HTTP/JSON

# Configuration
COLLECTOR_ENDPOINT="${OTEL_COLLECTOR_ENDPOINT:-http://localhost:4318}"
TENANTS="${TENANTS:-tenant-a,tenant-b,tenant-c}"
SERVICES="${SERVICES:-web-app,api-service,database,cache,auth-service}"
INTERVAL="${INTERVAL:-1}"

# Convert comma-separated values to arrays
IFS=',' read -ra TENANT_ARRAY <<< "$TENANTS"
IFS=',' read -ra SERVICE_ARRAY <<< "$SERVICES"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "🔍 OTLP Traces Test Client"
echo "📡 Collector: $COLLECTOR_ENDPOINT"
echo "👥 Tenants: $TENANTS"
echo "🏢 Services: $SERVICES"
echo "⏱️  Interval: ${INTERVAL}s"
echo ""

# Check if collector is reachable
echo "Checking collector connectivity..."
# Test basic connectivity - any HTTP response (including 404) means the server is reachable
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 --max-time 10 "$COLLECTOR_ENDPOINT" 2>/dev/null)
if [[ -z "$HTTP_STATUS" ]] || [[ "$HTTP_STATUS" == "000" ]]; then
    echo -e "${RED}❌ Collector not reachable at $COLLECTOR_ENDPOINT${NC}"
    echo "💡 Start it with: docker-compose up -d otel-collector"
    exit 1
fi
echo -e "${GREEN}✅ Collector is reachable (HTTP $HTTP_STATUS)${NC}"
echo ""

# Function to generate random hex-encoded ID
generate_hex() {
    local bytes=$1
    openssl rand -hex $bytes
}

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}🛑 Stopping traces client...${NC}"
    exit 0
}
trap cleanup SIGINT SIGTERM

iteration=0
while true; do
    # Pick random tenant and service
    tenant=${TENANT_ARRAY[$RANDOM % ${#TENANT_ARRAY[@]}]}
    service=${SERVICE_ARRAY[$RANDOM % ${#SERVICE_ARRAY[@]}]}
    
    # Generate timestamps - get current time in nanoseconds
    if command -v date >/dev/null 2>&1; then
        # Test if date supports nanoseconds by checking output length and format
        test_ns=$(date +%s%N 2>/dev/null)
        # Nanoseconds should be 19 digits long (10 for seconds + 9 for nanoseconds)
        if [[ "$test_ns" =~ ^[0-9]{19}$ ]]; then
            # Linux/GNU date supports nanoseconds
            end_time=$(date +%s%N)
        else
            # macOS/BSD/Alpine date - convert seconds to nanoseconds
            end_time=$(($(date +%s) * 1000000000))
        fi
    else
        # Fallback
        end_time=$(($(date +%s) * 1000000000))
    fi
    duration_ms=$((RANDOM % 500 + 10))
    start_time=$((end_time - duration_ms * 1000000))
    
    # Generate trace and span IDs (hex encoded for OTLP JSON)
    trace_id=$(generate_hex 16)  # 16 bytes = 32 hex chars for trace ID
    span_id=$(generate_hex 8)    # 8 bytes = 16 hex chars for span ID
    
    # Random operation and HTTP details
    operations=("GET" "POST" "PUT" "DELETE")
    operation=${operations[$RANDOM % ${#operations[@]}]}
    status_codes=(200 201 400 404 500)
    status_code=${status_codes[$RANDOM % ${#status_codes[@]}]}
    endpoint="/api/v1/endpoint-$((RANDOM % 10))"
    
    # Create OTLP traces JSON payload
    json_payload=$(cat <<EOF
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          {
            "key": "tenant.id",
            "value": {
              "stringValue": "$tenant"
            }
          },
          {
            "key": "service.name",
            "value": {
              "stringValue": "$service"
            }
          },
          {
            "key": "service.version",
            "value": {
              "stringValue": "1.0.0"
            }
          }
        ]
      },
      "scopeSpans": [
        {
          "scope": {
            "name": "test-tracer",
            "version": "1.0.0"
          },
          "spans": [
            {
              "traceId": "$trace_id",
              "spanId": "$span_id",
              "name": "$service-$operation-operation",
              "kind": 2,
              "startTimeUnixNano": "$start_time",
              "endTimeUnixNano": "$end_time",
              "status": {
                "code": 1
              },
              "attributes": [
                {
                  "key": "http.method",
                  "value": {
                    "stringValue": "$operation"
                  }
                },
                {
                  "key": "http.url",
                  "value": {
                    "stringValue": "https://$service.example.com$endpoint"
                  }
                },
                {
                  "key": "http.status_code",
                  "value": {
                    "intValue": $status_code
                  }
                },
                {
                  "key": "iteration",
                  "value": {
                    "intValue": $iteration
                  }
                },
                {
                  "key": "user.id",
                  "value": {
                    "stringValue": "user-$((RANDOM % 100))"
                  }
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
EOF
)

    # Send traces to collector
    echo -e "${BLUE}🔗 Sending traces for tenant: $tenant, service: $service, operation: $operation, iteration: $iteration${NC}"
    
    response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Scope-OrgID: $tenant" \
        -d "$json_payload" \
        "$COLLECTOR_ENDPOINT/v1/traces")
    
    http_code="${response: -3}"
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "${GREEN}✅ Traces sent successfully (HTTP $http_code) - TraceID: ${trace_id:0:8}...${NC}"
    else
        echo -e "${RED}❌ Failed to send traces (HTTP $http_code)${NC}"
    fi
    
    ((iteration++))
    sleep $INTERVAL
done
