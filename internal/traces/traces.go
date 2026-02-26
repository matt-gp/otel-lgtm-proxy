// Package traces provides functionality for processing trace data.
package traces

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/matt-gp/otel-lgtm-proxy/internal/certutil"
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	SIGNAL_TYPE = "traces"
)

var (
	signalTypeAttr    = attribute.String("signal.type", SIGNAL_TYPE)
	signalTypeLogAttr = log.String("signal.type", SIGNAL_TYPE)
)

type Traces struct {
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

//go:generate mockgen -package traces -source traces.go -destination traces_mock.go
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new Traces instance.
func New(config *config.Config, client Client, logger log.Logger, meter metric.Meter, tracer trace.Tracer) (*Traces, error) {

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

	if certutil.TLSEnabled(&config.Traces.TLS) {

		tlsConfig, err := certutil.CreateTLSConfig(&config.Traces)
		if err != nil {
			return nil, fmt.Errorf("failed to create tracer TLS config: %w", err)
		}

		client.(*http.Client).Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &Traces{
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

// Handler handles incoming trace requests.
func (t *Traces) Handler(w http.ResponseWriter, r *http.Request) {

	// Add signal type to baggage so it propagates to all child spans
	member, _ := baggage.NewMember("signal.type", SIGNAL_TYPE)
	bag, _ := baggage.New(member)
	ctx := baggage.ContextWithBaggage(r.Context(), bag)

	ctx, span := t.tracer.Start(ctx, "handler")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	traces, err := unmarshal(r)
	if err != nil {
		logger.Error(ctx, t.logger, err.Error(), signalTypeLogAttr)
		http.Error(w, "failed to unmarshal traces", http.StatusBadRequest)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal")
		return
	}

	if err := t.dispatch(ctx, t.partition(ctx, traces)); err != nil {
		logger.Error(ctx, t.logger, err.Error(), signalTypeLogAttr)
		http.Error(w, "failed to dispatch traces", http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to dispatch")
		return
	}

	span.SetStatus(codes.Ok, "processed successfully")
	w.WriteHeader(http.StatusAccepted)
}

// addHeaders adds the headers to the request.
func (t *Traces) addHeaders(tenant string, req *http.Request) {
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Add(t.config.Tenant.Header, fmt.Sprintf(t.config.Tenant.Format, tenant))

	// Add custom headers
	customHeaders := strings.Split(t.config.Logs.Headers, ",")
	for _, customHeader := range customHeaders {
		kv := strings.SplitN(customHeader, "=", 2)
		if len(kv) == 2 {
			req.Header.Add(kv[0], kv[1])
		}
	}
}

// partition partitions the request by tenant.
func (t *Traces) partition(ctx context.Context, req *tracepb.TracesData) map[string]*tracepb.TracesData {

	ctx, span := t.tracer.Start(ctx, "partition")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	tenantMap := make(map[string]*tracepb.TracesData)

	for _, resouceSpan := range req.ResourceSpans {
		logger.Trace(ctx, t.logger, fmt.Sprintf("%+v", resouceSpan.Resource.Attributes), signalTypeLogAttr)

		tenant := ""

		// First, check for the dedicated tenant label
		if t.config.Tenant.Label != "" {
			for _, attr := range resouceSpan.Resource.Attributes {
				if attr.Key == t.config.Tenant.Label {
					tenant = attr.Value.GetStringValue()
					break
				}
			}
		}

		// If not found and we have additional labels, check those
		if tenant == "" && len(t.config.Tenant.Labels) > 0 {
			for _, attr := range resouceSpan.Resource.Attributes {
				if slices.Contains(t.config.Tenant.Labels, attr.Key) {
					tenant = attr.Value.GetStringValue()
					break
				}
			}
		}

		if tenant == "" {
			if t.config.Tenant.Default == "" {
				logger.Warn(ctx, t.logger, "no tenant found in span attributes and no default tenant configured", signalTypeLogAttr)
				continue
			}

			tenant = t.config.Tenant.Default
			resouceSpan.Resource.Attributes = append(resouceSpan.Resource.Attributes, &v1.KeyValue{
				Key:   t.config.Tenant.Label,
				Value: &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: tenant}},
			})
		}

		if _, ok := tenantMap[tenant]; !ok {
			tenantMap[tenant] = &tracepb.TracesData{}
		}

		tenantMap[tenant].ResourceSpans = append(tenantMap[tenant].ResourceSpans, resouceSpan)
	}

	span.SetStatus(codes.Ok, "data partitioned")

	return tenantMap
}

// dispatch sends all the request to the target.
func (t *Traces) dispatch(ctx context.Context, tenantMap map[string]*tracepb.TracesData) error {

	ctx, span := t.tracer.Start(ctx, "dispatch")
	defer span.End()
	span.SetAttributes(signalTypeAttr)

	var wg sync.WaitGroup

	for tenant, traces := range tenantMap {
		wg.Add(1)
		go func(tenant string, traces *tracepb.TracesData) {
			defer wg.Done()

			signalAttributes := []attribute.KeyValue{
				signalTypeAttr,
				attribute.String("signal.tenant", tenant),
			}

			resp, err := t.send(ctx, tenant, traces)
			if err != nil {

				signalAttributes = append(signalAttributes, attribute.String("signal.status", "failed"))

				t.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
					signalAttributes...,
				))

				t.otelLgtmProxyRecords.Add(ctx, int64(len(traces.ResourceSpans)), metric.WithAttributes(
					signalAttributes...,
				))

				logger.Error(ctx, t.logger, err.Error(), signalTypeLogAttr)
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed to send")

				return
			}

			signalAttributes = append(signalAttributes, attribute.String("signal.status", "success"))

			t.otelLgtmProxyResponseCode.Add(ctx, 1, metric.WithAttributes(
				append(signalAttributes,
					attribute.String("signal.response", fmt.Sprintf("%d", resp.StatusCode)))...,
			))

			t.otelLgtmProxyRequests.Add(ctx, 1, metric.WithAttributes(
				signalAttributes...,
			))

			t.otelLgtmProxyRecords.Add(ctx, int64(len(traces.ResourceSpans)), metric.WithAttributes(
				signalAttributes...,
			))

			logger.Debug(ctx, t.logger, fmt.Sprintf("sent %d records status %d for tenant %s", len(traces.ResourceSpans), resp.StatusCode, tenant), signalTypeLogAttr)
			logger.Trace(ctx, t.logger, fmt.Sprintf("%+v", traces.ResourceSpans), signalTypeLogAttr)

			span.SetStatus(codes.Ok, "sent successfully")

		}(tenant, traces)
	}

	wg.Wait()
	return nil
}

// send sends an individual request to the target.
func (t *Traces) send(ctx context.Context, tenant string, traces *tracepb.TracesData) (http.Response, error) {

	start := time.Now()
	ctx, span := t.tracer.Start(ctx, "send")
	defer span.End()

	span.SetAttributes([]attribute.KeyValue{
		signalTypeAttr,
		attribute.String("signal.tenant", tenant),
		attribute.Int("signal.tenant.records", len(traces.ResourceSpans)),
	}...)

	body, err := marshal(traces)
	if err != nil {
		return http.Response{}, err
	}

	req, err := http.NewRequest(http.MethodPost, t.config.Traces.Address, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return http.Response{}, err
	}

	t.addHeaders(tenant, req)

	resp, err := t.client.Do(req)
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

	t.otelLgtmProxyLatency.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		append(respAttributes, signalTypeAttr)...,
	))

	return *resp, nil
}

// marshal marshals the request using protobuf binary format.
func marshal(traces *tracepb.TracesData) ([]byte, error) {
	return proto.Marshal(traces)
}

// unmarshal unmarshals the request.
func unmarshal(req *http.Request) (*tracepb.TracesData, error) {

	var traces tracepb.TracesData

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	contentType := req.Header.Get("Content-Type")

	// Try protojson first for JSON-like content
	if contentType == "application/json" || contentType == "" {
		if err := protojson.Unmarshal(body, &traces); err != nil {
			// If protojson fails, try binary protobuf
			if protoErr := proto.Unmarshal(body, &traces); protoErr != nil {
				return nil, err // return the original protojson error
			}
		}
	} else {
		// For protobuf content types, use binary protobuf directly
		if err := proto.Unmarshal(body, &traces); err != nil {
			return nil, err
		}
	}

	return &traces, nil
}
