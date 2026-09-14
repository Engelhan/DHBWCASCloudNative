package db

import "context"

// Migrate legt Schema + Demo-Event beim Programmstart an (idempotent, IF NOT EXISTS/ON CONFLICT
// DO NOTHING). Im echten CNPG-Cluster gibt es kein docker-entrypoint-initdb.d-Aequivalent (das
// funktioniert nur beim lokalen postgres-Image, siehe local/postgres/init.sql) - ein frisch
// bereitgestellter CNPG-Cluster hat also nur eine leere Datenbank, keine Tabellen.
//
// Das ist bewusst kein Widerspruch zu "catalog-api schreibt nie in reservations": diese Migration
// legt nur Struktur + Demo-Stammdaten beim Start an, nie eine Reservierungszeile zur Laufzeit -
// reservations bleibt dauerhaft nur vom (noch nicht implementierten) reservation-worker
// beschrieben.
func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id        TEXT PRIMARY KEY,
			name      TEXT NOT NULL,
			venue     TEXT NOT NULL,
			starts_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS seats (
			id          TEXT PRIMARY KEY,
			event_id    TEXT NOT NULL REFERENCES events (id),
			row_number  INT NOT NULL,
			seat_number INT NOT NULL,
			category    TEXT NOT NULL CHECK (category IN ('premium', 'parkett', 'balkon')),
			price_cents INT NOT NULL,
			status      TEXT NOT NULL DEFAULT 'free' CHECK (status IN ('free', 'occupied')),
			UNIQUE (event_id, row_number, seat_number)
		);

		-- Wird ausschliesslich vom (noch nicht implementierten) reservation-worker beschrieben.
		CREATE TABLE IF NOT EXISTS reservations (
			id         UUID PRIMARY KEY,
			event_id   TEXT NOT NULL REFERENCES events (id),
			seat_id    TEXT NOT NULL REFERENCES seats (id),
			session_id TEXT NOT NULL,
			first_name TEXT NOT NULL,
			last_name  TEXT NOT NULL,
			email      TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'confirmed', 'rejected')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		INSERT INTO events (id, name, venue, starts_at) VALUES
			('demo-konzert', 'TicketFlow Demo-Konzert', 'Halle 1', '2026-11-20T20:00:00+01:00')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO seats (id, event_id, row_number, seat_number, category, price_cents, status)
		SELECT
			'demo-konzert-seat-' || r || '-' || s,
			'demo-konzert',
			r,
			s,
			CASE WHEN r = 1 THEN 'premium' WHEN r IN (2, 3) THEN 'parkett' ELSE 'balkon' END,
			CASE WHEN r = 1 THEN 7900 WHEN r IN (2, 3) THEN 5900 ELSE 3900 END,
			CASE WHEN (r, s) IN ((1, 3), (2, 5), (3, 1), (4, 2), (5, 6)) THEN 'occupied' ELSE 'free' END
		FROM generate_series(1, 5) AS r, generate_series(1, 6) AS s
		ON CONFLICT (event_id, row_number, seat_number) DO NOTHING;
	`)
	return err
}
