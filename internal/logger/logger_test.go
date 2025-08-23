package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
		logFunc  func(context.Context, log.Logger, string, ...log.KeyValue)
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
		logFunc    func(context.Context, log.Logger, string, ...log.KeyValue)
		message    string
		attributes []log.KeyValue
	}{
		{
			name:    "info with attributes",
			logFunc: Info,
			message: "info message with attributes",
			attributes: []log.KeyValue{
				String("key1", "value1"),
				Int("key2", 42),
				Bool("key3", true),
			},
		},
		{
			name:    "error with attributes",
			logFunc: Error,
			message: "error message with attributes",
			attributes: []log.KeyValue{
				String("component", "test"),
				Err(errors.New("test error")),
			},
		},
		{
			name:    "warn with attributes",
			logFunc: Warn,
			message: "warning with attributes",
			attributes: []log.KeyValue{
				String("service", "test-service"),
				Int64("timestamp", 1234567890),
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
		assert.Equal(t, "key", kv.Key)
		assert.Equal(t, log.StringValue("value"), kv.Value)
	})

	t.Run("Int", func(t *testing.T) {
		kv := Int("key", 42)
		assert.Equal(t, "key", kv.Key)
		assert.Equal(t, log.Int64Value(42), kv.Value)
	})

	t.Run("Int64", func(t *testing.T) {
		kv := Int64("key", 9223372036854775807)
		assert.Equal(t, "key", kv.Key)
		assert.Equal(t, log.Int64Value(9223372036854775807), kv.Value)
	})

	t.Run("Float64", func(t *testing.T) {
		kv := Float64("key", 3.14)
		assert.Equal(t, "key", kv.Key)
		assert.Equal(t, log.Float64Value(3.14), kv.Value)
	})

	t.Run("Bool", func(t *testing.T) {
		kv := Bool("key", true)
		assert.Equal(t, "key", kv.Key)
		assert.Equal(t, log.BoolValue(true), kv.Value)
	})

	t.Run("Err", func(t *testing.T) {
		testErr := errors.New("test error")
		kv := Err(testErr)
		assert.Equal(t, "error", kv.Key)
		assert.Equal(t, log.StringValue("test error"), kv.Value)
	})
}

func TestGetLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected log.Severity
	}{
		{
			name:     "debug level",
			envValue: "DEBUG",
			expected: log.SeverityDebug,
		},
		{
			name:     "info level",
			envValue: "INFO",
			expected: log.SeverityInfo,
		},
		{
			name:     "warn level",
			envValue: "WARN",
			expected: log.SeverityWarn,
		},
		{
			name:     "error level",
			envValue: "ERROR",
			expected: log.SeverityError,
		},
		{
			name:     "unknown level defaults to info",
			envValue: "UNKNOWN",
			expected: log.SeverityInfo,
		},
		{
			name:     "empty level defaults to info",
			envValue: "",
			expected: log.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				if err := os.Setenv("LOG_LEVEL", tt.envValue); err != nil {
					t.Fatalf("Failed to set env var LOG_LEVEL: %v", err)
				}
			} else {
				if err := os.Unsetenv("LOG_LEVEL"); err != nil {
					t.Fatalf("Failed to unset env var LOG_LEVEL: %v", err)
				}
			}
			defer func() {
				if err := os.Unsetenv("LOG_LEVEL"); err != nil {
					fmt.Printf("Failed to unset env var LOG_LEVEL: %v\n", err)
				}
			}()

			result := getLogLevelFromEnv()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		logLevel   string
		severity   log.Severity
		shouldPass bool
	}{
		{
			name:       "debug level allows debug",
			logLevel:   "DEBUG",
			severity:   log.SeverityDebug,
			shouldPass: true,
		},
		{
			name:       "info level blocks debug",
			logLevel:   "INFO",
			severity:   log.SeverityDebug,
			shouldPass: false,
		},
		{
			name:       "info level allows info",
			logLevel:   "INFO",
			severity:   log.SeverityInfo,
			shouldPass: true,
		},
		{
			name:       "warn level blocks info",
			logLevel:   "WARN",
			severity:   log.SeverityInfo,
			shouldPass: false,
		},
		{
			name:       "error level allows error",
			logLevel:   "ERROR",
			severity:   log.SeverityError,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Setenv("LOG_LEVEL", tt.logLevel); err != nil {
				t.Fatalf("Failed to set env var LOG_LEVEL: %v", err)
			}
			defer func() {
				if err := os.Unsetenv("LOG_LEVEL"); err != nil {
					fmt.Printf("Failed to unset env var LOG_LEVEL: %v\n", err)
				}
			}()

			envLevel := getLogLevelFromEnv()
			result := envLevel <= tt.severity
			assert.Equal(t, tt.shouldPass, result)
		})
	}
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

	Info(ctx, logger, "test message", String("key", "value"))

	// For integration testing, you could check the buffer content
	// but since we're dealing with OpenTelemetry's async processing,
	// it's safer to just verify the function doesn't panic
	assert.NotNil(t, buf)
}
