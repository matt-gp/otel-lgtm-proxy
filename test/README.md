# Test Environment

This directory contains the configuration and Dockerfiles for the local development environment. Running `docker compose up -d` from the project root starts everything automatically.

## Architecture

```
┌──────────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  telemetrygen (×15)  │───▶│  OTEL Collector │───▶│  OTEL Proxy     │───▶│   LGTM Stack    │
│                      │    │                 │    │                 │    │                 │
│ 2 tenants            │    │ • Batching (1s) │    │ • Tenant        │    │ • Loki (Logs)   │
│ 5 services           │    │ • Forwarding    │    │   Partitioning  │    │ • Mimir (Metrics)│
│ 3 signal types each  │    │ • Health check  │    │ • Header        │    │ • Tempo (Traces) │
│ Varied rates         │    │                 │    │   Injection     │    │ • Grafana (UI)   │
└──────────────────────┘    └─────────────────┘    └─────────────────┘    └─────────────────┘
```

The collector's 1 second batch window ensures data from multiple services and tenants is bundled into each request to the proxy, exercising the fan-out code path (`processor.send` spans per tenant are visible in Tempo).

## Traffic Generators

15 `telemetrygen` containers run continuously, each emitting one signal type for one service under one tenant:

| Tenant | Service | Traces (req/s) | Logs (req/s) | Metrics (req/s) |
|--------|---------|---------------|-------------|----------------|
| tenant-a | web-app | 6 | 12 | 4 |
| tenant-a | api-service | 4 | 7 | 3 |
| tenant-a | auth-service | 2 | 5 | 2 |
| tenant-b | frontend | 5 | 8 | 4 |
| tenant-b | backend-api | 3 | 6 | 2 |

All generators start only after the collector passes its health check, which itself waits for Loki, Mimir, and Tempo to be healthy.

## Files

| File | Purpose |
|------|---------|
| `Dockerfile.telemetrygen` | Builds `telemetrygen` from source via `go install` (requires latest Go for current release) |
| `Dockerfile.collector` | Copies `otelcol-contrib` binary into Alpine so `wget` is available for the healthcheck |
| `otel-collector-config.yaml` | Collector config: OTLP receiver, 1s batch processor, health_check extension |
| `loki-config.yaml` | Loki configuration |
| `mimir-config.yaml` | Mimir configuration |
| `tempo-config.yaml` | Tempo configuration |
| `grafana-datasources.yaml` | Pre-provisioned Grafana datasources (Loki, Mimir, Tempo) |

## Verifying Fan-Out

To confirm the proxy is correctly partitioning multi-tenant batches, look for a `POST /v1/traces` trace in Tempo under the `default` org (the proxy's own telemetry). It should have one `processor.send` child span per tenant, each with a `signal.tenant.records` attribute showing how many ResourceSpans were in that tenant's partition.

## Troubleshooting

**Services not starting / stuck in `health: starting`**

```bash
docker compose ps          # check which service is unhealthy
docker compose logs <name> # inspect its output
```

**Proxy not receiving data**

```bash
# Check the collector can reach the proxy
docker compose logs otel-collector | grep -i error
```

**Data not appearing in Grafana**

Loki, Mimir, and Tempo expose readiness endpoints — use these to confirm they accepted the data:

```bash
curl http://localhost:3100/ready   # Loki
curl http://localhost:8080/ready   # Mimir
curl http://localhost:3200/ready   # Tempo
```
