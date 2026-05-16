// Package logger provides convenience functions for OpenTelemetry logging.
//
// This package wraps the OpenTelemetry logging API to provide simple functions
// for emitting logs at different severity levels:
//   - Trace: Most verbose, for detailed debugging
//   - Debug: Detailed information for debugging
//   - Info: General informational messages
//   - Warn: Warning messages for potentially problematic situations
//   - Error: Error messages for failures that don't stop execution
//   - Fatal: Critical errors that require immediate attention
//
// The log level is controlled by the OTEL_LOG_LEVEL environment variable,
// which filters out messages below the configured severity level.
package logger
