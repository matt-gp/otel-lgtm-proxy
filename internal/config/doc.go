// Package config provides configuration management for the otel-lgtm-proxy application.
//
// The configuration is loaded from environment variables and includes:
//   - Service metadata (name, version)
//   - HTTP server settings (address, TLS configuration)
//   - Tenant identification configuration
//   - Downstream endpoint configurations for logs, metrics, and traces
//   - TLS settings for both server and client connections
//   - Timeout settings for shutdown and HTTP requests
//
// Configuration is parsed using the env package, which supports default values
// and nested structures with environment variable prefixes.
package config
