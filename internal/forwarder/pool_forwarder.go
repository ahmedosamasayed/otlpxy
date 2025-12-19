package forwarder

import (
    "net/http"
    "zep-logger/internal/worker"
)

// PoolForwarder adapts worker.Pool to the Forwarder interface
type PoolForwarder struct {
    pool *worker.Pool
}

func NewPoolForwarder(pool *worker.Pool) *PoolForwarder {
    return &PoolForwarder{pool: pool}
}

func (p *PoolForwarder) Start() {
    if p.pool != nil {
        p.pool.Start()
    }
}

func (p *PoolForwarder) Stop() {
    if p.pool != nil {
        p.pool.Stop()
    }
}

func (p *PoolForwarder) Submit(body []byte, targetURL string, headers http.Header) error {
    if p.pool == nil {
        return nil
    }
    job := worker.Job{Body: body, TargetURL: targetURL, Headers: headers}
    return p.pool.SubmitJob(job)
}

func (p *PoolForwarder) GetQueueDepth() int {
    if p.pool == nil {
        return 0
    }
    return p.pool.GetQueueDepth()
}


