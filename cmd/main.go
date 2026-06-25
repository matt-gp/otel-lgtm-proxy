// Package main is the entry point of the application.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/matt-gp/core/logger"
	"github.com/matt-gp/core/otel"
	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/handler"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/cert"
	"go.opentelemetry.io/otel/attribute"
)

var (
	errAttrKey                  = "error"
	httpAddressAttrKey          = "http.address"
	httpTLSEnabledAttrKey       = "http.tls.enabled"
	httpClientURLAttrKey        = "http.client.url"
	httpClientTimeoutAttrKey    = "http.client.timeout"
	httpClientTLSEnabledAttrKey = "http.client.tls.enabled"
)

func main() {
	// Initialize context
	ctx := context.Background()

	// Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		panic(err)
	}

	// Initialize OpenTelemetry provider
	provider, err := otel.NewProvider(ctx)
	if err != nil {
		panic(err)
	}

	// Initialize OpenTelemetry providers
	loggingProvider := provider.LoggerProvider.Logger("logs")
	meterProvider := provider.MeterProvider.Meter("metrics")
	tracerProvider := provider.TracerProvider.Tracer("traces")

	// Initialize logger
	logger.SetProvider(loggingProvider)

	// Start application
	logger.Info(ctx, "Starting application")

	// Initialize signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create HTTP clients for logs
	logsClient, err := newClient(ctx, &cfg.Logs)
	if err != nil {
		logger.Error(ctx, "failed to create logs client", attribute.String(errAttrKey, err.Error()))
		os.Exit(1)
	}

	// Create HTTP clients for metrics
	metricsClient, err := newClient(ctx, &cfg.Metrics)
	if err != nil {
		logger.Error(ctx, "failed to create metrics client", attribute.String(errAttrKey, err.Error()))
		os.Exit(1)
	}

	// Create HTTP clients for traces
	tracesClient, err := newClient(ctx, &cfg.Traces)
	if err != nil {
		logger.Error(ctx, "failed to create traces client", attribute.String(errAttrKey, err.Error()))
		os.Exit(1)
	}

	// Initialize handlers
	h, err := handler.New(
		cfg,
		http.NewServeMux(),
		logsClient,
		metricsClient,
		tracesClient,
		meterProvider,
		tracerProvider,
	)
	if err != nil {
		logger.Error(ctx, err.Error())
		os.Exit(1)
	}

	// Health check endpoint
	h.Register(ctx, "GET /health", h.Health)

	// register the logs handler.
	h.Register(ctx, "POST /v1/logs", h.Logs)

	// register the metrics handler.
	h.Register(ctx, "POST /v1/metrics", h.Metrics)

	// register the traces handler.
	h.Register(ctx, "POST /v1/traces", h.Traces)

	// Initialize TLS configuration
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Add attributes for TLS configuration
	tlsEnabled := cert.TLSEnabled(&cfg.HTTP.TLS)
	httpAttributes := []attribute.KeyValue{
		attribute.String(httpAddressAttrKey, cfg.HTTP.Address),
		attribute.Bool(httpTLSEnabledAttrKey, tlsEnabled),
	}

	// Load TLS certificates
	if tlsEnabled {
		certPair, err := tls.LoadX509KeyPair(cfg.HTTP.TLS.CertFile, cfg.HTTP.TLS.KeyFile)
		if err != nil {
			logger.Error(ctx, "unable to read certificate or key file",
				append(httpAttributes, attribute.String(errAttrKey, err.Error()))...,
			)
			os.Exit(1)
		}

		caPool := x509.NewCertPool()
		caCert, err := os.ReadFile(cfg.HTTP.TLS.CAFile)
		if err != nil {
			logger.Error(ctx, "unable to read CA file",
				append(httpAttributes, attribute.String(errAttrKey, err.Error()))...,
			)
			os.Exit(1)
		}

		caPool.AppendCertsFromPEM(caCert)

		tlsConfig.Certificates = []tls.Certificate{certPair}
		tlsConfig.RootCAs = caPool
		tlsConfig.ClientAuth = cert.StringClientAuthType(cfg.HTTP.TLS.ClientAuthType)
	}

	// Create new HTTP server with the provided TLS configuration.
	server := h.NewServer(tlsConfig)

	go func() {
		logger.Info(ctx, "starting server", httpAttributes...)

		if tlsEnabled {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, err.Error(), httpAttributes...)
			os.Exit(1)
		}
	}()

	// Wait for the application to exit.
	<-ctx.Done()
	stop()

	// Shutdown the server.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.TimeoutShutdown)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "http close error",
			append(httpAttributes, attribute.String(errAttrKey, err.Error()))...,
		)
		os.Exit(1)
	}
}

// newClient creates a new HTTP client with the specified timeout and TLS configuration.
func newClient(ctx context.Context, endpoint *config.Endpoint) (*http.Client, error) {
	clientAttributes := []attribute.KeyValue{
		attribute.String(httpClientURLAttrKey, endpoint.Address),
		attribute.Int64(httpClientTimeoutAttrKey, int64(endpoint.Timeout.Seconds())),
		attribute.Bool(httpClientTLSEnabledAttrKey, cert.TLSEnabled(&endpoint.TLS)),
	}

	c := &http.Client{Timeout: endpoint.Timeout}
	if cert.TLSEnabled(&endpoint.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(endpoint)
		if err != nil {
			logger.Error(ctx, "failed to create TLS config",
				append(clientAttributes, attribute.String(errAttrKey, err.Error()))...,
			)
			return nil, err
		}
		c.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	logger.Info(ctx, "created HTTP client", clientAttributes...)

	return c, nil
}
