# src/ – Anwendungscode

Selbst geschriebene Services von TicketFlow (P1). Jeder Service hat ein eigenes `Dockerfile` und
eine eigene README mit Sprache/Framework, lokalem Start und den Umgebungsvariablen, die er
erwartet (diese werden im Cluster über ConfigMap/Secret injiziert, siehe [`charts/ticketflow/`](../charts/ticketflow/)).

| Service | Rolle | Kommuniziert mit |
|---|---|---|
| [`catalog-api`](catalog-api/) | REST-API: Events/Sitzplätze anzeigen, Reservierungsanfragen annehmen | Kafka (Producer), PostgreSQL (lesend) |
| [`reservation-worker`](reservation-worker/) | Konsumiert Reservierungsanfragen, sperrt Sitzplätze transaktional, KEDA-skaliert | Kafka (Consumer+Producer), PostgreSQL (schreibend) |
| [`notification-worker`](notification-worker/) | Konsumiert Reservierungsergebnisse, simuliert Benachrichtigung | Kafka (Consumer) |
| [`frontend`](frontend/) | Minimales Web-UI für Screencast/Demo | `catalog-api` (HTTP) |
| [`load-generator`](load-generator/) | Erzeugt den künstlichen Lastpfad (Flash Sale) | `catalog-api` (HTTP) |

Alle Container-Images werden reproduzierbar aus diesem Ordner gebaut (CI: `.github/workflows/`)
und mit dem Git-Short-SHA getaggt (kein `:latest`, siehe P3).
