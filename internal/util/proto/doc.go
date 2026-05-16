// Package proto provides utility functions for working with Protocol Buffer messages.
//
// This package handles serialization and deserialization of OTLP protobuf messages
// in HTTP requests and responses:
//   - Unmarshaling protobuf binary format (application/x-protobuf)
//   - Unmarshaling protobuf JSON format (application/json)
//   - Marshaling protobuf messages to binary format
//   - Content-type negotiation based on HTTP headers
//
// The package uses Google's protobuf library for binary encoding and protojson
// for JSON encoding, supporting both formats as specified in the OpenTelemetry
// Protocol specification.
package proto
