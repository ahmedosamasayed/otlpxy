package forwarder

import (
    "bytes"
    "net/http"
    "sync"
    "time"

    "go.uber.org/atomic"

    "zep-logger/internal/metrics"
    "zep-logger/pkg/logger"
)

// SemaphoreForwarder implements Forwarder using a semaphore-limited goroutine model
type SemaphoreForwarder struct {
    maxConcurrent   int
    tokens          chan struct{}
    httpClient      *http.Client
    wg              sync.WaitGroup
    waiters         atomic.Int64
    startOnce       sync.Once
    stopOnce        sync.Once
    stopped         atomic.Bool
    shutdownTimeout time.Duration
}

// NewSemaphoreForwarder creates a new semaphore-based forwarder
func NewSemaphoreForwarder(maxConcurrent int, shutdownTimeout time.Duration) *SemaphoreForwarder {
    if maxConcurrent <= 0 {
        maxConcurrent = 10000
    }

    transport := &http.Transport{
        Proxy:                 http.ProxyFromEnvironment,
        ForceAttemptHTTP2:     true,
        MaxIdleConns:          maxConcurrent * 2,
        MaxIdleConnsPerHost:   maxConcurrent,
        MaxConnsPerHost:       maxConcurrent * 2,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
    }

    return &SemaphoreForwarder{
        maxConcurrent:   maxConcurrent,
        tokens:          make(chan struct{}, maxConcurrent),
        httpClient:      &http.Client{Transport: transport, Timeout: 10 * time.Second},
        shutdownTimeout: shutdownTimeout,
    }
}

func (s *SemaphoreForwarder) Start() {
    s.startOnce.Do(func() {
        logger.Info("Semaphore forwarder started with maxConcurrent=%d", s.maxConcurrent)
    })
}

func (s *SemaphoreForwarder) Stop() {
    s.stopOnce.Do(func() {
        s.stopped.Store(true)
        logger.Info("Stopping semaphore forwarder: waiting for in-flight goroutines")

        done := make(chan struct{})
        go func() {
            defer close(done)
            s.wg.Wait()
        }()

        select {
        case <-done:
            logger.Info("Semaphore forwarder stopped: all goroutines finished")
        case <-time.After(s.shutdownTimeout):
            logger.Warn("Semaphore forwarder stop timed out after %v", s.shutdownTimeout)
        }
    })
}

func (s *SemaphoreForwarder) Submit(body []byte, targetURL string, headers http.Header) error {
    if s.stopped.Load() {
        return nil // during shutdown, readiness will block new traffic
    }

    s.wg.Add(1)
    go func() {
        defer s.wg.Done()

        s.waiters.Inc()
        s.tokens <- struct{}{} // acquire; blocks when at max concurrency
        s.waiters.Dec()
        defer func() { <-s.tokens }() // release

        metrics.ActiveWorkersGauge.Inc()
        defer metrics.ActiveWorkersGauge.Dec()

        req, err := http.NewRequest("POST", targetURL, bytes.NewReader(body))
        if err != nil {
            logger.Error("Semaphore forwarder: failed to create request: %v", err)
            metrics.JobsFailedCounter.Inc()
            return
        }
        for k, values := range headers {
            for _, v := range values {
                req.Header.Add(k, v)
            }
        }

        resp, err := s.httpClient.Do(req)
        if err != nil {
            logger.Error("Semaphore forwarder: forwarding to %s failed: %v", targetURL, err)
            metrics.JobsFailedCounter.Inc()
            return
        }
        resp.Body.Close()

        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            logger.Warn("Semaphore forwarder: collector returned %d for %s", resp.StatusCode, targetURL)
            metrics.JobsFailedCounter.Inc()
        } else {
            metrics.JobsProcessedCounter.Inc()
        }
    }()

    return nil
}

func (s *SemaphoreForwarder) GetQueueDepth() int {
    v := s.waiters.Load()
    if v < 0 {
        return 0
    }
    if v > int64(^uint(0)>>1) { // guard though unrealistic
        return int(^uint(0) >> 1)
    }
    return int(v)
}


