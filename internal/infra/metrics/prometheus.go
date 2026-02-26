package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	JobsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiapx_jobs_processed_total",
		Help: "Total number of jobs processed, by status",
	}, []string{"status"})

	JobProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "fiapx_job_processing_duration_seconds",
		Help:    "Duration of video processing pipeline",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"stage"})

	FramesExtractedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiapx_frames_extracted_total",
		Help: "Total number of frames extracted across all jobs",
	})

	ActiveWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fiapx_active_workers",
		Help: "Number of currently active workers processing jobs",
	})

	RetryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiapx_retry_total",
		Help: "Total number of retries",
	}, []string{"attempt"})
)
