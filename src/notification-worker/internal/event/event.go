// Package event defines the JSON contract notification-worker consumes
// from RESULT_TOPIC. No producer for this contract exists yet in the repo
// (reservation-worker isn't built) -- this is the first definition of it;
// reservation-worker must match this shape when it's implemented.
package event

import (
	"encoding/json"
	"fmt"
	"time"
)

type Type string

const (
	TypeReservationConfirmed Type = "ReservationConfirmed"
	TypeReservationRejected  Type = "ReservationRejected"
)

type Event struct {
	EventType       Type      `json:"eventType"`
	ReservationID   string    `json:"reservationId"`
	EventOccurredAt time.Time `json:"eventOccurredAt"`
	// CustomerEmail is the recipient for the optional SMTP channel. Required
	// only when SMTP is actually active (see Validate) -- the log-only
	// default path must keep working even before reservation-worker
	// reliably populates this field.
	CustomerEmail string `json:"customerEmail"`
	EventName     string `json:"eventName,omitempty"`
	SeatID        string `json:"seatId,omitempty"`
	// Reason is only meaningful when EventType is TypeReservationRejected.
	Reason *string `json:"reason,omitempty"`
}

func Unmarshal(data []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return Event{}, fmt.Errorf("unmarshal event: %w", err)
	}
	return e, nil
}

// Validate checks structural requirements. requireEmail should be true only
// when the active notifier actually needs CustomerEmail (i.e. SMTP is
// enabled).
func (e Event) Validate(requireEmail bool) error {
	switch e.EventType {
	case TypeReservationConfirmed, TypeReservationRejected:
	default:
		return fmt.Errorf("eventType must be ReservationConfirmed or ReservationRejected, got %q", e.EventType)
	}
	if e.ReservationID == "" {
		return fmt.Errorf("reservationId is required")
	}
	if requireEmail && e.CustomerEmail == "" {
		return fmt.Errorf("customerEmail is required")
	}
	return nil
}

// Result returns the lowercase result label used for the
// notifications_sent_total metric.
func (e Event) Result() string {
	if e.EventType == TypeReservationConfirmed {
		return "confirmed"
	}
	return "rejected"
}
