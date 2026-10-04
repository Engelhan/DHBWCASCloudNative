// Package notify implements the channels (log, SMTP) and a Dispatcher recording metrics per
// attempt. Retries and DLT handling live in kafkaconsumer.
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

// Notifier performs one delivery attempt; ctx's deadline is a hard timeout.
type Notifier interface {
	Notify(ctx context.Context, e event.Event) error
}

// Dispatcher wraps the active Notifier with metrics recording.
type Dispatcher struct {
	Notifier Notifier
	// Channel labels the error metric and tells whether CustomerEmail is required.
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
