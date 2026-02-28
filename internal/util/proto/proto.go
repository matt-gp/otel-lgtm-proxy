// Package proto provides utility functions for working with protobuf messages in the context of HTTP requests and responses.
package proto

import (
	"io"
	"net/http"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Marshal marshals the request using protobuf binary format.
func Marshal(payload any) ([]byte, error) {
	return proto.Marshal(payload.(proto.Message))
}

// Unmarshal unmarshals the request.
func Unmarshal(req *http.Request, targetType reflect.Type) (any, error) {
	// Create a new instance of the target type
	target := reflect.New(targetType.Elem()).Interface().(proto.Message)

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	contentType := req.Header.Get("Content-Type")

	// Try protojson first for JSON-like content
	if contentType == "application/json" || contentType == "" {
		if err := protojson.Unmarshal(body, target); err != nil {
			// If protojson fails, try binary protobuf
			if protoErr := proto.Unmarshal(body, target); protoErr != nil {
				return nil, err // return the original protojson error
			}
		}
	} else {
		// For protobuf content types, use binary protobuf directly
		if err := proto.Unmarshal(body, target); err != nil {
			return nil, err
		}
	}

	return target, nil
}
