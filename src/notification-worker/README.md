# notification-worker

Zweiter Kafka-Consumer, unabhängige Consumer-Group. Konsumiert `ReservationConfirmed`/
`ReservationRejected` und simuliert die Benachrichtigung des Käufers (Log-Ausgabe statt echtem
E-Mail/SMS-Versand, siehe `## Grenzen` in der Root-README).

## Zweck

- Zeigt, dass Kafka hier tatsächlich als Message *Bus* (mehrere unabhängige Konsumenten desselben
  Ereignisstroms) gebraucht wird und nicht nur als Point-to-Point-Queue zwischen genau zwei
  Services – das rechtfertigt Strimzi gegenüber einer einfacheren Queue-Tabelle in Postgres.
- `/metrics`: `notifications_sent_total{result="confirmed|rejected"}`.
- `/healthz`, `/readyz`.

## Konfiguration

| Variable | Quelle | Bedeutung |
|---|---|---|
| `KAFKA_BROKERS` | ConfigMap | Bootstrap-Server |
| `RESULT_TOPIC` | ConfigMap | Eingangstopic (`ReservationConfirmed`/`Rejected`) |
| `CONSUMER_GROUP` | ConfigMap | eigene, von `reservation-worker` unabhängige Group-ID |

## Image bauen

```bash
docker build -t ticketflow/notification-worker:<git-sha> src/notification-worker
```
