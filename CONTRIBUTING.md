# Contributing to otel-lgtm-proxy

Thank you for your interest in contributing to the otel-lgtm-proxy! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Testing](#testing)
- [Code Style](#code-style)
- [Submitting Changes](#submitting-changes)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Getting Started

### Prerequisites

- **Go 1.24+**: This project uses Go 1.24 features
- **Git**: For version control
- **Docker**: For running the development stack (optional)
- **golangci-lint**: For code linting
- **mockgen**: For generating test mocks

### Development Setup

1. **Fork and Clone**
   ```bash
   git clone https://github.com/YOUR_USERNAME/otel-lgtm-proxy.git
   cd otel-lgtm-proxy
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Install Development Tools**
   ```bash
   # Install golangci-lint
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   
   # Install mockgen
   go install go.uber.org/mock/mockgen@latest
   ```

4. **Verify Setup**
   ```bash
   go test ./...
   go build ./cmd
   ```

## Project Structure

```
├── cmd/                    # Application entry points
│   └── main.go            # Main application
├── internal/              # Private application code
│   ├── config/           # Configuration management
│   │   └── config.go
│   ├── certutil/         # TLS certificate utilities  
│   │   ├── cert_helpers.go
│   │   └── cert_helpers_test.go
│   ├── logger/           # Logging utilities
│   │   ├── logger.go
│   │   └── logger_test.go
│   ├── otel/             # OpenTelemetry setup
│   │   ├── otel.go
│   │   └── otel_test.go
│   ├── logs/             # Log telemetry processing
│   │   ├── logs.go
│   │   ├── logs_test.go
│   │   └── logs_mock.go
│   ├── metrics/          # Metric telemetry processing
│   │   ├── metrics.go
│   │   ├── metrics_test.go
│   │   └── metrics_mock.go
│   └── traces/           # Trace telemetry processing
│       ├── traces.go
│       ├── traces_test.go
│       └── traces_mock.go
├── test/                 # Testing tools and configurations
│   ├── docker-compose.yml # LGTM stack for development
│   ├── *.yaml            # Service configurations
│   └── send-*.sh         # Testing scripts
├── docker-compose.yml    # LGTM development stack
├── Dockerfile            # Container build
├── go.mod               # Go module definition
├── go.sum               # Go module checksums
└── README.md            # Project documentation
```

### Package Organization

- **`cmd/`**: Contains application entry points. Keep these minimal.
- **`internal/config/`**: Configuration parsing and validation.
- **`internal/certutil/`**: TLS configuration and certificate management.
- **`internal/logger/`**: OpenTelemetry logging wrapper with severity filtering.
- **`internal/otel/`**: OpenTelemetry provider initialization and configuration.
- **`internal/logs/`**: Log telemetry processing with tenant partitioning and forwarding.
- **`internal/metrics/`**: Metric telemetry processing with temporality handling.
- **`internal/traces/`**: Trace telemetry processing with correlation support.
- **`test/`**: Testing scripts and development environment configurations.

## Development Workflow

### Branch Strategy

- **`main`**: Production-ready code
- **`feature/description`**: New features
- **`bugfix/description`**: Bug fixes
- **`docs/description`**: Documentation updates

### Workflow

1. **Create Feature Branch**
   ```bash
   git checkout -b feature/add-grpc-support
   ```

2. **Make Changes**
   - Write code following project conventions
   - Add tests for new functionality
   - Update documentation as needed

3. **Test Changes**
   ```bash
   go test ./...
   go test -race ./...
   go test -cover ./...
   ```

4. **Lint Code**
   ```bash
   golangci-lint run
   ```

5. **Commit Changes**
   ```bash
   git add .
   git commit -m "feat: add gRPC endpoint support"
   ```

6. **Push and Create PR**
   ```bash
   git push origin feature/add-grpc-support
   ```

### Commit Message Convention

Use conventional commits:

- `feat:` New features
- `fix:` Bug fixes
- `docs:` Documentation changes
- `test:` Test changes
- `refactor:` Code refactoring
- `perf:` Performance improvements
- `chore:` Maintenance tasks

Examples:
```
feat: add support for gRPC endpoints
fix: correct tenant header forwarding logic
docs: update configuration documentation
test: add integration tests for service layer
```

## Testing

### Test Structure

- **Unit Tests**: Test individual functions and methods
- **Integration Tests**: Test component interactions
- **Mock Usage**: Use mocks for external dependencies

### Writing Tests

1. **Test File Naming**: `*_test.go` in the same package
2. **Test Function Naming**: `TestFunctionName` or `TestType_Method`
3. **Mock File Naming**: `*_mock.go` generated by mockgen

### Test Guidelines

```go
func TestConfig_Parse(t *testing.T) {
    tests := []struct {
        name    string
        env     map[string]string
        want    *Config
        wantErr bool
    }{
        {
            name: "valid configuration",
            env: map[string]string{
                "OTEL_SERVICE_NAME": "test-service",
            },
            want: &Config{
                OTEL: OTELConfig{
                    ServiceName: "test-service",
                },
            },
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detection
go test -race ./...

# Run specific package tests
go test ./internal/config

# Run specific test
go test -run TestConfig_Parse ./internal/config
```

### Generating Mocks

When adding new interfaces, generate mocks:

```bash
# Generate mocks for logs interfaces
mockgen -source=internal/logs/logs.go -destination=internal/logs/logs_mock.go -package=logs

# Generate mocks for metrics interfaces  
mockgen -source=internal/metrics/metrics.go -destination=internal/metrics/metrics_mock.go -package=metrics

# Generate mocks for traces interfaces
mockgen -source=internal/traces/traces.go -destination=internal/traces/traces_mock.go -package=traces
```

### Test Coverage

Maintain test coverage above 80%:

```bash
go test -cover ./...
```

## Code Style

### Go Guidelines

- Follow standard Go conventions
- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful variable and function names
- Write documentation for exported functions

### Linting

Use golangci-lint with the project configuration:

```bash
golangci-lint run
```

### Error Handling

```go
// Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to parse config: %w", err)
}

// Good: Handle errors at appropriate level
data, err := repository.GetData(ctx, id)
if err != nil {
    logger.Error("Failed to get data", "id", id, "error", err)
    return nil, err
}
```

### Logging

Use structured logging:

```go
// Good: Structured logging
logger.Info("Processing request", 
    "tenant", tenantID,
    "signal_type", "traces",
    "payload_size", len(data))

// Avoid: Formatted strings
logger.Info(fmt.Sprintf("Processing %s for tenant %s", signalType, tenantID))
```

## Submitting Changes

### Pull Request Process

1. **Fork the Repository**
2. **Create Feature Branch**
3. **Make Changes** following guidelines
4. **Write/Update Tests**
5. **Update Documentation**
6. **Submit Pull Request**

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Refactoring

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added/updated
```

### Review Process

- All PRs require review from maintainers
- Address feedback promptly
- Keep PRs focused and atomic
- Rebase before merging

## Protocol Requirements

### HTTP Protobuf Only

This project **ONLY** supports HTTP protobuf payloads:

- ✅ OTLP/HTTP with protobuf encoding
- ❌ OTLP/gRPC 
- ❌ JSON encoding
- ❌ Other serialization formats

### Adding New Signal Types

When adding support for new OpenTelemetry signals:

1. Create a new package in `internal/` (e.g., `internal/newsignal/`)
2. Implement partitioning logic similar to existing signal packages
3. Add HTTP handlers following the same pattern
4. Update configuration in `internal/config/` if needed
5. Add comprehensive tests with generated mocks
6. Update documentation

## Performance Considerations

- Minimize memory allocations in hot paths
- Use context for cancellation and timeouts
- Profile before optimizing
- Benchmark critical paths

## Security Guidelines

- Validate all inputs
- Use TLS for production deployments
- Follow secure coding practices
- Report security issues privately

## Documentation

- Update README.md for user-facing changes
- Add godoc comments for exported functions
- Update configuration documentation
- Include examples for new features

## Getting Help

- Check existing issues and discussions
- Create detailed issue reports
- Join community discussions
- Contact maintainers for sensitive issues

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (MIT License).
