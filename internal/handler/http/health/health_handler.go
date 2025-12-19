package health

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/atomic"
)

// HealthHandler handles health check endpoints for Kubernetes probes
// Follows constructor injection pattern - no global state
type HealthHandler struct {
	readiness *atomic.Bool
}

// NewHealthHandler creates a new HealthHandler with dependency injection
// readiness: Thread-safe boolean flag indicating if service is ready to handle traffic
func NewHealthHandler(readiness *atomic.Bool) *HealthHandler {
	return &HealthHandler{
		readiness: readiness,
	}
}

// HandleLiveness handles GET /healthz - liveness probe
// Always returns 200 OK to indicate the container is alive
// Used by Kubernetes to detect if the container needs to be restarted
func (h *HealthHandler) HandleLiveness(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

// HandleReadiness handles GET /readyz - readiness probe
// Returns 200 OK when ready to accept traffic, 503 when not ready
// Used by Kubernetes to manage traffic routing during deployments
func (h *HealthHandler) HandleReadiness(c echo.Context) error {
	if h.readiness.Load() {
		return c.NoContent(http.StatusOK)
	}
	return c.NoContent(http.StatusServiceUnavailable)
}
