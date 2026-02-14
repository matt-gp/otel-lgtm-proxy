// Package config provides the configuration for the application.
package config

import (
	"time"

	"github.com/caarlos0/env/v6"
)

// Config represents the configuration for the application.
type Config struct {
	Service         Service       `envPrefix:"OTEL_SERVICE_"`
	TimeoutShutdown time.Duration `env:"TIMEOUT_SHUTDOWN" envDefault:"15s"`

	Http   Endpoint `envPrefix:"HTTP_LISTEN_"`
	Tenant Tenant   `envPrefix:"TENANT_"`

	Logs    Endpoint `envPrefix:"OLP_LOGS_"`
	Metrics Endpoint `envPrefix:"OLP_METRICS_"`
	Traces  Endpoint `envPrefix:"OLP_TRACES_"`
}

type Service struct {
	Name    string `env:"NAME" envDefault:"otel-lgtm-proxy"`
	Version string `env:"VERSION" envDefault:"1.0.0"`
}

// Endpoint represents the configuration for an endpoint.
type Endpoint struct {
	Address string        `env:"ADDRESS"`
	Headers string        `env:"HEADERS" envDefault:""`
	Timeout time.Duration `env:"TIMEOUT" envDefault:"15s"`
	TLS     TLSConfig     `envPrefix:"TLS_"`
}

// TLSConfig represents the configuration for TLS.
type TLSConfig struct {
	CertFile           string `env:"CERT_FILE" envDefault:""`
	KeyFile            string `env:"KEY_FILE" envDefault:""`
	CAFile             string `env:"CA_FILE" envDefault:""`
	ClientAuthType     string `env:"CLIENT_AUTH_TYPE" envDefault:"NoClientCert"`
	InsecureSkipVerify bool   `env:"INSECURE_SKIP_VERIFY" envDefault:"false"`
}

// Tenant represents the configuration for a tenant.
type Tenant struct {
	Label   string   `env:"LABEL" envDefault:"tenant.id"`
	Labels  []string `env:"LABELS" envDefault:""`
	Format  string   `env:"FORMAT" envDefault:"%s"`
	Header  string   `env:"HEADER" envDefault:"X-Scope-OrgID"`
	Default string   `env:"DEFAULT" envDefault:"default"`
}

// Parse parses the configuration from environment variables
func Parse() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
