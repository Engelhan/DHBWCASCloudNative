package notify

import (
	"context"
	"log/slog"

	"ticketflow/notification-worker/internal/event"
)

// LogNotifier logs instead of sending; the demo-safe default.
type LogNotifier struct {
	Logger *slog.Logger
}

func (n LogNotifier) Notify(_ context.Context, e event.Event) error {
	n.Logger.Info("notification",
		"channel", ChannelLog,
		"eventType", e.EventType,
		"reservationId", e.ReservationID,
		"customerEmail", e.CustomerEmail,
		"result", e.Result(),
	)
	return nil
}
