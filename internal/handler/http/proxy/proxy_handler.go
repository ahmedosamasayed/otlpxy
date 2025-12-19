package proxy

import (
    "bytes"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/labstack/echo/v4"

    "zep-logger/internal/forwarder"
    "zep-logger/pkg/logger"
)

// ProxyHandler handles OTLP proxy endpoints
// Forwards requests asynchronously to internal OTel collector with API key injection
type ProxyHandler struct {
	targetURL  string
	apiKey     string
    forwarder  forwarder.Forwarder
    httpClient *http.Client
    syncLogs   bool
}

// NewProxyHandler creates a new ProxyHandler with the given target URL, API key, and worker pool
// targetURL: Internal OTel collector base URL (e.g., "http://localhost:4318")
// apiKey: Secret API key for Authorization header (can be empty)
// workerPool: Worker pool for async forwarding
func NewProxyHandler(targetURL string, apiKey string, forwarder forwarder.Forwarder, syncLogs bool) *ProxyHandler {
    transport := &http.Transport{
        Proxy:                 http.ProxyFromEnvironment,
        ForceAttemptHTTP2:     true,
        MaxIdleConns:          2000,
        MaxIdleConnsPerHost:   1000,
        MaxConnsPerHost:       1500,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
    }
	return &ProxyHandler{
		targetURL:  targetURL,
		apiKey:     apiKey,
        forwarder:  forwarder,
        httpClient: &http.Client{Transport: transport, Timeout: 10 * time.Second},
        syncLogs:   syncLogs,
	}
}

// HandleLogs handles POST /v1/logs requests
// Uses synchronous forwarding when syncLogs=true (REQUIRED for session replay)
// Falls back to async when syncLogs=false (breaks session replay but handles high load)
func (h *ProxyHandler) HandleLogs(c echo.Context) error {
    if !h.syncLogs {
        return h.handleAsync(c, "/v1/logs")
    }

    // Synchronous forwarding (REQUIRED for session replay to work)
    body, err := io.ReadAll(c.Request().Body)
    if err != nil {
        logger.Error("Failed to read request body: %v", err)
        return c.NoContent(http.StatusBadRequest)
    }

    contentType := c.Request().Header.Get("Content-Type")
    if contentType == "" {
        contentType = "application/x-protobuf"
    }

    headers := make(http.Header, len(c.Request().Header)+2)
    isHopByHop := func(name string) bool {
        switch strings.ToLower(name) {
        case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "proxy-connection":
            return true
        default:
            return false
        }
    }
    for k, vals := range c.Request().Header {
        if len(vals) == 0 {
            continue
        }
        if strings.EqualFold(k, "Host") || isHopByHop(k) {
            continue
        }
        for _, v := range vals {
            headers.Add(k, v)
        }
    }
    if headers.Get("Content-Type") == "" {
        headers.Set("Content-Type", contentType)
    }
    if h.apiKey != "" {
        headers.Set("Authorization", h.apiKey)
    }

    req, err := http.NewRequest(http.MethodPost, h.targetURL+"/v1/logs", bytes.NewReader(body))
    if err != nil {
        logger.Error("Failed to build upstream request: %v", err)
        return c.NoContent(http.StatusBadRequest)
    }
    req.Header = headers

    resp, err := h.httpClient.Do(req)
    if err != nil {
        logger.Error("Upstream error (sync logs): %v", err)
        return c.NoContent(http.StatusBadGateway)
    }
    defer resp.Body.Close()

    // Copy response headers from upstream, but skip problematic headers
    for k, values := range resp.Header {
        lowerKey := strings.ToLower(k)
        // Skip headers that should not be forwarded or might cause conflicts
        switch {
        case strings.HasPrefix(lowerKey, "access-control-"): // CORS headers (Echo handles these)
            continue
        case lowerKey == "vary": // Can conflict with CORS Vary header
            continue
        case lowerKey == "content-length": // Let Echo calculate this
            continue
        case lowerKey == "transfer-encoding": // Hop-by-hop header
            continue
        case lowerKey == "connection": // Hop-by-hop header
            continue
        }
        for _, v := range values {
            c.Response().Header().Add(k, v)
        }
    }
    c.Response().WriteHeader(resp.StatusCode)
    _, _ = io.Copy(c.Response(), resp.Body)
    return nil
}

// HandleTraces handles POST /v1/traces requests
// ALWAYS uses async forwarding for better performance (traces are fire-and-forget)
// Buffers request body, submits async job to worker pool, returns 202 Accepted immediately
func (h *ProxyHandler) HandleTraces(c echo.Context) error {
	// Traces don't need responses, always use async for optimal performance
	return h.handleAsync(c, "/v1/traces")
}

// handleAsync implements the async forwarding pattern for all OTLP endpoints
func (h *ProxyHandler) handleAsync(c echo.Context, path string) error {
	// Buffer request body before async submission (prevents race conditions)
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		logger.Error("Failed to read request body: %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	// Get Content-Type header from original request
	contentType := c.Request().Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/x-protobuf" // Default for OTLP
	}

    // Build headers for forwarding by copying incoming request headers (multi-valued)
    headers := make(http.Header, len(c.Request().Header)+2)

    // Helper to detect hop-by-hop headers that must not be forwarded per RFC 7230
    isHopByHop := func(name string) bool {
        switch strings.ToLower(name) {
        case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "proxy-connection":
            return true
        default:
            return false
        }
    }

    for k, vals := range c.Request().Header {
        if len(vals) == 0 {
            continue
        }
        if strings.EqualFold(k, "Host") || isHopByHop(k) {
            continue
        }
        // Preserve Cookie safely and all other header values
        for _, v := range vals {
            headers.Add(k, v)
        }
    }

    // Ensure Content-Type is set
    if headers.Get("Content-Type") == "" {
        headers.Set("Content-Type", contentType)
    }

    // Inject/override Authorization with the configured API key
    if h.apiKey != "" {
        headers.Set("Authorization", h.apiKey)
    }

    // Submit to forwarder (pool or semaphore)
    err = h.forwarder.Submit(body, h.targetURL+path, headers)
	if err != nil {
		// Queue is full - backpressure scenario
		logger.Warn("Worker pool queue full: rejecting request to %s", path)
		return c.NoContent(http.StatusServiceUnavailable)
	}

	// Return 202 Accepted immediately (client doesn't wait for collector)
	return c.NoContent(http.StatusAccepted)
}
