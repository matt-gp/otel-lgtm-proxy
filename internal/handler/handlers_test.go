package handler

import (
	"net/http"
	"testing"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/stretchr/testify/assert"
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

			handlers, err := New(
				tt.config,
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
				assert.Equal(t, tt.logsClient, handlers.logsClient)
				assert.Equal(t, tt.metricsClient, handlers.metricsClient)
				assert.Equal(t, tt.tracesClient, handlers.tracesClient)
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
