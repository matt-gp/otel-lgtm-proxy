// Package otelmetrics provides functionality for processing metric data.
package otelmetrics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/cert"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/proto"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/request"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

const signalType = "otelmetrics"

var errUnmarshalFailed = errors.New("failed to unmarshal metrics")

func signalTypeAttr() attribute.KeyValue {
	return attribute.String("signal.type", signalType)
}

func signalTypeLogAttr() log.KeyValue {
	return log.String("signal.type", signalType)
}

// OtelMetrics is a struct that handles processing of metric data.
type OtelMetrics struct {
	config                *config.Config
	client                Client
	logger                log.Logger
	meter                 metric.Meter
	tracer                trace.Tracer
	otelLgtmProxyRecords  metric.Int64Counter
	otelLgtmProxyRequests metric.Int64Counter
	otelLgtmProxyLatency  metric.Int64Histogram
}

// Client is an interface for making HTTP requests.
//
//go:generate mockgen -package otelmetrics -source otelmetrics.go -destination otelmetrics_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new Metrics instance.
func New(
	config *config.Config,
	client Client,
	logger log.Logger,
	meter metric.Meter,
	tracer trace.Tracer,
) (*OtelMetrics, error) {
	otelLgtmProxyRecords, err := meter.Int64Counter(
		"otel_lgtm_proxy_records_total",
		metric.WithDescription("Total number of otel lgtm proxy records processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy records counter: %w", err)
	}

	otelLgtmProxyRequests, err := meter.Int64Counter(
		"otel_lgtm_proxy_requests_total",
		metric.WithDescription("Total number of otel lgtm proxy requests processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy requests counter: %w", err)
	}

	otelLgtmProxyLatency, err := meter.Int64Histogram(
		"otel_lgtm_proxy_request_duration_seconds",
		metric.WithDescription("Latency of otel lgtm proxy requests"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel lgtm proxy latency histogram: %w", err)
	}

	if cert.TLSEnabled(&config.Metrics.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(&config.Metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to create meter TLS config: %w", err)
		}
		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &OtelMetrics{
		config:                config,
		client:                client,
		logger:                logger,
		meter:                 meter,
		tracer:                tracer,
		otelLgtmProxyRecords:  otelLgtmProxyRecords,
		otelLgtmProxyRequests: otelLgtmProxyRequests,
		otelLgtmProxyLatency:  otelLgtmProxyLatency,
	}, nil
}

// Handler handles incoming metric requests.
func (o *OtelMetrics) Handler(resp http.ResponseWriter, req *http.Request) {
	ctx, span := o.tracer.Start(
		req.Context(),
		"otelmetrics.Handler",
		trace.WithAttributes(
			signalTypeAttr(),
		),
	)
	defer span.End()

	result, err := proto.Unmarshal(req, reflect.TypeFor[*metricpb.MetricsData]())
	if err != nil {
		logger.Error(ctx, o.logger, err.Error(), signalTypeLogAttr())
		http.Error(resp, err.Error(), http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return
	}

	metrics, ok := result.(*metricpb.MetricsData)
	if !ok || metrics == nil {
		logger.Error(ctx, o.logger, errUnmarshalFailed.Error(), signalTypeLogAttr())
		http.Error(resp, errUnmarshalFailed.Error(), http.StatusBadRequest)
		span.RecordError(errUnmarshalFailed)
		span.SetStatus(codes.Error, errUnmarshalFailed.Error())

		return
	}

	err = o.dispatch(ctx, o.partition(ctx, metrics))
	if err != nil {
		logger.Error(ctx, o.logger, err.Error(), signalTypeLogAttr())
		http.Error(
			resp,
			err.Error(),
			http.StatusInternalServerError,
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	resp.WriteHeader(http.StatusAccepted)
}

// partition partitions the request by tenant.
func (o *OtelMetrics) partition(
	ctx context.Context,
	req *metricpb.MetricsData,
) map[string]*metricpb.MetricsData {
	ctx, span := o.tracer.Start(
		ctx,
		"otelmetrics.partition",
		trace.WithAttributes(
			signalTypeAttr(),
		),
	)
	defer span.End()

	tenantMetricMap := make(map[string]*metricpb.MetricsData)

	for _, resourceMetric := range req.GetResourceMetrics() {
		logger.Trace(ctx, o.logger, fmt.Sprintf("%+v", resourceMetric), signalTypeLogAttr())

		tenant := ""

		// First, check for the dedicated tenant label
		if o.config.Tenant.Label != "" {
			for _, attr := range resourceMetric.GetResource().GetAttributes() {
				if attr.GetKey() == o.config.Tenant.Label {
					tenant = attr.GetValue().GetStringValue()

					break
				}
			}
		}

		// If not found and we have additional labels, check those
		if tenant == "" && len(o.config.Tenant.Labels) > 0 {
			for _, attr := range resourceMetric.GetResource().GetAttributes() {
				if slices.Contains(o.config.Tenant.Labels, attr.GetKey()) {
					tenant = attr.GetValue().GetStringValue()

					break
				}
			}
		}

		if tenant == "" {
			if o.config.Tenant.Default == "" {
				logger.Warn(
					ctx,
					o.logger,
					"no tenant found in attributes and no default tenant configured",
					signalTypeLogAttr(),
				)

				continue
			}

			tenant = o.config.Tenant.Default
			resourceMetric.Resource.Attributes = append(
				resourceMetric.Resource.Attributes,
				&common.KeyValue{
					Key:   o.config.Tenant.Label,
					Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: tenant}},
				},
			)
		}

		if _, ok := tenantMetricMap[tenant]; !ok {
			tenantMetricMap[tenant] = &metricpb.MetricsData{
				ResourceMetrics: []*metricpb.ResourceMetrics{},
			}
		}

		tenantMetricMap[tenant].ResourceMetrics = append(
			tenantMetricMap[tenant].ResourceMetrics,
			resourceMetric,
		)
	}

	span.SetStatus(codes.Ok, "data partitioned")

	return tenantMetricMap
}

// dispatch sends all the request to the target.
func (o *OtelMetrics) dispatch(ctx context.Context, tenantMap map[string]*metricpb.MetricsData) error {
	waitGroup := sync.WaitGroup{}

	for tenant, metrics := range tenantMap {
		tenantAttribute := attribute.String("signal.tenant", tenant)

		ctx, span := o.tracer.Start(
			ctx,
			"otelmetrics.dispatch",
			trace.WithAttributes(
				signalTypeAttr(),
				tenantAttribute,
			),
		)
		defer span.End()

		waitGroup.Add(1)

		go func(tenant string, metrics *metricpb.MetricsData) {
			defer waitGroup.Done()

			resp, err := o.send(ctx, tenant, metrics)
			if err != nil {
				o.otelLgtmProxyRecords.Add(
					ctx,
					int64(len(metrics.GetResourceMetrics())),
					metric.WithAttributes(
						tenantAttribute,
						signalTypeAttr(),
					),
				)

				logger.Error(ctx, o.logger, err.Error(), signalTypeLogAttr())
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send")

				return
			}

			o.otelLgtmProxyRecords.Add(
				ctx,
				int64(len(metrics.GetResourceMetrics())),
				metric.WithAttributes(
					signalTypeAttr(),
					tenantAttribute,
					attribute.String(
						"signal.response.status.code",
						strconv.Itoa(resp.StatusCode),
					),
				),
			)

			o.otelLgtmProxyRequests.Add(
				ctx,
				1,
				metric.WithAttributes(
					signalTypeAttr(),
					tenantAttribute,
					attribute.String(
						"signal.response.status.code",
						strconv.Itoa(resp.StatusCode),
					),
				),
			)

			logger.Debug(
				ctx,
				o.logger,
				fmt.Sprintf(
					"sent %d records status %d for tenant %s",
					len(metrics.GetResourceMetrics()),
					resp.StatusCode,
					tenant,
				),
				signalTypeLogAttr(),
			)

			logger.Trace(
				ctx,
				o.logger,
				fmt.Sprintf("%+v", metrics.GetResourceMetrics()),
				signalTypeLogAttr(),
			)

			span.SetStatus(codes.Ok, "sent successfully")
		}(tenant, metrics)
	}

	waitGroup.Wait()

	return nil
}

// send sends an individual request to the target.
func (o *OtelMetrics) send(
	ctx context.Context,
	tenant string,
	metrics *metricpb.MetricsData,
) (http.Response, error) {
	start := time.Now()

	ctx, span := o.tracer.Start(ctx,
		"otelmetrics.send",
		trace.WithAttributes(
			signalTypeAttr(),
			attribute.String("signal.tenant", tenant),
			attribute.Int("signal.tenant.records", len(metrics.GetResourceMetrics())),
		),
	)
	defer span.End()

	body, err := proto.Marshal(metrics)
	if err != nil {
		return http.Response{}, fmt.Errorf("failed to marshal metrics: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		o.config.Metrics.Address,
		io.NopCloser(bytes.NewReader(body)),
	)
	if err != nil {
		return http.Response{}, fmt.Errorf("failed to create request: %w", err)
	}

	request.AddHeaders(tenant, req, o.config, o.config.Metrics.Headers)

	resp, err := o.client.Do(req)
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
				o.logger,
				fmt.Sprintf("failed to close response body: %v", closeErr),
				signalTypeLogAttr(),
			)
		}
	}()

	respAttr := attribute.String("signal.response.status.code", strconv.Itoa(resp.StatusCode))
	span.SetAttributes(respAttr)
	span.SetStatus(codes.Ok, "sent successfully")

	o.otelLgtmProxyLatency.Record(ctx,
		time.Since(start).Milliseconds(),
		metric.WithAttributes(
			respAttr,
			signalTypeAttr(),
			attribute.String("signal.tenant", tenant),
		),
	)

	return *resp, nil
}
