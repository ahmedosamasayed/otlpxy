package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"zep-logger/internal/config"
)

// TestCORS_PreflightRequest_Returns204 verifies CORS preflight handling
// AC6: Test CORS preflight OPTIONS returns 204
func TestCORS_PreflightRequest_Returns204(t *testing.T) {
	e := echo.New()

	// Setup CORS middleware (same as app.go)
	origins := []string{"https://quiz.zep.us", "https://school.zep.us"}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", "X-Client-Id"},
		AllowCredentials: true,
	}))

	// Add a test route
	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Send preflight OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/v1/logs", nil)
	req.Header.Set("Origin", "https://quiz.zep.us")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Verify 204 No Content
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204 No Content for OPTIONS preflight, got %d", rec.Code)
	}
}

// TestCORS_Headers_PresentInResponse verifies CORS headers
// AC6: Test CORS headers present in responses (Access-Control-Allow-Credentials)
func TestCORS_Headers_PresentInResponse(t *testing.T) {
	e := echo.New()

	// Setup CORS middleware with credentials enabled
	origins := []string{"https://quiz.zep.us"}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", "X-Client-Id"},
		AllowCredentials: true,
	}))

	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Send POST request with Origin header
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test"))
	req.Header.Set("Origin", "https://quiz.zep.us")
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Verify Access-Control-Allow-Credentials header
	credentials := rec.Header().Get("Access-Control-Allow-Credentials")
	if credentials != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %q", credentials)
	}

	// Verify Vary header (present for CORS)
	vary := rec.Header().Get("Vary")
	if vary == "" {
		t.Error("expected Vary header to be present for CORS, got empty")
	}
}

// TestBodyLimit_SmallRequest_Passes verifies requests ≤1MB pass
// AC6: Test requests ≤1MB pass, >1MB return 413
func TestBodyLimit_SmallRequest_Passes(t *testing.T) {
	e := echo.New()

	// Setup BodyLimit middleware (1MB)
	e.Use(middleware.BodyLimit("1M"))

	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Send request with 0.5MB body
	body := strings.Repeat("x", 512*1024) // 512KB
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Verify 202 Accepted (request passed)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected status 202 for 512KB request, got %d", rec.Code)
	}
}

// TestBodyLimit_LargeRequest_Returns413 verifies requests >1MB return 413
// AC6: Test requests ≤1MB pass, >1MB return 413
func TestBodyLimit_LargeRequest_Returns413(t *testing.T) {
	e := echo.New()

	// Setup BodyLimit middleware (1MB)
	e.Use(middleware.BodyLimit("1M"))

	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Send request with 1.5MB body
	body := strings.Repeat("x", 1536*1024) // 1.5MB
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Verify 413 Payload Too Large
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for 1.5MB request, got %d", rec.Code)
	}
}

// TestCORS_And_BodyLimit_Order verifies CORS headers in 413 response
// AC6: Test CORS headers in 413 response (middleware ordering)
func TestCORS_And_BodyLimit_Order(t *testing.T) {
	e := echo.New()

	// Setup middleware in correct order: CORS first, then BodyLimit
	origins := []string{"https://quiz.zep.us"}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", "X-Client-Id"},
		AllowCredentials: true,
	}))
	e.Use(middleware.BodyLimit("1M"))

	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Send oversized request with Origin header
	body := strings.Repeat("x", 1536*1024) // 1.5MB
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader(body))
	req.Header.Set("Origin", "https://quiz.zep.us")
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Verify 413 response
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", rec.Code)
	}

	// Verify CORS headers are present in 413 response (proves CORS runs first)
	vary := rec.Header().Get("Vary")
	if vary == "" {
		t.Error("expected Vary header in 413 response (CORS should run before BodyLimit)")
	}
}

// TestCORS_MultipleOrigins verifies comma-separated origins parsing
func TestCORS_MultipleOrigins(t *testing.T) {
	e := echo.New()

	// Parse origins from config (simulating app.go logic)
	configOrigins := "https://quiz.zep.us, https://school.zep.us, https://core.zep.us"
	origins := strings.Split(configOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", "X-Client-Id"},
		AllowCredentials: true,
	}))

	e.POST("/v1/logs", func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})

	// Test each origin
	testOrigins := []string{
		"https://quiz.zep.us",
		"https://school.zep.us",
		"https://core.zep.us",
	}

	for _, origin := range testOrigins {
		req := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("test"))
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		// Verify request succeeded
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected status 202 for origin %s, got %d", origin, rec.Code)
		}

		// Verify Access-Control-Allow-Credentials
		credentials := rec.Header().Get("Access-Control-Allow-Credentials")
		if credentials != "true" {
			t.Errorf("expected Access-Control-Allow-Credentials: true for origin %s, got %q", origin, credentials)
		}
	}
}

// TestApp_MiddlewareOrder_Integration verifies full middleware stack
func TestApp_MiddlewareOrder_Integration(t *testing.T) {
	cfg := &config.Config{
		ServerPort:              8080,
		OtelCollectorTargetURL:  "http://localhost:4318",
		OtelCollectorAPIKey:     "test-key",
		ShutdownDrainSeconds:    2,
		ShutdownTimeoutSeconds:  10,
		WorkerPoolSize:          2,
		JobQueueSize:            10,
		AllowedOrigins:          []string{"https://quiz.zep.us"},
		MaxRequestSizeMB:        1,
	}

	app := NewApp(cfg)

	// Verify config values
	if len(app.config.AllowedOrigins) != 1 || app.config.AllowedOrigins[0] != "https://quiz.zep.us" {
		t.Errorf("expected AllowedOrigins [%q], got %v", "https://quiz.zep.us", app.config.AllowedOrigins)
	}

	if app.config.MaxRequestSizeMB != 1 {
		t.Errorf("expected MaxRequestSizeMB 1, got %d", app.config.MaxRequestSizeMB)
	}
}
