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
	"go.opentelemetry.io/otel/log"
)

type logKey string

const (
	// LogKeyHTTPAddress is the log key for the HTTP server address.
	LogKeyHTTPAddress logKey = "http.address"
	// LogKeyHTTPTLSEnabled is the log key for whether TLS is enabled on the HTTP server.
	LogKeyHTTPTLSEnabled logKey = "http.tls.enabled"
	// LogKeyError is the log key for errors.
	LogKeyError logKey = "error"
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
	provider, err := otel.NewProvider(*cfg)
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
		logger.Error(
			ctx,
			loggingProvider,
			"failed to create logs client",
			log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
		)
		os.Exit(1)
	}

	// Create HTTP clients for metrics
	metricsClient, err := newClient(ctx, loggingProvider, &cfg.Metrics)
	if err != nil {
		logger.Error(
			ctx,
			loggingProvider,
			"failed to create metrics client",
			log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
		)
		os.Exit(1)
	}

	// Create HTTP clients for traces
	tracesClient, err := newClient(ctx, loggingProvider, &cfg.Traces)
	if err != nil {
		logger.Error(
			ctx,
			loggingProvider,
			"failed to create traces client",
			log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
		)
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
			logger.Error(
				ctx,
				loggingProvider,
				"unable to read certificate or key file",
				log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
			)
			os.Exit(1)
		}

		caPool := x509.NewCertPool()
		caCert, err := os.ReadFile(cfg.HTTP.TLS.CAFile)
		if err != nil {
			logger.Error(
				ctx,
				loggingProvider,
				"unable to read CA file",
				log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
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
		tlsEnabled := cert.TLSEnabled(&cfg.HTTP.TLS)
		logger.Info(
			ctx,
			loggingProvider,
			"starting server",
			log.KeyValue{Key: "http.address", Value: log.StringValue(cfg.HTTP.Address)},
			log.KeyValue{Key: "http.tls.enabled", Value: log.BoolValue(tlsEnabled)},
		)

		if tlsEnabled {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(
				ctx,
				loggingProvider,
				err.Error(),
				log.KeyValue{Key: string(LogKeyHTTPAddress), Value: log.StringValue(cfg.HTTP.Address)},
				log.KeyValue{Key: string(LogKeyHTTPTLSEnabled), Value: log.BoolValue(tlsEnabled)},
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
		logger.Error(ctx,
			loggingProvider,
			"http close error",
			log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
			log.KeyValue{Key: string(LogKeyHTTPAddress), Value: log.StringValue(cfg.HTTP.Address)},
			log.KeyValue{Key: string(LogKeyHTTPTLSEnabled), Value: log.BoolValue(cert.TLSEnabled(&cfg.HTTP.TLS))},
		)
		os.Exit(1)
	}
}

// newClient creates a new HTTP client with the specified timeout and TLS configuration.
func newClient(ctx context.Context, loggingProvider log.Logger, endpoint *config.Endpoint) (*http.Client, error) {
	clientAttributes := []log.KeyValue{
		{Key: "http.client.url", Value: log.StringValue(endpoint.Address)},
		{Key: "http.client.timeout", Value: log.Int64Value(int64(endpoint.Timeout.Seconds()))},
		{Key: "http.client.tls.enabled", Value: log.BoolValue(cert.TLSEnabled(&endpoint.TLS))},
	}

	c := &http.Client{Timeout: endpoint.Timeout}
	if cert.TLSEnabled(&endpoint.TLS) {
		tlsConfig, err := cert.CreateTLSConfig(endpoint)
		if err != nil {
			logger.Error(
				ctx,
				loggingProvider,
				"failed to create TLS config",
				append(clientAttributes,
					log.KeyValue{Key: string(LogKeyError), Value: log.StringValue(err.Error())},
				)...,
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
