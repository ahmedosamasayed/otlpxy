# Multi-stage Dockerfile for Zep Logger
# Supports: linux/amd64, linux/arm64

# Stage 1: Builder
# Use --platform=$BUILDPLATFORM for fast native builds
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first (better caching)
COPY go.mod go.sum ./

# Download dependencies (cached layer)
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Run tests before building (fail fast)
RUN go test -v ./...

# Build static binary for target platform
# CGO_ENABLED=0 creates fully static binary (no libc dependencies)
# -ldflags="-w -s" strips debug symbols (smaller binary)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o /app/zep-logger ./cmd/server

# Verify binary was created
RUN test -f /app/zep-logger

# Stage 2: Runtime
FROM alpine:3.19

# Install ca-certificates for HTTPS requests to OTel collector
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 10001 -g zep zep

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/zep-logger /app/zep-logger
COPY --from=builder /build/config.toml /app/config.toml

# Change ownership to non-root user
RUN chown -R zep:zep /app

# Switch to non-root user
USER zep

# Expose service port
EXPOSE 8080

# Add health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run the binary
ENTRYPOINT ["/app/zep-logger"]
