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

// Result mirrors notification-worker's event.Event (same JSON keys).
type Result struct {
	EventType       Type      `json:"eventType"`
	ReservationID   string    `json:"reservationId"`
	EventOccurredAt time.Time `json:"eventOccurredAt"`
	CustomerEmail   string    `json:"customerEmail"`
	EventName       string    `json:"eventName,omitempty"`
	SeatID          string    `json:"seatId,omitempty"`
	// Reason is only set when EventType is TypeReservationRejected.
	Reason *string `json:"reason,omitempty"`
}

func (r Result) Marshal() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshal reservation result: %w", err)
	}
	return data, nil
}

// Result returns the label for reservations_processed_total.
func (r Result) Result() string {
	if r.EventType == TypeReservationConfirmed {
		return "confirmed"
	}
	return "rejected"
}
