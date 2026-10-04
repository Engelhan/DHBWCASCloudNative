-- Demo data (formerly seeded by catalog-api, which now only reads from a replica).
-- Three events with 1000 seats each. GET /events sorts by starts_at and the frontend shows
-- events[0], so demo-konzert stays the default.
INSERT INTO events (id, name, venue, starts_at) VALUES
    ('demo-konzert',  'TicketFlow Demo-Konzert',  'Halle 1',          '2026-11-20T20:00:00+01:00'),
    ('rock-festival', 'TicketFlow Rock-Festival', 'Arena Nord',       '2026-12-05T19:30:00+01:00'),
    ('klassik-abend', 'TicketFlow Klassik-Abend', 'Konzerthaus Sued', '2027-01-17T19:00:00+01:00')
ON CONFLICT (id) DO NOTHING;

-- 40 rows x 25 seats: rows 1-8 premium, 9-24 parkett, 25-40 balkon. All seats start free so
-- every load test starts from the same state.
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
generate_series(1, 25) AS s
ON CONFLICT (event_id, row_number, seat_number) DO NOTHING;
