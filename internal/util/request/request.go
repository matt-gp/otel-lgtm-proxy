// Package request provides utility functions for working with HTTP requests in the context of the otel-lgtm-proxy application.
package request

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

// AddHeaders adds the headers to the request.
func AddHeaders(tenant string, req *http.Request, config *config.Config, headers string) {
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Add(config.Tenant.Header, fmt.Sprintf(config.Tenant.Format, tenant))

	// Add custom headers
	customHeaders := strings.Split(headers, ",")
	for _, customHeader := range customHeaders {
		kv := strings.SplitN(customHeader, "=", 2)
		if len(kv) == 2 {
			req.Header.Add(kv[0], kv[1])
		}
	}
}
