// Package proto provides utility functions for working with protobuf messages in the context of HTTP requests and responses.
package proto

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"testing"

	common "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name    string
		payload proto.Message
		wantErr bool
	}{
		{
			name: "marshal metrics data",
			payload: &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{
					{
						ScopeMetrics: []*metricpb.ScopeMetrics{
							{
								Metrics: []*metricpb.Metric{
									{
										Name: "test.metric",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "marshal traces data",
			payload: &tracepb.TracesData{
				ResourceSpans: []*tracepb.ResourceSpans{
					{
						ScopeSpans: []*tracepb.ScopeSpans{
							{
								Spans: []*tracepb.Span{
									{
										Name: "test.span",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "marshal logs data",
			payload: &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{
					{
						ScopeLogs: []*logpb.ScopeLogs{
							{
								LogRecords: []*logpb.LogRecord{
									{
										Body: &common.AnyValue{
											Value: &common.AnyValue_StringValue{
												StringValue: "test log",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) == 0 {
				t.Errorf("Marshal() returned empty bytes")
			}

			// Verify we can unmarshal what we marshaled
			if !tt.wantErr {
				target := reflect.New(reflect.TypeOf(tt.payload).Elem()).Interface().(proto.Message)
				if err := proto.Unmarshal(got, target); err != nil {
					t.Errorf("Failed to unmarshal marshaled data: %v", err)
				}
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	metricsData := &metricpb.MetricsData{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				ScopeMetrics: []*metricpb.ScopeMetrics{
					{
						Metrics: []*metricpb.Metric{
							{
								Name: "test.metric",
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		setupReq    func() *http.Request
		target      proto.Message
		wantErr     bool
		validateRes func(*testing.T, proto.Message)
	}{
		{
			name: "binary protobuf",
			setupReq: func() *http.Request {
				body, _ := proto.Marshal(metricsData)
				return &http.Request{
					Body: io.NopCloser(bytes.NewReader(body)),
					Header: http.Header{
						"Content-Type": []string{"application/x-protobuf"},
					},
				}
			},
			target:  &metricpb.MetricsData{},
			wantErr: false,
			validateRes: func(t *testing.T, msg proto.Message) {
				result := msg.(*metricpb.MetricsData)
				if len(result.ResourceMetrics) != 1 {
					t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
				}
			},
		},
		{
			name: "json",
			setupReq: func() *http.Request {
				body, _ := protojson.Marshal(metricsData)
				return &http.Request{
					Body: io.NopCloser(bytes.NewReader(body)),
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				}
			},
			target:  &metricpb.MetricsData{},
			wantErr: false,
			validateRes: func(t *testing.T, msg proto.Message) {
				result := msg.(*metricpb.MetricsData)
				if len(result.ResourceMetrics) != 1 {
					t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
				}
			},
		},
		{
			name: "empty content type defaults to binary protobuf",
			setupReq: func() *http.Request {
				body, _ := proto.Marshal(metricsData)
				return &http.Request{
					Body:   io.NopCloser(bytes.NewReader(body)),
					Header: http.Header{},
				}
			},
			target:  &metricpb.MetricsData{},
			wantErr: false,
			validateRes: func(t *testing.T, msg proto.Message) {
				result := msg.(*metricpb.MetricsData)
				if len(result.ResourceMetrics) != 1 {
					t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
				}
			},
		},
		{
			name: "invalid data",
			setupReq: func() *http.Request {
				return &http.Request{
					Body: io.NopCloser(bytes.NewReader([]byte("invalid data"))),
					Header: http.Header{
						"Content-Type": []string{"application/x-protobuf"},
					},
				}
			},
			target:  &metricpb.MetricsData{},
			wantErr: true,
		},
		{
			name: "read error",
			setupReq: func() *http.Request {
				return &http.Request{
					Body: &errorReader{},
					Header: http.Header{
						"Content-Type": []string{"application/x-protobuf"},
					},
				}
			},
			target:  &metricpb.MetricsData{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			result, err := Unmarshal(req, tt.target)

			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateRes != nil {
				tt.validateRes(t, result)
			}
		})
	}
}

// errorReader is a helper type that always returns an error when Read is called
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (e *errorReader) Close() error {
	return nil
}
