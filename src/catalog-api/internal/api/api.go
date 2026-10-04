// Package api contains the catalog-api HTTP handlers.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"ticketflow/catalog-api/internal/db"
	"ticketflow/catalog-api/internal/metrics"
	"ticketflow/catalog-api/internal/reservation"
	"ticketflow/catalog-api/internal/schemareg"
)

// pendingEntry remembers a reservation until reservation-worker has written its row
// (in-memory only, lost on restart).
type pendingEntry struct {
	EventID   string
	SeatID    string
	CreatedAt time.Time
}

type API struct {
	DB         *db.DB
	Producer   *reservation.Producer
	Schema     *schemareg.Client
	CORSOrigin string

	pendingMu sync.Mutex
	pending   map[string]pendingEntry
}

func New(database *db.DB, producer *reservation.Producer, schema *schemareg.Client, corsOrigin string) *API {
	a := &API{
		DB:         database,
		Producer:   producer,
		Schema:     schema,
		CORSOrigin: corsOrigin,
		pending:    make(map[string]pendingEntry),
	}
	go a.evictStalePending()
	return a
}

func (a *API) evictStalePending() {
	for range time.Tick(time.Minute) {
		cutoff := time.Now().Add(-15 * time.Minute)
		a.pendingMu.Lock()
		for id, entry := range a.pending {
			if entry.CreatedAt.Before(cutoff) {
				delete(a.pending, id)
			}
		}
		a.pendingMu.Unlock()
	}
}

func (a *API) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(a.cors)

	r.Get("/healthz", a.handleHealthz)
	r.Get("/readyz", a.handleReadyz)

	r.Get("/events", a.handleListEvents)
	r.Get("/events/{eventID}/seats", a.handleListSeats)
	r.Post("/events/{eventID}/reservations", a.handleCreateReservation)
	r.Get("/reservations/{id}", a.handleGetReservation)

	return r
}

func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbErr := a.DB.Ping(ctx)
	kafkaErr := a.Producer.Ping(ctx)
	schemaErr := a.Schema.Ping(ctx)

	ready := dbErr == nil && kafkaErr == nil && schemaErr == nil
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"status": readyString(ready),
		"checks": map[string]bool{
			"database":       dbErr == nil,
			"kafka":          kafkaErr == nil,
			"schemaRegistry": schemaErr == nil,
		},
	})
}

func readyString(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
}

func (a *API) handleListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := a.DB.ListEvents(r.Context())
	if err != nil {
		slog.Error("list events failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error"))
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (a *API) handleListSeats(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")

	event, err := a.DB.GetEvent(r.Context(), eventID)
	if err != nil {
		slog.Error("get event failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error"))
		return
	}
	if event == nil {
		writeJSON(w, http.StatusNotFound, errorBody("event_not_found"))
		return
	}

	seats, err := a.DB.ListSeats(r.Context(), eventID)
	if err != nil {
		slog.Error("list seats failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error"))
		return
	}
	writeJSON(w, http.StatusOK, seats)
}

type createReservationRequest struct {
	SeatID    string `json:"seat_id"`
	SessionID string `json:"session_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type createReservationResponse struct {
	ReservationID string `json:"reservation_id"`
	Status        string `json:"status"`
}

func (a *API) handleCreateReservation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	eventID := chi.URLParam(r, "eventID")

	var req createReservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		metrics.ReservationRequestsTotal.WithLabelValues("error").Inc()
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_body"))
		return
	}
	if req.SeatID == "" || req.SessionID == "" || req.FirstName == "" || req.LastName == "" || req.Email == "" {
		metrics.ReservationRequestsTotal.WithLabelValues("error").Inc()
		writeJSON(w, http.StatusBadRequest, errorBody("missing_fields"))
		return
	}

	seat, err := a.DB.GetSeat(r.Context(), eventID, req.SeatID)
	if err != nil {
		slog.Error("get seat failed", "error", err)
		metrics.ReservationRequestsTotal.WithLabelValues("error").Inc()
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error"))
		return
	}
	if seat == nil {
		metrics.ReservationRequestsTotal.WithLabelValues("error").Inc()
		writeJSON(w, http.StatusNotFound, errorBody("seat_not_found"))
		return
	}
	// Best-effort UX check only; the race-safe seat lock happens in reservation-worker.
	if seat.Status != "free" {
		metrics.ReservationRequestsTotal.WithLabelValues("seat_unavailable").Inc()
		writeJSON(w, http.StatusConflict, errorBody("seat_unavailable"))
		return
	}

	reservationID := uuid.NewString()
	event := reservation.Requested{
		ReservationID: reservationID,
		EventID:       eventID,
		SeatID:        req.SeatID,
		SessionID:     req.SessionID,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Email:         req.Email,
		RequestedAt:   time.Now().UTC(),
	}

	if err := a.Producer.PublishReservationRequested(r.Context(), event); err != nil {
		slog.Error("publish reservation requested failed", "error", err)
		metrics.ReservationRequestsTotal.WithLabelValues("error").Inc()
		writeJSON(w, http.StatusServiceUnavailable, errorBody("kafka_unavailable"))
		return
	}

	a.pendingMu.Lock()
	a.pending[reservationID] = pendingEntry{EventID: eventID, SeatID: req.SeatID, CreatedAt: time.Now()}
	a.pendingMu.Unlock()

	metrics.ReservationRequestsTotal.WithLabelValues("accepted").Inc()
	metrics.ReservationRequestDuration.WithLabelValues("accepted").Observe(time.Since(start).Seconds())

	writeJSON(w, http.StatusAccepted, createReservationResponse{ReservationID: reservationID, Status: "pending"})
}

type reservationStatusResponse struct {
	ID      string `json:"id"`
	EventID string `json:"event_id,omitempty"`
	SeatID  string `json:"seat_id,omitempty"`
	Status  string `json:"status"`
}

func (a *API) handleGetReservation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	res, err := a.DB.GetReservation(r.Context(), id)
	if err != nil {
		slog.Error("get reservation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("internal_error"))
		return
	}
	if res != nil {
		writeJSON(w, http.StatusOK, reservationStatusResponse{
			ID: res.ID, EventID: res.EventID, SeatID: res.SeatID, Status: res.Status,
		})
		return
	}

	// No row yet: pending if we still know the request, otherwise unknown.
	a.pendingMu.Lock()
	entry, ok := a.pending[id]
	a.pendingMu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody("reservation_not_found"))
		return
	}
	writeJSON(w, http.StatusOK, reservationStatusResponse{
		ID: id, EventID: entry.EventID, SeatID: entry.SeatID, Status: "pending",
	})
}

func errorBody(code string) map[string]string {
	return map[string]string{"error": code}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}
