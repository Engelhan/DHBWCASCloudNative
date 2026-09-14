// Package metrics defines the Prometheus metrics exposed on /metrics, as
// documented in src/reservation-worker/README.md ## Zweck.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ReservationsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "reservations_processed_total",
		Help: "Total number of processed reservation requests, by result.",
	}, []string{"result"}) // "confirmed" | "rejected"

	ReservationProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reservation_processing_duration_seconds",
		Help:    "Duration of processing a single ReservationRequested message (DB transaction + result publish).",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})

	// ProcessingErrors counts failed attempts by stage -- "unmarshal" for
	// malformed/invalid messages, "db" for the booking transaction, "publish"
	// for the RESULT_TOPIC write.
	ProcessingErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "reservation_processing_errors_total",
		Help: "Total number of failed reservation processing attempts, by stage.",
	}, []string{"stage"})
)
