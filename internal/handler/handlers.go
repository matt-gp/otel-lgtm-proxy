// Package handler contains the HTTP handlers for processing incoming OTLP signals.
package handler

import (
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/processor"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Handlers contains the dependencies needed for all OTLP signal handlers.
type Handlers struct {
	config        *config.Config
	logsClient    processor.Client
	metricsClient processor.Client
	tracesClient  processor.Client
	logger        log.Logger
	meter         metric.Meter
	tracer        trace.Tracer
}

// New creates a new Handlers instance.
func New(
	config *config.Config,
	logsClient processor.Client,
	metricsClient processor.Client,
	tracesClient processor.Client,
	logger log.Logger,
	meter metric.Meter,
	tracer trace.Tracer,
) *Handlers {
	return &Handlers{
		config:        config,
		logsClient:    logsClient,
		metricsClient: metricsClient,
		tracesClient:  tracesClient,
		logger:        logger,
		meter:         meter,
		tracer:        tracer,
	}
}
