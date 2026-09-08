package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"ticketflow/notification-worker/internal/event"
	"ticketflow/notification-worker/internal/metrics"
)

type fakeNotifier struct {
	err error
}

func (f fakeNotifier) Notify(_ context.Context, _ event.Event) error {
	return f.err
}

func TestDispatcher_Attempt_Success(t *testing.T) {
	before := testutil.ToFloat64(metrics.NotificationsSent.WithLabelValues("confirmed"))

	d := NewDispatcher(fakeNotifier{}, ChannelLog)
	e := event.Event{EventType: event.TypeReservationConfirmed, ReservationID: "r1"}

	if err := d.Attempt(context.Background(), e); err != nil {
		t.Fatalf("Attempt() unexpected error: %v", err)
	}

	after := testutil.ToFloat64(metrics.NotificationsSent.WithLabelValues("confirmed"))
	if after != before+1 {
		t.Errorf("notifications_sent_total{result=confirmed} = %v, want %v", after, before+1)
	}
}

func TestDispatcher_Attempt_Failure(t *testing.T) {
	before := testutil.ToFloat64(metrics.DispatchErrors.WithLabelValues(ChannelSMTP))

	wantErr := errors.New("smtp relay unreachable")
	d := NewDispatcher(fakeNotifier{err: wantErr}, ChannelSMTP)
	e := event.Event{EventType: event.TypeReservationRejected, ReservationID: "r2"}

	err := d.Attempt(context.Background(), e)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Attempt() error = %v, want %v", err, wantErr)
	}

	after := testutil.ToFloat64(metrics.DispatchErrors.WithLabelValues(ChannelSMTP))
	if after != before+1 {
		t.Errorf("notification_dispatch_errors_total{channel=smtp} = %v, want %v", after, before+1)
	}
}
