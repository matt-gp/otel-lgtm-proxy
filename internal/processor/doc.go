// Package processor contains the core logic for processing and forwarding telemetry data.
//
// The Processor is responsible for:
//   - Partitioning incoming OTLP data by tenant based on resource attributes
//   - Marshaling partitioned data back into protobuf format
//   - Forwarding requests to downstream backends (Loki, Mimir, Tempo)
//   - Injecting tenant-specific headers (X-Scope-OrgID)
//   - Managing concurrent requests with error aggregation
//   - Collecting metrics and traces for observability
//
// The package provides a generic Processor type that works with different
// OTLP resource types:
//   - ResourceLogs for logs (forwarded to Loki)
//   - ResourceMetrics for metrics (forwarded to Mimir)
//   - ResourceSpans for traces (forwarded to Tempo)
//
// Each processor uses a Client interface for HTTP communication, allowing
// for testing with mock clients and flexibility in request handling.
package processor
