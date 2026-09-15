package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResultMarshal_RoundTrip(t *testing.T) {
	reason := "SEAT_UNAVAILABLE"
	original := Result{
		EventType:       TypeReservationRejected,
		ReservationID:   "6c1f6e8e-2b3a-4c1d-9e0a-6a2b1c3d4e5f",
		EventOccurredAt: time.Date(2026, 8, 9, 12, 0, 1, 0, time.UTC),
		CustomerEmail:   "max@example.com",
		SeatID:          "demo-konzert-seat-7",
		Reason:          &reason,
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var got Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if got.EventType != original.EventType || got.ReservationID != original.ReservationID ||
		got.CustomerEmail != original.CustomerEmail || *got.Reason != *original.Reason {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, original)
	}
}

func TestResultMarshal_JSONKeys(t *testing.T) {
	original := Result{
		EventType:     TypeReservationConfirmed,
		ReservationID: "r1",
		CustomerEmail: "max@example.com",
		SeatID:        "s1",
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	// These are the exact keys notification-worker's internal/event.Event
	// expects (camelCase) -- a mismatch here would silently break the
	// downstream consumer.
	for _, key := range []string{"eventType", "reservationId", "eventOccurredAt", "customerEmail", "seatId"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("Marshal() output missing expected key %q: %s", key, data)
		}
	}
	if _, ok := raw["reason"]; ok {
		t.Errorf("Marshal() with nil Reason: expected omitted \"reason\" key, got present: %s", data)
	}
}

func TestResult_Result(t *testing.T) {
	if got := (Result{EventType: TypeReservationConfirmed}).Result(); got != "confirmed" {
		t.Errorf("Result() = %q, want %q", got, "confirmed")
	}
	if got := (Result{EventType: TypeReservationRejected}).Result(); got != "rejected" {
		t.Errorf("Result() = %q, want %q", got, "rejected")
	}
}
