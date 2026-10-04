// Package booking implements the race-safe seat claim.
package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticketflow/reservation-worker/internal/event"
)

const reasonSeatUnavailable = "SEAT_UNAVAILABLE"

// Process claims the seat via optimistic compare-and-swap (UPDATE ... WHERE status = 'free'),
// so only one concurrent claim can win. It is idempotent on ReservationID: a redelivered message
// re-derives the existing result instead of claiming again.
func Process(ctx context.Context, pool *pgxpool.Pool, req event.Requested) (event.Result, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return event.Result{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op once Commit has succeeded

	var existingStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM reservations WHERE id = $1`, req.ReservationID).Scan(&existingStatus)
	switch {
	case err == nil:
		return resultFor(req, existingStatus), nil
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return event.Result{}, fmt.Errorf("check existing reservation: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE seats SET status = 'occupied' WHERE id = $1 AND event_id = $2 AND status = 'free'`,
		req.SeatID, req.EventID)
	if err != nil {
		return event.Result{}, fmt.Errorf("claim seat: %w", err)
	}

	status := "rejected"
	if tag.RowsAffected() == 1 {
		status = "confirmed"
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO reservations (id, event_id, seat_id, session_id, first_name, last_name, email, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		req.ReservationID, req.EventID, req.SeatID, req.SessionID, req.FirstName, req.LastName, req.Email, status, now,
	); err != nil {
		return event.Result{}, fmt.Errorf("insert reservation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return event.Result{}, fmt.Errorf("commit transaction: %w", err)
	}

	return resultFor(req, status), nil
}

func resultFor(req event.Requested, status string) event.Result {
	result := event.Result{
		ReservationID:   req.ReservationID,
		EventOccurredAt: time.Now().UTC(),
		CustomerEmail:   req.Email,
		SeatID:          req.SeatID,
	}

	if status == "confirmed" {
		result.EventType = event.TypeReservationConfirmed
		return result
	}

	result.EventType = event.TypeReservationRejected
	reason := reasonSeatUnavailable
	result.Reason = &reason
	return result
}
