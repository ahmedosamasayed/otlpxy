package proxy

import (
	"github.com/labstack/echo/v4"
)

// SetupRoutes registers OTLP proxy routes with the Echo instance
// Follows separated routes pattern - route registration separate from handler logic
func (h *ProxyHandler) SetupRoutes(e *echo.Echo) {
	e.POST("/v1/logs", h.HandleLogs)
	e.POST("/v1/traces", h.HandleTraces)
}
