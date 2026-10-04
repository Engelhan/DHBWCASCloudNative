// Package metrics defines the Prometheus metrics exposed on /metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// NotificationsSent counts sent notifications by result.
	NotificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notifications_sent_total",
		Help: "Total number of notifications successfully dispatched, by reservation result.",
	}, []string{"result"})

	// DispatchErrors counts failed attempts plus malformed messages.
	DispatchErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_dispatch_errors_total",
		Help: "Total number of failed notification dispatch attempts, by channel.",
	}, []string{"channel"})

	// DLTPublished counts messages moved to the dead-letter topic.
	DLTPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notification_dlt_published_total",
		Help: "Total number of events published to the dead-letter topic.",
	})
)
