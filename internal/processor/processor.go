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

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/cert"
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

// Client is an interface for making HTTP requests.
//
//go:generate mockgen -package processor -source processor.go -destination processor_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// ResourceData is an interface for OTLP resource types.
type ResourceData interface {
	*logpb.ResourceLogs | *metricpb.ResourceMetrics | *tracepb.ResourceSpans
}

// Processor is a generic struct that processes incoming telemetry resource data and forwards it to the appropriate backend.
type Processor[T ResourceData] struct {
	config              *config.Config
	endpoint            *config.Endpoint
	signalType          string
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
	signalType string,
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
		"otel_lgtm_proxy_request_duration_seconds",
		metric.WithDescription("Latency of otel lgtm proxy requests"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy latency histogram: %w", err)
	}

	// Configure TLS if enabled
	if cert.TLSEnabled(&endpoint.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &Processor[T]{
		config:              config,
		endpoint:            endpoint,
		signalType:          signalType,
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

// signalTypeAttr returns a common attribute for the signal type to be used in metrics and traces.
func (p *Processor[T]) signalTypeAttr() attribute.KeyValue {
	return attribute.String("signal.type", p.signalType)
}

// signalTypeLogAttr returns a common log attribute for the signal type to be used in logs.
func (p *Processor[T]) signalTypeLogAttr() log.KeyValue {
	return log.String("signal.type", p.signalType)
}

// proxyRecordsMetricAdd adds the given count to the proxy records metric with common attributes.
func (p *Processor[T]) proxyRecordsMetricAdd(ctx context.Context, tenant string, count int64, attrs []attribute.KeyValue) {
	attrs = append(attrs, attribute.String("signal.tenant", tenant), p.signalTypeAttr())
	p.proxyRecordsMetric.Add(
		ctx,
		count,
		metric.WithAttributes(attrs...),
	)
}

// proxyRequestsMetricAdd adds 1 to the proxy requests metric with common attributes.
func (p *Processor[T]) proxyRequestsMetricAdd(ctx context.Context, tenant string, attrs []attribute.KeyValue) {
	attrs = append(attrs, attribute.String("signal.tenant", tenant), p.signalTypeAttr())
	p.proxyRequestsMetric.Add(
		ctx,
		1,
		metric.WithAttributes(attrs...),
	)
}

// proxyLatencyMetricRecord records the given latency to the proxy latency metric with common attributes.
func (p *Processor[T]) proxyLatencyMetricRecord(ctx context.Context, tenant string, latency int64, attrs []attribute.KeyValue) {
	attrs = append(attrs, attribute.String("signal.tenant", tenant), p.signalTypeAttr())
	p.proxyLatencyMetric.Record(
		ctx,
		latency,
		metric.WithAttributes(attrs...),
	)
}

// Partition partitions the resources by tenant.
func (p *Processor[T]) Partition(ctx context.Context, resources []T) map[string][]T {
	tenantMap := make(map[string][]T)

	for _, resourceData := range resources {
		tenant := p.extractTenantFromResource(resourceData)
		if tenant == "" {
			logger.Warn(
				ctx,
				p.logger,
				"No tenant found in attributes and no default tenant configured",
				p.signalTypeLogAttr(),
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
			tenantAttribute := attribute.String("signal.tenant", tenant)
			resp, err := p.send(ctx, tenant, resources)
			if err != nil {
				p.proxyRecordsMetricAdd(ctx, tenant, int64(len(resources)), nil)

				logger.Error(
					ctx,
					p.logger,
					err.Error(),
					p.signalTypeLogAttr(),
				)

				return err
			}

			signalResponseStatusCodeAttr := attribute.String(
				"signal.response.status.code",
				strconv.Itoa(resp.StatusCode),
			)

			p.proxyRecordsMetricAdd(
				ctx,
				tenant,
				int64(len(resources)),
				[]attribute.KeyValue{signalResponseStatusCodeAttr},
			)

			p.proxyRequestsMetricAdd(
				ctx,
				tenant,
				[]attribute.KeyValue{signalResponseStatusCodeAttr},
			)

			if resp.StatusCode >= http.StatusBadRequest {
				logger.Error(
					ctx,
					p.logger,
					fmt.Sprintf("received non-success status code: %d", resp.StatusCode),
					p.signalTypeLogAttr(),
					log.KeyValueFromAttribute(tenantAttribute),
					log.KeyValueFromAttribute(signalResponseStatusCodeAttr),
				)

				return fmt.Errorf("received non-success status code: %d", resp.StatusCode)
			}

			logger.Debug(
				ctx,
				p.logger,
				fmt.Sprintf(
					"sent %d records",
					len(resources),
				),
				p.signalTypeLogAttr(),
				log.KeyValueFromAttribute(tenantAttribute),
				log.KeyValueFromAttribute(signalResponseStatusCodeAttr),
			)

			logger.Trace(
				ctx,
				p.logger,
				fmt.Sprintf("%+v", resources),
				p.signalTypeLogAttr(),
				log.KeyValueFromAttribute(tenantAttribute),
				log.KeyValueFromAttribute(signalResponseStatusCodeAttr),
			)

			return nil
		})
	}

	return errGroup.Wait()
}

// send sends an individual request to the target.
func (p *Processor[T]) send(
	ctx context.Context,
	tenant string,
	resources []T,
) (http.Response, error) {
	start := time.Now()

	signalTenantAttr := attribute.String("signal.tenant", tenant)
	ctx, span := p.tracer.Start(ctx,
		"processor.send",
		trace.WithAttributes(
			p.signalTypeAttr(),
			signalTenantAttr,
			attribute.Int("signal.tenant.records", len(resources)),
		),
	)
	defer span.End()

	// Marshal resources to bytes
	body, err := p.marshalResources(resources)
	if err != nil {
		logger.Error(
			ctx,
			p.logger,
			err.Error(),
			p.signalTypeLogAttr(),
			log.KeyValueFromAttribute(signalTenantAttr),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal data")
		return http.Response{}, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint.Address,
		io.NopCloser(bytes.NewReader(body)),
	)
	if err != nil {
		logger.Error(
			ctx,
			p.logger,
			err.Error(),
			p.signalTypeLogAttr(),
			log.KeyValueFromAttribute(signalTenantAttr),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return http.Response{}, fmt.Errorf("failed to create request: %w", err)
	}

	request.AddHeaders(
		tenant,
		req,
		p.config,
		p.endpoint.Headers,
	)

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Error(
			ctx,
			p.logger,
			err.Error(),
			p.signalTypeLogAttr(),
			log.KeyValueFromAttribute(signalTenantAttr),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send")
		return http.Response{}, fmt.Errorf("failed to send request: %w", err)
	}

	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			logger.Error(
				ctx,
				p.logger,
				fmt.Sprintf("failed to close response body: %v", closeErr),
				p.signalTypeLogAttr(),
				log.KeyValueFromAttribute(signalTenantAttr),
			)
			span.RecordError(closeErr)
			span.SetStatus(codes.Error, "failed to close response body")
		}
	}()

	signalResponseStatusCodeAttr := attribute.String(
		"signal.response.status.code",
		strconv.Itoa(resp.StatusCode),
	)

	span.SetAttributes(signalResponseStatusCodeAttr)
	span.SetStatus(codes.Ok, "sent successfully")

	p.proxyLatencyMetricRecord(
		ctx,
		tenant,
		time.Since(start).Milliseconds(),
		[]attribute.KeyValue{signalResponseStatusCodeAttr},
	)

	return *resp, nil
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
