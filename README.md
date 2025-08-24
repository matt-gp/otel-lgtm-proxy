# otel-lgtm-proxy

[![Release](https://github.com/matt-gp/otel-lgtm-proxy/actions/workflows/release.yml/badge.svg)](https://github.com/matt-gp/otel-lgtm-proxy/actions/workflows/release.yml)
[![Test](https://github.com/matt-gp/otel-lgtm-proxy/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/matt-gp/otel-lgtm-proxy/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/matt-gp/otel-lgtm-proxy)](https://goreportcard.com/report/github.com/matt-gp/otel-lgtm-proxy)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

This service accepts OpenTelemetry protocol (OTLP) data in HTTP protobuf format for logs, metrics, and traces, partitions the payloads based on tenant identifiers in resource attributes, and forwards them to Grafana's LGTM (Loki, Grafana, Tempo, Mimir) stack with tenant-specific routing.

**🎯 Designed specifically for Grafana's LGTM Stack**

## Table of Contents

- [⚠️ Important Limitations](#️-important-limitations)
  - [HTTP Protobuf Only](#http-protobuf-only)
  - [Grafana LGTM Stack Only](#grafana-lgtm-stack-only)
- [Overview](#overview)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
- [Project Structure](#project-structure)
- [OpenTelemetry Collector Configuration](#opentelemetry-collector-configuration)
- [Endpoints](#endpoints)
- [Configuration](#configuration)
- [Metrics](#metrics)
- [Development](#development)
- [Docker](#docker)
- [Example Usage](#example-usage)
- [Testing](#testing)
- [License](#license)

## ⚠️ Important Limitations

### **HTTP Protobuf Only**
**This service ONLY supports HTTP protobuf payloads.** It does not support:
- OTLP/gRPC
- JSON format
- Any other serialization formats

All incoming data must be in protobuf format over HTTP as defined by the OpenTelemetry Protocol specification.

### **Grafana LGTM Stack Only**
**This proxy is specifically designed for Grafana's LGTM observability stack.** It will not work with other observability backends such as:
- Elastic Stack (Elasticsearch, Logstash, Kibana)
- Splunk
- Datadog
- New Relic
- Generic Prometheus/Jaeger setups

The proxy implements tenant partitioning and header injection patterns specific to Grafana's multi-tenant architecture for Loki (logs), Mimir (metrics), and Tempo (traces).

## Overview

The service provides multi-tenant observability for Grafana's LGTM stack by:
1. Receiving OTLP HTTP protobuf data on standardized endpoints (typically from OpenTelemetry Collectors)
2. Extracting tenant information from resource attributes  
3. Partitioning data by tenant
4. Forwarding partitioned data to Grafana's LGTM backends (Loki, Grafana, Tempo, Mimir) with appropriate tenant headers

This enables a single LGTM observability infrastructure to serve multiple tenants with proper data isolation.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Application   │───▶│                 │    │                 │    │   Grafana LGTM  │
│   (Tenant A)    │    │  OTEL Collector │───▶│  OTEL Proxy     │───▶│      Stack      │
└─────────────────┘    │                 │    │                 │    │                 │
                       │ • Batching      │    │ • Tenant        │    │ • Loki (Logs)   │
┌─────────────────┐    │ • Processing    │    │   Partitioning  │    │ • Mimir (Metrics)│
│   Application   │───▶│ • Forwarding    │    │ • Header        │    │ • Tempo (Traces) │
│   (Tenant B)    │    │                 │    │   Injection     │    │ • Grafana (UI)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
```

**Deployment Pattern:**
1. **Applications** send telemetry to OpenTelemetry Collectors using OTLP
2. **Collectors** batch, process, and forward data to this proxy  
3. **Proxy** partitions data by tenant and routes to Grafana's LGTM stack with tenant headers

## Getting Started

This section will help you quickly set up and run the otel-lgtm-proxy with Grafana's complete LGTM observability stack.

> **💡 Note**: This proxy is specifically designed for Grafana's LGTM stack and will not work with other observability platforms.

### Prerequisites

- **Docker & Docker Compose**: For running the LGTM (Loki, Grafana, Tempo, Mimir) stack
- **Go 1.24+**: For building and running the proxy
- **curl**: For testing endpoints

### Quick Start with Docker Compose

The repository includes a complete development environment with the LGTM observability stack:

```bash
# 1. Clone the repository
git clone https://github.com/matt-gp/otel-lgtm-proxy.git
cd otel-lgtm-proxy

# 2. Start the observability stack (Loki, Grafana, Tempo, Mimir)
docker-compose up -d

# 3. Wait for services to be ready (check health)
docker-compose ps

# 4. Build and run the proxy
go build -o otel-lgtm-proxy ./cmd
./otel-lgtm-proxy
```

The proxy will start on port `8080` and forward data to the local LGTM stack.

### Testing with Sample Data

The `test/` directory contains scripts for generating sample telemetry data:

```bash
# Send all types of telemetry (logs, metrics, traces)
cd test
./send-telemetry.sh all

# Send specific telemetry types
./send-telemetry.sh logs     # Only logs
./send-telemetry.sh metrics  # Only metrics  
./send-telemetry.sh traces   # Only traces

# Customize tenant and interval
TENANTS=tenant1,tenant2,tenant3 INTERVAL=2 ./send-telemetry.sh all
```

### Accessing the Observability Stack

Once everything is running, you can access:

| Service | URL | Description |
|---------|-----|-------------|
| **Grafana** | http://localhost:3000 | Visualization dashboard (admin/admin) |
| **Loki** | http://localhost:3100 | Logs storage and querying |
| **Mimir** | http://localhost:8080 | Metrics storage |
| **Tempo** | http://localhost:3200 | Traces storage and querying |
| **OTel Collector** | http://localhost:4318 | OTLP HTTP receiver |
| **Proxy Health** | http://localhost:8443/health | Proxy health check |

> **Note**: The docker-compose setup includes an OTel Collector that receives data on port 4318 and forwards it to the proxy on port 8443, which then routes it to the appropriate backends.

### Example Configuration

For manual testing (without docker-compose), the proxy can be configured via environment variables:

```bash
# Backend endpoints (pointing to local LGTM stack)
export OLP_LOGS_ADDRESS=http://localhost:3100/otlp/v1/logs
export OLP_METRICS_ADDRESS=http://localhost:8080/otlp/v1/metrics  
export OLP_TRACES_ADDRESS=http://localhost:3201/v1/traces

# Tenant configuration
export TENANT_LABEL=tenant.id           # Resource attribute to extract tenant from
export TENANT_HEADER=X-Scope-OrgID      # Header to add to backend requests
export TENANT_DEFAULT=default           # Default tenant if not found

# Server configuration  
export HTTP_LISTEN_ADDRESS=:8081        # Run on different port

# Start the proxy
./otel-lgtm-proxy
```

### Verifying the Setup

1. **Check proxy health** (if using docker-compose):
   ```bash
   curl http://localhost:8443/health
   ```

2. **Check all services are running**:
   ```bash
   docker-compose ps
   ```

3. **Send test data**:
   ```bash
   cd test
   ./send-telemetry.sh logs
   ```

4. **View in Grafana**:
   - Open http://localhost:3000 (admin/admin)
   - Go to Explore
   - Select Loki datasource
   - Query: `{tenant="tenant-a"}` to see tenant-partitioned logs

### What's Included

The development environment includes:

- **Loki**: Logs aggregation system
- **Grafana**: Visualization and dashboard platform (with pre-configured datasources)
- **Tempo**: Distributed tracing backend
- **Mimir**: Prometheus-compatible metrics storage
- **OpenTelemetry Collector**: OTLP receiver that forwards to the proxy
- **Proxy Service**: The main application (built from source)
- **Test Client**: Automated telemetry data generation
- **Configuration Files**: Pre-configured for local development

### Next Steps

- Read the [Configuration Documentation](#configuration) for production setup
- Explore the [Test Scripts Documentation](test/README.md) for advanced testing
- Check the [Development Guide](#development) for contributing

## Project Structure

The service is organized into modular, domain-specific packages:

```
cmd/
├── main.go                    # Application entry point

internal/
├── config/                    # Configuration management
│   ├── config.go             # Configuration struct and parsing
│   └── config_test.go        # Configuration tests
├── otel/                     # OpenTelemetry provider setup
│   ├── otel.go              # Provider initialization and configuration
│   └── otel_test.go         # Provider tests
├── logs/                     # Log telemetry processing
│   ├── logs.go              # Log handler, partitioning, and forwarding
│   └── logs_test.go         # Comprehensive table-driven tests
├── metrics/                  # Metric telemetry processing
│   ├── metrics.go           # Metric handler, partitioning, and forwarding
│   └── metrics_test.go      # Comprehensive table-driven tests
├── traces/                   # Trace telemetry processing
│   ├── traces.go            # Trace handler, partitioning, and forwarding
│   └── traces_test.go       # Comprehensive table-driven tests
├── certutil/                 # TLS certificate utilities
│   ├── cert_helpers.go      # TLS configuration helpers
│   └── cert_helpers_test.go # TLS tests
└── logger/                   # Structured logging utilities
    ├── logger.go            # Logging helpers
    └── logger_test.go       # Logging tests
```

### Package Responsibilities

- **`cmd/`**: Application bootstrapping and dependency injection
- **`internal/config/`**: Environment-based configuration with validation
- **`internal/otel/`**: OpenTelemetry provider setup with protocol configuration
- **`internal/logs/`**: OTLP log processing with tenant partitioning and forwarding to Loki
- **`internal/metrics/`**: OTLP metric processing with temporality handling for Mimir
- **`internal/traces/`**: OTLP trace processing with correlation support for Tempo
- **`internal/certutil/`**: TLS configuration and certificate management
- **`internal/logger/`**: Structured logging with OpenTelemetry integration

### Key Functions Per Package

**Logs Package (`internal/logs/`):**
- `New()` - Create logs processor with HTTP client and observability metrics
- `Handler()` - HTTP handler for `/v1/logs` endpoint
- `partition()` - Partition logs by tenant from resource attributes
- `dispatch()` - Concurrent forwarding to backend with tenant headers
- `send()` - HTTP client with protobuf marshaling and Loki-compatible headers

**Metrics Package (`internal/metrics/`):**
- `New()` - Create metrics processor with HTTP client and observability metrics
- `Handler()` - HTTP handler for `/v1/metrics` endpoint  
- `partition()` - Partition metrics by tenant from resource attributes
- `dispatch()` - Concurrent forwarding to backend with tenant headers
- `send()` - HTTP client with protobuf marshaling and Mimir-compatible headers

**Traces Package (`internal/traces/`):**
- `New()` - Create traces processor with HTTP client and observability metrics
- `Handler()` - HTTP handler for `/v1/traces` endpoint
- `partition()` - Partition traces by tenant from resource attributes
- `dispatch()` - Concurrent forwarding to backend with tenant headers
- `send()` - HTTP client with protobuf marshaling and Tempo-compatible headers

## OpenTelemetry Collector Configuration

Here's an example collector configuration that works with this proxy:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 200ms
    send_batch_size: 512
    send_batch_max_size: 1024
  
  memory_limiter:
    limit_mib: 256
    check_interval: 10s

exporters:
  otlphttp:
    endpoint: http://otel-proxy:8443
    compression: none
    retry_on_failure:
      enabled: true
      initial_interval: 100ms
      max_interval: 5s
      max_elapsed_time: 30s
    sending_queue:
      enabled: true
      num_consumers: 10
      queue_size: 1000

service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [resource, batch]
      exporters: [otlphttp]
    metrics:
      receivers: [otlp] 
      processors: [resource, batch]
      exporters: [otlphttp]
    traces:
      receivers: [otlp]
      processors: [resource, batch] 
      exporters: [otlphttp]
```

### Collector Configuration Notes

- **Tenant Identification**: Use the `resource` processor to add tenant information if not already present in your application telemetry
- **Batching**: Essential for performance - batches multiple telemetry items before forwarding
- **Endpoint**: Point to your proxy service (default port 8443, or 8080 if not using TLS)
- **Content-Type**: Must be `application/x-protobuf` for proper OTLP handling

### Application-Level Tenant Configuration

Alternatively, configure tenant identification directly in your applications:

**Go with OpenTelemetry SDK:**
```go
import (
    "go.opentelemetry.io/otel/sdk/resource"
    semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

resource := resource.NewWithAttributes(
    semconv.SchemaURL,
    semconv.ServiceName("my-service"),
    attribute.String("tenant.id", "my-tenant"),
)
```

**Environment Variables (many SDKs):**
```bash
export OTEL_RESOURCE_ATTRIBUTES="tenant.id=my-tenant,service.name=my-service"
```

**Python with OpenTelemetry SDK:**
```python
from opentelemetry.sdk.resources import Resource

resource = Resource.create({
    "service.name": "my-service",
    "tenant.id": "my-tenant"
})
```

## Endpoints

| Method | Path | Description |
| `POST` | `/v1/logs` | Accepts OTLP logs in protobuf format |
| `POST` | `/v1/metrics` | Accepts OTLP metrics in protobuf format |
| `POST` | `/v1/traces` | Accepts OTLP traces in protobuf format |

## Configuration

The service is configured via environment variables:

### Service Configuration
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `OTEL_SERVICE_NAME` | `otel-lgtm-proxy` | Service name for OpenTelemetry |
| `OTEL_SERVICE_VERSION` | `1.0.0` | Service version |
| `TIMEOUT_SHUTDOWN` | `15s` | Graceful shutdown timeout |

### HTTP Server
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `HTTP_LISTEN_ADDRESS` | `:8080` | Address for HTTP server |
| `HTTP_LISTEN_TIMEOUT` | `15s` | HTTP server timeout |

### TLS Configuration (HTTP Server)
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `HTTP_LISTEN_TLS_CERT_FILE` | | Path to TLS certificate |
| `HTTP_LISTEN_TLS_KEY_FILE` | | Path to TLS private key |
| `HTTP_LISTEN_TLS_CA_FILE` | | Path to CA certificate |
| `HTTP_LISTEN_TLS_CLIENT_AUTH_TYPE` | `NoClientCert` | Client authentication type |
| `HTTP_LISTEN_TLS_INSECURE_SKIP_VERIFY` | `false` | Skip TLS verification |

### Backend Targets
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `OLP_LOGS_ADDRESS` | | Target address for logs backend |
| `OLP_LOGS_TIMEOUT` | `15s` | Timeout for log requests |
| `OLP_METRICS_ADDRESS` | | Target address for metrics backend |
| `OLP_METRICS_TIMEOUT` | `15s` | Timeout for metric requests |
| `OLP_TRACES_ADDRESS` | | Target address for traces backend |
| `OLP_TRACES_TIMEOUT` | `15s` | Timeout for trace requests |

### TLS Configuration (Backend Targets)
Each target (logs, metrics, traces) supports TLS configuration with prefixes:
- `OLP_LOGS_TLS_*`
- `OLP_METRICS_TLS_*` 
- `OLP_TRACES_TLS_*`

Available TLS options for each:
- `*_CERT_FILE` - Client certificate
- `*_KEY_FILE` - Client private key  
- `*_CA_FILE` - CA certificate
- `*_CLIENT_AUTH_TYPE` - Authentication type
- `*_INSECURE_SKIP_VERIFY` - Skip verification

### Tenant Configuration
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TENANT_LABEL` | `tenant.id` | Resource attribute key containing tenant ID |
| `TENANT_FORMAT` | `%s` | Format string for tenant ID (e.g., `%s-prod`) |
| `TENANT_HEADER` | `X-Scope-OrgID` | HTTP header for tenant ID when forwarding |
| `TENANT_DEFAULT` | `default` | Default tenant when none specified |

### OpenTelemetry Configuration
Standard OpenTelemetry environment variables are supported:
- `OTEL_TRACES_EXPORTER` - Trace exporter (console, otlp, none)
- `OTEL_METRICS_EXPORTER` - Metrics exporter (console, otlp, prometheus, none)
- `OTEL_LOGS_EXPORTER` - Logs exporter (console, otlp, none)
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OTLP endpoint for self-monitoring
- `OTEL_SDK_DISABLED` - Disable OpenTelemetry SDK

## Tenant Partitioning

The service extracts tenant information from OpenTelemetry resource attributes using dedicated partitioning functions in each domain package:

```protobuf
Resource {
  attributes: [
    {
      key: "tenant.id"     // Configurable via TENANT_LABEL
      value: "my-tenant"   // Used as tenant identifier
    }
  ]
}
```

### Partitioning Functions

Each domain package implements tenant-specific partitioning:
- **`logs.partition()`** - Groups log records by tenant from resource attributes
- **`metrics.partition()`** - Groups metric records by tenant from resource attributes
- **`traces.partition()`** - Groups span records by tenant from resource attributes

If no tenant attribute is found, the default tenant (`TENANT_DEFAULT`) is used.

## Forwarding Behavior

When forwarding data to observability backends:
1. Data is partitioned by tenant using domain-specific `partition()` functions
2. Each partition is dispatched concurrently using `dispatch()` functions
3. Individual HTTP requests are sent via `send()` functions
4. The tenant ID is added as a configurable HTTP header (default: `X-Scope-OrgID`)
5. Content-Type is set to `application/x-protobuf`
6. Original protobuf format is preserved with proper headers via `addHeaders()`

## Observability

The service exposes metrics about its operation:

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `otel_lgtm_proxy_requests_total` | Counter | Total number of proxy requests | `signal.type`, `signal.tenant`, `signal.status` |
| `otel_lgtm_proxy_records_total` | Counter | Total number of records processed | `signal.type`, `signal.tenant`, `signal.status` |
| `otel_lgtm_proxy_request_duration_seconds` | Histogram | Request latency | `signal.type`, `signal.tenant` |
| `otel_lgtm_proxy_response_code_total` | Counter | Response codes | `signal.type`, `signal.tenant`, `signal.response` |

## Development

This project uses standard Go tooling for development workflow management.

### Quick Start

```bash
# Install any missing tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build the application
go build -o otel-lgtm-proxy ./cmd

# Run the application in development mode
go run ./cmd

# In another terminal, check service health
curl http://localhost:8080/health
```

### Build Targets

```bash
# Build the application
go build -o otel-lgtm-proxy ./cmd

# Build with race detection
go build -race -o otel-lgtm-proxy ./cmd

# Build and run locally
go run ./cmd

# Install to GOPATH/bin
go install ./cmd

# Clean build artifacts
go clean
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with race detection
go test -race ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage report
go tool cover -html=coverage.out

# Show coverage by function
go tool cover -func=coverage.out
```

### Code Quality

```bash
# Run all code quality checks
go vet ./... && golangci-lint run

# Individual tools
golangci-lint run  # Run linters
go fmt ./...       # Format code
go vet ./...       # Run go vet
```

### Dependencies

```bash
# Download dependencies
go mod download

# Update dependencies
go get -u ./...
go mod tidy

# Generate mocks (if using mockgen)
go generate ./...
```

### Docker

```bash
# Build Docker image
docker build -t otel-lgtm-proxy .

# Run in Docker
docker run -p 8080:8080 otel-lgtm-proxy
```

### Development Environment

For local development with observability backends, you can use Docker Compose or set up your own LGTM (Loki, Grafana, Tempo, Mimir) stack:

```bash
# Example with Docker Compose (if you have a docker-compose.yml)
docker-compose up -d

# Check service health
curl http://localhost:8080/health

# View application logs
docker logs <container-name>
```

### Build Information

```bash
# Show build and environment info
go version
go env
```

## Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o otel-lgtm-proxy ./cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/otel-lgtm-proxy .
CMD ["./otel-lgtm-proxy"]
```

## Example Usage

### Basic Setup with LGTM Stack
```bash
# Set LGTM backend endpoints
export OLP_LOGS_ADDRESS=http://loki:3100/otlp/v1/logs
export OLP_METRICS_ADDRESS=http://mimir:8080/otlp/v1/metrics  
export OLP_TRACES_ADDRESS=http://tempo:3201/v1/traces

# Configure tenant extraction
export TENANT_LABEL=service.namespace
export TENANT_HEADER=X-Scope-OrgID
export TENANT_DEFAULT=shared

# Start service
./otel-lgtm-proxy
```

### Production Setup with TLS and LGTM Stack
```bash
# Configure TLS for Loki (logs)
export OLP_LOGS_TLS_CERT_FILE=/certs/client.crt
export OLP_LOGS_TLS_KEY_FILE=/certs/client.key
export OLP_LOGS_TLS_CA_FILE=/certs/ca.crt

# Configure TLS for Mimir (metrics)
export OLP_METRICS_TLS_CERT_FILE=/certs/client.crt
export OLP_METRICS_TLS_KEY_FILE=/certs/client.key
export OLP_METRICS_TLS_CA_FILE=/certs/ca.crt

# Configure TLS for Tempo (traces)
export OLP_TRACES_TLS_CERT_FILE=/certs/client.crt
export OLP_TRACES_TLS_KEY_FILE=/certs/client.key
export OLP_TRACES_TLS_CA_FILE=/certs/ca.crt

# Configure server TLS
export HTTP_LISTEN_TLS_CERT_FILE=/certs/server.crt
export HTTP_LISTEN_TLS_KEY_FILE=/certs/server.key

./otel-lgtm-proxy
```

### Kubernetes Deployment with LGTM Stack
```yaml
# Example ConfigMap for LGTM backend configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-proxy-config
data:
  OLP_LOGS_ADDRESS: "http://loki.monitoring:3100/otlp/v1/logs"
  OLP_METRICS_ADDRESS: "http://mimir.monitoring:8080/otlp/v1/metrics"
  OLP_TRACES_ADDRESS: "http://tempo.monitoring:3201/v1/traces"
  TENANT_LABEL: "k8s.namespace.name"
  TENANT_HEADER: "X-Scope-OrgID"
  TENANT_DEFAULT: "default-namespace"
```

## Testing

This project includes comprehensive unit testing with coverage reporting.

### Unit Testing

```bash
# Run unit tests
go test ./...

# Run tests with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# View coverage in browser (generates coverage.html)
go tool cover -html=coverage.out -o coverage.html
```

### Manual Testing Tools

The project includes bash scripts for manual testing and load generation:

```bash
# Send all telemetry types (logs, metrics, traces) concurrently
cd test && ./send-telemetry.sh all

# Send specific types
./send-telemetry.sh logs     # Only logs
./send-telemetry.sh metrics  # Only metrics  
./send-telemetry.sh traces   # Only traces

# Custom configuration
TENANTS=tenant1,tenant2 INTERVAL=2 ./send-telemetry.sh all
```

The scripts continuously generate realistic telemetry data with random content and multi-tenant headers until stopped.

### Docker Testing

```bash
# Build test image
docker build -t otel-lgtm-proxy-test .

# Run test container
docker run --rm otel-lgtm-proxy-test go test ./...
```

## Contributing

Thank you for your interest in contributing! Please see the CONTRIBUTING.md file for guidelines, including:

- Code of Conduct
- Development setup and prerequisites
- Project structure and organization
- Branching and commit conventions
- Testing and code style
- Submitting changes and pull request process
- Protocol and performance requirements
- Security and documentation standards

All contributions are welcome. Please open issues or pull requests for any improvements, bug fixes, or new features.

## License

[MIT License](LICENSE)
