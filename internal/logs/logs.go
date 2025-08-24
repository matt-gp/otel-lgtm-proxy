// Package logs provides functionality for processing log data.
package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/certutil"
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	logpb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Logs struct {
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

//go:generate mockgen -package logs -source logs.go -destination logs_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new Logs instance.
func New(config *config.Config, client Client, logger log.Logger, meter metric.Meter, tracer trace.Tracer) (*Logs, error) {

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

	if certutil.TLSEnabled(&config.Logs.TLS) {

		tlsConfig, err := certutil.CreateTLSConfig(&config.Logs)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger TLS config: %w", err)
		}
		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &Logs{
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

// Handler handles incoming log requests.
func (l *Logs) Handler(w http.ResponseWriter, req *http.Request) {

	ctx, span := l.tracer.Start(req.Context(), "handler")
	span.SetAttributes(attribute.String("signal.type", "logs"))
	defer span.End()

	logs, err := unmarshal(req)
	if err != nil {
		logger.Error(ctx, l.logger, err.Error())
		http.Error(w, "failed to unmarshal logs", http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal logs")
		return
	}

	if err := l.dispatch(ctx, l.partition(ctx, logs)); err != nil {
		logger.Error(ctx, l.logger, err.Error())
		http.Error(w, "failed to dispatch logs", http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to dispatch logs")
		return
	}

	span.SetStatus(codes.Ok, "logs processed successfully")
	w.WriteHeader(http.StatusAccepted)
}

// addHeaders adds the headers to the request.
func (l *Logs) addHeaders(tenant string, req *http.Request) {
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Add(l.config.Tenant.Header, fmt.Sprintf(l.config.Tenant.Format, tenant))

	// Add custom headers
	customHeaders := strings.Split(l.config.Logs.Headers, ",")
	for _, customHeader := range customHeaders {
		kv := strings.SplitN(customHeader, "=", 2)
		if len(kv) == 2 {
			req.Header.Add(kv[0], kv[1])
		}
	}
}

// partition partitions the request by tenant.
func (l *Logs) partition(ctx context.Context, req *logpb.LogsData) map[string]*logpb.LogsData {

	ctx, span := l.tracer.Start(ctx, "partition")
	span.SetAttributes(attribute.String("signal.type", "logs"))
	defer span.End()

	tenantMap := make(map[string]*logpb.LogsData)

	var tenant string
	for _, resourceLog := range req.ResourceLogs {
		logger.Trace(ctx, l.logger, fmt.Sprintf("%+v", resourceLog))
		for _, attr := range resourceLog.Resource.Attributes {
			if attr.Key == l.config.Tenant.Label {
				tenant = attr.Value.GetStringValue()
				break
			}
		}

		if tenant == "" {
			if l.config.Tenant.Default == "" {
				logger.Warn(ctx, l.logger, "No tenant found in attributes and no default tenant configured")
				continue
			}

			tenant = l.config.Tenant.Default
			resourceLog.Resource.Attributes = append(resourceLog.Resource.Attributes, &v1.KeyValue{
				Key:   l.config.Tenant.Label,
				Value: &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: tenant}},
			})
		}

		if _, ok := tenantMap[tenant]; !ok {
			tenantMap[tenant] = &logpb.LogsData{}
		}

		tenantMap[tenant].ResourceLogs = append(tenantMap[tenant].ResourceLogs, resourceLog)
	}

	span.SetStatus(codes.Ok, "data partitioned")

	return tenantMap
}

// dispatch sends all the request to the target.
func (l *Logs) dispatch(ctx context.Context, tenantMap map[string]*logpb.LogsData) error {

	ctx, span := l.tracer.Start(ctx, "dispatch")
	defer span.End()

	var wg sync.WaitGroup

	for tenant, logs := range tenantMap {
		wg.Add(1)
		go func(tenant string, logs *logpb.LogsData) {
			defer wg.Done()

			signalAttributes := []attribute.KeyValue{
				attribute.String("signal.type", "logs"),
				attribute.String("signal.tenant", tenant),
			}

			resp, err := l.send(ctx, tenant, logs)
			if err != nil {

				l.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
					append(signalAttributes, attribute.String("signal.status", "failed"))...,
				))

				l.otelLgtmProxyRecords.Add(ctx, int64(len(logs.ResourceLogs)), metric.WithAttributes(
					append(signalAttributes, attribute.String("signal.status", "failed"))...,
				))

				logger.Error(ctx, l.logger, err.Error())
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send logs")

				return
			}

			l.otelLgtmProxyResponseCode.Add(ctx, 1, metric.WithAttributes(
				append(signalAttributes,
					attribute.String("signal.status", "success"),
					attribute.String("signal.response",
						fmt.Sprintf("%d", resp.StatusCode),
					))...,
			))

			l.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
				append(signalAttributes, attribute.String("signal.status", "success"))...,
			))

			l.otelLgtmProxyRecords.Add(ctx, int64(len(logs.ResourceLogs)), metric.WithAttributes(
				append(signalAttributes, attribute.String("signal.status", "success"))...,
			))

			logger.Debug(ctx, l.logger, fmt.Sprintf("sent %d logs status %d for tenant %s", len(logs.ResourceLogs), resp.StatusCode, tenant))
			logger.Trace(ctx, l.logger, fmt.Sprintf("%+v", logs.ResourceLogs))

			span.SetStatus(codes.Ok, "logs sent successfully")

		}(tenant, logs)
	}

	wg.Wait()
	return nil
}

// send sends an individual request to the target.
func (l *Logs) send(ctx context.Context, tenant string, logs *logpb.LogsData) (http.Response, error) {

	start := time.Now()
	ctx, span := l.tracer.Start(ctx, "send")
	defer span.End()

	span.SetAttributes([]attribute.KeyValue{
		attribute.String("signal.type", "logs"),
		attribute.String("signal.tenant", tenant),
		attribute.Int("signal.tenant.records", len(logs.ResourceLogs)),
	}...)

	body, err := marshal(logs)
	if err != nil {
		return http.Response{}, err
	}

	// Use detached context for the HTTP request to avoid trace context injection
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.config.Logs.Address, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return http.Response{}, err
	}

	l.addHeaders(tenant, req)

	resp, err := l.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send logs")
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
	span.SetStatus(codes.Ok, "logs sent successfully")

	l.otelLgtmProxyLatency.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		respAttributes...,
	))

	return *resp, nil
}

// marshal marshals the request using protobuf binary format.
func marshal(logs *logpb.LogsData) ([]byte, error) {
	return proto.Marshal(logs)
}

// unmarshal unmarshals the request.
func unmarshal(req *http.Request) (*logpb.LogsData, error) {

	var logs logpb.LogsData

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	contentType := req.Header.Get("Content-Type")

	// Try protojson first for JSON-like content
	if contentType == "application/json" || contentType == "" {
		if err := protojson.Unmarshal(body, &logs); err != nil {
			// If protojson fails, try binary protobuf
			if protoErr := proto.Unmarshal(body, &logs); protoErr != nil {
				return nil, err // return the original protojson error
			}
		}
	} else {
		// For protobuf content types, use binary protobuf directly
		if err := proto.Unmarshal(body, &logs); err != nil {
			return nil, err
		}
	}
	return &logs, nil
}
