package processor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"go.uber.org/mock/gomock"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		endpoint    *config.Endpoint
		signalType  string
		client      Client
		wantErr     bool
		errContains string
	}{
		{
			name: "successful creation without TLS",
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			endpoint: &config.Endpoint{
				Address: "http://localhost:3100",
			},
			signalType: "logs",
			client:     &http.Client{},
			wantErr:    false,
		},
		{
			name: "successful creation with TLS disabled",
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			endpoint: &config.Endpoint{
				Address: "https://localhost:3100",
				TLS: config.TLSConfig{
					InsecureSkipVerify: false,
				},
			},
			signalType: "logs",
			client:     &http.Client{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			getResource := func(rl *logpb.ResourceLogs) *resourcepb.Resource {
				return rl.GetResource()
			}
			marshalResources := func(resources []*logpb.ResourceLogs) ([]byte, error) {
				return []byte{}, nil
			}

			proc, err := New(
				tt.config,
				tt.endpoint,
				tt.signalType,
				tt.client,
				logger,
				meter,
				tracer,
				getResource,
				marshalResources,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, proc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, proc)
				assert.Equal(t, tt.config, proc.config)
				assert.Equal(t, tt.endpoint, proc.endpoint)
				assert.Equal(t, tt.signalType, proc.signalType)
				assert.NotNil(t, proc.proxyRecordsMetric)
				assert.NotNil(t, proc.proxyRequestsMetric)
				assert.NotNil(t, proc.proxyLatencyMetric)
			}
		})
	}
}

func TestExtractTenantFromResource(t *testing.T) {
	tests := []struct {
		name           string
		resource       *logpb.ResourceLogs
		config         *config.Config
		expectedTenant string
		expectModified bool // whether the resource should be modified with default tenant
	}{
		{
			name: "extract from primary label",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			expectedTenant: "tenant-a",
			expectModified: false,
		},
		{
			name: "extract from first secondary label",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "tenantId", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Labels:  []string{"tenantId", "tenant_id"},
					Default: "default",
				},
			},
			expectedTenant: "tenant-b",
			expectModified: false,
		},
		{
			name: "extract from second secondary label",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "tenant_id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-c"}}},
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Labels:  []string{"tenantId", "tenant_id"},
					Default: "default",
				},
			},
			expectedTenant: "tenant-c",
			expectModified: false,
		},
		{
			name: "use default tenant when no tenant attribute",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "shared",
				},
			},
			expectedTenant: "shared",
			expectModified: true,
		},
		{
			name: "return empty when no tenant and no default",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "",
				},
			},
			expectedTenant: "",
			expectModified: false,
		},
		{
			name: "primary label takes precedence over secondary labels",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "primary-tenant"}}},
						{Key: "tenantId", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "secondary-tenant"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Labels:  []string{"tenantId"},
					Default: "default",
				},
			},
			expectedTenant: "primary-tenant",
			expectModified: false,
		},
		{
			name: "empty primary label not configured",
			resource: &logpb.ResourceLogs{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "tenantId", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-d"}}},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "",
					Labels:  []string{"tenantId"},
					Default: "default",
				},
			},
			expectedTenant: "tenant-d",
			expectModified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			getResource := func(rl *logpb.ResourceLogs) *resourcepb.Resource {
				return rl.GetResource()
			}
			marshalResources := func(resources []*logpb.ResourceLogs) ([]byte, error) {
				return []byte{}, nil
			}

			proc, err := New(
				tt.config,
				&config.Endpoint{Address: "http://localhost:3100"},
				"logs",
				&http.Client{},
				logger,
				meter,
				tracer,
				getResource,
				marshalResources,
			)
			require.NoError(t, err)

			originalAttrCount := len(tt.resource.Resource.Attributes)
			tenant := proc.extractTenantFromResource(tt.resource)

			assert.Equal(t, tt.expectedTenant, tenant)

			if tt.expectModified {
				// Should have added the tenant label to attributes
				assert.Equal(t, originalAttrCount+1, len(tt.resource.Resource.Attributes))
				// Verify the added attribute
				found := false
				for _, attr := range tt.resource.Resource.Attributes {
					if attr.Key == tt.config.Tenant.Label {
						assert.Equal(t, tt.expectedTenant, attr.GetValue().GetStringValue())
						found = true
						break
					}
				}
				assert.True(t, found, "tenant label should be added to attributes")
			} else {
				// Should not have modified the attributes
				assert.Equal(t, originalAttrCount, len(tt.resource.Resource.Attributes))
			}
		})
	}
}

func TestPartition(t *testing.T) {
	tests := []struct {
		name            string
		resources       []*logpb.ResourceLogs
		config          *config.Config
		expectedTenants map[string]int // tenant -> number of resources
	}{
		{
			name:      "empty resources returns empty map",
			resources: []*logpb.ResourceLogs{},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			expectedTenants: map[string]int{},
		},
		{
			name: "single tenant groups all resources together",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			expectedTenants: map[string]int{
				"tenant-a": 3,
			},
		},
		{
			name: "multiple tenants partition resources correctly",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-c"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
						},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "default",
				},
			},
			expectedTenants: map[string]int{
				"tenant-a": 2,
				"tenant-b": 2,
				"tenant-c": 1,
			},
		},
		{
			name: "resources without tenant are skipped",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "my-service"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
						},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "", // No default, so resource without tenant should be skipped
				},
			},
			expectedTenants: map[string]int{
				"tenant-a": 1,
				"tenant-b": 1,
			},
		},
		{
			name: "resources with default tenant are grouped",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "service-1"}}},
						},
					},
				},
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "service-2"}}},
						},
					},
				},
			},
			config: &config.Config{
				Tenant: config.Tenant{
					Label:   "tenant.id",
					Default: "shared",
				},
			},
			expectedTenants: map[string]int{
				"shared": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			getResource := func(rl *logpb.ResourceLogs) *resourcepb.Resource {
				return rl.GetResource()
			}
			marshalResources := func(resources []*logpb.ResourceLogs) ([]byte, error) {
				return []byte{}, nil
			}

			proc, err := New(
				tt.config,
				&config.Endpoint{Address: "http://localhost:3100"},
				"logs",
				&http.Client{},
				logger,
				meter,
				tracer,
				getResource,
				marshalResources,
			)
			require.NoError(t, err)

			result := proc.Partition(context.Background(), tt.resources)

			assert.Equal(t, len(tt.expectedTenants), len(result), "unexpected number of tenants")

			for tenant, expectedCount := range tt.expectedTenants {
				resources, ok := result[tenant]
				assert.True(t, ok, "tenant %s not found in result", tenant)
				assert.Equal(t, expectedCount, len(resources), "unexpected number of resources for tenant %s", tenant)
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	tests := []struct {
		name          string
		tenantMap     map[string][]*logpb.ResourceLogs
		mockResponses []struct {
			statusCode int
			body       string
			err        error
		}
		wantErr bool
	}{
		{
			name: "successful dispatch to single tenant",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusOK, body: "ok", err: nil},
			},
			wantErr: false,
		},
		{
			name: "successful dispatch to multiple tenants",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
				"tenant-b": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusOK, body: "ok", err: nil},
				{statusCode: http.StatusOK, body: "ok", err: nil},
			},
			wantErr: false,
		},
		{
			name: "http 400 bad request should return error",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusBadRequest, body: "bad request", err: nil},
			},
			wantErr: true,
		},
		{
			name: "http 404 not found should return error",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusNotFound, body: "not found", err: nil},
			},
			wantErr: true,
		},
		{
			name: "http 500 internal server error should return error",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusInternalServerError, body: "server error", err: nil},
			},
			wantErr: true,
		},
		{
			name: "http 503 service unavailable should return error",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusServiceUnavailable, body: "service unavailable", err: nil},
			},
			wantErr: true,
		},
		{
			name: "multiple tenants with one failing should return error",
			tenantMap: map[string][]*logpb.ResourceLogs{
				"tenant-a": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
							},
						},
					},
				},
				"tenant-b": {
					{
						Resource: &resourcepb.Resource{
							Attributes: []*commonpb.KeyValue{
								{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
							},
						},
					},
				},
			},
			mockResponses: []struct {
				statusCode int
				body       string
				err        error
			}{
				{statusCode: http.StatusOK, body: "ok", err: nil},
				{statusCode: http.StatusBadRequest, body: "bad request", err: nil},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			mockClient := NewMockClient(ctrl)

			// Set up mock expectations
			for _, resp := range tt.mockResponses {
				if resp.err != nil {
					mockClient.EXPECT().Do(gomock.Any()).Return(nil, resp.err).Times(1)
				} else {
					httpResp := &http.Response{
						StatusCode: resp.statusCode,
						Body:       io.NopCloser(bytes.NewBufferString(resp.body)),
					}
					mockClient.EXPECT().Do(gomock.Any()).Return(httpResp, nil).Times(1)
				}
			}

			getResource := func(rl *logpb.ResourceLogs) *resourcepb.Resource {
				return rl.GetResource()
			}
			marshalResources := func(resources []*logpb.ResourceLogs) ([]byte, error) {
				return []byte("marshaled"), nil
			}

			proc, err := New(
				&config.Config{
					Tenant: config.Tenant{
						Label:   "tenant.id",
						Default: "default",
					},
				},
				&config.Endpoint{Address: "http://localhost:3100"},
				"logs",
				mockClient,
				logger,
				meter,
				tracer,
				getResource,
				marshalResources,
			)
			require.NoError(t, err)

			err = proc.Dispatch(context.Background(), tt.tenantMap)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSend(t *testing.T) {
	tests := []struct {
		name         string
		tenant       string
		resources    []*logpb.ResourceLogs
		mockResponse *http.Response
		mockError    error
		marshalError error
		wantErr      bool
		errContains  string
	}{
		{
			name:   "successful send",
			tenant: "tenant-a",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
			},
			mockResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
			},
			mockError: nil,
			wantErr:   false,
		},
		{
			name:   "marshal error",
			tenant: "tenant-a",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-a"}}},
						},
					},
				},
			},
			mockResponse: nil,
			marshalError: errors.New("marshal failed"),
			wantErr:      true,
			errContains:  "failed to marshal data",
		},
		{
			name:   "http client error",
			tenant: "tenant-b",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-b"}}},
						},
					},
				},
			},
			mockResponse: nil,
			mockError:    errors.New("connection refused"),
			wantErr:      true,
			errContains:  "failed to send request",
		},
		{
			name:   "non-200 response",
			tenant: "tenant-c",
			resources: []*logpb.ResourceLogs{
				{
					Resource: &resourcepb.Resource{
						Attributes: []*commonpb.KeyValue{
							{Key: "tenant.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "tenant-c"}}},
						},
					},
				},
			},
			mockResponse: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString("server error")),
			},
			mockError: nil,
			wantErr:   false, // Non-200 is not an error at the send level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			logger := noop.NewLoggerProvider().Logger("test")
			meter := noopmetric.NewMeterProvider().Meter("test")
			tracer := nooptrace.NewTracerProvider().Tracer("test")

			mockClient := NewMockClient(ctrl)
			if tt.marshalError == nil {
				mockClient.EXPECT().Do(gomock.Any()).Return(tt.mockResponse, tt.mockError).Times(1)
			}

			getResource := func(rl *logpb.ResourceLogs) *resourcepb.Resource {
				return rl.GetResource()
			}
			marshalResources := func(resources []*logpb.ResourceLogs) ([]byte, error) {
				if tt.marshalError != nil {
					return nil, tt.marshalError
				}
				return []byte("marshaled"), nil
			}

			proc, err := New(
				&config.Config{
					Tenant: config.Tenant{
						Label:   "tenant.id",
						Header:  "X-Scope-OrgID",
						Default: "default",
					},
				},
				&config.Endpoint{Address: "http://localhost:3100"},
				"logs",
				mockClient,
				logger,
				meter,
				tracer,
				getResource,
				marshalResources,
			)
			require.NoError(t, err)

			resp, err := proc.send(context.Background(), tt.tenant, tt.resources)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.mockResponse.StatusCode, resp.StatusCode)
			}
		})
	}
}
