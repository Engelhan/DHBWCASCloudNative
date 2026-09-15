// Package db manages reservation-worker's connection to CNPG-Postgres.
package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// Pool exposes the underlying pool for internal/migrate and internal/booking,
// which need transactional access beyond simple typed queries.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}
