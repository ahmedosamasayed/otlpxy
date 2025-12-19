package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"zep-logger/internal/forwarder"
	"zep-logger/internal/worker"
)

// TestProxyHandler_HandleLogs_InjectsAuthHeader verifies Authorization header injection
// AC1: Proxy handler test verifies Authorization header injection using httptest
func TestProxyHandler_HandleLogs_InjectsAuthHeader(t *testing.T) {
	// Mock OTel collector to verify auth header
	var mu sync.Mutex
	authHeaderReceived := ""
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaderReceived = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create worker pool and handler
	pool := worker.NewPool(2, 10, 5*time.Second)
	pf := forwarder.NewPoolForwarder(pool)
	pf.Start()
	defer pf.Stop()

    handler := NewProxyHandler(mockCollector.URL, "test-api-key", pf, false)

	// Test request
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test-data"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute handler
	err := handler.HandleLogs(c)
	if err != nil {
		t.Fatalf("HandleLogs returned error: %v", err)
	}

	// Give worker time to process
	time.Sleep(100 * time.Millisecond)

	// Verify Authorization header was injected
	mu.Lock()
	received := authHeaderReceived
	mu.Unlock()
	expectedAuth := "test-api-key"
	if received != expectedAuth {
		t.Errorf("expected Authorization header %q, got %q", expectedAuth, received)
	}
}

// TestProxyHandler_HandleLogs_Returns202Accepted verifies 202 Accepted response
// AC2: Proxy handler test verifies 202 Accepted response
func TestProxyHandler_HandleLogs_Returns202Accepted(t *testing.T) {
	// Mock OTel collector
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create worker pool and handler
	pool := worker.NewPool(2, 10, 5*time.Second)
	pf := forwarder.NewPoolForwarder(pool)
	pf.Start()
	defer pf.Stop()

	handler := NewProxyHandler(mockCollector.URL, "test-api-key", pf, false)

	// Test request
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test-data"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute handler
	err := handler.HandleLogs(c)
	if err != nil {
		t.Fatalf("HandleLogs returned error: %v", err)
	}

	// Verify 202 Accepted
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected status 202 Accepted, got %d", rec.Code)
	}

	// Verify empty body
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %d bytes", rec.Body.Len())
	}
}

// TestProxyHandler_HandleTraces_Returns202Accepted verifies traces endpoint returns 202
// AC2: Proxy handler test verifies 202 Accepted response
func TestProxyHandler_HandleTraces_Returns202Accepted(t *testing.T) {
	// Mock OTel collector
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create worker pool and handler
	pool := worker.NewPool(2, 10, 5*time.Second)
	pf := forwarder.NewPoolForwarder(pool)
	pf.Start()
	defer pf.Stop()

    handler := NewProxyHandler(mockCollector.URL, "test-api-key", pf, false)

	// Test request
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("test-trace-data"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Execute handler
	err := handler.HandleTraces(c)
	if err != nil {
		t.Fatalf("HandleTraces returned error: %v", err)
	}

	// Verify 202 Accepted
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected status 202 Accepted, got %d", rec.Code)
	}
}

// TestProxyHandler_RequestBodyBuffering verifies async submission with buffered body
// AC1, AC2: Test request body buffering (async submission)
func TestProxyHandler_RequestBodyBuffering(t *testing.T) {
	var mu sync.Mutex
	receivedBody := ""
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read body sent by worker
		bodyBytes, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(bodyBytes)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	pool := worker.NewPool(2, 10, 5*time.Second)
	pf := forwarder.NewPoolForwarder(pool)
	pf.Start()
	defer pf.Stop()

    handler := NewProxyHandler(mockCollector.URL, "test-api-key", pf, false)

	e := echo.New()
	testData := "buffered-test-data"
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(testData))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLogs(c)
	if err != nil {
		t.Fatalf("HandleLogs returned error: %v", err)
	}

	// Verify 202 response is immediate (before worker processes)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected status 202 Accepted, got %d", rec.Code)
	}

	// Give worker time to forward request
	time.Sleep(100 * time.Millisecond)

	// Verify buffered body was forwarded correctly
	mu.Lock()
	received := receivedBody
	mu.Unlock()
	if received != testData {
		t.Errorf("expected body %q, got %q", testData, received)
	}
}

// TestProxyHandler_NoAuthHeader_WhenAPIKeyEmpty verifies no auth header if API key is empty
func TestProxyHandler_NoAuthHeader_WhenAPIKeyEmpty(t *testing.T) {
	var mu sync.Mutex
	authHeaderReceived := "not-called"
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaderReceived = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

    pool := worker.NewPool(2, 10, 5*time.Second)
    pf := forwarder.NewPoolForwarder(pool)
    pf.Start()
    defer pf.Stop()

    // Create handler with empty API key
    handler := NewProxyHandler(mockCollector.URL, "", pf, false)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test-data"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLogs(c)
	if err != nil {
		t.Fatalf("HandleLogs returned error: %v", err)
	}

	// Give worker time to process
	time.Sleep(100 * time.Millisecond)

	// Verify no Authorization header was sent
	mu.Lock()
	received := authHeaderReceived
	mu.Unlock()
	if received != "" {
		t.Errorf("expected no Authorization header, got %q", received)
	}
}

// TestProxyHandler_QueueFull_Returns503 verifies backpressure handling
func TestProxyHandler_QueueFull_Returns503(t *testing.T) {
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response to fill queue
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create small pool with tiny queue to test backpressure
	pool := worker.NewPool(1, 2, 5*time.Second)
	pf := forwarder.NewPoolForwarder(pool)
	pf.Start()
	defer pf.Stop()

    handler := NewProxyHandler(mockCollector.URL, "test-api-key", pf, false)

	e := echo.New()

	// Fill the queue
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test-data"))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		_ = handler.HandleLogs(c)
	}

	// This request should fail with 503 (queue full)
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test-data"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLogs(c)
	if err != nil {
		t.Fatalf("HandleLogs returned error: %v", err)
	}

	// Verify 503 Service Unavailable
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 Service Unavailable, got %d", rec.Code)
	}
}
