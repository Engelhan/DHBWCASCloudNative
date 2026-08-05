package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnmarshal_RoundTrip(t *testing.T) {
	reason := "SOLD_OUT"
	original := Event{
		EventType:       TypeReservationRejected,
		ReservationID:   "6c1f6e8e-2b3a-4c1d-9e0a-6a2b1c3d4e5f",
		EventOccurredAt: time.Date(2026, 8, 5, 18, 32, 10, 0, time.UTC),
		CustomerEmail:   "buyer@example.com",
		EventName:       "Rock Night Vol. 3",
		SeatID:          "A-12",
		Reason:          &reason,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if got.EventType != original.EventType || got.ReservationID != original.ReservationID ||
		got.CustomerEmail != original.CustomerEmail || *got.Reason != *original.Reason {
		t.Errorf("Unmarshal() round-trip mismatch: got %+v, want %+v", got, original)
	}
}

func TestUnmarshal_InvalidJSON(t *testing.T) {
	if _, err := Unmarshal([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name         string
		event        Event
		requireEmail bool
		wantErr      bool
	}{
		{
			name:         "valid confirmed, email not required",
			event:        Event{EventType: TypeReservationConfirmed, ReservationID: "r1"},
			requireEmail: false,
			wantErr:      false,
		},
		{
			name:         "missing email but required",
			event:        Event{EventType: TypeReservationConfirmed, ReservationID: "r1"},
			requireEmail: true,
			wantErr:      true,
		},
		{
			name:         "email present and required",
			event:        Event{EventType: TypeReservationConfirmed, ReservationID: "r1", CustomerEmail: "a@b.com"},
			requireEmail: true,
			wantErr:      false,
		},
		{
			name:         "missing reservationId",
			event:        Event{EventType: TypeReservationConfirmed},
			requireEmail: false,
			wantErr:      true,
		},
		{
			name:         "unknown eventType",
			event:        Event{EventType: "SomethingElse", ReservationID: "r1"},
			requireEmail: false,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate(tt.requireEmail)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResult(t *testing.T) {
	if got := (Event{EventType: TypeReservationConfirmed}).Result(); got != "confirmed" {
		t.Errorf("Result() = %q, want %q", got, "confirmed")
	}
	if got := (Event{EventType: TypeReservationRejected}).Result(); got != "rejected" {
		t.Errorf("Result() = %q, want %q", got, "rejected")
	}
}
