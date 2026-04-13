// Package handler contains the HTTP handlers for processing incoming OTLP signals.
package handler

import (
	"net/http"

	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
)

// Health handles incoming health check requests.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		logger.Error(r.Context(), h.logger, err.Error())
	}
}
