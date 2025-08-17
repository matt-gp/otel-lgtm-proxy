#!/bin/bash

# OTLP Metrics Test Client
# Sends metrics to the OpenTelemetry Collector via HTTP/JSON

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

echo "📊 OTLP Metrics Test Client"
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
    echo -e "${YELLOW}🛑 Stopping metrics client...${NC}"
    exit 0
}
trap cleanup SIGINT SIGTERM

iteration=0
# Generate start time - get current Unix timestamp in nanoseconds
if command -v date >/dev/null 2>&1; then
    # Test if date supports nanoseconds by checking output length and format
    test_ns=$(date +%s%N 2>/dev/null)
    # Nanoseconds should be 19 digits long (10 for seconds + 9 for nanoseconds)
    if [[ "$test_ns" =~ ^[0-9]{19}$ ]]; then
        # Linux/GNU date supports nanoseconds
        start_time=$(date +%s%N)
    else
        # macOS/BSD/Alpine date - convert seconds to nanoseconds
        start_time=$(($(date +%s) * 1000000000))
    fi
else
    # Fallback
    start_time=$(($(date +%s) * 1000000000))
fi

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
    
    # Generate random memory usage (500-1500 MB)
    memory_usage=$((RANDOM % 1000 + 500))
    
    # Create OTLP metrics JSON payload
    json_payload=$(cat <<EOF
{
  "resourceMetrics": [
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
      "scopeMetrics": [
        {
          "scope": {
            "name": "test-meter",
            "version": "1.0.0"
          },
          "metrics": [
            {
              "name": "request_count",
              "description": "Number of requests",
              "unit": "1",
              "sum": {
                "aggregationTemporality": 2,
                "isMonotonic": true,
                "dataPoints": [
                  {
                    "timeUnixNano": "$timestamp",
                    "asInt": "$((iteration + 1))",
                    "attributes": [
                      {
                        "key": "service",
                        "value": {
                          "stringValue": "$service"
                        }
                      }
                    ]
                  }
                ]
              }
            },
            {
              "name": "memory_usage",
              "description": "Memory usage in bytes",
              "unit": "bytes",
              "gauge": {
                "dataPoints": [
                  {
                    "timeUnixNano": "$timestamp",
                    "asDouble": $memory_usage,
                    "attributes": [
                      {
                        "key": "service",
                        "value": {
                          "stringValue": "$service"
                        }
                      }
                    ]
                  }
                ]
              }
            },
            {
              "name": "response_time",
              "description": "Response time in milliseconds",
              "unit": "ms",
              "gauge": {
                "dataPoints": [
                  {
                    "timeUnixNano": "$timestamp",
                    "asDouble": $((RANDOM % 500 + 50)),
                    "attributes": [
                      {
                        "key": "service",
                        "value": {
                          "stringValue": "$service"
                        }
                      },
                      {
                        "key": "endpoint",
                        "value": {
                          "stringValue": "/api/v1/endpoint-$((RANDOM % 5))"
                        }
                      }
                    ]
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}
EOF
)

    # Send metrics to collector
    echo -e "${BLUE}📈 Sending metrics for tenant: $tenant, service: $service, iteration: $iteration${NC}"
    
    response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Scope-OrgID: $tenant" \
        -d "$json_payload" \
        "$COLLECTOR_ENDPOINT/v1/metrics")
    
    http_code="${response: -3}"
    
    if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo -e "${GREEN}✅ Metrics sent successfully (HTTP $http_code)${NC}"
    else
        echo -e "${RED}❌ Failed to send metrics (HTTP $http_code)${NC}"
    fi
    
    ((iteration++))
    sleep $INTERVAL
done
