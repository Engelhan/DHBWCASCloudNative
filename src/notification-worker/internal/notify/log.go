package notify

import (
	"context"
	"log/slog"

	"ticketflow/notification-worker/internal/event"
)

// LogNotifier writes a structured log line instead of sending a real
// notification. It is the always-available, demo-safe default channel and
// practically never fails.
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
