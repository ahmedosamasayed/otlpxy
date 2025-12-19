package httpiface

import "github.com/labstack/echo/v4"

// HttpRouter defines the interface for HTTP route registration
// All HTTP handlers must implement this interface to register their routes with Echo
type HttpRouter interface {
	// SetupRoutes registers the handler's HTTP routes with the Echo instance
	SetupRoutes(e *echo.Echo)
}
