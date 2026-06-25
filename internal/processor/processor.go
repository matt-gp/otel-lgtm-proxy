// Package processor contains the Processor struct and related types for processing incoming telemetry data and forwarding it to the appropriate backend.
package processor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/matt-gp/core/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/request"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"golang.org/x/sync/errgroup"
)

var (
	signalTenantAttrKey             = "signal.tenant"
	signalResponseStatusCodeAttrKey = "signal.response.status.code"
	signalTenantRecordsAttrKey      = "signal.tenant.records"
)

// Client is an interface for making HTTP requests.
//
//go:generate mockgen -package processor -source processor.go -destination processor_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// Ensure that http.Client implements the Client interface.
var _ Client = (*http.Client)(nil)

// ResourceData is an interface for OTLP resource types.
type ResourceData interface {
	*logpb.ResourceLogs | *metricpb.ResourceMetrics | *tracepb.ResourceSpans
}

// Processor is a generic struct that processes incoming telemetry resource data and forwards it to the appropriate backend.
type Processor[T ResourceData] struct {
	config              *config.Config
	endpoint            *config.Endpoint
	signalTypeAttr      attribute.KeyValue
	client              Client
	logger              log.Logger
	meter               metric.Meter
	tracer              trace.Tracer
	proxyRecordsMetric  metric.Int64Counter
	proxyRequestsMetric metric.Int64Counter
	proxyLatencyMetric  metric.Int64Histogram
	getResource         func(T) *resourcepb.Resource
	marshalResources    func([]T) ([]byte, error)
}

// New creates a new generic Processor for any resource type.
func New[T ResourceData](
	config *config.Config,
	endpoint *config.Endpoint,
	signalTypeAttr attribute.KeyValue,
	client Client,
	logger log.Logger,
	meter metric.Meter,
	tracer trace.Tracer,
	getResource func(T) *resourcepb.Resource,
	marshalResources func([]T) ([]byte, error),
) (*Processor[T], error) {
	// Create a counter for the total number of records processed by the proxy
	proxyRecordsMetric, err := meter.Int64Counter(
		"otel_lgtm_proxy_records_total",
		metric.WithDescription("Total number of otel lgtm proxy records processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy records counter: %w", err)
	}

	// Create a counter for the total number of requests processed by the proxy
	proxyRequestsMetric, err := meter.Int64Counter(
		"otel_lgtm_proxy_requests_total",
		metric.WithDescription("Total number of otel lgtm proxy requests processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy requests counter: %w", err)
	}

	// Create a histogram for the latency of requests processed by the proxy
	proxyLatencyMetric, err := meter.Int64Histogram(
		"otel_lgtm_proxy_request_duration_ms",
		metric.WithDescription("Latency of otel lgtm proxy requests"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy latency histogram: %w", err)
	}

	return &Processor[T]{
		config:              config,
		endpoint:            endpoint,
		signalTypeAttr:      signalTypeAttr,
		client:              client,
		logger:              logger,
		meter:               meter,
		tracer:              tracer,
		proxyRecordsMetric:  proxyRecordsMetric,
		proxyRequestsMetric: proxyRequestsMetric,
		proxyLatencyMetric:  proxyLatencyMetric,
		getResource:         getResource,
		marshalResources:    marshalResources,
	}, nil
}

// proxyRecordsMetricAdd adds the given count to the proxy records metric with common attributes.
func (p *Processor[T]) proxyRecordsMetricAdd(ctx context.Context, count int64, attrs []attribute.KeyValue) {
	p.proxyRecordsMetric.Add(ctx, count, metric.WithAttributes(attrs...))
}

// proxyRequestsMetricAdd adds 1 to the proxy requests metric with common attributes.
func (p *Processor[T]) proxyRequestsMetricAdd(ctx context.Context, attrs []attribute.KeyValue) {
	p.proxyRequestsMetric.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// proxyLatencyMetricRecord records the given latency to the proxy latency metric with common attributes.
func (p *Processor[T]) proxyLatencyMetricRecord(ctx context.Context, latency int64, attrs []attribute.KeyValue) {
	p.proxyLatencyMetric.Record(ctx, latency, metric.WithAttributes(attrs...))
}

// Partition partitions the resources by tenant.
func (p *Processor[T]) Partition(ctx context.Context, resources []T) map[string][]T {
	tenantMap := make(map[string][]T)

	for _, resourceData := range resources {
		tenant := p.extractTenantFromResource(resourceData)
		if tenant == "" {
			logger.Warn(ctx, p.logger,
				"No tenant found in attributes and no default tenant configured",
				p.signalTypeAttr,
			)
			continue
		}

		tenantMap[tenant] = append(tenantMap[tenant], resourceData)
	}

	return tenantMap
}

// Dispatch sends all the requests to the target.
func (p *Processor[T]) Dispatch(ctx context.Context, tenantMap map[string][]T) error {
	errGroup, ctx := errgroup.WithContext(ctx)
	for tenant, resources := range tenantMap {
		errGroup.Go(func() error {
			sharedAttributes := []attribute.KeyValue{
				attribute.String(signalTenantAttrKey, tenant),
				p.signalTypeAttr,
			}
			statusCode, err := p.send(ctx, tenant, resources)
			if err != nil {
				p.proxyRecordsMetricAdd(ctx, int64(len(resources)), sharedAttributes)
				logger.Error(ctx, p.logger, err.Error(), sharedAttributes...)
				return err
			}

			sharedAttributes = append(sharedAttributes, attribute.String(
				signalResponseStatusCodeAttrKey,
				strconv.Itoa(statusCode),
			))

			p.proxyRecordsMetricAdd(ctx, int64(len(resources)), sharedAttributes)
			p.proxyRequestsMetricAdd(ctx, sharedAttributes)

			if statusCode >= http.StatusBadRequest {
				logger.Error(ctx, p.logger, fmt.Sprintf("received non-success status code: %d", statusCode), sharedAttributes...)
				return fmt.Errorf("received non-success status code: %d", statusCode)
			}

			logger.Debug(ctx, p.logger, fmt.Sprintf("sent %d records", len(resources)), sharedAttributes...)
			logger.Trace(ctx, p.logger, fmt.Sprintf("%+v", resources), sharedAttributes...)

			return nil
		})
	}

	return errGroup.Wait()
}

// send sends an individual request to the target.
func (p *Processor[T]) send(ctx context.Context, tenant string, resources []T) (int, error) {
	start := time.Now()

	sharedAttributes := []attribute.KeyValue{
		attribute.String(signalTenantAttrKey, tenant),
		p.signalTypeAttr,
	}
	ctx, span := p.tracer.Start(ctx, "processor.send",
		trace.WithAttributes(
			append(sharedAttributes, attribute.Int(signalTenantRecordsAttrKey, len(resources)))...,
		),
	)
	defer span.End()

	body, err := p.marshalResources(resources)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal data")
		return 0, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.endpoint.Address, io.NopCloser(bytes.NewReader(body)),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	request.AddHeaders(ctx, tenant, req, p.config, p.endpoint.Headers)

	resp, err := p.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send")
		return 0, fmt.Errorf("failed to send request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			span.RecordError(closeErr)
		}
	}()

	statusCodeAttr := attribute.String(signalResponseStatusCodeAttrKey, strconv.Itoa(resp.StatusCode))
	span.SetAttributes(statusCodeAttr)
	sharedAttributes = append(sharedAttributes, statusCodeAttr)

	if resp.StatusCode >= http.StatusBadRequest {
		span.SetStatus(codes.Error, fmt.Sprintf("non-success status: %d", resp.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "sent successfully")
	}

	p.proxyLatencyMetricRecord(ctx, time.Since(start).Milliseconds(), sharedAttributes)

	return resp.StatusCode, nil
}

// extractTenantFromResource extracts the tenant information from the resource attributes
// based on the configured tenant labels and returns it.
func (p *Processor[T]) extractTenantFromResource(resourceData T) string {
	tenant := ""
	resource := p.getResource(resourceData)

	// First, check for the dedicated tenant label
	if p.config.Tenant.Label != "" {
		for _, attr := range resource.GetAttributes() {
			if attr.GetKey() == p.config.Tenant.Label {
				tenant = attr.GetValue().GetStringValue()
				break
			}
		}
	}

	// If not found and we have additional labels, check those
	if tenant == "" && len(p.config.Tenant.Labels) > 0 {
		for _, attr := range resource.GetAttributes() {
			if slices.Contains(p.config.Tenant.Labels, attr.GetKey()) {
				tenant = attr.GetValue().GetStringValue()
				break
			}
		}
	}

	if tenant == "" {
		if p.config.Tenant.Default == "" {
			return ""
		}

		tenant = p.config.Tenant.Default
		resource.Attributes = append(resource.Attributes, &commonpb.KeyValue{
			Key:   p.config.Tenant.Label,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: tenant}},
		})
	}

	return tenant
}
