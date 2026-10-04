// Package metrics defines the Prometheus metrics documented in the README.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ReservationRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "reservation_requests_total",
		Help: "Number of received reservation requests by result.",
	}, []string{"result"}) // "accepted" | "seat_unavailable" | "error"

	ReservationRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reservation_request_duration_seconds",
		Help:    "Duration of POST /events/{id}/reservations (DB read + Kafka publish).",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})
)
