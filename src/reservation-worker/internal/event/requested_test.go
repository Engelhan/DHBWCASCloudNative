package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnmarshalRequested_RoundTrip(t *testing.T) {
	original := Requested{
		ReservationID: "6c1f6e8e-2b3a-4c1d-9e0a-6a2b1c3d4e5f",
		EventID:       "demo-konzert",
		SeatID:        "demo-konzert-seat-7",
		SessionID:     "9e0a-session",
		FirstName:     "Max",
		LastName:      "Mustermann",
		Email:         "max@example.com",
		RequestedAt:   time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	got, err := UnmarshalRequested(data)
	if err != nil {
		t.Fatalf("UnmarshalRequested() error: %v", err)
	}
	// Field-wise: time.Time equality via == is fragile, use Equal.
	if got.ReservationID != original.ReservationID || got.EventID != original.EventID ||
		got.SeatID != original.SeatID || got.SessionID != original.SessionID ||
		got.FirstName != original.FirstName || got.LastName != original.LastName ||
		got.Email != original.Email || !got.RequestedAt.Equal(original.RequestedAt) {
		t.Errorf("UnmarshalRequested() round-trip mismatch: got %+v, want %+v", got, original)
	}
}

func TestUnmarshalRequested_InvalidJSON(t *testing.T) {
	if _, err := UnmarshalRequested([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRequestedValidate(t *testing.T) {
	valid := Requested{
		ReservationID: "r1",
		EventID:       "e1",
		SeatID:        "s1",
		SessionID:     "sess1",
		FirstName:     "Max",
		LastName:      "Mustermann",
		Email:         "max@example.com",
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("Validate() on fully populated Requested: unexpected error %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Requested)
	}{
		{"missing reservation_id", func(r *Requested) { r.ReservationID = "" }},
		{"missing event_id", func(r *Requested) { r.EventID = "" }},
		{"missing seat_id", func(r *Requested) { r.SeatID = "" }},
		{"missing session_id", func(r *Requested) { r.SessionID = "" }},
		{"missing first_name", func(r *Requested) { r.FirstName = "" }},
		{"missing last_name", func(r *Requested) { r.LastName = "" }},
		{"missing email", func(r *Requested) { r.Email = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid
			tt.mutate(&r)
			if err := r.Validate(); err == nil {
				t.Errorf("Validate() with %s: expected error, got nil", tt.name)
			}
		})
	}
}
