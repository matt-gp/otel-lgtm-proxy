package protoutil

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"testing"

	v1 "go.opentelemetry.io/proto/otlp/common/v1"
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
										Body: &v1.AnyValue{
											Value: &v1.AnyValue_StringValue{
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

func TestUnmarshal_BinaryProtobuf(t *testing.T) {
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

	body, err := proto.Marshal(metricsData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"application/x-protobuf"},
		},
	}

	got, err := Unmarshal(req, reflect.TypeOf(&metricpb.MetricsData{}))
	if err != nil {
		t.Errorf("Unmarshal() error = %v", err)
		return
	}

	result, ok := got.(*metricpb.MetricsData)
	if !ok {
		t.Errorf("Unmarshal() returned wrong type")
		return
	}

	if len(result.ResourceMetrics) != 1 {
		t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
	}
}

func TestUnmarshal_JSON(t *testing.T) {
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

	body, err := protojson.Marshal(metricsData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}

	got, err := Unmarshal(req, reflect.TypeOf(&metricpb.MetricsData{}))
	if err != nil {
		t.Errorf("Unmarshal() error = %v", err)
		return
	}

	result, ok := got.(*metricpb.MetricsData)
	if !ok {
		t.Errorf("Unmarshal() returned wrong type")
		return
	}

	if len(result.ResourceMetrics) != 1 {
		t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
	}
}

func TestUnmarshal_EmptyContentType(t *testing.T) {
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

	// Test with JSON body but no content-type
	body, err := protojson.Marshal(metricsData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	req := &http.Request{
		Body:   io.NopCloser(bytes.NewReader(body)),
		Header: http.Header{},
	}

	got, err := Unmarshal(req, reflect.TypeOf(&metricpb.MetricsData{}))
	if err != nil {
		t.Errorf("Unmarshal() error = %v", err)
		return
	}

	result, ok := got.(*metricpb.MetricsData)
	if !ok {
		t.Errorf("Unmarshal() returned wrong type")
		return
	}

	if len(result.ResourceMetrics) != 1 {
		t.Errorf("Expected 1 ResourceMetric, got %d", len(result.ResourceMetrics))
	}
}

func TestUnmarshal_InvalidData(t *testing.T) {
	req := &http.Request{
		Body: io.NopCloser(bytes.NewReader([]byte("invalid data"))),
		Header: http.Header{
			"Content-Type": []string{"application/x-protobuf"},
		},
	}

	_, err := Unmarshal(req, reflect.TypeOf(&metricpb.MetricsData{}))
	if err == nil {
		t.Error("Unmarshal() expected error with invalid data, got nil")
	}
}

func TestUnmarshal_ReadError(t *testing.T) {
	req := &http.Request{
		Body: &errorReader{},
		Header: http.Header{
			"Content-Type": []string{"application/x-protobuf"},
		},
	}

	_, err := Unmarshal(req, reflect.TypeOf(&metricpb.MetricsData{}))
	if err == nil {
		t.Error("Unmarshal() expected error when reading body fails, got nil")
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
