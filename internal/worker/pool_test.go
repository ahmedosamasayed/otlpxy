package worker

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerPool_BoundedConcurrency verifies max workers respected
// AC3: Worker pool test verifies bounded concurrency and job processing
func TestWorkerPool_BoundedConcurrency(t *testing.T) {
	concurrentWorkers := int32(0)
	maxConcurrentWorkers := int32(0)
	var mu sync.Mutex

	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track concurrent workers
		current := atomic.AddInt32(&concurrentWorkers, 1)

		mu.Lock()
		if current > maxConcurrentWorkers {
			maxConcurrentWorkers = current
		}
		mu.Unlock()

		// Simulate work
		time.Sleep(50 * time.Millisecond)

		atomic.AddInt32(&concurrentWorkers, -1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create pool with 2 workers
	pool := NewPool(2, 100, 5*time.Second)
	pool.Start()
	defer pool.Stop()

	// Submit 10 jobs
	for i := 0; i < 10; i++ {
        job := Job{
            TargetURL: mockCollector.URL,
            Body:      []byte("test"),
            Headers:   http.Header{},
        }
		err := pool.SubmitJob(job)
		if err != nil {
			t.Fatalf("failed to submit job %d: %v", i, err)
		}
	}

	// Wait for all jobs to complete
	time.Sleep(1 * time.Second)

	// Verify bounded concurrency (max 2 workers)
	if maxConcurrentWorkers > 2 {
		t.Errorf("expected max 2 concurrent workers, got %d", maxConcurrentWorkers)
	}
}

// TestWorkerPool_JobQueueBufferSize verifies queue size enforced
// AC3: Worker pool test verifies job queue buffer size enforced
func TestWorkerPool_JobQueueBufferSize(t *testing.T) {
	// Create pool with small queue (5 jobs)
	pool := NewPool(1, 5, 5*time.Second)
	pool.Start()
	defer pool.Stop()

	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow processing to fill queue
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Submit jobs up to queue capacity
	successCount := 0
	for i := 0; i < 10; i++ {
        job := Job{
            TargetURL: mockCollector.URL,
            Body:      []byte("test"),
            Headers:   http.Header{},
        }
		err := pool.SubmitJob(job)
		if err == nil {
			successCount++
		}
	}

	// Verify queue size was enforced (should accept ~6 jobs: 1 in-flight + 5 queued)
	if successCount < 5 || successCount > 7 {
		t.Errorf("expected ~6 jobs accepted (1 worker + 5 queue), got %d", successCount)
	}
}

// TestWorkerPool_Backpressure verifies queue full returns error
// AC3: Worker pool test verifies backpressure (queue full returns error)
func TestWorkerPool_Backpressure(t *testing.T) {
	// Create pool with tiny queue (1 job)
	pool := NewPool(1, 1, 5*time.Second)
	pool.Start()
	defer pool.Stop()

	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Very slow processing to guarantee queue fills
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

    job := Job{
        TargetURL: mockCollector.URL,
        Body:      []byte("test"),
        Headers:   http.Header{},
    }

	// Fill queue (1 in-flight + 1 queued)
	_ = pool.SubmitJob(job)
	_ = pool.SubmitJob(job)

	// This should fail with backpressure error
	err := pool.SubmitJob(job)
	if err == nil {
		t.Error("expected backpressure error when queue is full, got nil")
	}
}

// TestWorkerPool_GracefulShutdown verifies in-flight jobs complete
// AC3: Worker pool test verifies graceful shutdown (in-flight jobs complete)
func TestWorkerPool_GracefulShutdown(t *testing.T) {
	completedJobs := int32(0)

	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate work
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt32(&completedJobs, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	pool := NewPool(2, 10, 5*time.Second)
	pool.Start()

	// Submit 5 jobs
	for i := 0; i < 5; i++ {
        job := Job{
            TargetURL: mockCollector.URL,
            Body:      []byte("test"),
            Headers:   http.Header{},
        }
		_ = pool.SubmitJob(job)
	}

	// Stop pool (should wait for in-flight jobs)
	pool.Stop()

	// Verify all jobs completed
	if completedJobs != 5 {
		t.Errorf("expected 5 jobs completed, got %d", completedJobs)
	}
}

// TestWorkerPool_StartStopLifecycle verifies Start() and Stop() methods
// AC3: Worker pool test verifies Start() and Stop() lifecycle methods
func TestWorkerPool_StartStopLifecycle(t *testing.T) {
	pool := NewPool(2, 10, 5*time.Second)

	// Verify pool can be started
	pool.Start()

	// Submit a job to verify pool is running
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

    job := Job{
        TargetURL: mockCollector.URL,
        Body:      []byte("test"),
        Headers:   http.Header{},
    }
	err := pool.SubmitJob(job)
	if err != nil {
		t.Fatalf("failed to submit job after Start(): %v", err)
	}

	// Verify pool can be stopped
	pool.Stop()

	// Verify Stop() is idempotent (calling twice is safe)
	pool.Stop()
}

// TestWorkerPool_MultipleStartCalls verifies startOnce behavior
func TestWorkerPool_MultipleStartCalls(t *testing.T) {
	pool := NewPool(2, 10, 5*time.Second)

	// Start multiple times
	pool.Start()
	pool.Start()
	pool.Start()

	// Should not panic or create duplicate workers
	// Verify pool still works correctly
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

    job := Job{
        TargetURL: mockCollector.URL,
        Body:      []byte("test"),
        Headers:   http.Header{},
    }
	err := pool.SubmitJob(job)
	if err != nil {
		t.Fatalf("pool not working after multiple Start() calls: %v", err)
	}

	pool.Stop()
}

// TestWorkerPool_GetQueueDepth verifies queue depth metric
func TestWorkerPool_GetQueueDepth(t *testing.T) {
	pool := NewPool(1, 10, 5*time.Second)
	pool.Start()
	defer pool.Stop()

	// Initial depth should be 0
	if depth := pool.GetQueueDepth(); depth != 0 {
		t.Errorf("expected initial queue depth 0, got %d", depth)
	}

	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow processing to keep jobs in queue
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Submit jobs
	for i := 0; i < 3; i++ {
        job := Job{
            TargetURL: mockCollector.URL,
            Body:      []byte("test"),
            Headers:   http.Header{},
        }
		_ = pool.SubmitJob(job)
	}

	// Queue depth should be > 0 (some jobs waiting)
	time.Sleep(50 * time.Millisecond)
	depth := pool.GetQueueDepth()
	if depth == 0 {
		t.Error("expected queue depth > 0 after submitting jobs, got 0")
	}
}

// TestWorkerPool_ShutdownTimeout verifies timeout behavior
func TestWorkerPool_ShutdownTimeout(t *testing.T) {
	mockCollector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Very slow job (exceeds shutdown timeout)
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockCollector.Close()

	// Create pool with short shutdown timeout (1 second)
	pool := NewPool(2, 10, 1*time.Second)
	pool.Start()

	// Submit long-running job
	job := Job{
		TargetURL: mockCollector.URL,
		Body:      []byte("test"),
		Headers:   http.Header{},
	}
	_ = pool.SubmitJob(job)

	// Give job time to start
	time.Sleep(100 * time.Millisecond)

	// Stop should return after timeout (not wait 3 seconds)
	start := time.Now()
	pool.Stop()
	elapsed := time.Since(start)

	// Verify Stop returned within ~1 second (not 3 seconds)
	if elapsed > 2*time.Second {
		t.Errorf("Stop() took %v, expected ~1s (timeout)", elapsed)
	}
}
