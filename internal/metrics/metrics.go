package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// QueueDepthGauge tracks the current depth of the worker pool job queue
	QueueDepthGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "zep_logger",
		Name:      "worker_pool_queue_depth",
		Help:      "Current number of jobs in the worker pool queue",
	})

	// JobsProcessedCounter tracks the total number of jobs successfully forwarded
	JobsProcessedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "zep_logger",
		Name:      "worker_pool_jobs_processed_total",
		Help:      "Total number of jobs successfully processed by the worker pool",
	})

	// JobsFailedCounter tracks the total number of jobs that failed to forward
	JobsFailedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "zep_logger",
		Name:      "worker_pool_jobs_failed_total",
		Help:      "Total number of jobs that failed to process (request errors, collector errors)",
	})

	// ActiveWorkersGauge tracks the number of workers currently processing jobs
	ActiveWorkersGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "zep_logger",
		Name:      "worker_pool_active_workers",
		Help:      "Current number of workers actively processing jobs (sending HTTP requests)",
	})
)
