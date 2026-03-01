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
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Traces handles incoming OTLP trace requests.
func (h *Handlers) Traces(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(
		r.Context(),
		"Handlers.Traces",
		trace.WithAttributes(attribute.String("signal.type", "traces")),
	)
	defer span.End()

	data, err := proto.Unmarshal(r, &tracepb.TracesData{})
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
		&h.config.Traces,
		"traces",
		h.tracesClient,
		h.logger,
		h.meter,
		h.tracer,
		func(rs *tracepb.ResourceSpans) *resourcepb.Resource {
			return rs.GetResource()
		},
		func(resources []*tracepb.ResourceSpans) ([]byte, error) {
			data := &tracepb.TracesData{
				ResourceSpans: resources,
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

	// Process the trace data
	if err := proc.Dispatch(ctx, proc.Partition(ctx, data.GetResourceSpans())); err != nil {
		logger.Error(ctx, h.logger, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	w.WriteHeader(http.StatusAccepted)
}
