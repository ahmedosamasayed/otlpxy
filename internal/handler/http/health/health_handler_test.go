package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/atomic"
)

// TestHealthHandler_Liveness_AlwaysReturns200 verifies liveness endpoint
// AC4: Health handler test verifies liveness always returns 200 OK
func TestHealthHandler_Liveness_AlwaysReturns200(t *testing.T) {
	readiness := atomic.NewBool(false)
	handler := NewHealthHandler(readiness)

	e := echo.New()

	// Test with readiness=false
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleLiveness(c)
	if err != nil {
		t.Fatalf("HandleLiveness returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK when readiness=false, got %d", rec.Code)
	}

	// Test with readiness=true
	readiness.Store(true)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = handler.HandleLiveness(c)
	if err != nil {
		t.Fatalf("HandleLiveness returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK when readiness=true, got %d", rec.Code)
	}
}

// TestHealthHandler_Readiness_WhenTrue_Returns200 verifies readiness endpoint when ready
// AC4: Health handler test verifies readiness flag behavior (true/false)
func TestHealthHandler_Readiness_WhenTrue_Returns200(t *testing.T) {
	readiness := atomic.NewBool(true)
	handler := NewHealthHandler(readiness)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleReadiness(c)
	if err != nil {
		t.Fatalf("HandleReadiness returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK when readiness=true, got %d", rec.Code)
	}

	// Verify empty body
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %d bytes", rec.Body.Len())
	}
}

// TestHealthHandler_Readiness_WhenFalse_Returns503 verifies readiness endpoint when not ready
// AC4: Health handler test verifies readiness flag behavior (true/false)
func TestHealthHandler_Readiness_WhenFalse_Returns503(t *testing.T) {
	readiness := atomic.NewBool(false)
	handler := NewHealthHandler(readiness)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleReadiness(c)
	if err != nil {
		t.Fatalf("HandleReadiness returned error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 Service Unavailable when readiness=false, got %d", rec.Code)
	}
}

// TestHealthHandler_Readiness_ToggleBehavior verifies readiness flag toggle
// AC4: Health handler test verifies readiness flag toggle behavior
func TestHealthHandler_Readiness_ToggleBehavior(t *testing.T) {
	readiness := atomic.NewBool(false)
	handler := NewHealthHandler(readiness)

	e := echo.New()

	// Test 1: readiness=false → 503
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleReadiness(c)
	if err != nil {
		t.Fatalf("HandleReadiness returned error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when readiness=false, got %d", rec.Code)
	}

	// Toggle readiness to true
	readiness.Store(true)

	// Test 2: readiness=true → 200
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = handler.HandleReadiness(c)
	if err != nil {
		t.Fatalf("HandleReadiness returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 when readiness=true, got %d", rec.Code)
	}

	// Toggle readiness back to false
	readiness.Store(false)

	// Test 3: readiness=false again → 503
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = handler.HandleReadiness(c)
	if err != nil {
		t.Fatalf("HandleReadiness returned error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when readiness toggled back to false, got %d", rec.Code)
	}
}

// TestHealthHandler_ConcurrentReadinessChecks verifies thread safety
func TestHealthHandler_ConcurrentReadinessChecks(t *testing.T) {
	readiness := atomic.NewBool(true)
	handler := NewHealthHandler(readiness)

	e := echo.New()

	// Run concurrent readiness checks
	const numRequests = 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.HandleReadiness(c)
			if err != nil {
				t.Errorf("HandleReadiness returned error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}

			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}

// TestHealthHandler_SetupRoutes verifies route registration
func TestHealthHandler_SetupRoutes(t *testing.T) {
	readiness := atomic.NewBool(true)
	handler := NewHealthHandler(readiness)

	e := echo.New()
	handler.SetupRoutes(e)

	// Test /healthz route
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected /healthz to return 200, got %d", rec.Code)
	}

	// Test /readyz route
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected /readyz to return 200, got %d", rec.Code)
	}
}
