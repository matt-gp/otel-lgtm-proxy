package certutil

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

func TLSEnabled(cfg *config.TLSConfig) bool {
	return cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != ""
}

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
	}, nil
}
