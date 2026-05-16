// Package handler contains HTTP handlers for processing incoming OTLP signals.
//
// This package provides handlers for the three OpenTelemetry signal types:
//   - Logs: /v1/logs endpoint
//   - Metrics: /v1/metrics endpoint
//   - Traces: /v1/traces endpoint
//
// Each handler:
//   - Accepts HTTP POST requests with protobuf-encoded OTLP data
//   - Validates and unmarshals the incoming payload
//   - Processes the data through signal-specific processors
//   - Returns appropriate HTTP status codes and error responses
//
// The package also includes a health check endpoint at /healthz for monitoring
// the service's operational status.
package handler
