// Package main is the entry point of the application.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	logger.Info(ctx, loggingProvider, "Starting application")

	// Initialize signal handling
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize handlers
	h, err := handler.New(
		cfg,
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

	// Initialize HTTP router
	router := http.NewServeMux()

	// Health check endpoint
	router.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Error(ctx, loggingProvider, err.Error())
		}
	})

	// register the logs handler.
	logger.Info(ctx, loggingProvider, "receiving logs on /v1/logs")
	router.HandleFunc("POST /v1/logs", h.Logs)

	// register the metrics handler.
	logger.Info(ctx, loggingProvider, "receiving metrics on /v1/metrics")
	router.HandleFunc("POST /v1/metrics", h.Metrics)

	// register the traces handler.
	logger.Info(ctx, loggingProvider, "receiving traces on /v1/traces")
	router.HandleFunc("POST /v1/traces", h.Traces)

	// Initialize TLS configuration
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Load TLS certificates
	if cert.TLSEnabled(&cfg.HTTP.TLS) {
		certPair, err := tls.LoadX509KeyPair(cfg.HTTP.TLS.CertFile, cfg.HTTP.TLS.KeyFile)
		if err != nil {
			logger.Error(ctx, loggingProvider, err.Error())
			os.Exit(1)
		}

		caPool := x509.NewCertPool()
		caCert, err := os.ReadFile(cfg.HTTP.TLS.CAFile)
		if err != nil {
			logger.Error(ctx, loggingProvider, err.Error())
			os.Exit(1)
		}

		caPool.AppendCertsFromPEM(caCert)

		tlsConfig.Certificates = []tls.Certificate{certPair}
		tlsConfig.RootCAs = caPool
		tlsConfig.ClientAuth = cert.StringClientAuthType(cfg.HTTP.TLS.ClientAuthType)
	}

	server := http.Server{
		MaxHeaderBytes: 1 << 20, // 1MB max header size
		Addr:           cfg.HTTP.Address,
		Handler:        otelhttp.NewHandler(router, "otel-lgtm-proxy"),
		TLSConfig:      tlsConfig,
	}

	go func() {
		if cert.TLSEnabled(&cfg.HTTP.TLS) {
			logger.Info(
				ctx,
				loggingProvider,
				fmt.Sprintf("starting https server on %s", cfg.HTTP.Address),
			)
			if err := server.ListenAndServeTLS("", ""); err != nil {
				logger.Error(ctx, loggingProvider, err.Error())
				os.Exit(1)
			}
		} else {
			logger.Info(
				ctx,
				loggingProvider,
				fmt.Sprintf("starting http server on %s", cfg.HTTP.Address),
			)
			if err := server.ListenAndServe(); err != nil {
				logger.Error(ctx, loggingProvider, err.Error())
				os.Exit(1)
			}
		}
	}()

	// Wait for the application to exit.
	<-ctx.Done()
	stop()

	// Shutdown the server.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.TimeoutShutdown)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, loggingProvider, fmt.Sprintf("http close error: %v", err))
	}
}
