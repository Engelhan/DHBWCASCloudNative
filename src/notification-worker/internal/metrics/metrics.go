// Package metrics defines the Prometheus metrics exposed on /metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// NotificationsSent matches the contract documented in
	// src/notification-worker/README.md before this implementation existed.
	NotificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notifications_sent_total",
		Help: "Total number of notifications successfully dispatched, by reservation result.",
	}, []string{"result"})

	// DispatchErrors counts every failed delivery attempt, plus malformed
	// ("unmarshal") messages that could never be dispatched at all.
	DispatchErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_dispatch_errors_total",
		Help: "Total number of failed notification dispatch attempts, by channel.",
	}, []string{"channel"})

	// DLTPublished counts messages moved to the dead-letter topic, either
	// after exhausting retries or because they were malformed.
	DLTPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "notification_dlt_published_total",
		Help: "Total number of events published to the dead-letter topic.",
	})
)
