// Package logger provides convenience functions for OpenTelemetry logging.
package logger

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
)

// Debug emits a debug log using OpenTelemetry logging.
func Debug(ctx context.Context, logger log.Logger, msg string, attrs ...attribute.KeyValue) {
	record := log.Record{}
	record.SetSeverity(log.SeverityDebug)
	record.SetBody(log.StringValue(msg))
	for _, attr := range attrs {
		record.AddAttributes(log.KeyValueFromAttribute(attr))
	}
	logger.Emit(ctx, record)
}

// Trace emits a trace log using OpenTelemetry logging.
func Trace(ctx context.Context, logger log.Logger, msg string, attrs ...attribute.KeyValue) {
	record := log.Record{}
	record.SetSeverity(log.SeverityTrace)
	record.SetBody(log.StringValue(msg))
	for _, attr := range attrs {
		record.AddAttributes(log.KeyValueFromAttribute(attr))
	}
	logger.Emit(ctx, record)
}

// Info emits an info log using OpenTelemetry logging
func Info(ctx context.Context, logger log.Logger, msg string, attrs ...attribute.KeyValue) {
	record := log.Record{}
	record.SetSeverity(log.SeverityInfo)
	record.SetBody(log.StringValue(msg))
	for _, attr := range attrs {
		record.AddAttributes(log.KeyValueFromAttribute(attr))
	}
	logger.Emit(ctx, record)
}

// Warn emits a warning log using OpenTelemetry logging
func Warn(ctx context.Context, logger log.Logger, msg string, attrs ...attribute.KeyValue) {
	record := log.Record{}
	record.SetSeverity(log.SeverityWarn)
	record.SetBody(log.StringValue(msg))
	for _, attr := range attrs {
		record.AddAttributes(log.KeyValueFromAttribute(attr))
	}
	logger.Emit(ctx, record)
}

// Error emits an error log using OpenTelemetry logging
func Error(ctx context.Context, logger log.Logger, msg string, attrs ...attribute.KeyValue) {
	record := log.Record{}
	record.SetSeverity(log.SeverityError)
	record.SetBody(log.StringValue(msg))
	for _, attr := range attrs {
		record.AddAttributes(log.KeyValueFromAttribute(attr))
	}
	logger.Emit(ctx, record)
}

// String creates a string-valued log attribute.
func String(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// Int creates an integer-valued log attribute.
func Int(key string, value int) attribute.KeyValue {
	return attribute.Int(key, value)
}

// Int64 creates an int64-valued log attribute.
func Int64(key string, value int64) attribute.KeyValue {
	return attribute.Int64(key, value)
}

// Float64 creates a float64-valued log attribute.
func Float64(key string, value float64) attribute.KeyValue {
	return attribute.Float64(key, value)
}

// Bool creates a boolean-valued log attribute.
func Bool(key string, value bool) attribute.KeyValue {
	return attribute.Bool(key, value)
}

// Err creates an error log attribute with key "error".
func Err(err error) attribute.KeyValue {
	return attribute.String("error", err.Error())
}
