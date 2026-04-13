package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		logsClient    *http.Client
		metricsClient *http.Client
		tracesClient  *http.Client
		wantErr       bool
	}{
		{
			name: "creates handlers with all dependencies and processors",
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
				Logs: config.Endpoint{
					Address: "http://localhost:3100",
				},
				Metrics: config.Endpoint{
					Address: "http://localhost:9009",
				},
				Traces: config.Endpoint{
					Address: "http://localhost:4318",
				},
			},
			logsClient:    &http.Client{},
			metricsClient: &http.Client{},
			tracesClient:  &http.Client{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")
			router := http.NewServeMux()

			handlers, err := New(
				tt.config,
				router,
				tt.logsClient,
				tt.metricsClient,
				tt.tracesClient,
				logger,
				meter,
				tracer,
			)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, handlers)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handlers)
				assert.Equal(t, tt.config, handlers.config)
				assert.Equal(t, router, handlers.router)
				assert.NotNil(t, handlers.logger)
				assert.NotNil(t, handlers.meter)
				assert.NotNil(t, handlers.tracer)
				// Verify processors were created
				assert.NotNil(t, handlers.logsProcessor)
				assert.NotNil(t, handlers.metricsProcessor)
				assert.NotNil(t, handlers.tracesProcessor)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	t.Run("registers handler on router", func(t *testing.T) {
		cfg := &config.Config{
			Tenant: config.Tenant{
				Label:   "tenant.id",
				Default: "default",
			},
			Logs: config.Endpoint{
				Address: "http://localhost:3100",
			},
			Metrics: config.Endpoint{
				Address: "http://localhost:9009",
			},
			Traces: config.Endpoint{
				Address: "http://localhost:4318",
			},
		}

		router := http.NewServeMux()
		logger := noop.NewLoggerProvider().Logger("test")
		meter := noopmetric.NewMeterProvider().Meter("test")
		tracer := nooptrace.NewTracerProvider().Tracer("test")

		handlers, err := New(
			cfg,
			router,
			&http.Client{},
			&http.Client{},
			&http.Client{},
			logger,
			meter,
			tracer,
		)
		require.NoError(t, err)

		// Register a test handler
		testHandlerCalled := false
		testHandler := func(w http.ResponseWriter, r *http.Request) {
			testHandlerCalled = true
			w.WriteHeader(http.StatusOK)
		}

		handlers.Register("GET /test", testHandler)

		// Test the registered handler
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.True(t, testHandlerCalled, "handler should have been called")
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name              string
		address           string
		tlsConfig         *tls.Config
		expectTLSConfig   bool
		expectMaxHeader   int
	}{
		{
			name:            "creates server without TLS",
			address:         ":8080",
			tlsConfig:       nil,
			expectTLSConfig: false,
			expectMaxHeader: 1 << 20,
		},
		{
			name:    "creates server with TLS",
			address: ":8443",
			tlsConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
			expectTLSConfig: true,
			expectMaxHeader: 1 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HTTP: config.Endpoint{
					Address: tt.address,
				},
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
				Logs: config.Endpoint{
					Address: "http://localhost:3100",
				},
				Metrics: config.Endpoint{
					Address: "http://localhost:9009",
				},
				Traces: config.Endpoint{
					Address: "http://localhost:4318",
				},
			}

			router := http.NewServeMux()
			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			handlers, err := New(
				cfg,
				router,
				&http.Client{},
				&http.Client{},
				&http.Client{},
				logger,
				meter,
				tracer,
			)
			require.NoError(t, err)

			server := handlers.NewServer(tt.tlsConfig)

			assert.NotNil(t, server)
			assert.Equal(t, tt.address, server.Addr)
			assert.Equal(t, router, server.Handler)
			assert.Equal(t, tt.expectMaxHeader, server.MaxHeaderBytes)

			if tt.expectTLSConfig {
				assert.NotNil(t, server.TLSConfig)
				assert.Equal(t, tt.tlsConfig.MinVersion, server.TLSConfig.MinVersion)
			} else {
				assert.Nil(t, server.TLSConfig)
			}
		})
	}
}
