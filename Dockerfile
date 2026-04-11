ARG GO_IMAGE_VERSION=1.26-alpine
ARG ALPINE_IMAGE_VERSION=latest
ARG APP_NAME=otel-lgtm-proxy

FROM golang:${GO_IMAGE_VERSION} AS builder
ARG APP_NAME

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -trimpath -o ${APP_NAME} ./cmd/main.go

FROM alpine:${ALPINE_IMAGE_VERSION}
ARG APP_NAME

RUN apk --no-cache add ca-certificates && \
    addgroup -S ${APP_NAME} && adduser -S ${APP_NAME} -G ${APP_NAME}

WORKDIR /app

COPY --from=builder \
  --chown=${APP_NAME}:${APP_NAME} \
  --chmod=700 \
  /app/${APP_NAME} .

USER ${APP_NAME}
CMD ["./otel-lgtm-proxy"]
