// Package otellogs provides functionality for processing log data.
package otellogs

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
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
)

const signalType = "otellogs"

var errUnmarshalFailed = errors.New("failed to unmarshal logs")

func signalTypeAttr() attribute.KeyValue {
	return attribute.String("signal.type", signalType)
}

func signalTypeLogAttr() log.KeyValue {
	return log.String("signal.type", signalType)
}

// OtelLogs handles log processing and routing.
type OtelLogs struct {
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
//go:generate mockgen -package otellogs -source otellogs.go -destination otellogs_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new Logs instance.
func New(
	config *config.Config,
	client Client,
	logger log.Logger,
	meter metric.Meter,
	tracer trace.Tracer,
) (*OtelLogs, error) {
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

	if cert.TLSEnabled(&config.Logs.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(&config.Logs)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger TLS config: %w", err)
		}
		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &OtelLogs{
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

// Handler handles incoming log requests.
func (o *OtelLogs) Handler(resp http.ResponseWriter, req *http.Request) {
	ctx, span := o.tracer.Start(
		req.Context(),
		"otellogs.Handler",
		trace.WithAttributes(
			signalTypeAttr(),
		),
	)
	defer span.End()

	result, err := proto.Unmarshal(req, reflect.TypeFor[*logpb.LogsData]())
	if err != nil {
		logger.Error(ctx, o.logger, err.Error(), signalTypeLogAttr())
		http.Error(resp, err.Error(), http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return
	}

	logs, ok := result.(*logpb.LogsData)
	if !ok || logs == nil {
		logger.Error(ctx, o.logger, errUnmarshalFailed.Error(), signalTypeLogAttr())
		http.Error(resp, errUnmarshalFailed.Error(), http.StatusBadRequest)
		span.RecordError(errUnmarshalFailed)
		span.SetStatus(codes.Error, errUnmarshalFailed.Error())

		return
	}

	err = o.dispatch(ctx, o.partition(ctx, logs))
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
func (o *OtelLogs) partition(ctx context.Context, req *logpb.LogsData) map[string]*logpb.LogsData {
	ctx, span := o.tracer.Start(
		ctx,
		"otellogs.partition",
		trace.WithAttributes(
			signalTypeAttr(),
		),
	)
	defer span.End()

	tenantMap := make(map[string]*logpb.LogsData)

	for _, resourceLog := range req.GetResourceLogs() {
		logger.Trace(
			ctx,
			o.logger,
			fmt.Sprintf("%+v", resourceLog),
			signalTypeLogAttr(),
		)

		tenant := ""

		// First, check for the dedicated tenant label
		if o.config.Tenant.Label != "" {
			for _, attr := range resourceLog.GetResource().GetAttributes() {
				if attr.GetKey() == o.config.Tenant.Label {
					tenant = attr.GetValue().GetStringValue()

					break
				}
			}
		}

		// If not found and we have additional labels, check those
		if tenant == "" && len(o.config.Tenant.Labels) > 0 {
			for _, attr := range resourceLog.GetResource().GetAttributes() {
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
					"No tenant found in attributes and no default tenant configured",
					signalTypeLogAttr(),
				)

				continue
			}

			tenant = o.config.Tenant.Default
			resourceLog.Resource.Attributes = append(resourceLog.Resource.Attributes, &common.KeyValue{
				Key:   o.config.Tenant.Label,
				Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: tenant}},
			})
		}

		if _, ok := tenantMap[tenant]; !ok {
			tenantMap[tenant] = &logpb.LogsData{
				ResourceLogs: []*logpb.ResourceLogs{},
			}
		}

		tenantMap[tenant].ResourceLogs = append(tenantMap[tenant].ResourceLogs, resourceLog)
	}

	span.SetStatus(codes.Ok, "data partitioned")

	return tenantMap
}

// dispatch sends all the request to the target.
func (o *OtelLogs) dispatch(ctx context.Context, tenantMap map[string]*logpb.LogsData) error {
	waitGroup := sync.WaitGroup{}

	for tenant, logs := range tenantMap {
		tenantAttribute := attribute.String("signal.tenant", tenant)

		ctx, span := o.tracer.Start(
			ctx,
			"otellogs.dispatch",
			trace.WithAttributes(
				signalTypeAttr(),
				tenantAttribute,
			),
		)
		defer span.End()

		waitGroup.Add(1)

		go func(tenant string, logs *logpb.LogsData) {
			defer waitGroup.Done()

			resp, err := o.send(ctx, tenant, logs)
			if err != nil {
				o.otelLgtmProxyRecords.Add(
					ctx,
					int64(len(logs.GetResourceLogs())),
					metric.WithAttributes(
						tenantAttribute,
						signalTypeAttr(),
					),
				)

				logger.Error(
					ctx,
					o.logger,
					err.Error(),
					signalTypeLogAttr(),
				)

				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send")

				return
			}

			o.otelLgtmProxyRecords.Add(
				ctx,
				int64(len(logs.GetResourceLogs())),
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
					len(logs.GetResourceLogs()),
					resp.StatusCode,
					tenant,
				),
				signalTypeLogAttr(),
			)

			logger.Trace(
				ctx,
				o.logger,
				fmt.Sprintf("%+v", logs.GetResourceLogs()),
				signalTypeLogAttr(),
			)

			span.SetStatus(codes.Ok, "sent successfully")
		}(tenant, logs)
	}

	waitGroup.Wait()

	return nil
}

// send sends an individual request to the target.
func (o *OtelLogs) send(
	ctx context.Context,
	tenant string,
	logs *logpb.LogsData,
) (http.Response, error) {
	start := time.Now()

	ctx, span := o.tracer.Start(ctx,
		"otellogs.send",
		trace.WithAttributes(
			signalTypeAttr(),
			attribute.String("signal.tenant", tenant),
			attribute.Int("signal.tenant.records", len(logs.GetResourceLogs())),
		),
	)
	defer span.End()

	body, err := proto.Marshal(logs)
	if err != nil {
		return http.Response{}, fmt.Errorf("failed to marshal logs: %w", err)
	}

	// Use detached context for the HTTP request to avoid trace context injection
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		o.config.Logs.Address,
		io.NopCloser(bytes.NewReader(body)),
	)
	if err != nil {
		return http.Response{}, fmt.Errorf("failed to create request: %w", err)
	}

	request.AddHeaders(
		tenant,
		req,
		o.config,
		o.config.Logs.Headers,
	)

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
	span.SetStatus(codes.Ok, "logs sent successfully")

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
