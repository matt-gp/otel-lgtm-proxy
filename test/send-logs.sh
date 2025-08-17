#!/bin/bash

# OTLP Logs Test Client
# Sends logs to the OpenTelemetry Collector via HTTP/JSON

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

echo "🔍 OTLP Logs Test Client"
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

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}🛑 Stopping logs client...${NC}"
    exit 0
}
trap cleanup SIGINT SIGTERM

iteration=0
while true; do
    # Pick random tenant and service
    tenant=${TENANT_ARRAY[$RANDOM % ${#TENANT_ARRAY[@]}]}
    service=${SERVICE_ARRAY[$RANDOM % ${#SERVICE_ARRAY[@]}]}
    
    # Generate current timestamp in nanoseconds
    if command -v date >/dev/null 2>&1; then
        # Test if date supports nanoseconds by checking output length and format
        test_ns=$(date +%s%N 2>/dev/null)
        # Nanoseconds should be 19 digits long (10 for seconds + 9 for nanoseconds)
        if [[ "$test_ns" =~ ^[0-9]{19}$ ]]; then
            # Linux/GNU date supports nanoseconds
            timestamp=$(date +%s%N)
        else
            # macOS/BSD/Alpine date - convert seconds to nanoseconds
            timestamp=$(($(date +%s) * 1000000000))
        fi
    else
        # Fallback
        timestamp=$(($(date +%s) * 1000000000))
    fi
    
    # Generate random log level and operation
    log_levels=("INFO" "WARN" "ERROR" "DEBUG")
    log_level=${log_levels[$RANDOM % ${#log_levels[@]}]}
    operation="operation-$((RANDOM % 10))"
    
    # Create OTLP logs JSON payload
    json_payload=$(cat <<EOF
{
  "resourceLogs": [
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
      "scopeLogs": [
        {
          "scope": {
            "name": "test-logger",
            "version": "1.0.0"
          },
          "logRecords": [
            {
              "timeUnixNano": "$timestamp",
              "severityNumber": 9,
              "severityText": "$log_level",
              "body": {
                "stringValue": "Test log message from $tenant/$service - iteration $iteration - $(date)"
              },
              "attributes": [
                {
                  "key": "operation",
                  "value": {
                    "stringValue": "$operation"
                  }
                },
                {
                  "key": "iteration",
                  "value": {
                    "intValue": $iteration
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

    # Send logs to collector
    echo -e "${BLUE}📝 Sending logs for tenant: $tenant, service: $service, iteration: $iteration${NC}"
    
    response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Scope-OrgID: $tenant" \
        -d "$json_payload" \
        "$COLLECTOR_ENDPOINT/v1/logs")
    
    http_code="${response: -3}"
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "${GREEN}✅ Logs sent successfully (HTTP $http_code)${NC}"
    else
        echo -e "${RED}❌ Failed to send logs (HTTP $http_code)${NC}"
    fi
    
    ((iteration++))
    sleep $INTERVAL
done
