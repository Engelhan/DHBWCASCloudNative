-- Schema fuer die TicketFlow-Kernentitaeten. Muss mit dem lokalen Dev-Seed
-- local/postgres/init.sql (docker-compose) uebereinstimmen, das denselben
-- Stand direkt per Postgres-Init-Script anlegt statt ueber diese Migration.
-- IF NOT EXISTS deshalb bewusst: macht diese Migration sicher idempotent
-- sowohl gegen eine leere CNPG-Datenbank (echter Cluster-Pfad, siehe
-- charts/postgres-cluster) als auch gegen eine von init.sql bereits
-- vorbefuellte lokale DB.
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

-- status startet nie bei 'pending' hier: reservation-worker schreibt diese
-- Zeile erst, nachdem es das Ergebnis (confirmed/rejected) kennt. "pending"
-- wird nur von catalog-api synthetisiert, solange noch keine Zeile existiert
-- (siehe src/catalog-api/internal/api ## handleGetReservation).
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

CREATE INDEX IF NOT EXISTS idx_reservations_event_id ON reservations (event_id);
