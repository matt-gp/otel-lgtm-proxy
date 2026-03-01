// Package handler contains the HTTP handlers for processing incoming OTLP signals.
package handler

import (
	"net/http"

	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/processor"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/proto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// Logs handles incoming OTLP log requests.
func (h *Handlers) Logs(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(
		r.Context(),
		"Handlers.Logs",
		trace.WithAttributes(attribute.String("signal.type", "logs")),
	)
	defer span.End()

	data, err := proto.Unmarshal(r, &logpb.LogsData{})
	if err != nil {
		logger.Error(ctx, h.logger, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	// Create processor for this request
	proc, err := processor.New(
		h.config,
		&h.config.Logs,
		"logs",
		h.logsClient,
		h.logger,
		h.meter,
		h.tracer,
		func(rl *logpb.ResourceLogs) *resourcepb.Resource {
			return rl.GetResource()
		},
		func(resources []*logpb.ResourceLogs) ([]byte, error) {
			data := &logpb.LogsData{
				ResourceLogs: resources,
			}
			return proto.Marshal(data)
		},
	)
	if err != nil {
		logger.Error(ctx, h.logger, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	// Process the log data
	if err := proc.Dispatch(ctx, proc.Partition(ctx, data.GetResourceLogs())); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	w.WriteHeader(http.StatusAccepted)
}
