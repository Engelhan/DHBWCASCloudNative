# catalog-api

Öffentliche REST-API von TicketFlow.

## Zweck

- Events und deren Sitzplatzverfügbarkeit anzeigen (liest aus PostgreSQL/CNPG).
- Reservierungsanfragen entgegennehmen und als `ReservationRequested`-Event auf Kafka
  veröffentlichen (schreibt **nicht** synchron in die Datenbank – das übernimmt
  [`reservation-worker`](../reservation-worker/)).
- `/metrics` (Prometheus-Format): u.a. `reservation_requests_total`,
  `reservation_request_duration_seconds`.
- `/healthz` (liveness) und `/readyz` (readiness: prüft DB, Kafka und Schema-Registry).

## Stack

Go + [chi](https://github.com/go-chi/chi) (schlanker Router, kein Web-Framework) – Begründung
(Konsistenz mit `reservation-worker`, kleines statisches Binary/Image, wenig
Runtime-Abhängigkeiten) siehe Root-README `## Entscheidungen` P1. DB-Zugriff über `pgx/v5`
(read-only, siehe unten), Kafka-Producer über `segmentio/kafka-go`, Metriken über
`prometheus/client_golang`.

## REST-API

| Methode | Pfad | Zweck |
|---|---|---|
| `GET` | `/events` | Liste aller Events |
| `GET` | `/events/{eventID}/seats` | Sitzplätze + Status (`free`/`occupied`) eines Events |
| `POST` | `/events/{eventID}/reservations` | Reservierungsanfrage entgegennehmen, `ReservationRequested` publishen |
| `GET` | `/reservations/{id}` | Status einer Reservierung (`pending`/`confirmed`/`rejected`) |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness (prüft DB, Kafka und Schema-Registry) |
| `GET` | `/metrics` | Prometheus-Metriken |

`POST /events/{eventID}/reservations` erwartet `{"seat_id", "session_id", "first_name",
"last_name", "email"}` und antwortet `202 Accepted` mit `{"reservation_id", "status": "pending"}`
– catalog-api schreibt dabei **nicht** synchron in Postgres, sondern published nur das Event (s.u.)
und merkt sich die Anfrage kurzzeitig im Speicher, um `GET /reservations/{id}` sofort mit
`status: "pending"` beantworten zu können, bevor der `reservation-worker` eine Zeile in
`reservations` geschrieben hat.

## Kafka-Event-Schema

**`ReservationRequested`** (von catalog-api published). Topic: `RESERVATION_TOPIC`
(`reservation-requested`). Message-Key = `seat_id` (Ordering pro Sitzplatz). Value (JSON):

```json
{
  "reservation_id": "uuid",
  "event_id": "demo-konzert",
  "seat_id": "demo-konzert-seat-7",
  "session_id": "uuid (Frontend, pro Browser)",
  "first_name": "Max",
  "last_name": "Mustermann",
  "email": "max@example.com",
  "requested_at": "2026-08-09T12:00:00Z"
}
```

`reservation-worker` konsumiert genau dieses Format (`internal/event.Requested`, gleiche JSON-Keys).

**`ReservationConfirmed`/`ReservationRejected`** (vom `reservation-worker` published, Topic
`RESULT_TOPIC`/`reservation-result`) – Schema siehe
[`src/reservation-worker/README.md`](../reservation-worker/README.md).

**Wire-Format seit der Apicurio-Integration:** der Message-`Value` ist nicht mehr reines JSON,
sondern Confluent-kompatibel gerahmt - 1 Magic-Byte (`0x0`) + 4-Byte-Big-Endian-`globalId` +
das obige JSON als Payload. Das kanonische Schema (JSON Schema, identisch zum obigen Contract)
liegt unter `charts/kafka-cluster/files/schemas/reservation-requested.schema.json` und wird bei
jedem Helm-Sync in Apicurio Registry (`charts/apicurio`) registriert; catalog-api validiert
lokal dagegen (embedded, siehe `internal/schemareg`) vor jedem Publish - kein Registry-Roundtrip
pro Nachricht. Siehe `charts/README.md` ## apicurio für Details/offene Punkte.

## Datenmodell (Postgres, Schema im `reservation-worker`)

`events`, `seats` (`row_number`, `seat_number`, `category` — `premium`/`parkett`/`balkon`,
`price_cents`, Status `free`/`occupied`) werden von catalog-api **nur gelesen**, und zwar über
den Read-Only-Service von CNPG (`<db>-postgres-cluster-ro`, also die Replica) statt über den
Primary. Schema und Demo-Stammdaten legt allein der `reservation-worker` über seine versionierten
Migrationen an ([`0001_init.sql`](../reservation-worker/internal/migrate/sql/0001_init.sql),
[`0002_seed_demo_events.sql`](../reservation-worker/internal/migrate/sql/0002_seed_demo_events.sql)):
drei Veranstaltungen in drei Hallen mit je 1000 Plätzen (40 Reihen à 25), alle auf `free`. Bis der
Worker einmal gelaufen ist, gibt es die Tabellen noch nicht, und `GET /events` antwortet mit einem
Fehler. Weil die Replica asynchron repliziert, kann eine gerade gebuchte Reservierung einen
Moment lang noch als `free` bzw. `pending` erscheinen. `reservations`
wird ausschließlich vom `reservation-worker` geschrieben (Status `pending`/`confirmed`/`rejected`);
catalog-api liest dort nur für `GET /reservations/{id}` – existiert (noch) keine Zeile, gilt die
Anfrage als `pending`.

## Konfiguration (zur Laufzeit, kein Hardcoding im Image)

| Variable | Quelle | Bedeutung |
|---|---|---|
| `DB_HOST` | ConfigMap | Hostname des CNPG-Postgres, im Cluster der Read-Only-Service `<cluster>-ro` |
| `DB_PORT` | ConfigMap | Postgres-Port, Default `5432` |
| `DB_NAME` | ConfigMap | Datenbankname |
| `DB_SSLMODE` | ConfigMap | im Cluster `require` |
| `DB_USER` | Secret | Postgres-Benutzername |
| `DB_PASSWORD` | Secret | Postgres-Passwort |
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server des Strimzi-Kafka-Clusters (kommagetrennt) |
| `RESERVATION_TOPIC` | ConfigMap | Kafka-Topic für `ReservationRequested` |
| `KAFKA_SASL_ENABLED` | ConfigMap | SCRAM-SHA-512-Login am Kafka-Listener (im Cluster `true`, B2) |
| `KAFKA_SASL_USERNAME` | ConfigMap | Name der `KafkaUser`-CR |
| `KAFKA_SASL_PASSWORD` | Secret | aus dem von Strimzi erzeugten Secret der `KafkaUser` |
| `REGISTRY_URL` | ConfigMap | Apicurio-Registry v3-REST-API-Root, z.B. `http://apicurio.kafka.svc:8080/apis/registry/v3` |
| `REGISTRY_GROUP_ID` | ConfigMap | Registry-Group, Default `ticketflow` |
| `LOG_LEVEL` | ConfigMap | z.B. `info` |
| `PORT` | ConfigMap | HTTP-Port, Default `8080` |
| `CORS_ALLOWED_ORIGIN` | ConfigMap | Default `*`; nur für Aufrufe außerhalb des Same-Origin-Ingress relevant (z. B. load-generator) |

catalog-api baut den Postgres-Connection-String selbst aus diesen Einzelteilen zusammen (Team-
Entscheidung), statt ein fertiges `DATABASE_URL`-Secret zu erwarten. `DB_USER`/`DB_PASSWORD`
kommen direkt aus dem von CNPG erzeugten Secret `<databaseName>-postgres-cluster-app`.

## Image bauen

```bash
docker build -t ticketflow/catalog-api:<git-sha> src/catalog-api
```
