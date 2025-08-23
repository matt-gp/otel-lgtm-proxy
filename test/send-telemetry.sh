#!/bin/bash

# OTLP Test Client - Master Script
# Orchestrates sending logs, metrics, and traces to OpenTelemetry Collector

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Configuration
COLLECTOR_ENDPOINT="${OTEL_COLLECTOR_ENDPOINT:-http://localhost:4318}"
TENANTS="${TENANTS:-tenant-a,tenant-b,tenant-c}"
SERVICES="${SERVICES:-web-app,api-service,database,cache,auth-service}"
INTERVAL="${INTERVAL:-1}"

show_usage() {
    echo "🚀 OTLP Test Client"
    echo ""
    echo "Usage: $0 [OPTIONS] [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  all        Send all telemetry types (logs, metrics, traces) concurrently"
    echo "  logs       Send only logs"
    echo "  metrics    Send only metrics"
    echo "  traces     Send only traces"
    echo "  help       Show this help message"
    echo ""
    echo "Options:"
    echo "  Environment variables:"
    echo "    OTEL_COLLECTOR_ENDPOINT  Collector endpoint (default: http://localhost:4318)"
    echo "    TENANTS                  Comma-separated tenant list (default: tenant-a,tenant-b,tenant-c)"
    echo "    SERVICES                 Comma-separated service list (default: web-app,api-service,database,cache,auth-service)"
    echo "    INTERVAL                 Interval between sends in seconds (default: 1)"
    echo ""
    echo "Examples:"
    echo "  $0 all                                    # Send all telemetry types"
    echo "  $0 logs                                   # Send only logs"
    echo "  INTERVAL=2 $0 metrics                     # Send metrics every 2 seconds"
    echo "  TENANTS=acme,widgets $0 traces            # Send traces for specific tenants"
    echo ""
}

check_dependencies() {
    local missing_deps=0
    
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}❌ curl is required but not installed${NC}"
        missing_deps=1
    fi
    
    if ! command -v openssl &> /dev/null; then
        echo -e "${RED}❌ openssl is required but not installed${NC}"
        missing_deps=1
    fi
    
    if [ $missing_deps -eq 1 ]; then
        echo ""
        echo "Please install the missing dependencies and try again."
        exit 1
    fi
}

check_collector() {
    echo "🔍 Checking collector connectivity..."
    # Test basic connectivity - any HTTP response (including 404) means the server is reachable
    HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 --max-time 10 "$COLLECTOR_ENDPOINT" 2>/dev/null)
    if [[ -z "$HTTP_STATUS" ]] || [[ "$HTTP_STATUS" == "000" ]]; then
        echo -e "${RED}❌ Collector not reachable at $COLLECTOR_ENDPOINT${NC}"
        echo ""
        echo "💡 Troubleshooting:"
        echo "   1. Start the collector: docker-compose up -d otel-collector"
        echo "   2. Check if it's running: docker-compose ps otel-collector"
        echo "   3. Verify the endpoint: curl $COLLECTOR_ENDPOINT"
        exit 1
    fi
    echo -e "${GREEN}✅ Collector is reachable (HTTP $HTTP_STATUS)${NC}"
}

run_logs() {
    echo -e "${BLUE}📝 Starting logs client...${NC}"
    exec "$SCRIPT_DIR/send-logs.sh"
}

run_metrics() {
    echo -e "${BLUE}📊 Starting metrics client...${NC}"
    exec "$SCRIPT_DIR/send-metrics.sh"
}

run_traces() {
    echo -e "${BLUE}🔗 Starting traces client...${NC}"
    exec "$SCRIPT_DIR/send-traces.sh"
}

run_all() {
    echo -e "${PURPLE}🚀 Starting all telemetry clients...${NC}"
    echo "📡 Collector: $COLLECTOR_ENDPOINT"
    echo "👥 Tenants: $TENANTS"
    echo "🏢 Services: $SERVICES"
    echo "⏱️  Interval: ${INTERVAL}s"
    echo ""
    echo "Press Ctrl+C to stop all clients"
    echo ""
    
    # Start all clients in background
    "$SCRIPT_DIR/send-logs.sh" &
    logs_pid=$!
    
    "$SCRIPT_DIR/send-metrics.sh" &
    metrics_pid=$!
    
    "$SCRIPT_DIR/send-traces.sh" &
    traces_pid=$!
    
    # Store PIDs for cleanup
    echo $logs_pid > /tmp/otlp-test-logs.pid
    echo $metrics_pid > /tmp/otlp-test-metrics.pid
    echo $traces_pid > /tmp/otlp-test-traces.pid
    
    # Cleanup function
    cleanup() {
        echo ""
        echo -e "${YELLOW}🛑 Stopping all clients...${NC}"
        
        # Kill background processes
        [ -n "$logs_pid" ] && kill $logs_pid 2>/dev/null
        [ -n "$metrics_pid" ] && kill $metrics_pid 2>/dev/null
        [ -n "$traces_pid" ] && kill $traces_pid 2>/dev/null
        
        # Clean up PID files
        rm -f /tmp/otlp-test-*.pid
        
        echo -e "${GREEN}✅ All clients stopped${NC}"
        exit 0
    }
    
    trap cleanup SIGINT SIGTERM
    
    # Wait for all background processes
    wait
}

# Make individual scripts executable
chmod +x "$SCRIPT_DIR/send-logs.sh" 2>/dev/null
chmod +x "$SCRIPT_DIR/send-metrics.sh" 2>/dev/null
chmod +x "$SCRIPT_DIR/send-traces.sh" 2>/dev/null

# Parse command
case "${1:-help}" in
    "all")
        check_dependencies
        check_collector
        run_all
        ;;
    "logs")
        check_dependencies
        check_collector
        run_logs
        ;;
    "metrics")
        check_dependencies
        check_collector
        run_metrics
        ;;
    "traces")
        check_dependencies
        check_collector
        run_traces
        ;;
    "help"|"-h"|"--help")
        show_usage
        ;;
    *)
        echo -e "${RED}❌ Unknown command: $1${NC}"
        echo ""
        show_usage
        exit 1
        ;;
esac
