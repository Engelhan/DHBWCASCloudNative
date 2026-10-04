// Package event defines the JSON contract consumed from RESULT_TOPIC (produced by
// reservation-worker).
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
	// CustomerEmail is only required when SMTP is active.
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

// Validate checks required fields; requireEmail only when SMTP is enabled.
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

// Result returns the label for notifications_sent_total.
func (e Event) Result() string {
	if e.EventType == TypeReservationConfirmed {
		return "confirmed"
	}
	return "rejected"
}
