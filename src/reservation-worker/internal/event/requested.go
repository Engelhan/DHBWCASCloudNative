// Package event defines the consumed and published JSON contracts.
package event

import (
	"encoding/json"
	"fmt"
	"time"
)

// Requested mirrors catalog-api's reservation.Requested (same JSON keys).
type Requested struct {
	ReservationID string    `json:"reservation_id"`
	EventID       string    `json:"event_id"`
	SeatID        string    `json:"seat_id"`
	SessionID     string    `json:"session_id"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	Email         string    `json:"email"`
	RequestedAt   time.Time `json:"requested_at"`
}

func UnmarshalRequested(data []byte) (Requested, error) {
	var r Requested
	if err := json.Unmarshal(data, &r); err != nil {
		return Requested{}, fmt.Errorf("unmarshal reservation requested: %w", err)
	}
	return r, nil
}

// Validate checks required fields; a failing message is a poison pill (skip and commit).
func (r Requested) Validate() error {
	if r.ReservationID == "" {
		return fmt.Errorf("reservation_id is required")
	}
	if r.EventID == "" {
		return fmt.Errorf("event_id is required")
	}
	if r.SeatID == "" {
		return fmt.Errorf("seat_id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if r.FirstName == "" {
		return fmt.Errorf("first_name is required")
	}
	if r.LastName == "" {
		return fmt.Errorf("last_name is required")
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}
