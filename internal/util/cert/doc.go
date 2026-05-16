// Package cert provides utility functions for TLS certificate management.
//
// This package handles TLS configuration for both server and client connections:
//   - Loading X.509 certificates and private keys from files
//   - Loading CA certificates for client verification
//   - Creating TLS configurations for HTTP servers
//   - Creating TLS configurations for HTTP clients
//   - Converting string representations of client auth types to TLS constants
//
// The package supports mutual TLS (mTLS) authentication with configurable
// client certificate verification policies (NoClientCert, RequestClientCert,
// RequireAndVerifyClientCert, etc.).
package cert
