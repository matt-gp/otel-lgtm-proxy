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
	"sync"
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

func (p *Processor[T]) signalTypeAttr() attribute.KeyValue {
	return attribute.String("signal.type", p.signalType)
}

func (p *Processor[T]) signalTypeLogAttr() log.KeyValue {
	return log.String("signal.type", p.signalType)
}

// Partition partitions the resources by tenant.
func (p *Processor[T]) Partition(ctx context.Context, resources []T) map[string][]T {
	ctx, span := p.tracer.Start(
		ctx,
		fmt.Sprintf("%s.partition", p.signalType),
		trace.WithAttributes(
			p.signalTypeAttr(),
		),
	)
	defer span.End()

	tenantMap := make(map[string][]T)

	for _, resourceData := range resources {
		resource := p.getResource(resourceData)
		logger.Trace(
			ctx,
			p.logger,
			fmt.Sprintf("%+v", resource),
			p.signalTypeLogAttr(),
		)

		tenant := ""

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
				logger.Warn(
					ctx,
					p.logger,
					"No tenant found in attributes and no default tenant configured",
					p.signalTypeLogAttr(),
				)
				continue
			}

			tenant = p.config.Tenant.Default
			resource.Attributes = append(resource.Attributes, &commonpb.KeyValue{
				Key:   p.config.Tenant.Label,
				Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: tenant}},
			})
		}

		tenantMap[tenant] = append(tenantMap[tenant], resourceData)
	}

	span.SetStatus(codes.Ok, "data partitioned")
	return tenantMap
}

// Dispatch sends all the requests to the target.
func (p *Processor[T]) Dispatch(ctx context.Context, tenantMap map[string][]T) error {
	waitGroup := sync.WaitGroup{}

	for tenant, resources := range tenantMap {
		ctx, span := p.tracer.Start(
			ctx,
			fmt.Sprintf("%s.dispatch", p.signalType),
			trace.WithAttributes(
				p.signalTypeAttr(),
				attribute.String("signal.tenant", tenant),
			),
		)
		defer span.End()

		waitGroup.Add(1)

		go func(tenant string, resources []T) {
			defer waitGroup.Done()

			tenantAttribute := attribute.String("signal.tenant", tenant)

			resp, err := p.send(ctx, tenant, resources)
			if err != nil {
				p.proxyRecordsMetric.Add(
					ctx,
					int64(len(resources)),
					metric.WithAttributes(
						tenantAttribute,
						p.signalTypeAttr(),
					),
				)

				logger.Error(
					ctx,
					p.logger,
					err.Error(),
					p.signalTypeLogAttr(),
				)

				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send")
				return
			}

			p.proxyRecordsMetric.Add(
				ctx,
				int64(len(resources)),
				metric.WithAttributes(
					p.signalTypeAttr(),
					tenantAttribute,
					attribute.String(
						"signal.response.status.code",
						strconv.Itoa(resp.StatusCode),
					),
				),
			)

			p.proxyRequestsMetric.Add(
				ctx,
				1,
				metric.WithAttributes(
					p.signalTypeAttr(),
					tenantAttribute,
					attribute.String(
						"signal.response.status.code",
						strconv.Itoa(resp.StatusCode),
					),
				),
			)

			logger.Debug(
				ctx,
				p.logger,
				fmt.Sprintf(
					"sent %d records status %d for tenant %s",
					len(resources),
					resp.StatusCode,
					tenant,
				),
				p.signalTypeLogAttr(),
			)

			logger.Trace(
				ctx,
				p.logger,
				fmt.Sprintf("%+v", resources),
				p.signalTypeLogAttr(),
			)

			span.SetStatus(codes.Ok, "sent successfully")
		}(tenant, resources)
	}

	waitGroup.Wait()
	return nil
}

// send sends an individual request to the target.
func (p *Processor[T]) send(
	ctx context.Context,
	tenant string,
	resources []T,
) (http.Response, error) {
	start := time.Now()

	ctx, span := p.tracer.Start(ctx,
		fmt.Sprintf("%s.send", p.signalType),
		trace.WithAttributes(
			p.signalTypeAttr(),
			attribute.String("signal.tenant", tenant),
			attribute.Int("signal.tenant.records", len(resources)),
		),
	)
	defer span.End()

	// Marshal resources to bytes
	body, err := p.marshalResources(resources)
	if err != nil {
		return http.Response{}, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint.Address,
		io.NopCloser(bytes.NewReader(body)),
	)
	if err != nil {
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
			)
			span.RecordError(closeErr)
			span.SetStatus(codes.Error, "failed to close response body")
		}
	}()

	respAttr := attribute.String(
		"signal.response.status.code",
		strconv.Itoa(resp.StatusCode),
	)

	span.SetAttributes(respAttr)
	span.SetStatus(codes.Ok, "sent successfully")

	p.proxyLatencyMetric.Record(ctx,
		time.Since(start).Milliseconds(),
		metric.WithAttributes(
			respAttr,
			p.signalTypeAttr(),
			attribute.String("signal.tenant", tenant),
		),
	)

	return *resp, nil
}
