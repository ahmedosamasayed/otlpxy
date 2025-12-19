package forwarder

import (
    "bytes"
    "fmt"
    "net/http"
    "sync"
    "time"

    "go.uber.org/atomic"

    "zep-logger/internal/metrics"
    "zep-logger/pkg/logger"
)

type hybridJob struct {
    body      []byte
    targetURL string
    headers   http.Header
}

// HybridForwarder: dispatcher worker pool + concurrency semaphore
// - Bounded job queue provides backpressure and memory predictability
// - Semaphore bounds actual HTTP concurrency at a higher level
type HybridForwarder struct {
    workerCount     int
    jobQueue        chan hybridJob
    tokens          chan struct{}
    httpClient      *http.Client
    wg              sync.WaitGroup
    sendWG          sync.WaitGroup
    startOnce       sync.Once
    stopOnce        sync.Once
    stopped         atomic.Bool
    shutdownTimeout time.Duration
}

func NewHybridForwarder(workerCount int, jobQueueSize int, maxConcurrent int, shutdownTimeout time.Duration) *HybridForwarder {
    if workerCount <= 0 {
        workerCount = 1
    }
    if jobQueueSize <= 0 {
        jobQueueSize = 10000
    }
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

    return &HybridForwarder{
        workerCount:     workerCount,
        jobQueue:        make(chan hybridJob, jobQueueSize),
        tokens:          make(chan struct{}, maxConcurrent),
        httpClient:      &http.Client{Transport: transport, Timeout: 10 * time.Second},
        shutdownTimeout: shutdownTimeout,
    }
}

func (h *HybridForwarder) Start() {
    h.startOnce.Do(func() {
        logger.Info("Hybrid forwarder starting: workers=%d, queueSize=%d, maxConcurrent=%d", h.workerCount, cap(h.jobQueue), cap(h.tokens))
        for i := 0; i < h.workerCount; i++ {
            h.wg.Add(1)
            go h.worker(i)
        }
        logger.Info("Hybrid forwarder started")
    })
}

func (h *HybridForwarder) Stop() {
    h.stopOnce.Do(func() {
        h.stopped.Store(true)
        logger.Info("Stopping hybrid forwarder: closing job queue and waiting for workers")
        close(h.jobQueue)

        bothDone := make(chan struct{})
        go func() {
            h.wg.Wait()     // wait workers
            h.sendWG.Wait() // wait in-flight sends
            close(bothDone)
        }()

        select {
        case <-bothDone:
            logger.Info("Hybrid forwarder stopped: workers and in-flight sends finished")
        case <-time.After(h.shutdownTimeout):
            logger.Warn("Hybrid forwarder stop timed out after %v", h.shutdownTimeout)
        }
    })
}

func (h *HybridForwarder) Submit(body []byte, targetURL string, headers http.Header) error {
    if h.stopped.Load() {
        return fmt.Errorf("hybrid forwarder stopped")
    }
    job := hybridJob{body: body, targetURL: targetURL, headers: headers}
    select {
    case h.jobQueue <- job:
        return nil
    default:
        logger.Warn("Hybrid job queue full: rejecting new job (queue size: %d)", cap(h.jobQueue))
        return fmt.Errorf("hybrid queue full (capacity: %d)", cap(h.jobQueue))
    }
}

func (h *HybridForwarder) GetQueueDepth() int {
    return len(h.jobQueue)
}

func (h *HybridForwarder) worker(id int) {
    defer h.wg.Done()
    client := h.httpClient
    for job := range h.jobQueue {
        // Acquire concurrency token (blocks when at max concurrency)
        h.tokens <- struct{}{}

        // Fire-and-forget sender goroutine; worker immediately returns to fetch next job
        h.sendWG.Add(1)
        go func(j hybridJob) {
            defer h.sendWG.Done()
            defer func() { <-h.tokens }() // release token at end

            metrics.ActiveWorkersGauge.Inc()
            defer metrics.ActiveWorkersGauge.Dec()

            req, err := http.NewRequest("POST", j.targetURL, bytes.NewReader(j.body))
            if err != nil {
                logger.Error("Hybrid send: failed to create request: %v", err)
                metrics.JobsFailedCounter.Inc()
                return
            }
            for k, values := range j.headers {
                for _, v := range values {
                    req.Header.Add(k, v)
                }
            }

            resp, err := client.Do(req)
            if err != nil {
                logger.Error("Hybrid send: forwarding to %s failed: %v", j.targetURL, err)
                metrics.JobsFailedCounter.Inc()
                return
            }
            resp.Body.Close()

            if resp.StatusCode < 200 || resp.StatusCode >= 300 {
                logger.Warn("Hybrid send: collector returned %d for %s", resp.StatusCode, j.targetURL)
                metrics.JobsFailedCounter.Inc()
            } else {
                metrics.JobsProcessedCounter.Inc()
            }
        }(job)
    }
}


