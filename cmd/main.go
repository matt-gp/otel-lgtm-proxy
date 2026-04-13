// Package main is the entry point of the application.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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
		log.KeyValue{Key: "service_name", Value: log.StringValue(cfg.Service.Name)},
		log.KeyValue{Key: "service_version", Value: log.StringValue(cfg.Service.Version)},
	)

	// Initialize signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize handlers
	h, err := handler.New(
		cfg,
		http.NewServeMux(),
		&http.Client{Timeout: cfg.Logs.Timeout},
		&http.Client{Timeout: cfg.Metrics.Timeout},
		&http.Client{Timeout: cfg.Traces.Timeout},
		loggingProvider,
		meterProvider,
		tracerProvider,
	)
	if err != nil {
		logger.Error(ctx, loggingProvider, err.Error())
		os.Exit(1)
	}

	// Health check endpoint
	healthEndpoint := "/health"
	logger.Info(
		ctx,
		loggingProvider,
		"registering health check endpoint",
		log.KeyValue{Key: "endpoint", Value: log.StringValue(healthEndpoint)},
	)
	h.Register("GET "+healthEndpoint, h.Health)

	// register the logs handler.
	logsEndpoint := "/v1/logs"
	logger.Info(
		ctx,
		loggingProvider,
		"receiving logs",
		log.KeyValue{Key: "endpoint", Value: log.StringValue(logsEndpoint)},
	)
	h.Register("POST "+logsEndpoint, h.Logs)

	// register the metrics handler.
	metricsEndpoint := "/v1/metrics"
	logger.Info(
		ctx,
		loggingProvider,
		"receiving metrics",
		log.KeyValue{Key: "endpoint", Value: log.StringValue(metricsEndpoint)},
	)
	h.Register("POST "+metricsEndpoint, h.Metrics)

	// register the traces handler.
	tracesEndpoint := "/v1/traces"
	logger.Info(
		ctx,
		loggingProvider,
		"receiving traces",
		log.KeyValue{Key: "endpoint", Value: log.StringValue(tracesEndpoint)},
	)
	h.Register("POST "+tracesEndpoint, h.Traces)

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
				log.KeyValue{Key: "error", Value: log.StringValue(err.Error())},
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
				log.KeyValue{Key: "error", Value: log.StringValue(err.Error())},
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
			log.KeyValue{Key: "address", Value: log.StringValue(cfg.HTTP.Address)},
			log.KeyValue{Key: "tls_enabled", Value: log.BoolValue(tlsEnabled)},
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
				log.KeyValue{Key: "address", Value: log.StringValue(cfg.HTTP.Address)},
				log.KeyValue{Key: "tls_enabled", Value: log.BoolValue(tlsEnabled)},
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
		logger.Error(ctx, loggingProvider, fmt.Sprintf("http close error: %v", err), log.KeyValue{Key: "address", Value: log.StringValue(cfg.HTTP.Address)})
	}
}
