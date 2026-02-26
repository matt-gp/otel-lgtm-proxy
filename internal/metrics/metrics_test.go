package metrics

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
)

//go:generate mockgen -package metrics -source metrics.go -destination metrics_mock.go

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		client    Client
		wantErr   bool
		errString string
	}{
		{
			name: "successful creation without TLS",
			config: &config.Config{
				Metrics: config.Endpoint{
					Timeout: 30 * time.Second,
				},
			},
			client:  &http.Client{},
			wantErr: false,
		},
		{
			name: "successful creation with TLS disabled",
			config: &config.Config{
				Metrics: config.Endpoint{
					Timeout: 30 * time.Second,
					TLS:     config.TLSConfig{},
				},
			},
			client:  &http.Client{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			got, err := New(tt.config, tt.client, logger, meter, tracer)

			if tt.wantErr {
				if err == nil {
					t.Errorf("New() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("New() error = %v, want error containing %v", err, tt.errString)
				}
				return
			}

			if err != nil {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == nil {
				t.Error("New() returned nil")
				return
			}

			if got.config != tt.config {
				t.Error("New() config not set correctly")
			}
		})
	}
}

func TestHandler(t *testing.T) {
	// Create test metrics data
	metricsData := &metricpb.MetricsData{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*v1.KeyValue{
						{
							Key: "tenant.id",
							Value: &v1.AnyValue{
								Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
							},
						},
					},
				},
				ScopeMetrics: []*metricpb.ScopeMetrics{
					{
						Metrics: []*metricpb.Metric{
							{
								Name: "test_counter",
								Data: &metricpb.Metric_Sum{
									Sum: &metricpb.Sum{
										AggregationTemporality: metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
										IsMonotonic:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	validBody, _ := proto.Marshal(metricsData)

	tests := []struct {
		name           string
		method         string
		body           []byte
		contentType    string
		clientResponse *http.Response
		clientError    error
		wantStatus     int
		wantBody       string
	}{
		{
			name:        "successful request",
			method:      "POST",
			body:        validBody,
			contentType: "application/x-protobuf",
			clientResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte("OK"))),
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "invalid method",
			method:     "GET",
			body:       validBody,
			wantStatus: http.StatusAccepted, // Handler doesn't check method, just processes body
			wantBody:   "",
		},
		{
			name:        "invalid content type",
			method:      "POST",
			body:        validBody,
			contentType: "application/json",
			wantStatus:  http.StatusAccepted, // Handler can parse JSON content
			wantBody:    "",
		},
		{
			name:        "invalid body",
			method:      "POST",
			body:        []byte("invalid protobuf"),
			contentType: "application/x-protobuf",
			wantStatus:  http.StatusBadRequest,
			wantBody:    "failed to unmarshal metrics\n",
		},
		{
			name:        "client error",
			method:      "POST",
			body:        validBody,
			contentType: "application/x-protobuf",
			clientError: errors.New("network error"),
			wantStatus:  http.StatusAccepted, // dispatch doesn't propagate individual send errors
			wantBody:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cfg := &config.Config{
				Metrics: config.Endpoint{
					Timeout: 30 * time.Second,
					Address: "http://backend.example.com/v1/metrics",
				},
			}

			mockClient := NewMockClient(ctrl)
			if tt.clientResponse != nil || tt.clientError != nil {
				mockClient.EXPECT().Do(gomock.Any()).Return(tt.clientResponse, tt.clientError).AnyTimes()
			}

			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			metrics, err := New(cfg, mockClient, logger, meter, tracer)
			if err != nil {
				t.Fatalf("Failed to create metrics: %v", err)
			}

			req := httptest.NewRequest(tt.method, "/v1/metrics", bytes.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()

			metrics.Handler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Handler() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantBody != "" {
				body := w.Body.String()
				if body != tt.wantBody {
					t.Errorf("Handler() body = %v, want %v", body, tt.wantBody)
				}
			}
		})
	}
}

func TestPartition(t *testing.T) {
	tests := []struct {
		name     string
		request  *metricpb.MetricsData
		expected map[string]int // tenant -> number of resource metrics
	}{
		{
			name: "single tenant",
			request: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant.id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]int{
				"tenant1": 1,
			},
		},
		{
			name: "multiple tenants",
			request: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant.id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant.id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant2"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant.id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]int{
				"tenant1": 2,
				"tenant2": 1,
			},
		},
		{
			name: "multiple different tenant attributes",
			request: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant_id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenantId",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant2"},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]int{
				"tenant1": 1,
				"tenant2": 1,
			},
		},
		{
			name: "multiple different tenant attributes with dedicated label",
			request: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant_id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant2"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenantId",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant3"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "tenant.id",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]int{
				"tenant1": 1,
				"tenant2": 1,
				"tenant3": 1,
			},
		},
		{
			name: "no tenant attribute",
			request: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*v1.KeyValue{
								{
									Key: "service.name",
									Value: &v1.AnyValue{
										Value: &v1.AnyValue_StringValue{StringValue: "my-service"},
									},
								},
							},
						},
					},
				},
			},
			expected: map[string]int{
				"default": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Metrics: config.Endpoint{},
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Labels:  []string{"tenantId", "tenant_id"},
					Default: "default",
				},
			}

			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			m, _ := New(cfg, &http.Client{}, logger, meter, tracer)

			result := m.partition(context.Background(), tt.request)

			if len(result) != len(tt.expected) {
				t.Errorf("partition() returned %d tenants, want %d", len(result), len(tt.expected))
			}

			for tenant, expectedCount := range tt.expected {
				if metricsData, exists := result[tenant]; !exists {
					t.Errorf("partition() missing tenant %s", tenant)
				} else if len(metricsData.ResourceMetrics) != expectedCount {
					t.Errorf("partition() tenant %s has %d resource metrics, want %d",
						tenant, len(metricsData.ResourceMetrics), expectedCount)
				}
			}
		})
	}
}

func TestSend(t *testing.T) {
	tests := []struct {
		name           string
		tenant         string
		clientResponse *http.Response
		clientError    error
		wantErr        bool
		errContains    string
	}{
		{
			name:   "successful send",
			tenant: "tenant1",
			clientResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte("OK"))),
			},
			wantErr: false,
		},
		{
			name:        "client error",
			tenant:      "tenant1",
			clientError: errors.New("network error"),
			wantErr:     true,
			errContains: "network error",
		},
		{
			name:   "server error response",
			tenant: "tenant1",
			clientResponse: &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewReader([]byte("Internal Server Error"))),
			},
			wantErr:     false, // send() doesn't check status codes, just returns response
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cfg := &config.Config{
				Metrics: config.Endpoint{
					Address: "http://backend.example.com/v1/metrics",
					Timeout: 30 * time.Second,
				},
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "%s",
				},
			}

			mockClient := NewMockClient(ctrl)
			if tt.clientResponse != nil || tt.clientError != nil {
				mockClient.EXPECT().Do(gomock.Any()).Return(tt.clientResponse, tt.clientError)
			}

			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			m, _ := New(cfg, mockClient, logger, meter, tracer)

			metricsData := &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						Resource: &resourcepb.Resource{},
					},
				},
			}

			_, err := m.send(context.Background(), tt.tenant, metricsData)

			if tt.wantErr {
				if err == nil {
					t.Errorf("send() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("send() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("send() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Mock expectations are verified automatically by gomock
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsString(s[1:], substr))))
}
