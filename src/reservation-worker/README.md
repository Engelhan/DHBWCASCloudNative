# reservation-worker

Kafka-Consumer, der die eigentliche Buchungslogik ausführt. Dies ist der Service, der unter Last
(P6) über KEDA skaliert wird.

## Zweck

- Konsumiert `ReservationRequested` aus Kafka (Consumer-Group `reservation-worker`).
- Sperrt den angefragten Sitzplatz transaktional in PostgreSQL/CNPG (Optimistic Locking über
  die `status`-Spalte je Sitzplatz: `UPDATE seats SET status='occupied' WHERE ... AND
  status='free'` – betrifft bei parallelen Zugriffen auf denselben Sitzplatz garantiert nur eine
  Zeile, das verhindert Overselling ohne `SELECT ... FOR UPDATE`).
- Veröffentlicht das Ergebnis als `ReservationConfirmed` oder `ReservationRejected` auf Kafka.
- Baut sein Datenbankschema (`events`/`seats`/`reservations`) beim Start selbst über versionierte,
  eingebettete SQL-Migrationen auf (`internal/migrate`, `schema_migrations`-Tabelle) – siehe
  ## Datenbankschema.
- `/metrics` (Prometheus-Format): `reservations_processed_total{result="confirmed|rejected"}`,
  `reservation_processing_duration_seconds{result}`,
  `reservation_processing_errors_total{stage="unmarshal|db|publish"}`. Consumer-Lag für KEDA
  (P6) wird nicht von diesem Service selbst exportiert, sondern von KEDAs eigenem
  `kafka`-Scaler direkt gegen den Kafka-Cluster ermittelt (siehe
  [`charts/ticketflow/`](../../charts/ticketflow/)).
- `/healthz` (liveness) und `/readyz` (readiness: prüft DB- und Kafka-Erreichbarkeit, analog zu
  [`catalog-api`](../catalog-api/)).

## Warum ein eigener Service statt Logik in `catalog-api`

Die Sitzplatz-Sperrung ist der eigentliche Engpass unter Last (DB-Transaktionen, Konfliktbehandlung).
Als eigener Consumer lässt er sich unabhängig von der API horizontal skalieren – die API bleibt
leichtgewichtig und reagiert auch dann noch schnell, wenn die Verarbeitung hinterherhängt.

## Stack

Go + [`pgx/v5`](https://github.com/jackc/pgx) (DB-Zugriff/Transaktionen) +
[`segmentio/kafka-go`](https://github.com/segmentio/kafka-go) (Consumer + Producer) +
`prometheus/client_golang` – gleicher Stack wie [`catalog-api`](../catalog-api/)/
[`notification-worker`](../notification-worker/) (siehe Root-README `## Entscheidungen` P1),
keine zusätzliche Migrations-Bibliothek: `internal/migrate` ist ein bewusst minimaler,
selbstgeschriebener Runner (siehe ## Datenbankschema) statt einer weiteren Abhängigkeit.

## Kafka-Event-Schemas

**Eingang** (`RESERVATION_TOPIC`, Key = `seat_id`) – exakt das von `catalog-api` bereits
veröffentlichte Schema, siehe [`src/catalog-api/README.md`
## Kafka-Event-Schema](../catalog-api/README.md#kafka-event-schema-entwurf-noch-nicht-final-mit-carlosreservation-worker-abgestimmt):

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

**Ausgang** (`RESULT_TOPIC`, Key = `reservation_id`) – exakt das von `notification-worker`
bereits konsumierte Schema, siehe [`src/notification-worker/internal/event/event.go`](../notification-worker/internal/event/event.go)
(camelCase, abweichend vom Eingangsschema oben – zwei unabhängig entstandene Konventionen, hier
bewusst 1:1 übernommen statt vereinheitlicht, um nicht in bereits gemergten Code einzugreifen):

```json
{
  "eventType": "ReservationConfirmed",
  "reservationId": "uuid",
  "eventOccurredAt": "2026-08-09T12:00:01Z",
  "customerEmail": "max@example.com",
  "seatId": "demo-konzert-seat-7",
  "reason": "SEAT_UNAVAILABLE"
}
```

`reason` ist nur bei `ReservationRejected` gesetzt (aktuell immer `SEAT_UNAVAILABLE` – deckt
sowohl "bereits belegt" als auch "unbekannter Sitzplatz" ab, siehe `internal/booking`).
`eventName` (im Zielschema vorgesehen, `omitempty`) wird bewusst nicht befüllt: das
Eingangsereignis trägt nur `event_id`, kein lesbarer Name, und ein zusätzlicher DB-Lookup dafür
stand nicht im Scope dieser ersten Implementierung.

## Datenbankschema

`internal/migrate` wendet beim Start eingebettete SQL-Dateien aus `internal/migrate/sql/`
gegen `DATABASE_URL` an (Reihenfolge nach Dateiname, je Datei eine Transaktion, Fortschritt in
einer `schema_migrations`-Tabelle) – das ist die "DB Schema aufbauen und über Applikation
versionieren"-Aufgabe. `0001_init.sql` legt `events`/`seats`/`reservations` an (identisches
Schema zu [`local/postgres/init.sql`](../../local/postgres/init.sql), dem reinen
Dev-Seed-Skript für docker-compose) – `CREATE TABLE IF NOT EXISTS`, damit die Migration sowohl
gegen eine leere CNPG-Datenbank (echter Cluster-Pfad über
[`charts/postgres-cluster/`](../../charts/postgres-cluster/)) als auch gegen die von `init.sql`
bereits vorbefüllte lokale Dev-DB idempotent ist. `reservation-worker` schreibt ausschließlich
in `reservations`; `events`/`seats` werden nur gelesen (Sitzplatz-Claim) bzw. von `catalog-api`
gelesen. Die Demo-Seed-Daten (`demo-konzert` + Sitzplan) kommen weiterhin nur aus
`local/postgres/init.sql` – die reale Cluster-DB hat nach den Migrationen leere `events`/
`seats`-Tabellen; das Befüllen für den Screencast ist noch offen.

## Konfiguration

| Variable | Quelle | Bedeutung |
|---|---|---|
| `DATABASE_URL` | Secret | Connection-String zum CNPG-Cluster |
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server des Strimzi-Kafka-Clusters (kommagetrennt) |
| `RESERVATION_TOPIC` | ConfigMap | Kafka-Eingangstopic für `ReservationRequested` |
| `RESULT_TOPIC` | ConfigMap | Kafka-Ausgangstopic für `ReservationConfirmed`/`ReservationRejected` |
| `CONSUMER_GROUP` | ConfigMap | Kafka-Consumer-Group-ID (Grundlage für KEDA-Lag-Metrik) |
| `PROCESS_MAX_RETRIES` | ConfigMap | Anzahl Verarbeitungsversuche (DB-Transaktion + Publish) vor Aufgabe, Default `3` |
| `PROCESS_RETRY_BACKOFF` | ConfigMap | Wartezeit zwischen Versuchen, Default `2s` |
| `LOG_LEVEL` | ConfigMap | z.B. `info` |
| `HTTP_ADDR` | ConfigMap | Adresse für `/metrics`, `/healthz`, `/readyz`, Default `:8080` |

Offen und **nicht** Teil dieses Charts (siehe [`charts/README.md`](../../charts/README.md)
## `reservation-worker`): wie `DATABASE_URL` zur Deploy-Zeit konkret befüllt wird –
`charts/postgres-cluster` legt aktuell nur ein `username`/`password`-Secret im Namespace
`application` an, kein fertiges `DATABASE_URL`, und dieser Service liegt (wie
`charts/ticketflow`) im Namespace `ticketflow`. Cross-Namespace-Secret-Zugriff ist in
Kubernetes nicht direkt möglich; bis das geklärt ist, bleibt `DATABASE_URL` ein manuell zu
befüllender Secret-Platzhalter (siehe `charts/reservation-worker/values.yaml`).

## Lokal starten

Über das repo-weite `docker-compose.yml` (siehe Root-README `## Betrieb`):

```bash
docker compose up postgres kafka reservation-worker
```

## Image bauen

```bash
docker build -t ticketflow/reservation-worker:<git-sha> src/reservation-worker
```
