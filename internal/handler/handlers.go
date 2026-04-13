// Package handler contains the HTTP handlers for processing incoming OTLP signals.
package handler

import (
	"crypto/tls"
	"net/http"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/processor"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/proto"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Handlers contains the dependencies needed for all OTLP signal handlers.
type Handlers struct {
	config           *config.Config
	router           *http.ServeMux
	logger           log.Logger
	meter            metric.Meter
	tracer           trace.Tracer
	logsProcessor    processor.Processor[*logpb.ResourceLogs]
	metricsProcessor processor.Processor[*metricpb.ResourceMetrics]
	tracesProcessor  processor.Processor[*tracepb.ResourceSpans]
}

// New creates a new Handlers instance.
func New(
	config *config.Config,
	router *http.ServeMux,
	logsClient processor.Client,
	metricsClient processor.Client,
	tracesClient processor.Client,
	logger log.Logger,
	meter metric.Meter,
	tracer trace.Tracer,
) (*Handlers, error) {
	// Create logs processor
	logsProcessor, err := processor.New(
		config,
		&config.Logs,
		"logs",
		logsClient,
		logger,
		meter,
		tracer,
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
		return nil, err
	}

	// Create metrics processor
	metricsProcessor, err := processor.New(
		config,
		&config.Metrics,
		"metrics",
		metricsClient,
		logger,
		meter,
		tracer,
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
		return nil, err
	}

	// Create traces processor
	tracesProcessor, err := processor.New(
		config,
		&config.Traces,
		"traces",
		tracesClient,
		logger,
		meter,
		tracer,
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
		return nil, err
	}

	return &Handlers{
		config:           config,
		router:           router,
		logger:           logger,
		meter:            meter,
		tracer:           tracer,
		logsProcessor:    *logsProcessor,
		metricsProcessor: *metricsProcessor,
		tracesProcessor:  *tracesProcessor,
	}, nil
}

// Register registers the given handler function for the specified pattern on the provided router.
func (h *Handlers) Register(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
	h.router.Handle(pattern, otelhttp.NewHandler(http.HandlerFunc(handlerFunc), pattern))
}

// NewServer creates a new HTTP server with the provided TLS configuration.
func (h *Handlers) NewServer(tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		MaxHeaderBytes: 1 << 20, // 1MB max header size
		Addr:           h.config.HTTP.Address,
		Handler:        h.router,
		TLSConfig:      tlsConfig,
	}
}
