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
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// Metrics handles incoming OTLP metric requests.
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(
		r.Context(),
		"Handlers.Metrics",
		trace.WithAttributes(attribute.String("signal.type", "metrics")),
	)
	defer span.End()

	data, err := proto.Unmarshal(r, &metricpb.MetricsData{})
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
		&h.config.Metrics,
		"metrics",
		h.metricsClient,
		h.logger,
		h.meter,
		h.tracer,
		func(rm *metricpb.ResourceMetrics) *resourcepb.Resource {
			return rm.GetResource()
		},
		func(resources []*metricpb.ResourceMetrics) ([]byte, error) {
			data := &metricpb.MetricsData{
				ResourceMetrics: resources,
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

	// Process the metric data
	if err := proc.Dispatch(ctx, proc.Partition(ctx, data.GetResourceMetrics())); err != nil {
		logger.Error(ctx, h.logger, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	w.WriteHeader(http.StatusAccepted)
}
