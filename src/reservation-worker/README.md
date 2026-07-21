# reservation-worker

Kafka-Consumer, der die eigentliche Buchungslogik ausführt. Dies ist der Service, der unter Last
(P6) über KEDA skaliert wird.

## Zweck

- Konsumiert `ReservationRequested` aus Kafka (Consumer-Group `reservation-worker`).
- Sperrt den angefragten Sitzplatz transaktional in PostgreSQL/CNPG (Optimistic Locking über
  eine Versions-/Status-Spalte je Sitzplatz – verhindert Overselling bei parallelen Workern).
- Veröffentlicht das Ergebnis als `ReservationConfirmed` oder `ReservationRejected` auf Kafka.
- `/metrics`: u.a. `reservations_processed_total{result="confirmed|rejected"}`,
  `reservation_processing_duration_seconds`, sowie der vom Kafka-Client exportierte
  Consumer-Lag (Grundlage für KEDA, siehe [`charts/ticketflow/`](../../charts/ticketflow/)).
- `/healthz`, `/readyz`.

## Warum ein eigener Service statt Logik in `catalog-api`

Die Sitzplatz-Sperrung ist der eigentliche Engpass unter Last (DB-Transaktionen, Konfliktbehandlung).
Als eigener Consumer lässt er sich unabhängig von der API horizontal skalieren – die API bleibt
leichtgewichtig und reagiert auch dann noch schnell, wenn die Verarbeitung hinterherhängt.

## Geplanter Stack

`TODO`: gleiche Sprache wie `catalog-api` zur Reduktion der Toolchain-Vielfalt (Kandidat: Go).

## Konfiguration

| Variable | Quelle | Bedeutung |
|---|---|---|
| `DATABASE_URL` | Secret | Connection-String zum CNPG-Cluster |
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server |
| `RESERVATION_TOPIC` | ConfigMap | Eingangstopic |
| `RESULT_TOPIC` | ConfigMap | Ausgangstopic für Confirmed/Rejected |
| `CONSUMER_GROUP` | ConfigMap | Kafka-Consumer-Group-ID (Grundlage für KEDA-Lag-Metrik) |

## Image bauen

```bash
docker build -t ticketflow/reservation-worker:<git-sha> src/reservation-worker
```
