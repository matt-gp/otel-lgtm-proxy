// Package cert provides common utility functions for TLS certificate management.
package cert

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

// TLSEnabled returns true if all required TLS configuration fields are set.
func TLSEnabled(cfg *config.TLSConfig) bool {
	return cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != ""
}

// StringClientAuthType converts a string representation of client auth type to tls.ClientAuthType.
func StringClientAuthType(clientAuthType string) tls.ClientAuthType {
	switch clientAuthType {
	case "RequestClientCert":
		return tls.RequestClientCert
	case "RequireAnyClientCert":
		return tls.RequireAnyClientCert
	case "VerifyClientCertIfGiven":
		return tls.VerifyClientCertIfGiven
	case "RequireAndVerifyClientCert":
		return tls.RequireAndVerifyClientCert
	default:
		return tls.NoClientCert
	}
}

// CreateTLSConfig creates a TLS configuration from an endpoint configuration.
func CreateTLSConfig(config *config.Endpoint) (*tls.Config, error) {
	certs, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caCert, err := os.ReadFile(config.TLS.CAFile)
	if err != nil {
		return nil, err
	}

	caPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{certs},
		RootCAs:      caPool,
		ClientAuth:   StringClientAuthType(config.TLS.ClientAuthType),
		MinVersion:   tls.VersionTLS13,
	}, nil
}
