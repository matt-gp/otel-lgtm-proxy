# Testing Guide

This directory contains testing tools for the otel-lgtm-proxy.

## Test Scripts

The testing toolkit consists of simple bash scripts that send OTLP data via HTTP/JSON to the collector:

- **`send-telemetry.sh`** - Master script that can run all or individual telemetry types
- **`send-logs.sh`** - Sends logs with realistic log levels and content  
- **`send-metrics.sh`** - Sends counters and gauges with random values
- **`send-traces.sh`** - Sends HTTP operation traces with spans

### Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Bash Scripts   │───▶│  OTEL Collector │───▶│  OTEL Proxy     │───▶│   LGTM Stack    │
│                 │    │                 │    │                 │    │                 │
│ • Multi-tenant  │    │ • Batching      │    │ • Tenant        │    │ • Loki (Logs)   │
│ • Random data   │    │ • Forwarding    │    │   Partitioning  │    │ • Mimir (Metrics)│
│ • HTTP/JSON     │    │ • OTLP HTTP     │    │ • Header        │    │ • Tempo (Traces) │
│ • Continuous    │    │                 │    │   Injection     │    │ • Grafana (UI)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
```

The scripts:
1. **Generate telemetry** in OTLP JSON format with realistic content
2. **Send to collector** via HTTP POST to the OTLP endpoints
3. **Use multiple tenants** with X-Scope-OrgID headers
4. **Run continuously** until stopped with Ctrl+C

## Quick Start

### 1. Start the Infrastructure

```bash
# Start the LGTM stack and OTEL collector
docker-compose up -d loki mimir tempo grafana otel-collector
```

### 2. Build and Run Your Proxy

```bash
# Build the proxy
go build -o otel-lgtm-proxy ./cmd

# Run the proxy
./otel-lgtm-proxy
```

### 3. Run the Test Scripts

```bash
# Send all telemetry types concurrently
./send-telemetry.sh all

# Or send specific types
./send-telemetry.sh logs     # Only logs
./send-telemetry.sh metrics  # Only metrics  
./send-telemetry.sh traces   # Only traces
```

The scripts will:
- Check if the collector is reachable
- Start sending telemetry data every second
- Use random tenants and services for realistic testing
- Continue until you stop them with Ctrl+C

## Configuration

### Environment Variables

All scripts can be configured with these environment variables:

```bash
# Collector endpoint (default: http://localhost:4318)
export OTEL_COLLECTOR_ENDPOINT=http://localhost:4318

# Tenants to simulate (default: tenant-a,tenant-b,tenant-c)
export TENANTS=tenant1,tenant2,tenant3

# Services to simulate (default: web-app,api-service,database,cache,auth-service)
export SERVICES=app1,app2,app3

# Interval between sends in seconds (default: 1)
export INTERVAL=2
```

### Examples with Custom Configuration

```bash
# Send metrics every 2 seconds for specific tenants
INTERVAL=2 TENANTS=acme-corp,widgets-inc ./send-telemetry.sh metrics

# Send all telemetry with custom services
SERVICES=frontend,backend,database ./send-telemetry.sh all

# Use different collector endpoint
OTEL_COLLECTOR_ENDPOINT=http://collector.example.com:4318 ./send-telemetry.sh traces
```

## Test Data Generated

### Logs
- INFO level log messages with tenant context
- Random operations and log content
- Structured metadata with tenant and service info

### Metrics
- **request_count**: Cumulative counter of requests per tenant/service
- **memory_usage**: Gauge showing random memory usage values
- Properly tagged with tenant and service labels

### Traces  
- HTTP operation spans with realistic names
- Random HTTP methods (GET, POST, PUT, DELETE)
- Random status codes (200, 201, 400, 404, 500)
- Proper trace/span ID generation per tenant

## Troubleshooting

### "Collector not reachable"
```bash
# Check if collector is running
docker-compose ps otel-collector

# Check collector logs
docker-compose logs otel-collector

# Start collector if needed
docker-compose up -d otel-collector
```

### "Failed to send" errors
```bash
# Check proxy logs
tail -f proxy.log

# Verify proxy is listening on correct port
curl http://localhost:8443/health

# Check collector configuration
cat otel-collector-config.yaml
```

### Data not appearing in backends
```bash
# Check if services are healthy
curl http://localhost:3100/ready  # Loki
curl http://localhost:9009/ready  # Mimir  
curl http://localhost:3200/ready  # Tempo
curl http://localhost:3000/api/health  # Grafana

# Check proxy logs for forwarding errors
grep -i error proxy.log

# Verify tenant headers are being set
docker-compose logs otel-proxy | grep "X-Scope-OrgID"
```

## Manual Data Verification

### Query Loki (Logs)
```bash
# Query logs for a specific tenant
curl -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={tenant="tenant-a"}' \
  --data-urlencode 'start=1h' \
  --data-urlencode 'end=now'
```

### Query Mimir (Metrics) 
```bash
# Query metrics for a specific tenant (with tenant header)
curl -H "X-Scope-OrgID: tenant-a" \
  "http://localhost:9009/prometheus/api/v1/query?query=request_count"
```

### Query Tempo (Traces)
```bash
# Search for traces from a specific service
curl "http://localhost:3200/api/search?tags=service.name=web-app"
```

### View in Grafana
1. Open http://localhost:3000 (admin/admin)
2. Add data sources:
   - Loki: http://loki:3100
   - Prometheus: http://mimir:9009  
   - Tempo: http://tempo:3200
3. Create dashboards to visualize tenant-specific data

## Development Tips
