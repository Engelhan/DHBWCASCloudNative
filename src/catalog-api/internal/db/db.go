// Package db provides catalog-api's read-only Postgres access (reservations are written by
// reservation-worker).
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Venue    string    `json:"venue"`
	StartsAt time.Time `json:"starts_at"`
}

type Seat struct {
	ID         string `json:"id"`
	EventID    string `json:"event_id"`
	RowNumber  int    `json:"row_number"`
	SeatNumber int    `json:"seat_number"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Status     string `json:"status"`
}

type Reservation struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	SeatID    string    `json:"seat_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DB struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() {
	d.pool.Close()
}

func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

func (d *DB) ListEvents(ctx context.Context) ([]Event, error) {
	rows, err := d.pool.Query(ctx, `SELECT id, name, venue, starts_at FROM events ORDER BY starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Name, &e.Venue, &e.StartsAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (d *DB) GetEvent(ctx context.Context, eventID string) (*Event, error) {
	var e Event
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, venue, starts_at FROM events WHERE id = $1`, eventID,
	).Scan(&e.ID, &e.Name, &e.Venue, &e.StartsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (d *DB) ListSeats(ctx context.Context, eventID string) ([]Seat, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, event_id, row_number, seat_number, category, price_cents, status
		 FROM seats WHERE event_id = $1 ORDER BY row_number, seat_number`, eventID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seats []Seat
	for rows.Next() {
		var s Seat
		if err := rows.Scan(&s.ID, &s.EventID, &s.RowNumber, &s.SeatNumber, &s.Category, &s.PriceCents, &s.Status); err != nil {
			return nil, err
		}
		seats = append(seats, s)
	}
	return seats, rows.Err()
}

func (d *DB) GetSeat(ctx context.Context, eventID, seatID string) (*Seat, error) {
	var s Seat
	err := d.pool.QueryRow(ctx,
		`SELECT id, event_id, row_number, seat_number, category, price_cents, status
		 FROM seats WHERE event_id = $1 AND id = $2`, eventID, seatID,
	).Scan(&s.ID, &s.EventID, &s.RowNumber, &s.SeatNumber, &s.Category, &s.PriceCents, &s.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetReservation returns nil while reservation-worker has not written the row yet.
func (d *DB) GetReservation(ctx context.Context, id string) (*Reservation, error) {
	var r Reservation
	err := d.pool.QueryRow(ctx,
		`SELECT id, event_id, seat_id, status, created_at, updated_at FROM reservations WHERE id = $1`, id,
	).Scan(&r.ID, &r.EventID, &r.SeatID, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
