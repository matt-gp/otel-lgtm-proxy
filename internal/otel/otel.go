package otel

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type Provider struct {
	TracerProvider *trace.TracerProvider
	MeterProvider  *metric.MeterProvider
	LoggerProvider *log.LoggerProvider
}

// NewProvider creates a new OpenTelemetry provider using environment variables
// This function relies heavily on OpenTelemetry's built-in environment variable support
func NewProvider(config config.Config) (*Provider, error) {
	// Check if OpenTelemetry is disabled
	if os.Getenv("OTEL_SDK_DISABLED") == "true" {
		return &Provider{}, nil
	}

	// Create resource with service information - the SDK will automatically
	// merge this with OTEL_RESOURCE_ATTRIBUTES from environment
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.Service.Name),
			semconv.ServiceVersionKey.String(config.Service.Version),
		),
		resource.WithFromEnv(),   // Automatically parse OTEL_RESOURCE_ATTRIBUTES
		resource.WithProcess(),   // Add process information
		resource.WithOS(),        // Add OS information
		resource.WithContainer(), // Add container information if available
		resource.WithHost(),      // Add host information
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := &Provider{}

	// Initialize providers - each init function handles environment variables internally
	if err := provider.initTracing(res); err != nil {
		return nil, fmt.Errorf("failed to initialize tracing: %w", err)
	}

	if err := provider.initMetrics(res); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	if err := provider.initLogging(res); err != nil {
		return nil, fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Set global providers
	if provider.TracerProvider != nil {
		otel.SetTracerProvider(provider.TracerProvider)
	}
	if provider.MeterProvider != nil {
		otel.SetMeterProvider(provider.MeterProvider)
	}
	if provider.LoggerProvider != nil {
		global.SetLoggerProvider(provider.LoggerProvider)
	}

	// Set propagator - automatically configured from OTEL_PROPAGATORS
	propagatorNames := os.Getenv("OTEL_PROPAGATORS")
	if propagatorNames == "" {
		propagatorNames = "tracecontext,baggage" // Default propagators
	}

	var propagators []propagation.TextMapPropagator
	for _, name := range strings.Split(propagatorNames, ",") {
		switch strings.TrimSpace(name) {
		case "tracecontext":
			propagators = append(propagators, propagation.TraceContext{})
		case "baggage":
			propagators = append(propagators, propagation.Baggage{})
		}
	}
	if len(propagators) > 0 {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagators...))
	}

	return provider, nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error

	if p.TracerProvider != nil {
		if err := p.TracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
		}
	}

	if p.MeterProvider != nil {
		if err := p.MeterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
		}
	}

	if p.LoggerProvider != nil {
		if err := p.LoggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("logger provider shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

func (p *Provider) initLogging(res *resource.Resource) error {
	exporter := os.Getenv("OTEL_LOGS_EXPORTER")
	if exporter == "" {
		exporter = "console" // Default to console
	}

	// Skip if logs are disabled - but still create a no-op provider
	if exporter == "none" {
		// Create a no-op logger provider for when logging is disabled
		p.LoggerProvider = log.NewLoggerProvider(
			log.WithResource(res),
		)
		return nil
	}

	var logExporter log.Exporter
	var err error

	switch exporter {
	case "otlp":
		// Check protocol preference - HTTP or gRPC
		protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		if protocol == "" {
			protocol = "http/protobuf" // Default to HTTP with protobuf to avoid trace ID encoding issues
		}

		switch protocol {
		case "http/protobuf", "http":
			logExporter, err = otlploghttp.New(context.Background())
		case "grpc":
			logExporter, err = otlploggrpc.New(context.Background())
		default:
			return fmt.Errorf("unsupported OTLP protocol: %q", protocol)
		}
	case "console":
		logExporter, err = stdoutlog.New()
	default:
		return fmt.Errorf("unknown logs exporter: %q", exporter)
	}

	if err != nil {
		return err
	}

	// Batch processor automatically configures from OTEL_BLRP_* environment variables
	p.LoggerProvider = log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(res),
	)

	return nil
}

func (p *Provider) initMetrics(res *resource.Resource) error {
	exporter := os.Getenv("OTEL_METRICS_EXPORTER")
	if exporter == "" {
		exporter = "console" // Default to console
	}

	// Skip if metrics are disabled
	if exporter == "none" {
		return nil
	}

	var reader metric.Reader
	var err error

	switch exporter {
	case "otlp":
		// Check protocol preference - HTTP or gRPC
		protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		if protocol == "" {
			protocol = "http/protobuf" // Default to HTTP with protobuf to avoid trace ID encoding issues
		}

		var metricExporter metric.Exporter
		switch protocol {
		case "http/protobuf", "http":
			metricExporter, err = otlpmetrichttp.New(context.Background(),
				otlpmetrichttp.WithTemporalitySelector(metric.DefaultTemporalitySelector),
			)
		case "grpc":
			metricExporter, err = otlpmetricgrpc.New(context.Background(),
				otlpmetricgrpc.WithTemporalitySelector(metric.DefaultTemporalitySelector),
			)
		default:
			return fmt.Errorf("unsupported OTLP protocol: %q", protocol)
		}

		if err != nil {
			return err
		}
		// Periodic reader automatically configures interval from OTEL_METRIC_EXPORT_INTERVAL
		reader = metric.NewPeriodicReader(metricExporter)
	case "console":
		metricExporter, err := stdoutmetric.New()
		if err != nil {
			return err
		}
		// Periodic reader automatically configures interval from OTEL_METRIC_EXPORT_INTERVAL
		reader = metric.NewPeriodicReader(metricExporter)
	default:
		return fmt.Errorf("unknown metrics exporter: %q", exporter)
	}

	if err != nil {
		return err
	}

	p.MeterProvider = metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)

	return nil
}

func (p *Provider) initTracing(res *resource.Resource) error {
	exporter := os.Getenv("OTEL_TRACES_EXPORTER")
	if exporter == "" {
		exporter = "console" // Default to console
	}

	// Skip if traces are disabled
	if exporter == "none" {
		return nil
	}

	var traceExporter trace.SpanExporter
	var err error

	switch exporter {
	case "otlp":
		// Check protocol preference - HTTP or gRPC
		protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		if protocol == "" {
			protocol = "http/protobuf" // Default to HTTP with protobuf to avoid trace ID encoding issues
		}

		switch protocol {
		case "http/protobuf", "http":
			traceExporter, err = otlptracehttp.New(context.Background())
		case "grpc":
			traceExporter, err = otlptracegrpc.New(context.Background())
		default:
			return fmt.Errorf("unsupported OTLP protocol: %q", protocol)
		}
	case "console":
		traceExporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		return fmt.Errorf("unknown traces exporter: %q", exporter)
	}

	if err != nil {
		return err
	}

	// Create tracer provider with automatic batch processor configuration
	// The SDK automatically configures batch processor options from environment variables:
	// OTEL_BSP_MAX_EXPORT_BATCH_SIZE, OTEL_BSP_EXPORT_TIMEOUT, OTEL_BSP_SCHEDULE_DELAY, etc.

	// Get sampler configuration from environment variables
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	if samplerType == "" {
		samplerType = "parentbased_always_on" // Default sampler
	}

	var sampler trace.Sampler
	switch samplerType {
	case "always_on":
		sampler = trace.AlwaysSample()
	case "always_off":
		sampler = trace.NeverSample()
	case "traceidratio":
		// Use default ratio of 1.0 if not specified
		sampler = trace.TraceIDRatioBased(1.0)
	case "parentbased_always_on":
		sampler = trace.ParentBased(trace.AlwaysSample())
	case "parentbased_always_off":
		sampler = trace.ParentBased(trace.NeverSample())
	case "parentbased_traceidratio":
		sampler = trace.ParentBased(trace.TraceIDRatioBased(1.0))
	default:
		sampler = trace.ParentBased(trace.AlwaysSample())
	}

	p.TracerProvider = trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithBatcher(traceExporter), // SDK automatically applies env var configs
		trace.WithSampler(sampler),
	)

	return nil
}
