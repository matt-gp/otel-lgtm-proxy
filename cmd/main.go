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

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
	"github.com/matt-gp/otel-lgtm-proxy/internal/handler"
	"github.com/matt-gp/otel-lgtm-proxy/internal/logger"
	"github.com/matt-gp/otel-lgtm-proxy/internal/otel"
	"github.com/matt-gp/otel-lgtm-proxy/internal/util/cert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
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
	provider, err := otel.NewProvider(ctx, *cfg)
	if err != nil {
		panic(err)
	}

	// Initialize OpenTelemetry providers
	loggingProvider := provider.LoggerProvider.Logger("logs")
	meterProvider := provider.MeterProvider.Meter("metrics")
	tracerProvider := provider.TracerProvider.Tracer("traces")

	// Start application
	logger.Info(
		ctx,
		loggingProvider,
		"Starting application",
	)

	// Initialize signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create HTTP clients for logs
	logsClient, err := newClient(ctx, loggingProvider, &cfg.Logs)
	if err != nil {
		logger.Error(ctx, loggingProvider, "failed to create logs client", logger.Err(err))
		os.Exit(1)
	}

	// Create HTTP clients for metrics
	metricsClient, err := newClient(ctx, loggingProvider, &cfg.Metrics)
	if err != nil {
		logger.Error(ctx, loggingProvider, "failed to create metrics client", logger.Err(err))
		os.Exit(1)
	}

	// Create HTTP clients for traces
	tracesClient, err := newClient(ctx, loggingProvider, &cfg.Traces)
	if err != nil {
		logger.Error(ctx, loggingProvider, "failed to create traces client", logger.Err(err))
		os.Exit(1)
	}

	// Initialize handlers
	h, err := handler.New(
		cfg,
		http.NewServeMux(),
		logsClient,
		metricsClient,
		tracesClient,
		loggingProvider,
		meterProvider,
		tracerProvider,
	)
	if err != nil {
		logger.Error(
			ctx,
			loggingProvider,
			err.Error(),
		)
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

	// Load TLS certificates
	if cert.TLSEnabled(&cfg.HTTP.TLS) {
		certPair, err := tls.LoadX509KeyPair(cfg.HTTP.TLS.CertFile, cfg.HTTP.TLS.KeyFile)
		if err != nil {
			logger.Error(ctx, loggingProvider, "unable to read certificate or key file", logger.Err(err))
			os.Exit(1)
		}

		caPool := x509.NewCertPool()
		caCert, err := os.ReadFile(cfg.HTTP.TLS.CAFile)
		if err != nil {
			logger.Error(ctx, loggingProvider, "unable to read CA file", logger.Err(err))
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
		tlsEnabled := cert.TLSEnabled(&cfg.HTTP.TLS)
		logger.Info(ctx, loggingProvider, "starting server",
			logger.String(otel.HTTPAddressAttrKey, cfg.HTTP.Address),
			logger.Bool(otel.HTTPTLSEnabledAttrKey, tlsEnabled),
		)

		if tlsEnabled {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, loggingProvider, err.Error(),
				logger.String(otel.HTTPAddressAttrKey, cfg.HTTP.Address),
				logger.Bool(otel.HTTPTLSEnabledAttrKey, tlsEnabled),
			)
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
		logger.Error(ctx, loggingProvider, "http close error",
			logger.Err(err),
			logger.String(otel.HTTPAddressAttrKey, cfg.HTTP.Address),
			logger.Bool(otel.HTTPTLSEnabledAttrKey, cert.TLSEnabled(&cfg.HTTP.TLS)),
		)
		os.Exit(1)
	}
}

// newClient creates a new HTTP client with the specified timeout and TLS configuration.
func newClient(ctx context.Context, loggingProvider log.Logger, endpoint *config.Endpoint) (*http.Client, error) {
	clientAttributes := []attribute.KeyValue{
		logger.String(otel.HTTPClientURLAttrKey, endpoint.Address),
		logger.Int64(otel.HTTPClientTimeoutAttrKey, int64(endpoint.Timeout.Seconds())),
		logger.Bool(otel.HTTPClientTLSEnabledAttrKey, cert.TLSEnabled(&endpoint.TLS)),
	}

	c := &http.Client{Timeout: endpoint.Timeout}
	if cert.TLSEnabled(&endpoint.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(endpoint)
		if err != nil {
			logger.Error(
				ctx,
				loggingProvider,
				"failed to create TLS config",
				append(clientAttributes, logger.Err(err))...,
			)
			return nil, err
		}
		c.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	logger.Info(
		ctx,
		loggingProvider,
		"created HTTP client",
		clientAttributes...,
	)

	return c, nil
}
