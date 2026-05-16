// Package request provides utility functions for HTTP request handling.
//
// This package provides helper functions for working with HTTP requests
// in the context of forwarding telemetry to Grafana's LGTM stack:
//   - Adding tenant identification headers (X-Scope-OrgID)
//   - Setting content-type headers for protobuf payloads
//   - Parsing and adding custom headers from configuration
//
// The tenant header format is configurable to support different naming
// conventions and multi-tenant authentication schemes required by
// Loki, Mimir, and Tempo.
package request
