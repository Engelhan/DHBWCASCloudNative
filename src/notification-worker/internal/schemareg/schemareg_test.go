package schemareg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func testServer(t *testing.T, globalID int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"globalId": ` + strconv.FormatInt(globalID, 10) + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newResolvedClient(t *testing.T, globalID int64) *Client {
	t.Helper()
	srv := testServer(t, globalID)
	c, err := New(Config{BaseURL: srv.URL, GroupID: "ticketflow", ArtifactID: "reservation-result-value"}, ResultSchemaJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Resolve(ctx); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return c
}

func validResultPayload() []byte {
	return []byte(`{
		"eventType": "ReservationConfirmed",
		"reservationId": "r1",
		"eventOccurredAt": "2026-09-15T12:00:00Z",
		"customerEmail": "max@example.com"
	}`)
}

func TestUnwrapAcceptsValidFramedPayload(t *testing.T) {
	c := newResolvedClient(t, 7)

	payload := validResultPayload()
	framed := append([]byte{magicByte, 0, 0, 0, 7}, payload...)

	out, err := c.Unwrap(framed)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if string(out) != string(payload) {
		t.Fatalf("Unwrap payload = %s, want %s", out, payload)
	}
}

func TestUnwrapRejectsInvalidPayload(t *testing.T) {
	c := newResolvedClient(t, 1)

	framed := append([]byte{magicByte, 0, 0, 0, 1}, []byte(`{}`)...)
	if _, err := c.Unwrap(framed); err == nil {
		t.Fatal("Unwrap: expected validation error, got nil")
	}
}

func TestUnwrapRejectsBadMagicByte(t *testing.T) {
	c := newResolvedClient(t, 1)

	framed := append([]byte{0xFF, 0, 0, 0, 1}, validResultPayload()...)
	if _, err := c.Unwrap(framed); err == nil {
		t.Fatal("Unwrap: expected magic byte error, got nil")
	}
}

func TestUnwrapRejectsShortMessage(t *testing.T) {
	c := newResolvedClient(t, 1)

	if _, err := c.Unwrap([]byte{magicByte, 0, 0}); err == nil {
		t.Fatal("Unwrap: expected too-short error, got nil")
	}
}

func TestUnwrapToleratesGlobalIDMismatch(t *testing.T) {
	c := newResolvedClient(t, 7)

	payload := validResultPayload()
	// Different globalId than resolved: still valid, only logged.
	framed := append([]byte{magicByte, 0, 0, 0, 9}, payload...)

	if _, err := c.Unwrap(framed); err != nil {
		t.Fatalf("Unwrap: expected no error on globalId mismatch, got %v", err)
	}
}
