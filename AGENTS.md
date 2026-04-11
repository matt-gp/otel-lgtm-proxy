# AGENTS.md

## Project Overview

OpenTelemetry LGTM (Logs, Metrics, Traces) proxy that receives OTLP signals and forwards them to separate backends (Loki, Mimir, Tempo) with tenant ID injection.

- **Language**: Go 1.26.0
- **Module**: `github.com/matt-gp/otel-lgtm-proxy`

## Setup Commands

- Install deps: `go mod download`
- Generate mocks: `go generate ./...`
- Build binary: `go build -o otel-lgtm-proxy ./cmd/main.go`
- Run locally: `./otel-lgtm-proxy`
- Build Docker: `docker build -t otel-lgtm-proxy .`
- Run full stack: `docker compose up`

## Testing Instructions

- Run all tests: `go test -v ./...`
- Run with coverage: `go test -v -race -coverprofile=coverage.out -covermode=atomic ./...`
- View coverage: `go tool cover -html=coverage.out`
- Run linter: `golangci-lint run`
- Must have >90% test coverage before merging

## Code Style

- Use `golangci-lint` with project config (max complexity: 35)
- Always check and handle errors explicitly
- Use dependency injection via constructor functions named `New()`
- Config via environment variables using `github.com/caarlos0/env/v6`
- Package-level comments required: `// Package <name> <description>.`
- Use table-driven tests for multiple test cases
- Generate mocks with `//go:generate mockgen` directives

## Docker Build

- Multi-stage builds with Alpine base
- Build flags: `-ldflags="-s -w" -trimpath`
- Run as non-root user
- `CGO_ENABLED=0` for static binaries

## Project-Specific Patterns

- Use `processor.Processor[T]` generic type for signal processing
- Extract tenant ID from resource attributes using configured label(s)
- Always propagate context for tracing and cancellation
- Use protobuf getter methods (`Get*()`) instead of direct field access

## CI/CD

- Tests and lint must pass before merge
- Docker build test validates container startup and health endpoint
- Coverage reports uploaded to Codecov
