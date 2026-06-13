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
