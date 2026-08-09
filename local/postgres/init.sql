-- Lokales Dev-Seeding fuer docker-compose (lokal, nicht Teil des Terraform/Ansible/ArgoCD/Helm-Pfads).
-- Im Cluster legt spaeter charts/ticketflow/templates/database/cnpg-cluster.yaml die DB an; das
-- Schema hier ist der aktuelle Entwurf, den catalog-api (lesend) und reservation-worker
-- (schreibend, noch nicht implementiert) gegen echtes Postgres/Kafka lokal ausprobieren koennen.

CREATE TABLE events (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    venue     TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE seats (
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
-- catalog-api liest hier nur: existiert noch keine Zeile zu einer reservation_id, gilt sie als
-- "pending" (Anfrage wurde publiziert, aber noch nicht verarbeitet).
CREATE TABLE reservations (
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
    ('demo-konzert', 'TicketFlow Demo-Konzert', 'Halle 1', '2026-11-20T20:00:00+01:00');

-- 5 Reihen a 6 Plaetze, 3 Kategorien (Reihe 1 = premium, 2-3 = parkett, 4-5 = balkon), ein paar
-- Plaetze quer durch alle Kategorien schon "belegt", damit der Sitzplan im Screencast nicht leer
-- wirkt.
INSERT INTO seats (id, event_id, row_number, seat_number, category, price_cents, status)
SELECT
    'demo-konzert-seat-' || r || '-' || s,
    'demo-konzert',
    r,
    s,
    CASE WHEN r = 1 THEN 'premium' WHEN r IN (2, 3) THEN 'parkett' ELSE 'balkon' END,
    CASE WHEN r = 1 THEN 7900 WHEN r IN (2, 3) THEN 5900 ELSE 3900 END,
    CASE WHEN (r, s) IN ((1, 3), (2, 5), (3, 1), (4, 2), (5, 6)) THEN 'occupied' ELSE 'free' END
FROM generate_series(1, 5) AS r, generate_series(1, 6) AS s;
