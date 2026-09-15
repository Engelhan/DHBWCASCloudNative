# catalog-api

Öffentliche REST-API von TicketFlow.

## Zweck

- Events und deren Sitzplatzverfügbarkeit anzeigen (liest aus PostgreSQL/CNPG).
- Reservierungsanfragen entgegennehmen und als `ReservationRequested`-Event auf Kafka
  veröffentlichen (schreibt **nicht** synchron in die Datenbank – das übernimmt
  [`reservation-worker`](../reservation-worker/)).
- `/metrics` (Prometheus-Format): u.a. `reservation_requests_total`,
  `reservation_request_duration_seconds`.
- `/healthz` (liveness) und `/readyz` (readiness: prüft DB- und Kafka-Erreichbarkeit).

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
| `GET` | `/readyz` | Readiness (prüft DB- und Kafka-Erreichbarkeit) |
| `GET` | `/metrics` | Prometheus-Metriken |

`POST /events/{eventID}/reservations` erwartet `{"seat_id", "session_id", "first_name",
"last_name", "email"}` und antwortet `202 Accepted` mit `{"reservation_id", "status": "pending"}`
– catalog-api schreibt dabei **nicht** synchron in Postgres, sondern published nur das Event (s.u.)
und merkt sich die Anfrage kurzzeitig im Speicher, um `GET /reservations/{id}` sofort mit
`status: "pending"` beantworten zu können, bevor der `reservation-worker` eine Zeile in
`reservations` geschrieben hat.

## Kafka-Event-Schema

**`ReservationRequested`** (von catalog-api published). Topic: `RESERVATION_TOPIC` (env, lokal
z.B. `reservation-requested`). Message-Key = `seat_id` (Ordering pro Sitzplatz). Value (JSON):

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

`TODO`: Feldnamen/-inhalt final mit dem `reservation-worker`-Owner abstimmen, sobald der Consumer
existiert.

**`ReservationConfirmed`/`ReservationRejected`** (vom `reservation-worker` published, Topic
`RESULT_TOPIC`/`reservation-result`) – Namen sind festgelegt, siehe
[`src/reservation-worker/README.md`](../reservation-worker/README.md). Payload-Schema folgt dort,
sobald der Service existiert.

`TODO`: mit dem `reservation-worker`-Owner abstimmen, bevor das Schema als endgültig gilt.

**Wire-Format seit der Apicurio-Integration:** der Message-`Value` ist nicht mehr reines JSON,
sondern Confluent-kompatibel gerahmt - 1 Magic-Byte (`0x0`) + 4-Byte-Big-Endian-`globalId` +
das obige JSON als Payload. Das kanonische Schema (JSON Schema, identisch zum obigen Contract)
liegt unter `charts/ticketflow/files/schemas/reservation-requested.schema.json` und wird bei
jedem Helm-Sync in Apicurio Registry (`charts/apicurio`) registriert; catalog-api validiert
lokal dagegen (embedded, siehe `internal/schemareg`) vor jedem Publish - kein Registry-Roundtrip
pro Nachricht. Siehe `charts/README.md` ## apicurio für Details/offene Punkte.

## Datenmodell (Postgres, siehe `local/postgres/init.sql`)

`events`, `seats` (`row_number`, `seat_number`, `category` — `premium`/`parkett`/`balkon`,
`price_cents`, Status `free`/`occupied`) werden von catalog-api **nur gelesen**. `reservations`
wird ausschließlich vom `reservation-worker` geschrieben (Status `pending`/`confirmed`/`rejected`);
catalog-api liest dort nur für `GET /reservations/{id}` – existiert (noch) keine Zeile, gilt die
Anfrage als `pending`.

## Konfiguration (zur Laufzeit, kein Hardcoding im Image)

| Variable | Quelle | Bedeutung |
|---|---|---|
| `DB_HOST` | ConfigMap | Hostname des CNPG-Postgres (z.B. `<cluster>-rw.<namespace>.svc`) |
| `DB_PORT` | ConfigMap | Postgres-Port, Default `5432` |
| `DB_NAME` | ConfigMap | Datenbankname |
| `DB_SSLMODE` | ConfigMap | z.B. `disable` (lokal) / `require` (Cluster) |
| `DB_USER` | Secret | Postgres-Benutzername |
| `DB_PASSWORD` | Secret | Postgres-Passwort |
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server des Strimzi-Kafka-Clusters (kommagetrennt) |
| `RESERVATION_TOPIC` | ConfigMap | Kafka-Topic für `ReservationRequested` |
| `REGISTRY_URL` | ConfigMap | Apicurio-Registry v3-REST-API-Root, z.B. `http://apicurio.kafka.svc:8080/apis/registry/v3` |
| `REGISTRY_GROUP_ID` | ConfigMap | Registry-Group, Default `ticketflow` |
| `LOG_LEVEL` | ConfigMap | z.B. `info` |
| `PORT` | ConfigMap | HTTP-Port, Default `8080` |
| `CORS_ALLOWED_ORIGIN` | ConfigMap | Origin des Frontends, Default `*` (nur für lokale Entwicklung sinnvoll) |

catalog-api baut den Postgres-Connection-String selbst aus diesen Einzelteilen zusammen (Team-
Entscheidung), statt ein fertiges `DATABASE_URL`-Secret zu erwarten – CNPG-Cluster (siehe
`charts/postgres-cluster`) liefern ohnehin nur `username`/`password` getrennt.

## Lokal starten

Über das repo-weite `docker-compose.yml` (siehe Root-README `## Betrieb`):

```bash
docker compose up postgres kafka catalog-api
```

Postgres/Kafka werden dabei aus `local/` vorbefüllt bzw. bereitgestellt – dieser Compose-Stack ist
rein lokales Entwickler-Tooling, kein Teil des Terraform/Ansible/ArgoCD/Helm-Deploy-Pfads.

## Image bauen

```bash
docker build -t ticketflow/catalog-api:<git-sha> src/catalog-api
```
