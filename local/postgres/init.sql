-- Lokales Dev-Seeding fuer docker-compose (lokal, nicht Teil des Terraform/Ansible/ArgoCD/Helm-Pfads).
-- Im Cluster legt spaeter charts/postgres-cluster/templates/postgres-cluster.yaml die DB an; das
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

-- Drei Veranstaltungen in drei Hallen, je 1000 Plaetze. Reihenfolge nach starts_at: catalog-api
-- liefert GET /events so sortiert und das Frontend zeigt aktuell events[0] - "demo-konzert"
-- bleibt dadurch das voreingestellte Event.
INSERT INTO events (id, name, venue, starts_at) VALUES
    ('demo-konzert',  'TicketFlow Demo-Konzert',  'Halle 1',          '2026-11-20T20:00:00+01:00'),
    ('rock-festival', 'TicketFlow Rock-Festival', 'Arena Nord',       '2026-12-05T19:30:00+01:00'),
    ('klassik-abend', 'TicketFlow Klassik-Abend', 'Konzerthaus Sued', '2027-01-17T19:00:00+01:00');

-- 1000 Plaetze je Halle: 40 Reihen a 25 Plaetze, drei Kategorien (Reihe 1-8 premium,
-- 9-24 parkett, 25-40 balkon), Preise je Veranstaltung leicht unterschiedlich.
--
-- ALLE Plaetze starten auf "free" - kein vorbelegter Sitz mehr. Der Flash-Sale-Lastpfad soll die
-- Halle selbst leerkaufen; vorbelegte Plaetze haetten nur den Sitzplan im Screencast gefuellt und
-- dabei die Ausgangslage jedes Lasttests verfaelscht.
INSERT INTO seats (id, event_id, row_number, seat_number, category, price_cents, status)
SELECT
    e.id || '-seat-' || r || '-' || s,
    e.id,
    r,
    s,
    CASE WHEN r <= 8 THEN 'premium' WHEN r <= 24 THEN 'parkett' ELSE 'balkon' END,
    CASE WHEN r <= 8 THEN e.premium_cents WHEN r <= 24 THEN e.parkett_cents ELSE e.balkon_cents END,
    'free'
FROM (VALUES
    ('demo-konzert',  7900, 5900, 3900),
    ('rock-festival', 9900, 6900, 4400),
    ('klassik-abend', 8900, 6400, 4200)
) AS e (id, premium_cents, parkett_cents, balkon_cents),
generate_series(1, 40) AS r,
generate_series(1, 25) AS s;
