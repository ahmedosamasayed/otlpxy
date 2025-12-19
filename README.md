## otlpxy

A lightweight OTLP proxy service with async forwarding, health/readiness probes, Prometheus metrics, CORS, and request size limits. Designed to accept OTLP logs/traces from browsers or services, inject an API key, and securely forward to your OpenTelemetry Collector.

### Features
- Async forwarding via bounded worker pool (backpressure when queue is full)
- Health and readiness probes: `/healthz`, `/readyz`
- Prometheus metrics: `/metrics` (includes HTTP metrics and queue depth gauge)
- CORS first, request body size limits, panic recovery, request logging
- Graceful shutdown with readiness drain and timeout

### Requirements
- Go 1.23+
- Docker (optional)

### Configuration
The service reads `config.toml` from the current working directory (or `./config`).

Example `config.toml`:
```toml
# OpenTelemetry Collector Target URL (required)
otel_collector_target_url = "https://otel.example.com"

# Optional API key (added to Authorization: <key>)
otel_collector_api_key = "<your-api-key>"

# Graceful shutdown
shutdown_drain_seconds = 2
shutdown_timeout_seconds = 10

# Server
server_port = 8080

# CORS and request size limits
allowed_origins = "https://app.example.com,https://admin.example.com"
max_request_size_mb = 1

# Worker pool
worker_pool_size = 0      # 0 uses default 2×NumCPU
job_queue_size = 10000
```

Notes:
- The application uses the config file (no env parsing is configured). For Docker, mount the file into the container as shown below.

### Build and Run (local)
Using Makefile:
```bash
make build
make run
```

Directly with Go:
```bash
go run ./cmd/server
```

### Build and Run (Docker)
Build a local image for your host architecture and load it into Docker:
```bash
make docker-build
```

Run the container, mounting your `config.toml` into `/app` (the service reads from its working directory):
```bash
docker run -d \
  --name zep-logger \
  -p 8080:8080 \
  -v $(pwd)/config.toml:/app/config.toml:ro \
  zep-logger:latest
```

Stop and remove the container:
```bash
docker rm -f zep-logger
```

### Make targets
```bash
make build         # Build binary for current platform
make run           # Build and run the application
make test          # Run all tests
make docker-build  # Build Docker image (host arch) and load locally
make docker-run    # Run Docker container locally (expects config mounted if not in image)
make clean         # Remove build artifacts
make help          # Show available commands
```

### HTTP Endpoints
- Health:
  - `GET /healthz` → 200 when alive
  - `GET /readyz` → 200 when ready, 503 during drain/shutdown
- Metrics:
  - `GET /metrics` Prometheus exposition (includes `zep_logger_worker_pool_queue_depth`)
- OTLP proxy:
  - `POST /v1/logs` → 202 Accepted (forwarded asynchronously)
  - `POST /v1/traces` → 202 Accepted (forwarded asynchronously)

Example (logs):
```bash
curl -sS -X POST http://localhost:8080/v1/logs \
  -H 'Content-Type: application/x-protobuf' \
  --data-binary @payload-logs.pb
```

### Observability
- Logging: structured to stdout/stderr
- Metrics:
  - HTTP metrics via `echoprometheus`
  - Queue depth gauge: `zep_logger_worker_pool_queue_depth`

### Graceful Shutdown
On SIGINT/SIGTERM, readiness flips to false, a drain window allows load balancers to stop routing, the worker pool finishes in-flight jobs (up to timeout), then the HTTP server shuts down.

### Development
- Go version: 1.23+
- Lint/format are not wired to Makefile by default; use your preferred tools
- Tests: `make test`

### Security
- Ensure `config.toml` is not committed with real secrets
- Use least-privilege API keys; rotate regularly
- Consider adding rate-limits and auth at the ingress layer if exposing publicly

### License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

### Contributing
Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

### Security
If you discover a security vulnerability, please see our [Security Policy](SECURITY.md).

