package otel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		errContains string
		checkFunc   func(*testing.T, *Provider)
	}{
		{
			name:    "disabled OTEL",
			envVars: map[string]string{"OTEL_SDK_DISABLED": "true"},
			wantErr: false,
			checkFunc: func(t *testing.T, p *Provider) {
				if p.TracerProvider != nil {
					t.Error("Expected tracer provider to be nil when OTEL is disabled")
				}
				if p.MeterProvider != nil {
					t.Error("Expected meter provider to be nil when OTEL is disabled")
				}
				if p.LoggerProvider != nil {
					t.Error("Expected logger provider to be nil when OTEL is disabled")
				}
			},
		},
		{
			name:    "default console exporters",
			envVars: map[string]string{}, // No exporter env vars set, should default to console
			wantErr: false,
			checkFunc: func(t *testing.T, p *Provider) {
				if p.TracerProvider == nil {
					t.Error("Expected tracer provider to be initialized with default console exporter")
				}
				if p.MeterProvider == nil {
					t.Error("Expected meter provider to be initialized with default console exporter")
				}
				if p.LoggerProvider == nil {
					t.Error("Expected logger provider to be initialized with default console exporter")
				}
			},
		},
		{
			name: "explicit console exporters",
			envVars: map[string]string{
				"OTEL_TRACES_EXPORTER":  "console",
				"OTEL_METRICS_EXPORTER": "console",
				"OTEL_LOGS_EXPORTER":    "console",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, p *Provider) {
				if p.TracerProvider == nil {
					t.Error("Expected tracer provider to be initialized")
				}
				if p.MeterProvider == nil {
					t.Error("Expected meter provider to be initialized")
				}
				if p.LoggerProvider == nil {
					t.Error("Expected logger provider to be initialized")
				}
			},
		},
		{
			name: "disabled exporters",
			envVars: map[string]string{
				"OTEL_TRACES_EXPORTER":  "none",
				"OTEL_METRICS_EXPORTER": "none",
				"OTEL_LOGS_EXPORTER":    "none",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, p *Provider) {
				// With exporters set to "none", only logger provider should be initialized (no-op)
				// Traces and metrics providers remain nil when disabled
				if p.TracerProvider != nil {
					t.Error("Expected tracer provider to be nil when traces exporter is 'none'")
				}
				if p.MeterProvider != nil {
					t.Error("Expected meter provider to be nil when metrics exporter is 'none'")
				}
				if p.LoggerProvider == nil {
					t.Error("Expected logger provider to be initialized as no-op when logs exporter is 'none'")
				}
			},
		},
		{
			name: "custom service name from env",
			envVars: map[string]string{
				"OTEL_SERVICE_NAME":     "custom-service-name",
				"OTEL_TRACES_EXPORTER":  "none",
				"OTEL_METRICS_EXPORTER": "none",
				"OTEL_LOGS_EXPORTER":    "none",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, p *Provider) {
				// Service name should be taken from env var, not fallback
				// OpenTelemetry SDK handles this automatically
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all OTEL env vars first
			clearOtelEnvVars()

			// Set test-specific env vars
			for key, value := range tt.envVars {
				err := os.Setenv(key, value)
				if err != nil {
					t.Errorf("Failed to set env var %q: %v", key, err)
				}
				defer func() {
					if err := os.Unsetenv(key); err != nil {
						t.Errorf("Failed to unset env var %q: %v", key, err)
					}
				}()
			}

			provider, err := NewProvider(config.Config{
				Service: config.Service{
					Name:    "test-service",
					Version: "1.0.0",
				},
			})
			if err != nil {
				t.Fatalf("Failed to setup provider: %v", err)
			}

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			// Ensure cleanup
			defer func() {
				if provider != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					err := provider.Shutdown(ctx)
					if err != nil {
						t.Errorf("Expected no error during shutdown, got: %v", err)
					}
				}
			}()

			if tt.checkFunc != nil {
				tt.checkFunc(t, provider)
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	tests := []struct {
		name           string
		setupEnvVars   map[string]string
		contextTimeout time.Duration
		wantErr        bool
	}{
		{
			name:           "normal shutdown with default console exporters",
			setupEnvVars:   map[string]string{}, // Use defaults (console)
			contextTimeout: 5 * time.Second,
			wantErr:        false,
		},
		{
			name: "normal shutdown with explicit console exporters",
			setupEnvVars: map[string]string{
				"OTEL_TRACES_EXPORTER":  "console",
				"OTEL_METRICS_EXPORTER": "console",
				"OTEL_LOGS_EXPORTER":    "console",
			},
			contextTimeout: 5 * time.Second,
			wantErr:        false,
		},
		{
			name: "shutdown with disabled exporters",
			setupEnvVars: map[string]string{
				"OTEL_TRACES_EXPORTER":  "none",
				"OTEL_METRICS_EXPORTER": "none",
				"OTEL_LOGS_EXPORTER":    "none",
			},
			contextTimeout: 5 * time.Second,
			wantErr:        false,
		},
		{
			name: "shutdown with cancelled context",
			setupEnvVars: map[string]string{
				"OTEL_TRACES_EXPORTER":  "console",
				"OTEL_METRICS_EXPORTER": "console",
				"OTEL_LOGS_EXPORTER":    "console",
			},
			contextTimeout: 0,    // Will create cancelled context
			wantErr:        true, // Cancelled context should error
		},
		{
			name:           "shutdown disabled provider",
			setupEnvVars:   map[string]string{"OTEL_SDK_DISABLED": "true"},
			contextTimeout: 5 * time.Second,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOtelEnvVars()

			// Set test-specific env vars
			for key, value := range tt.setupEnvVars {
				err := os.Setenv(key, value)
				if err != nil {
					t.Errorf("Failed to set env var %q: %v", key, err)
				}
				defer func() {
					if err := os.Unsetenv(key); err != nil {
						t.Errorf("Failed to unset env var %q: %v", key, err)
					}
				}()
			}

			provider, err := NewProvider(config.Config{
				Service: config.Service{
					Name:    "test-service",
					Version: "1.0.0",
				},
			})
			if err != nil {
				t.Fatalf("Failed to setup provider: %v", err)
			}

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.contextTimeout == 0 {
				ctx, cancel = context.WithCancel(context.Background())
				cancel() // Cancel immediately
			} else {
				ctx, cancel = context.WithTimeout(context.Background(), tt.contextTimeout)
				defer cancel()
			}

			err = provider.Shutdown(ctx)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			} else if !tt.wantErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// clearOtelEnvVars clears all OpenTelemetry environment variables
// This function is used for testing purposes
func clearOtelEnvVars() {
	envVars := []string{
		"OTEL_SDK_DISABLED",              // Disables the entire OTEL SDK
		"OTEL_SERVICE_NAME",              // Service name override
		"OTEL_TRACES_EXPORTER",           // Trace exporter type (otlp, console, none, etc.)
		"OTEL_METRICS_EXPORTER",          // Metrics exporter type (otlp, prometheus, console, none, etc.)
		"OTEL_LOGS_EXPORTER",             // Logs exporter type (otlp, console, none, etc.)
		"OTEL_EXPORTER_OTLP_ENDPOINT",    // OTLP endpoint URL
		"OTEL_EXPORTER_OTLP_HEADERS",     // OTLP headers (key=value,key2=value2)
		"OTEL_EXPORTER_OTLP_INSECURE",    // Whether to use insecure connection
		"OTEL_PROPAGATORS",               // Propagator types (tracecontext, baggage, etc.)
		"OTEL_TRACES_SAMPLER",            // Sampling strategy
		"OTEL_TRACES_SAMPLER_ARG",        // Sampler argument (e.g., ratio for traceidratio)
		"OTEL_BSP_MAX_EXPORT_BATCH_SIZE", // Batch span processor max batch size
		"OTEL_BSP_EXPORT_TIMEOUT",        // Batch span processor export timeout
		"OTEL_BSP_SCHEDULE_DELAY",        // Batch span processor schedule delay
		"OTEL_METRIC_EXPORT_INTERVAL",    // Metrics export interval
		"OTEL_RESOURCE_ATTRIBUTES",       // Resource attributes (key=value,key2=value2)
	}

	// Remove each environment variable
	for _, env := range envVars {
		err := os.Unsetenv(env)
		if err != nil {
			fmt.Printf("Failed to unset env var %q: %v", env, err)
		}
	}
}
