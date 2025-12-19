package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/atomic"

	"zep-logger/internal/config"
	"zep-logger/internal/handler/http/health"
	httpiface "zep-logger/internal/handler/http/interface"
	"zep-logger/internal/handler/http/proxy"
	"zep-logger/internal/forwarder"
	"zep-logger/internal/metrics"
	"zep-logger/internal/worker"
	"zep-logger/pkg/logger"
)

// App represents the application with its lifecycle management
type App struct {
	config       *config.Config
	echo         *echo.Echo
	readiness    *atomic.Bool
	httpHandlers []httpiface.HttpRouter
    workerPool   *worker.Pool
    forwarder    forwarder.Forwarder
	cancel       context.CancelFunc
}

// NewApp creates a new App instance with the given configuration
// Follows constructor injection pattern - all dependencies passed via parameters
func NewApp(cfg *config.Config) *App {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	app := &App{
		config:    cfg,
		echo:      e,
		readiness: atomic.NewBool(false),
	}

	return app
}

// injectDependency initializes all HTTP handlers and worker pool
// This centralizes handler initialization and makes it easy to add new handlers
func (a *App) injectDependency() {
	// Initialize worker pool for async request forwarding
	shutdownTimeout := time.Duration(a.config.ShutdownTimeoutSeconds) * time.Second
    // Choose forwarder based on config
    switch a.config.ForwardingMode {
    case "semaphore":
        a.workerPool = nil
        a.forwarder = forwarder.NewSemaphoreForwarder(a.config.SemaphoreMaxConcurrent, shutdownTimeout)
        logger.Info("Using semaphore-based forwarder (maxConcurrent=%d)", a.config.SemaphoreMaxConcurrent)
    case "hybrid":
        a.workerPool = nil
        a.forwarder = forwarder.NewHybridForwarder(a.config.WorkerPoolSize, a.config.JobQueueSize, a.config.SemaphoreMaxConcurrent, shutdownTimeout)
        logger.Info("Using hybrid forwarder (workers=%d, queueSize=%d, maxConcurrent=%d)", a.config.WorkerPoolSize, a.config.JobQueueSize, a.config.SemaphoreMaxConcurrent)
    default:
        a.workerPool = worker.NewPool(a.config.WorkerPoolSize, a.config.JobQueueSize, shutdownTimeout)
        a.forwarder = forwarder.NewPoolForwarder(a.workerPool)
        logger.Info("Using pool-based forwarder (workers=%d, queueSize=%d)", a.config.WorkerPoolSize, a.config.JobQueueSize)
    }

    a.httpHandlers = []httpiface.HttpRouter{
        health.NewHealthHandler(a.readiness),
        proxy.NewProxyHandler(a.config.OtelCollectorTargetURL, a.config.OtelCollectorAPIKey, a.forwarder, a.config.SyncLogsDebug),
        // Future handlers will be added here:
        // admin.NewAdminHandler(...) in Story 1.7
    }
}

// preProcess is called before server starts
// Use this hook for initialization tasks that need to happen before accepting traffic
func (a *App) preProcess() {
	logger.Info("Preparing to start server...")

	// Start worker pool before accepting HTTP traffic
    if a.forwarder != nil {
        a.forwarder.Start()
    }
}

// postProcess is called after shutdown signal is received
// Use this hook for cleanup tasks before graceful shutdown begins
func (a *App) postProcess() {
	logger.Info("Shutting down gracefully...")
}

// Run starts the Echo server and handles graceful shutdown
// This implements the full lifecycle: startup -> run -> graceful shutdown
func (a *App) Run() error {
	// Create context for application lifecycle management
	_, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	// Initialize all dependencies
	a.injectDependency()
	a.preProcess()

	// Start Echo server in goroutine
	go func() {
		e := a.echo
		addr := fmt.Sprintf(":%d", a.config.ServerPort)

		// Add middleware in correct order (Story 1.9 - CORS must be FIRST)
		// Per ADR-005: CORS first to handle preflight before auth/validation

		// 1. CORS middleware (Story 1.9)
        e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
            AllowOrigins:     a.config.AllowedOrigins,
            AllowMethods:     []string{http.MethodPost, http.MethodOptions},
            AllowHeaders:     []string{"Content-Type", "Content-Encoding", "X-Client-Id", "Authorization", "Accept", "Origin", "User-Agent", "Traceparent", "Baggage", "X-Requested-With"},
            AllowCredentials: true, // Enable cookies/credentials for browser RUM
        }))

		// 2. Body size limit middleware (Story 1.9)
		// Protects against memory exhaustion from large payloads
		limit := fmt.Sprintf("%dM", a.config.MaxRequestSizeMB)
		e.Use(middleware.BodyLimit(limit))

		// 3. Logging
		e.Use(middleware.Logger())

		// 4. Panic recovery
		e.Use(middleware.Recover())

		// 5. Readiness check middleware (Story 1.6 - Graceful Shutdown)
		// This middleware rejects requests when readiness=false, except for health endpoints
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				if !a.readiness.Load() {
					p := c.Request().URL.Path
					// Allow health check endpoints and metrics even during shutdown
					if p != "/healthz" && p != "/readyz" && p != "/metrics" {
						logger.Info("readiness=false: reject new request path=%s", p)
						return c.NoContent(http.StatusServiceUnavailable)
					}
				}
				return next(c)
			}
		})

		// 6. Prometheus metrics middleware (Story 1.8)
		// This automatically tracks HTTP requests and exposes /metrics endpoint
		e.Use(echoprometheus.NewMiddleware("zep_logger"))
		e.GET("/metrics", echoprometheus.NewHandler())

		// 7. Update queue depth metric on each request
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
                if a.forwarder != nil {
                    metrics.QueueDepthGauge.Set(float64(a.forwarder.GetQueueDepth()))
				}
				return next(c)
			}
		})

		// 8. Setup all handler routes
		for _, handler := range a.httpHandlers {
			handler.SetupRoutes(e)
		}

		logger.Info("Starting Zep Logger server on %s", addr)

		// Mark readiness true just before starting to accept connections
		a.readiness.Store(true)

		// Start server
		// http.ErrServerClosed is expected during graceful shutdown, not an actual error
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	logger.Info("Server ready. Waiting for interrupt signal...")
	<-quit

	// Post-process hook
	a.postProcess()

	// Begin graceful shutdown sequence
	// Step 1: Mark as not ready (load balancers will stop routing traffic)
	a.readiness.Store(false)
	drainDuration := time.Duration(a.config.ShutdownDrainSeconds) * time.Second
	logger.Info("readiness=false: start drain window duration=%v", drainDuration)

	// Step 2: Drain period - allow load balancers to detect unhealthy state
	time.Sleep(drainDuration)

	// Step 3: Stop worker pool (finish in-flight jobs)
	logger.Info("Stopping worker pool...")
    if a.forwarder != nil {
        a.forwarder.Stop()
    }

	// Step 4: Shutdown Echo server with timeout
	shutdownTimeout := time.Duration(a.config.ShutdownTimeoutSeconds) * time.Second
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	logger.Info("Shutting down Echo server...")
	if err := a.echo.Shutdown(shutdownCtx); err != nil {
		logger.Error("Shutdown error: %v", err)
		a.cancel()
		return err
	}

	// Step 5: Cancel application context (signals cleanup to other goroutines)
	a.cancel()

	logger.Info("Server stopped gracefully")
	return nil
}
