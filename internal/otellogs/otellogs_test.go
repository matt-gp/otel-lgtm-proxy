package otellogs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
)

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
				Logs: config.Endpoint{
					Timeout: 30 * time.Second,
				},
			},
			client:  &http.Client{},
			wantErr: false,
		},
		{
			name: "successful creation with TLS disabled",
			config: &config.Config{
				Logs: config.Endpoint{
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
	// Create test logs data
	logsData := &logpb.LogsData{
		ResourceLogs: []*logpb.ResourceLogs{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*common.KeyValue{
						{
							Key: "tenant.id",
							Value: &common.AnyValue{
								Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
							},
						},
					},
				},
				ScopeLogs: []*logpb.ScopeLogs{
					{
						LogRecords: []*logpb.LogRecord{
							{
								Body: &common.AnyValue{
									Value: &common.AnyValue_StringValue{
										StringValue: "test log message",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	validBody, _ := proto.Marshal(logsData)

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
			wantBody:    "",
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
				Logs: config.Endpoint{
					Timeout: 30 * time.Second,
					Address: "http://backend.example.com/v1/logs",
				},
			}

			mockClient := NewMockClient(ctrl)
			if tt.clientResponse != nil || tt.clientError != nil {
				mockClient.EXPECT().
					Do(gomock.Any()).
					Return(tt.clientResponse, tt.clientError).
					AnyTimes()
			}

			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			logs, err := New(cfg, mockClient, logger, meter, tracer)
			if err != nil {
				t.Fatalf("Failed to create logs: %v", err)
			}

			req := httptest.NewRequest(tt.method, "/v1/logs", bytes.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()

			logs.Handler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Handler() status = %v, want %v", w.Code, tt.wantStatus)
			}

			body := w.Body.String()
			if tt.wantBody != "" && body != tt.wantBody {
				t.Errorf("Handler() body = %v, want %v", body, tt.wantBody)
			}
		})
	}
}

func TestPartition(t *testing.T) {
	tests := []struct {
		name     string
		request  *logpb.LogsData
		expected map[string]int // tenant -> number of resource logs
	}{
		{
			name: "single tenant",
			request: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant.id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
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
			request: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant.id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant.id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant2"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant.id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
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
			request: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant_id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenantId",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant2"},
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
			request: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant_id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant2"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenantId",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant3"},
									},
								},
							},
						},
					},
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "tenant.id",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "tenant1"},
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
			request: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{
							Attributes: []*common.KeyValue{
								{
									Key: "service.name",
									Value: &common.AnyValue{
										Value: &common.AnyValue_StringValue{StringValue: "my-service"},
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
				Logs: config.Endpoint{},
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Labels:  []string{"tenantId", "tenant_id"},
					Default: "default",
				},
			}

			logger := noop.NewLoggerProvider().Logger("test")
			meter := metricnoop.NewMeterProvider().Meter("test")
			tracer := tracenoop.NewTracerProvider().Tracer("test")

			l, _ := New(cfg, &http.Client{}, logger, meter, tracer)

			result := l.partition(context.Background(), tt.request)

			if len(result) != len(tt.expected) {
				t.Errorf("partition() returned %d tenants, want %d", len(result), len(tt.expected))
			}

			for tenant, expectedCount := range tt.expected {
				if logsData, exists := result[tenant]; !exists {
					t.Errorf("partition() missing tenant %s", tenant)
				} else if len(logsData.ResourceLogs) != expectedCount {
					t.Errorf("partition() tenant %s has %d resource logs, want %d",
						tenant, len(logsData.ResourceLogs), expectedCount)
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
				Logs: config.Endpoint{
					Address: "http://backend.example.com/v1/logs",
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

			l, _ := New(cfg, mockClient, logger, meter, tracer)

			logsData := &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						Resource: &resourcepb.Resource{},
					},
				},
			}

			_, err := l.send(context.Background(), tt.tenant, logsData)

			if tt.wantErr {
				if err == nil {
					t.Errorf("send() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
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
