// Package metrics provides functionality for processing metric data.
package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/certutil"
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/httputil"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/protoutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

const (
	SIGNAL_TYPE = "metrics"
)

var (
	signalTypeAttr    = attribute.String("signal.type", SIGNAL_TYPE)
	signalTypeLogAttr = log.String("signal.type", SIGNAL_TYPE)
)

type Metrics struct {
	config                    *config.Config
	client                    Client
	logger                    log.Logger
	meter                     metric.Meter
	tracer                    trace.Tracer
	otelLgtmProxyRequests     metric.Int64Counter
	otelLgtmProxyRecords      metric.Int64Counter
	otelLgtmProxyLatency      metric.Int64Histogram
	otelLgtmProxyResponseCode metric.Int64Counter
}

//go:generate mockgen -package metrics -source metrics.go -destination metrics_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new Metrics instance.
func New(config *config.Config, client Client, logger log.Logger, meter metric.Meter, tracer trace.Tracer) (*Metrics, error) {

	otelLgtmProxyRequests, err := meter.Int64Counter(
		"otel_lgtm_proxy_requests_total",
		metric.WithDescription("Total number of otel lgtm proxy requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy requests counter: %w", err)
	}

	otelLgtmProxyRecords, err := meter.Int64Counter(
		"otel_lgtm_proxy_records_total",
		metric.WithDescription("Total number of otel lgtm proxy records processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy records counter: %w", err)
	}

	otelLgtmProxyLatency, err := meter.Int64Histogram(
		"otel_lgtm_proxy_request_duration_seconds",
		metric.WithDescription("Latency of otel lgtm proxy requests"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy latency histogram: %w", err)
	}

	otelLgtmProxyResponseCode, err := meter.Int64Counter(
		"otel_lgtm_proxy_response_code_total",
		metric.WithDescription("Status code of otel lgtm proxy responses"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel_lgtm_proxy_response_code_total counter: %w", err)
	}

	if certutil.TLSEnabled(&config.Metrics.TLS) {

		tlsConfig, err := certutil.CreateTLSConfig(&config.Metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to create meter TLS config: %w", err)
		}
		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &Metrics{
		config:                    config,
		client:                    client,
		logger:                    logger,
		meter:                     meter,
		tracer:                    tracer,
		otelLgtmProxyRequests:     otelLgtmProxyRequests,
		otelLgtmProxyRecords:      otelLgtmProxyRecords,
		otelLgtmProxyLatency:      otelLgtmProxyLatency,
		otelLgtmProxyResponseCode: otelLgtmProxyResponseCode,
	}, nil
}

// Handler handles incoming metric requests.
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {

	// Add signal type to baggage so it propagates to all child spans
	member, _ := baggage.NewMember("signal.type", SIGNAL_TYPE)
	bag, _ := baggage.New(member)
	ctx := baggage.ContextWithBaggage(r.Context(), bag)

	ctx, span := m.tracer.Start(ctx, "handler")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	result, err := protoutil.Unmarshal(r, reflect.TypeOf(&metricpb.MetricsData{}))
	if err != nil {
		logger.Error(ctx, m.logger, err.Error(), signalTypeLogAttr)
		http.Error(w, "failed to unmarshal metrics", http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal metrics")
		return
	}

	metrics := result.(*metricpb.MetricsData)
	if metrics == nil {
		logger.Error(ctx, m.logger, err.Error(), signalTypeLogAttr)
		http.Error(w, "failed to unmarshal metrics", http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal")
		return
	}

	if err := m.dispatch(ctx, m.partition(ctx, metrics)); err != nil {
		logger.Error(ctx, m.logger, err.Error(), signalTypeLogAttr)
		http.Error(w, "failed to dispatch metrics", http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to dispatch")
		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	w.WriteHeader(http.StatusAccepted)
}

// partition partitions the request by tenant.
func (m *Metrics) partition(ctx context.Context, req *metricpb.MetricsData) map[string]*metricpb.MetricsData {

	ctx, span := m.tracer.Start(ctx, "partition")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	tenantMetricMap := make(map[string]*metricpb.MetricsData)

	for _, resourceMetric := range req.ResourceMetrics {
		logger.Trace(ctx, m.logger, fmt.Sprintf("%+v", resourceMetric), signalTypeLogAttr)

		tenant := ""

		// First, check for the dedicated tenant label
		if m.config.Tenant.Label != "" {
			for _, attr := range resourceMetric.Resource.Attributes {
				if attr.Key == m.config.Tenant.Label {
					tenant = attr.Value.GetStringValue()
					break
				}
			}
		}

		// If not found and we have additional labels, check those
		if tenant == "" && len(m.config.Tenant.Labels) > 0 {
			for _, attr := range resourceMetric.Resource.Attributes {
				if slices.Contains(m.config.Tenant.Labels, attr.Key) {
					tenant = attr.Value.GetStringValue()
					break
				}
			}
		}

		if tenant == "" {
			if m.config.Tenant.Default == "" {
				logger.Warn(ctx, m.logger, "no tenant found in attributes and no default tenant configured", signalTypeLogAttr)
				continue
			}

			tenant = m.config.Tenant.Default
			resourceMetric.Resource.Attributes = append(resourceMetric.Resource.Attributes, &v1.KeyValue{
				Key:   m.config.Tenant.Label,
				Value: &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: tenant}},
			})
		}

		if _, ok := tenantMetricMap[tenant]; !ok {
			tenantMetricMap[tenant] = &metricpb.MetricsData{}
		}

		tenantMetricMap[tenant].ResourceMetrics = append(tenantMetricMap[tenant].ResourceMetrics, resourceMetric)
	}

	span.SetStatus(codes.Ok, "data partitioned")

	return tenantMetricMap
}

// dispatch sends all the request to the target.
func (m *Metrics) dispatch(ctx context.Context, tenantMap map[string]*metricpb.MetricsData) error {

	ctx, span := m.tracer.Start(ctx, "dispatch")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	var wg sync.WaitGroup

	for tenant, metrics := range tenantMap {
		wg.Add(1)
		go func(tenant string, metrics *metricpb.MetricsData) {
			defer wg.Done()

			signalAttributes := []attribute.KeyValue{
				signalTypeAttr,
				attribute.String("signal.tenant", tenant),
			}

			resp, err := m.send(ctx, tenant, metrics)
			if err != nil {

				signalAttributes = append(signalAttributes, attribute.String("signal.status", "failed"))

				m.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
					signalAttributes...,
				))

				m.otelLgtmProxyRecords.Add(ctx, int64(len(metrics.ResourceMetrics)), metric.WithAttributes(
					signalAttributes...,
				))

				logger.Error(ctx, m.logger, err.Error(), signalTypeLogAttr)
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send")

				return
			}

			signalAttributes = append(signalAttributes, attribute.String("signal.status", "success"))

			m.otelLgtmProxyResponseCode.Add(ctx, 1, metric.WithAttributes(
				append(signalAttributes,
					attribute.String("signal.response", fmt.Sprintf("%d", resp.StatusCode)))...,
			))

			m.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
				signalAttributes...,
			))

			m.otelLgtmProxyRecords.Add(ctx, int64(len(metrics.ResourceMetrics)), metric.WithAttributes(
				signalAttributes...,
			))

			logger.Debug(ctx, m.logger, fmt.Sprintf("sent %d records status %d for tenant %s", len(metrics.ResourceMetrics), resp.StatusCode, tenant), signalTypeLogAttr)
			logger.Trace(ctx, m.logger, fmt.Sprintf("%+v", metrics.ResourceMetrics), signalTypeLogAttr)

			span.SetStatus(codes.Ok, "sent successfully")

		}(tenant, metrics)
	}

	wg.Wait()
	return nil
}

// send sends an individual request to the target.
func (m *Metrics) send(ctx context.Context, tenant string, metrics *metricpb.MetricsData) (http.Response, error) {

	start := time.Now()
	ctx, span := m.tracer.Start(ctx, "send")
	defer span.End()

	span.SetAttributes([]attribute.KeyValue{
		signalTypeAttr,
		attribute.String("signal.tenant", tenant),
		attribute.Int("signal.tenant.records", len(metrics.ResourceMetrics)),
	}...)

	body, err := protoutil.Marshal(metrics)
	if err != nil {
		return http.Response{}, err
	}

	req, err := http.NewRequest(http.MethodPost, m.config.Metrics.Address, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return http.Response{}, err
	}

	httputil.AddHeaders(tenant, req, m.config, m.config.Metrics.Headers)

	resp, err := m.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send")
		return http.Response{}, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Failed to close response body: %v\n", err)
		}
	}()

	respAttributes := []attribute.KeyValue{
		attribute.Int64("signal.response.size", resp.ContentLength),
		attribute.String("signal.response.status", resp.Status),
	}

	span.SetAttributes(respAttributes...)
	span.SetStatus(codes.Ok, "sent successfully")

	m.otelLgtmProxyLatency.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		append(respAttributes, signalTypeAttr)...,
	))

	return *resp, nil
}
