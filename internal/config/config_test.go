package config

import (
	"testing"
	"time"
)

func TestParse_Defaults(t *testing.T) {
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	// Service defaults
	if cfg.Service.Name != "otel-lgtm-proxy" {
		t.Errorf("Service.Name = %v, want otel-lgtm-proxy", cfg.Service.Name)
	}
	if cfg.Service.Version != "1.0.0" {
		t.Errorf("Service.Version = %v, want 1.0.0", cfg.Service.Version)
	}

	// Tenant defaults
	if cfg.Tenant.Label != "tenant.id" {
		t.Errorf("Tenant.Label = %v, want tenant.id", cfg.Tenant.Label)
	}
	if len(cfg.Tenant.Labels) != 0 {
		t.Errorf("Tenant.Labels = %v, want empty slice", cfg.Tenant.Labels)
	}
	if cfg.Tenant.Format != "%s" {
		t.Errorf("Tenant.Format = %v, want %%s", cfg.Tenant.Format)
	}
	if cfg.Tenant.Header != "X-Scope-OrgID" {
		t.Errorf("Tenant.Header = %v, want X-Scope-OrgID", cfg.Tenant.Header)
	}
	if cfg.Tenant.Default != "default" {
		t.Errorf("Tenant.Default = %v, want default", cfg.Tenant.Default)
	}

	// Endpoint defaults
	if cfg.Logs.Timeout != 15*time.Second {
		t.Errorf("Logs.Timeout = %v, want 15s", cfg.Logs.Timeout)
	}
	if cfg.Metrics.Timeout != 15*time.Second {
		t.Errorf("Metrics.Timeout = %v, want 15s", cfg.Metrics.Timeout)
	}
	if cfg.Traces.Timeout != 15*time.Second {
		t.Errorf("Traces.Timeout = %v, want 15s", cfg.Traces.Timeout)
	}
	if cfg.TimeoutShutdown != 15*time.Second {
		t.Errorf("TimeoutShutdown = %v, want 15s", cfg.TimeoutShutdown)
	}

	// TLS defaults
	if cfg.Logs.TLS.ClientAuthType != "NoClientCert" {
		t.Errorf("Logs.TLS.ClientAuthType = %v, want NoClientCert", cfg.Logs.TLS.ClientAuthType)
	}
	if cfg.Logs.TLS.InsecureSkipVerify != false {
		t.Errorf("Logs.TLS.InsecureSkipVerify = %v, want false", cfg.Logs.TLS.InsecureSkipVerify)
	}
}

func TestParse_AllValues(t *testing.T) {
	// Set all environment variables
	t.Setenv("OTEL_SERVICE_NAME", "test-proxy")
	t.Setenv("OTEL_SERVICE_VERSION", "3.2.1")
	t.Setenv("TIMEOUT_SHUTDOWN", "25s")

	t.Setenv("HTTP_LISTEN_ADDRESS", ":9090")
	t.Setenv("HTTP_LISTEN_TIMEOUT", "10s")
	t.Setenv("HTTP_LISTEN_TLS_CERT_FILE", "/certs/server.crt")
	t.Setenv("HTTP_LISTEN_TLS_KEY_FILE", "/certs/server.key")
	t.Setenv("HTTP_LISTEN_TLS_CA_FILE", "/certs/server-ca.crt")

	t.Setenv("TENANT_LABEL", "app.tenant")
	t.Setenv("TENANT_LABELS", "tenantId,namespace,org.id")
	t.Setenv("TENANT_FORMAT", "%s-staging")
	t.Setenv("TENANT_HEADER", "X-Tenant")
	t.Setenv("TENANT_DEFAULT", "public")

	t.Setenv("OLP_LOGS_ADDRESS", "https://loki.example.com/otlp/v1/logs")
	t.Setenv("OLP_LOGS_TIMEOUT", "60s")
	t.Setenv("OLP_LOGS_HEADERS", "Authorization=Bearer xyz")
	t.Setenv("OLP_LOGS_TLS_CERT_FILE", "/certs/logs-client.crt")
	t.Setenv("OLP_LOGS_TLS_KEY_FILE", "/certs/logs-client.key")
	t.Setenv("OLP_LOGS_TLS_CA_FILE", "/certs/logs-ca.crt")
	t.Setenv("OLP_LOGS_TLS_CLIENT_AUTH_TYPE", "RequireAndVerifyClientCert")
	t.Setenv("OLP_LOGS_TLS_INSECURE_SKIP_VERIFY", "true")

	t.Setenv("OLP_METRICS_ADDRESS", "https://mimir.example.com/otlp/v1/metrics")
	t.Setenv("OLP_METRICS_TIMEOUT", "90s")
	t.Setenv("OLP_METRICS_HEADERS", "X-Custom=value")
	t.Setenv("OLP_METRICS_TLS_CERT_FILE", "/certs/metrics-client.crt")
	t.Setenv("OLP_METRICS_TLS_KEY_FILE", "/certs/metrics-client.key")

	t.Setenv("OLP_TRACES_ADDRESS", "https://tempo.example.com/v1/traces")
	t.Setenv("OLP_TRACES_TIMEOUT", "120s")
	t.Setenv("OLP_TRACES_TLS_CA_FILE", "/certs/traces-ca.crt")
	t.Setenv("OLP_TRACES_TLS_INSECURE_SKIP_VERIFY", "false")

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	// Service
	if cfg.Service.Name != "test-proxy" {
		t.Errorf("Service.Name = %v, want test-proxy", cfg.Service.Name)
	}
	if cfg.Service.Version != "3.2.1" {
		t.Errorf("Service.Version = %v, want 3.2.1", cfg.Service.Version)
	}

	// Timeouts
	if cfg.TimeoutShutdown != 25*time.Second {
		t.Errorf("TimeoutShutdown = %v, want 25s", cfg.TimeoutShutdown)
	}

	// HTTP Listener
	if cfg.Http.Address != ":9090" {
		t.Errorf("Http.Address = %v, want :9090", cfg.Http.Address)
	}
	if cfg.Http.Timeout != 10*time.Second {
		t.Errorf("Http.Timeout = %v, want 10s", cfg.Http.Timeout)
	}
	if cfg.Http.TLS.CertFile != "/certs/server.crt" {
		t.Errorf("Http.TLS.CertFile = %v, want /certs/server.crt", cfg.Http.TLS.CertFile)
	}
	if cfg.Http.TLS.KeyFile != "/certs/server.key" {
		t.Errorf("Http.TLS.KeyFile = %v, want /certs/server.key", cfg.Http.TLS.KeyFile)
	}
	if cfg.Http.TLS.CAFile != "/certs/server-ca.crt" {
		t.Errorf("Http.TLS.CAFile = %v, want /certs/server-ca.crt", cfg.Http.TLS.CAFile)
	}

	// Tenant
	if cfg.Tenant.Label != "app.tenant" {
		t.Errorf("Tenant.Label = %v, want app.tenant", cfg.Tenant.Label)
	}
	expectedLabels := []string{"tenantId", "namespace", "org.id"}
	if len(cfg.Tenant.Labels) != len(expectedLabels) {
		t.Errorf("Tenant.Labels length = %v, want %v", len(cfg.Tenant.Labels), len(expectedLabels))
	}
	for i, label := range expectedLabels {
		if cfg.Tenant.Labels[i] != label {
			t.Errorf("Tenant.Labels[%d] = %v, want %v", i, cfg.Tenant.Labels[i], label)
		}
	}
	if cfg.Tenant.Format != "%s-staging" {
		t.Errorf("Tenant.Format = %v, want %%s-staging", cfg.Tenant.Format)
	}
	if cfg.Tenant.Header != "X-Tenant" {
		t.Errorf("Tenant.Header = %v, want X-Tenant", cfg.Tenant.Header)
	}
	if cfg.Tenant.Default != "public" {
		t.Errorf("Tenant.Default = %v, want public", cfg.Tenant.Default)
	}

	// Logs endpoint
	if cfg.Logs.Address != "https://loki.example.com/otlp/v1/logs" {
		t.Errorf("Logs.Address = %v, want https://loki.example.com/otlp/v1/logs", cfg.Logs.Address)
	}
	if cfg.Logs.Timeout != 60*time.Second {
		t.Errorf("Logs.Timeout = %v, want 60s", cfg.Logs.Timeout)
	}
	if cfg.Logs.Headers != "Authorization=Bearer xyz" {
		t.Errorf("Logs.Headers = %v, want Authorization=Bearer xyz", cfg.Logs.Headers)
	}
	if cfg.Logs.TLS.CertFile != "/certs/logs-client.crt" {
		t.Errorf("Logs.TLS.CertFile = %v, want /certs/logs-client.crt", cfg.Logs.TLS.CertFile)
	}
	if cfg.Logs.TLS.KeyFile != "/certs/logs-client.key" {
		t.Errorf("Logs.TLS.KeyFile = %v, want /certs/logs-client.key", cfg.Logs.TLS.KeyFile)
	}
	if cfg.Logs.TLS.CAFile != "/certs/logs-ca.crt" {
		t.Errorf("Logs.TLS.CAFile = %v, want /certs/logs-ca.crt", cfg.Logs.TLS.CAFile)
	}
	if cfg.Logs.TLS.ClientAuthType != "RequireAndVerifyClientCert" {
		t.Errorf("Logs.TLS.ClientAuthType = %v, want RequireAndVerifyClientCert", cfg.Logs.TLS.ClientAuthType)
	}
	if cfg.Logs.TLS.InsecureSkipVerify != true {
		t.Errorf("Logs.TLS.InsecureSkipVerify = %v, want true", cfg.Logs.TLS.InsecureSkipVerify)
	}

	// Metrics endpoint
	if cfg.Metrics.Address != "https://mimir.example.com/otlp/v1/metrics" {
		t.Errorf("Metrics.Address = %v, want https://mimir.example.com/otlp/v1/metrics", cfg.Metrics.Address)
	}
	if cfg.Metrics.Timeout != 90*time.Second {
		t.Errorf("Metrics.Timeout = %v, want 90s", cfg.Metrics.Timeout)
	}
	if cfg.Metrics.Headers != "X-Custom=value" {
		t.Errorf("Metrics.Headers = %v, want X-Custom=value", cfg.Metrics.Headers)
	}
	if cfg.Metrics.TLS.CertFile != "/certs/metrics-client.crt" {
		t.Errorf("Metrics.TLS.CertFile = %v, want /certs/metrics-client.crt", cfg.Metrics.TLS.CertFile)
	}
	if cfg.Metrics.TLS.KeyFile != "/certs/metrics-client.key" {
		t.Errorf("Metrics.TLS.KeyFile = %v, want /certs/metrics-client.key", cfg.Metrics.TLS.KeyFile)
	}

	// Traces endpoint
	if cfg.Traces.Address != "https://tempo.example.com/v1/traces" {
		t.Errorf("Traces.Address = %v, want https://tempo.example.com/v1/traces", cfg.Traces.Address)
	}
	if cfg.Traces.Timeout != 120*time.Second {
		t.Errorf("Traces.Timeout = %v, want 120s", cfg.Traces.Timeout)
	}
	if cfg.Traces.TLS.CAFile != "/certs/traces-ca.crt" {
		t.Errorf("Traces.TLS.CAFile = %v, want /certs/traces-ca.crt", cfg.Traces.TLS.CAFile)
	}
	if cfg.Traces.TLS.InsecureSkipVerify != false {
		t.Errorf("Traces.TLS.InsecureSkipVerify = %v, want false", cfg.Traces.TLS.InsecureSkipVerify)
	}
}
