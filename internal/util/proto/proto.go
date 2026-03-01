// Package proto provides utility functions for working with protobuf messages in the context of HTTP requests and responses.
package proto

import (
	"io"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	contentTypeProtoJSON = "application/json"
)

// Marshal marshals the request using protobuf binary format.
func Marshal(payload any) ([]byte, error) {
	return proto.Marshal(payload.(proto.Message))
}

// Unmarshal unmarshals the request.
func Unmarshal[T proto.Message](req *http.Request, targetType T) (T, error) {
	var zero T

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return zero, err
	}

	switch req.Header.Get("Content-Type") {
	case contentTypeProtoJSON:
		if err := protojson.Unmarshal(body, targetType); err != nil {
			return zero, err
		}
	default:
		// Default to binary protobuf
		if err := proto.Unmarshal(body, targetType); err != nil {
			return zero, err
		}
	}

	return targetType, nil
}
