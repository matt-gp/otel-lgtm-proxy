// Package otel provides OpenTelemetry provider initialization and configuration.
//
// This package sets up the OpenTelemetry SDK components including:
//   - Trace provider with OTLP or stdout exporters
//   - Metric provider with OTLP or stdout exporters
//   - Log provider with OTLP or stdout exporters
//   - Resource detection with service name and version
//   - Propagators for distributed tracing context
//
// The package supports multiple exporter types:
//   - OTLP over HTTP or gRPC for production use
//   - Stdout exporters for development and debugging
//
// Configuration is driven by standard OpenTelemetry environment variables
// (OTEL_EXPORTER_OTLP_PROTOCOL, OTEL_EXPORTER_OTLP_ENDPOINT, etc.).
//
// The Setup function initializes all providers and returns a shutdown function
// that should be deferred to ensure proper cleanup of resources.
package otel
