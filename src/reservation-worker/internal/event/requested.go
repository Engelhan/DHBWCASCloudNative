// Package event defines the JSON contracts reservation-worker consumes from
// RESERVATION_TOPIC and publishes to RESULT_TOPIC.
package event

import (
	"encoding/json"
	"fmt"
	"time"
)

// Requested mirrors src/catalog-api/internal/reservation.Requested exactly
// (same JSON keys) -- catalog-api is the only producer of this contract, see
// src/catalog-api/README.md ## Kafka-Event-Schema.
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

// Validate checks structural requirements. A message failing this can never
// succeed on retry -- callers should treat it as a poison pill (log, skip,
// commit the offset regardless).
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
