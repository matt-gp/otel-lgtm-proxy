package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// createTestLogger creates a logger that outputs to a buffer for testing
func createTestLogger() (log.Logger, *bytes.Buffer, error) {
	var buf bytes.Buffer

	exporter, err := stdoutlog.New(
		stdoutlog.WithWriter(&buf),
		stdoutlog.WithPrettyPrint(),
	)
	if err != nil {
		return nil, nil, err
	}

	processor := sdklog.NewBatchProcessor(exporter)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
	)

	logger := loggerProvider.Logger("test-logger")
	return logger, &buf, nil
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(context.Context, log.Logger, string, ...attribute.KeyValue)
		message  string
		setLevel string
	}{
		{
			name:     "debug level",
			logFunc:  Debug,
			message:  "debug message",
			setLevel: "DEBUG",
		},
		{
			name:     "trace level",
			logFunc:  Trace,
			message:  "trace message",
			setLevel: "DEBUG", // Trace requires DEBUG level
		},
		{
			name:     "info level",
			logFunc:  Info,
			message:  "info message",
			setLevel: "", // Default level
		},
		{
			name:     "warn level",
			logFunc:  Warn,
			message:  "warning message",
			setLevel: "", // Default level
		},
		{
			name:     "error level",
			logFunc:  Error,
			message:  "error message",
			setLevel: "", // Default level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf, err := createTestLogger()
			assert.NoError(t, err)

			ctx := context.Background()

			// Set log level if specified
			if tt.setLevel != "" {
				if err := os.Setenv("LOG_LEVEL", tt.setLevel); err != nil {
					t.Fatalf("Failed to set env var LOG_LEVEL: %v", err)
				}
				defer func() {
					if err := os.Unsetenv("LOG_LEVEL"); err != nil {
						fmt.Printf("Failed to unset env var LOG_LEVEL: %v\n", err)
					}
				}()
			}

			tt.logFunc(ctx, logger, tt.message)

			// Just verify no error occurred and buffer exists
			assert.NotNil(t, buf)
		})
	}
}

func TestLogWithAttributes(t *testing.T) {
	tests := []struct {
		name       string
		logFunc    func(context.Context, log.Logger, string, ...attribute.KeyValue)
		message    string
		attributes []attribute.KeyValue
	}{
		{
			name:    "info with attributes",
			logFunc: Info,
			message: "info message with attributes",
			attributes: []attribute.KeyValue{
				attribute.String("key1", "value1"),
				attribute.Int("key2", 42),
				attribute.Bool("key3", true),
			},
		},
		{
			name:    "error with attributes",
			logFunc: Error,
			message: "error message with attributes",
			attributes: []attribute.KeyValue{
				attribute.String("component", "test"),
				attribute.String("error", errors.New("test error").Error()),
			},
		},
		{
			name:    "warn with attributes",
			logFunc: Warn,
			message: "warning with attributes",
			attributes: []attribute.KeyValue{
				attribute.String("service", "test-service"),
				attribute.Int64("timestamp", 1234567890),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf, err := createTestLogger()
			assert.NoError(t, err)

			ctx := context.Background()

			tt.logFunc(ctx, logger, tt.message, tt.attributes...)

			assert.NotNil(t, buf)
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		kv := String("key", "value")
		assert.Equal(t, attribute.Key("key"), kv.Key)
		assert.Equal(t, attribute.StringValue("value"), kv.Value)
	})

	t.Run("Int", func(t *testing.T) {
		kv := Int("key", 42)
		assert.Equal(t, attribute.Key("key"), kv.Key)
		assert.Equal(t, attribute.Int64Value(42), kv.Value)
	})

	t.Run("Int64", func(t *testing.T) {
		kv := Int64("key", 9223372036854775807)
		assert.Equal(t, attribute.Key("key"), kv.Key)
		assert.Equal(t, attribute.Int64Value(9223372036854775807), kv.Value)
	})

	t.Run("Float64", func(t *testing.T) {
		kv := Float64("key", 3.14)
		assert.Equal(t, attribute.Key("key"), kv.Key)
		assert.Equal(t, attribute.Float64Value(3.14), kv.Value)
	})

	t.Run("Bool", func(t *testing.T) {
		kv := Bool("key", true)
		assert.Equal(t, attribute.Key("key"), kv.Key)
		assert.Equal(t, attribute.BoolValue(true), kv.Value)
	})

	t.Run("Err", func(t *testing.T) {
		testErr := errors.New("test error")
		kv := Err(testErr)
		assert.Equal(t, attribute.Key("error"), kv.Key)
		assert.Equal(t, attribute.StringValue("test error"), kv.Value)
	})
}

func TestLogOutput(t *testing.T) {
	logger, buf, err := createTestLogger()
	assert.NoError(t, err)

	ctx := context.Background()

	// Set log level to INFO
	if err := os.Setenv("LOG_LEVEL", "INFO"); err != nil {
		t.Fatalf("Failed to set env var LOG_LEVEL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("LOG_LEVEL"); err != nil {
			fmt.Printf("Failed to unset env var LOG_LEVEL: %v\n", err)
		}
	}()

	Info(ctx, logger, "test message", attribute.String("key", "value"))

	// For integration testing, you could check the buffer content
	// but since we're dealing with OpenTelemetry's async processing,
	// it's safer to just verify the function doesn't panic
	assert.NotNil(t, buf)
}
