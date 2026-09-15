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

func newResolvedClient(t *testing.T, globalID int64, schemaJSON []byte) *Client {
	t.Helper()
	srv := testServer(t, globalID)
	c, err := New(Config{BaseURL: srv.URL, GroupID: "ticketflow", ArtifactID: "reservation-requested-value"}, schemaJSON)
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

func TestWrapUnwrapRoundTrip(t *testing.T) {
	c := newResolvedClient(t, 42, RequestedSchemaJSON)

	payload := []byte(`{
		"reservation_id": "r1", "event_id": "e1", "seat_id": "s1", "session_id": "sess1",
		"first_name": "Max", "last_name": "Mustermann", "email": "max@example.com",
		"requested_at": "2026-09-15T12:00:00Z"
	}`)

	framed, err := c.Wrap(payload)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if len(framed) != frameHeaderSz+len(payload) {
		t.Fatalf("framed length = %d, want %d", len(framed), frameHeaderSz+len(payload))
	}
	if framed[0] != magicByte {
		t.Fatalf("magic byte = 0x%x, want 0x%x", framed[0], magicByte)
	}

	out, err := c.Unwrap(framed)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if string(out) != string(payload) {
		t.Fatalf("Unwrap payload = %s, want %s", out, payload)
	}
}

func TestWrapRejectsInvalidPayload(t *testing.T) {
	c := newResolvedClient(t, 1, RequestedSchemaJSON)

	// missing every required field
	_, err := c.Wrap([]byte(`{}`))
	if err == nil {
		t.Fatal("Wrap: expected validation error, got nil")
	}
}

func TestUnwrapRejectsInvalidPayload(t *testing.T) {
	c := newResolvedClient(t, 1, RequestedSchemaJSON)

	framed := append([]byte{magicByte, 0, 0, 0, 1}, []byte(`{}`)...)
	_, err := c.Unwrap(framed)
	if err == nil {
		t.Fatal("Unwrap: expected validation error, got nil")
	}
}

func TestUnwrapRejectsBadMagicByte(t *testing.T) {
	c := newResolvedClient(t, 1, RequestedSchemaJSON)

	framed := append([]byte{0xFF, 0, 0, 0, 1}, []byte(`{}`)...)
	_, err := c.Unwrap(framed)
	if err == nil {
		t.Fatal("Unwrap: expected magic byte error, got nil")
	}
}

func TestUnwrapRejectsShortMessage(t *testing.T) {
	c := newResolvedClient(t, 1, RequestedSchemaJSON)

	if _, err := c.Unwrap([]byte{magicByte, 0, 0}); err == nil {
		t.Fatal("Unwrap: expected too-short error, got nil")
	}
}

func TestWrapWithoutResolveFails(t *testing.T) {
	c, err := New(Config{BaseURL: "http://unused.invalid", GroupID: "g", ArtifactID: "a"}, RequestedSchemaJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	payload := []byte(`{
		"reservation_id": "r1", "event_id": "e1", "seat_id": "s1", "session_id": "sess1",
		"first_name": "Max", "last_name": "Mustermann", "email": "max@example.com",
		"requested_at": "2026-09-15T12:00:00Z"
	}`)
	if _, err := c.Wrap(payload); err == nil {
		t.Fatal("Wrap: expected error when globalId not resolved, got nil")
	}
}
