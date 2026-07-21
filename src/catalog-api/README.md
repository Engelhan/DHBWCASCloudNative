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

## Geplanter Stack

`TODO`: Sprache/Framework final festlegen (Kandidat: Go + `net/http`/`chi` oder Python +
FastAPI – Begründung folgt in der Root-README unter `## Entscheidungen`, sobald entschieden).

## Konfiguration (zur Laufzeit, kein Hardcoding im Image)

| Variable | Quelle | Bedeutung |
|---|---|---|
| `DATABASE_URL` | Secret | Connection-String zum CNPG-Cluster |
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server des Strimzi-Kafka-Clusters |
| `RESERVATION_TOPIC` | ConfigMap | Kafka-Topic für `ReservationRequested` |
| `LOG_LEVEL` | ConfigMap | z.B. `info` |

## Lokal starten

`TODO`, sobald Code existiert (`docker compose up` gegen lokale Postgres/Kafka-Instanzen).

## Image bauen

```bash
docker build -t ticketflow/catalog-api:<git-sha> src/catalog-api
```
