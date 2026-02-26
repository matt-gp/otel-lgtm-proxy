package httputil

import (
	"net/http/httptest"
	"testing"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

func TestAddHeaders(t *testing.T) {
	tests := []struct {
		name    string
		tenant  string
		config  *config.Config
		headers string
		want    map[string]string
	}{
		{
			name:   "basic tenant header with custom headers",
			tenant: "tenant1",
			config: &config.Config{
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "%s",
				},
			},
			headers: "Authorization=Bearer token123",
			want: map[string]string{
				"X-Scope-OrgID": "tenant1",
				"Content-Type":  "application/x-protobuf",
				"Authorization": "Bearer token123",
			},
		},
		{
			name:   "no custom headers",
			tenant: "tenant2",
			config: &config.Config{
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "%s",
				},
			},
			headers: "",
			want: map[string]string{
				"X-Scope-OrgID": "tenant2",
				"Content-Type":  "application/x-protobuf",
			},
		},
		{
			name:   "multiple custom headers",
			tenant: "tenant3",
			config: &config.Config{
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "%s",
				},
			},
			headers: "Authorization=Bearer token123,X-Custom-Header=CustomValue",
			want: map[string]string{
				"X-Scope-OrgID":   "tenant3",
				"Content-Type":    "application/x-protobuf",
				"Authorization":   "Bearer token123",
				"X-Custom-Header": "CustomValue",
			},
		},
		{
			name:   "tenant format with prefix",
			tenant: "tenant4",
			config: &config.Config{
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "prefix-%s",
				},
			},
			headers: "",
			want: map[string]string{
				"X-Scope-OrgID": "prefix-tenant4",
				"Content-Type":  "application/x-protobuf",
			},
		},
		{
			name:   "invalid custom header format",
			tenant: "tenant5",
			config: &config.Config{
				Tenant: config.Tenant{
					Header: "X-Scope-OrgID",
					Format: "%s",
				},
			},
			headers: "InvalidHeader",
			want: map[string]string{
				"X-Scope-OrgID": "tenant5",
				"Content-Type":  "application/x-protobuf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", nil)
			AddHeaders(tt.tenant, req, tt.config, tt.headers)

			for key, expectedValue := range tt.want {
				actualValue := req.Header.Get(key)
				if actualValue != expectedValue {
					t.Errorf("AddHeaders() header[%s] = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}
