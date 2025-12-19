package health

import (
	"github.com/labstack/echo/v4"
)

// SetupRoutes registers health check routes with the Echo instance
// Follows separated routes pattern - route registration separate from handler logic
func (h *HealthHandler) SetupRoutes(e *echo.Echo) {
	e.GET("/healthz", h.HandleLiveness)
	e.GET("/readyz", h.HandleReadiness)
}
