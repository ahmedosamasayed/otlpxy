package worker

import (
    "bytes"
    "fmt"
    "net/http"
    "runtime"
    "sync"
    "time"
    "zep-logger/internal/metrics"
    "zep-logger/pkg/logger"
)

// Job represents a forwarding task to be processed by the worker pool
// Contains all necessary information to forward a request to the OTel collector
type Job struct {
	Body      []byte            // Buffered request body
	TargetURL string            // Target OTel collector URL
    Headers   http.Header       // Headers to include in forwarded request (multi-valued)
}

// Pool represents a bounded goroutine worker pool for async request forwarding
// Implements a fixed-size pool of workers processing jobs from a buffered channel
type Pool struct {
	workerCount     int           // Number of worker goroutines
	jobQueue        chan Job      // Buffered channel for queuing forwarding jobs
	wg              sync.WaitGroup
	stopOnce        sync.Once     // Ensures Stop() is called only once
	startOnce       sync.Once     // Ensures Start() is called only once
	shutdownTimeout time.Duration // Maximum time to wait for workers to finish during shutdown
    httpClient      *http.Client  // Shared HTTP client with connection pooling
    permits         chan struct{} // Counts in-flight + queued jobs for deterministic backpressure
}

// NewPool creates a new worker pool with the specified configuration
// Follows constructor injection pattern - all dependencies passed via parameters
//
// Parameters:
//   - workerCount: Number of worker goroutines (default: 2×NumCPU per ADR-001)
//   - jobQueueSize: Buffer capacity for job queue (default: 10000)
//   - shutdownTimeout: Maximum time to wait for workers during shutdown (e.g., 10s)
//
// Returns configured Pool instance ready to be started
func NewPool(workerCount int, jobQueueSize int, shutdownTimeout time.Duration) *Pool {
    // Use 50×NumCPU as default if workerCount is 0 or negative (IO-bound workload)
    // For HTTP forwarding, workers spend most time waiting on network I/O,
    // so we can use many more workers than CPU cores to maximize throughput
    if workerCount <= 0 {
        workerCount = 50 * runtime.NumCPU()
        logger.Info("Worker pool size not configured, using default: %d (50×NumCPU for I/O-bound workload)", workerCount)
    }

	// Use 10000 as default job queue size
	if jobQueueSize <= 0 {
		jobQueueSize = 10000
		logger.Info("Job queue size not configured, using default: %d", jobQueueSize)
	}

	logger.Info("Creating worker pool: workers=%d, queueSize=%d, shutdownTimeout=%v", workerCount, jobQueueSize, shutdownTimeout)

    // Create a shared HTTP client with aggressive connection pooling
    // Tuned for high-concurrency I/O-bound workloads
    transport := &http.Transport{
        Proxy:                 http.ProxyFromEnvironment,
        ForceAttemptHTTP2:     true,
        MaxIdleConns:          workerCount * 2,        // Higher connection reuse
        MaxIdleConnsPerHost:   workerCount,            // One connection per worker
        MaxConnsPerHost:       workerCount * 2,        // Allow connection bursts
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
    }

    return &Pool{
        workerCount:     workerCount,
        jobQueue:        make(chan Job, jobQueueSize),
        shutdownTimeout: shutdownTimeout,
        httpClient:      &http.Client{Transport: transport, Timeout: 10 * time.Second},
        permits:         make(chan struct{}, workerCount+jobQueueSize),
    }
}

// Start spawns all worker goroutines to begin processing jobs
// Workers will block waiting for jobs on the job queue channel
// This method should be called during application startup (e.g., in preProcess)
// It is safe to call multiple times - workers will only be started once
func (p *Pool) Start() {
	p.startOnce.Do(func() {
		logger.Info("Starting worker pool with %d workers", p.workerCount)

		for i := 0; i < p.workerCount; i++ {
			p.wg.Add(1)
			go p.worker(i)
		}

		logger.Info("Worker pool started successfully")
	})
}

// Stop gracefully shuts down the worker pool
// Closes the job queue channel and waits for all workers to finish processing
// In-flight jobs will complete before shutdown finishes, up to shutdownTimeout
// If the timeout is exceeded, Stop returns but some workers may still be running
// This method is safe to call multiple times (only executes once)
func (p *Pool) Stop() {
	p.stopOnce.Do(func() {
		logger.Info("Stopping worker pool: closing job queue and waiting for workers to finish")

		// Close the job queue to signal workers to exit
		close(p.jobQueue)

		// Wait for all workers to finish, with timeout protection
		done := make(chan struct{})
		go func() {
			defer close(done)
			p.wg.Wait()
		}()

		select {
		case <-done:
			logger.Info("Worker pool stopped: all workers finished gracefully")
		case <-time.After(p.shutdownTimeout):
			logger.Warn("Worker pool stop timed out after %v: some workers may not have finished", p.shutdownTimeout)
		}
	})
}

// GetQueueDepth returns the current number of jobs in the queue
// This is useful for monitoring and metrics collection
func (p *Pool) GetQueueDepth() int {
	return len(p.jobQueue)
}

// SubmitJob submits a new forwarding job to the worker pool
// Returns error if the job queue is full (backpressure handling)
//
// Parameters:
//   - job: Job containing request body, target URL, and headers
//
// Returns error if queue is full, nil on success
func (p *Pool) SubmitJob(job Job) error {
    // First, check system-wide capacity: in-flight (workers) + queued (buffer)
    select {
    case p.permits <- struct{}{}:
        // There is capacity in the system; deliver the job. Use a blocking send
        // because a worker may be ready to receive immediately or buffer has space.
        p.jobQueue <- job
        return nil
    default:
        // No capacity available -> backpressure
        logger.Warn("Job queue full: rejecting new job (queue size: %d)", cap(p.jobQueue))
        return fmt.Errorf("worker pool queue full (capacity: %d)", cap(p.jobQueue))
    }
}

// worker is the main worker goroutine loop
// Processes jobs from the queue until the channel is closed
// Each worker runs independently and concurrently with other workers
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	logger.Info("Worker %d started", id)

    // Reuse shared HTTP client with connection pooling
    client := p.httpClient

    for job := range p.jobQueue {
		// Increment active workers gauge
		metrics.ActiveWorkersGauge.Inc()

		// Build HTTP POST request from Job struct
		req, err := http.NewRequest("POST", job.TargetURL, bytes.NewReader(job.Body))
		if err != nil {
			logger.Error("Worker %d: failed to create request: %v", id, err)
			metrics.JobsFailedCounter.Inc()
			metrics.ActiveWorkersGauge.Dec()
			// Release permit for this job
			<-p.permits
			continue
		}

        // Set headers from job (Content-Type, Authorization, etc.) with full values
        for key, values := range job.Headers {
            for _, v := range values {
                req.Header.Add(key, v)
            }
        }

		// Execute HTTP request with timeout
		resp, err := client.Do(req)
		if err != nil {
			// Log forwarding errors to stderr (don't propagate to client)
			logger.Error("Worker %d: forwarding to %s failed: %v", id, job.TargetURL, err)
			metrics.JobsFailedCounter.Inc()
			metrics.ActiveWorkersGauge.Dec()
			// Release permit for this job
			<-p.permits
			continue
		}

		// Close response body to reuse connection
		resp.Body.Close()

		// Log non-2xx responses as warnings (operational visibility)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Warn("Worker %d: collector returned %d for %s", id, resp.StatusCode, job.TargetURL)
			metrics.JobsFailedCounter.Inc()
		} else {
			// Job processed successfully
			metrics.JobsProcessedCounter.Inc()
		}

		// Decrement active workers gauge
		metrics.ActiveWorkersGauge.Dec()
        // Release permit after finishing this job
        <-p.permits
	}

	logger.Info("Worker %d stopped", id)
}
