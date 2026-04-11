FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -trimpath -o otel-lgtm-proxy ./cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app

COPY --from=builder /app/otel-lgtm-proxy .

CMD ["./otel-lgtm-proxy"]
