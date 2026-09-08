// Package metrics definiert die in src/catalog-api/README.md dokumentierten Prometheus-Metriken.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ReservationRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "reservation_requests_total",
		Help: "Anzahl entgegengenommener Reservierungsanfragen nach Ergebnis.",
	}, []string{"result"}) // "accepted" | "seat_unavailable" | "error"

	ReservationRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reservation_request_duration_seconds",
		Help:    "Dauer der Bearbeitung von POST /events/{id}/reservations (DB-Read + Kafka-Publish).",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})
)
