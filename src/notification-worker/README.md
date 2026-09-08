# notification-worker

Zweiter Kafka-Consumer, unabhängige Consumer-Group. Konsumiert `ReservationConfirmed`/
`ReservationRejected` und benachrichtigt den Käufer – standardmäßig per Log-Ausgabe (siehe
`## Grenzen` in der Root-README), optional per echtem SMTP-Mailversand, wenn konfiguriert.
Erster implementierter Service im Repo (Go) – siehe `../../CLAUDE.md` für den Gesamtkontext.

## Zweck

- Zeigt, dass Kafka hier tatsächlich als Message *Bus* (mehrere unabhängige Konsumenten desselben
  Ereignisstroms) gebraucht wird und nicht nur als Point-to-Point-Queue zwischen genau zwei
  Services – das rechtfertigt Strimzi gegenüber einer einfacheren Queue-Tabelle in Postgres.
- `/metrics`: `notifications_sent_total{result="confirmed|rejected"}`,
  `notification_dispatch_errors_total{channel="smtp|unmarshal"}`,
  `notification_dlt_published_total`.
- `/healthz`, `/readyz` (Readiness spiegelt Kafka-Erreichbarkeit, nicht ob je eine Nachricht
  ankam – es kann legitim lange keinen Traffic geben, solange `reservation-worker` noch nicht
  existiert).

## Event-Contract (`RESULT_TOPIC`)

`reservation-worker` (noch nicht implementiert) muss dieses JSON auf `RESULT_TOPIC`
veröffentlichen:

```json
{
  "eventType": "ReservationConfirmed",
  "reservationId": "6c1f6e8e-2b3a-4c1d-9e0a-6a2b1c3d4e5f",
  "eventOccurredAt": "2026-08-05T18:32:10Z",
  "customerEmail": "buyer@example.com",
  "eventName": "Rock Night Vol. 3",
  "seatId": "A-12",
  "reason": null
}
```

`eventType`: `"ReservationConfirmed"` \| `"ReservationRejected"`. `customerEmail` ist nur
Pflicht, wenn SMTP aktiv ist (Log-Only-Pfad funktioniert auch ohne). `reason` nur bei
`ReservationRejected` relevant (z.B. `"SOLD_OUT"`).

## Konfiguration

| Variable | Quelle | Default | Bedeutung |
|---|---|---|---|
| `KAFKA_BROKERS` | ConfigMap | – (Pflicht) | Bootstrap-Server, kommasepariert |
| `RESULT_TOPIC` | ConfigMap | – (Pflicht) | Eingangstopic (`ReservationConfirmed`/`Rejected`) |
| `CONSUMER_GROUP` | ConfigMap | – (Pflicht) | eigene, von `reservation-worker` unabhängige Group-ID |
| `DLT_TOPIC` | ConfigMap | leer | Dead-Letter-Topic für Nachrichten, die alle Zustellversuche ausgeschöpft haben (leer = kein DLT-Publish) |
| `NOTIFY_MAX_RETRIES` | ConfigMap | `3` | Zustellversuche pro Nachricht, bevor sie auf dem DLT landet |
| `NOTIFY_RETRY_BACKOFF` | ConfigMap | `2s` | Pause zwischen Versuchen (Go-Duration-String) |
| `LOG_LEVEL` | ConfigMap | `info` | – |
| `HTTP_ADDR` | ConfigMap | `:8080` | – |
| `KAFKA_TLS_ENABLED` | ConfigMap | `false` | aktiviert mTLS zum Kafka-Broker |
| `KAFKA_TLS_CA_CERT` | **Secret** | leer | PEM, Pflicht wenn `KAFKA_TLS_ENABLED=true` |
| `KAFKA_TLS_CLIENT_CERT` | **Secret** | leer | PEM, Pflicht wenn `KAFKA_TLS_ENABLED=true` |
| `KAFKA_TLS_CLIENT_KEY` | **Secret** | leer | PEM, Pflicht wenn `KAFKA_TLS_ENABLED=true` |
| `SMTP_ENABLED` | ConfigMap | `false` | schaltet echten Mailversand frei; sonst reine Log-Ausgabe |
| `SMTP_HOST` | ConfigMap | leer | Pflicht wenn `SMTP_ENABLED=true` |
| `SMTP_PORT` | ConfigMap | `587` | – |
| `SMTP_FROM` | ConfigMap | leer | Pflicht wenn `SMTP_ENABLED=true` |
| `SMTP_TLS_MODE` | ConfigMap | `starttls` | `none`\|`starttls`\|`tls` |
| `SMTP_USERNAME` | **Secret** | leer | leer = unauthenticated SMTP |
| `SMTP_PASSWORD` | **Secret** | leer | – |

Ungültige Kombinationen (z.B. `SMTP_ENABLED=true` ohne Host/From, oder `SMTP_TLS_MODE=none` mit
gesetztem Username) lassen den Prozess beim Start hart fehlschlagen (sichtbares
CrashLoopBackOff), statt später still Notifications zu verlieren.

## Zustell-Semantik

Der Kafka-Offset wird **nur bei tatsächlich erfolgreichem Versand** committet. Schlagen alle
`NOTIFY_MAX_RETRIES` Versuche fehl, wird die Nachricht (falls `DLT_TOPIC` gesetzt) mit
Fehler-Metadaten auf das Dead-Letter-Topic publiziert, aber **nicht** committet – sie wird bei
einem Neustart/Rebalance erneut zugestellt, bis sie entweder erfolgreich versendet wird oder
wieder auf dem DLT landet. Bewusstes at-least-once-Verhalten: nichts wird still verworfen, aber
doppelte DLT-Einträge über Zeit sind möglich. Fehlerhafte/nicht parsebare Nachrichten (Poison
Pills) werden dagegen sofort committet (nach best-effort DLT-Publish), da ein erneuter Versuch
sie nie erfolgreich machen würde.

## Lokal starten

```bash
cd src/notification-worker
export KAFKA_BROKERS=localhost:9092
export RESULT_TOPIC=reservation-results
export CONSUMER_GROUP=notification-worker-local
go run .
```

SMTP ist standardmäßig aus (`SMTP_ENABLED=false`), es wird also keine Mail-Infrastruktur zum
lokalen Testen benötigt – Notifications erscheinen als strukturierte Log-Zeilen auf stdout.

## Image bauen

```bash
docker build -t ticketflow/notification-worker:<git-sha> src/notification-worker
```
