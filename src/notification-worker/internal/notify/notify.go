// Package notify implements the notification channels (log, SMTP) and a
// thin Dispatcher that records metrics for a single delivery attempt.
// Retries and dead-letter handling live one layer up, in kafkaconsumer,
// since they need to coordinate with Kafka offset commits.
package notify

import (
	"context"

	"ticketflow/notification-worker/internal/event"
	"ticketflow/notification-worker/internal/metrics"
)

const (
	ChannelLog  = "log"
	ChannelSMTP = "smtp"
)

// Notifier delivers a single notification attempt for an event.
// Implementations should treat ctx's deadline as a hard timeout for the
// underlying I/O.
type Notifier interface {
	Notify(ctx context.Context, e event.Event) error
}

// Dispatcher wraps the active Notifier with metrics recording.
type Dispatcher struct {
	Notifier Notifier
	// Channel labels the notification_dispatch_errors_total metric and
	// tells callers whether CustomerEmail is required (see
	// kafkaconsumer.handleMessage).
	Channel string
}

func NewDispatcher(n Notifier, channel string) *Dispatcher {
	return &Dispatcher{Notifier: n, Channel: channel}
}

// Attempt performs exactly one delivery attempt and records the outcome.
func (d *Dispatcher) Attempt(ctx context.Context, e event.Event) error {
	if err := d.Notifier.Notify(ctx, e); err != nil {
		metrics.DispatchErrors.WithLabelValues(d.Channel).Inc()
		return err
	}
	metrics.NotificationsSent.WithLabelValues(e.Result()).Inc()
	return nil
}
