package forwarder

import (
    "net/http"
)

// Forwarder defines the abstraction for forwarding requests asynchronously
// Implementations may use a bounded worker pool or a semaphore-based goroutine model
type Forwarder interface {
    // Start initializes any background workers/resources
    Start()

    // Stop gracefully stops the forwarder, waiting for in-flight tasks up to an internal timeout
    Stop()

    // Submit forwards a request body to the target URL with given headers asynchronously
    // Returns error if the implementation cannot accept more work immediately (e.g., pool queue full)
    Submit(body []byte, targetURL string, headers http.Header) error

    // GetQueueDepth returns current backlog depth (queue in pool mode, waiters in semaphore mode)
    GetQueueDepth() int
}


