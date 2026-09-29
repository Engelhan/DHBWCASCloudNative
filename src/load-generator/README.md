# load-generator

Erzeugt den künstlichen Lastpfad, der laut Aufgabenstellung reproduzierbar auslösbar sein muss
(Abschnitt 3) und die beiden Pflichtszenen des Screencasts bedient (Last erzeugen → Replicas
wachsen → Wirkung im Grafana-Dashboard).

## Szenario "Flash Sale"

Simuliert viele gleichzeitige Nutzer, die beim Vorverkaufsstart eines einzelnen Events
Sitzplätze reservieren wollen (`flash-sale.js`):

1. `setup()` lädt einmalig die freien Sitzplätze des Ziel-Events (nicht pro Anfrage – die
   eigentliche Last soll `POST /events/{id}/reservations` treffen, nicht das Nachschauen).
2. Ramp-up auf `VUS` virtuelle Nutzer innerhalb von 10s, hält `DURATION`, rampt in 5s ab
   (`options.scenarios.flash_sale`, Executor `ramping-vus`).
3. Jeder Nutzer wählt einen Sitzplatz und ruft `POST /events/{id}/reservations` auf
   `catalog-api` auf. Ein fester Anteil (`HOT_SEAT_RATIO`, Default 20%) trifft absichtlich
   immer denselben Sitzplatz, um echte Sitzplatzkonflikte (`ReservationRejected` im
   `reservation-worker`) zu erzeugen – ohne das entstehen bei genug freien Plätzen kaum
   zufällige Konflikte.
4. `202`/`409` gelten beide als erwartetes Ergebnis (kein Testfehler) – nur andere Statuscodes
   zählen als `reservations_other_error`.

## Werkzeug

[k6](https://k6.io/) – leichtgewichtig, gut skriptbar (JavaScript), kein zusätzlicher Server
nötig (anders als Locust, das einen eigenen Master/Worker-Prozess braucht). Installation:
`brew install k6` (macOS) bzw. siehe [k6-Docs](https://k6.io/docs/get-started/installation/).

## Verwendung

```bash
cd src/load-generator
k6 run flash-sale.js \
  --env API_URL=https://ticketflow.<dns-zone> \
  --env EVENT_ID=demo-konzert \
  --env VUS=500 \
  --env DURATION=3m
```

Lokal gegen den docker-compose-Stack (siehe Root-README `## Betrieb`):

```bash
docker compose up -d
k6 run flash-sale.js --env API_URL=http://localhost:8080 --env EVENT_ID=demo-konzert --env VUS=50 --env DURATION=30s
```

Ohne lokale k6-Installation, im offiziellen `grafana/k6`-Container (nur Docker nötig; gleiche
Parameter als Umgebungsvariablen, weitere Argumente gehen an `k6 run`):

```bash
./run.sh                                   # Volllast gegen den Cluster
VUS=2000 DURATION=5m ./run.sh              # noch mehr Last
API_URL=http://localhost:8080 EVENT_ID=demo-konzert VUS=50 DURATION=30s ./run.sh   # docker-compose
```

Ohne `API_URL` zielt das Skript auf `https://ticketflow.<dnsZone>` (Zone aus
`gitops/env/cluster.yaml`, derselbe Host wie der Frontend-Ingress) und ist auf maximale Last
eingestellt: `EVENT_ID=all` (Last verteilt auf alle Events aus `GET /events`, also dreimal so viele
freie Sitzplätze, bevor `catalog-api` nur noch per Vorabcheck mit `409` antwortet und nichts mehr
in Kafka landet), 1000 VUs, 3 Minuten. Unter Linux läuft der Container im Host-Netz (der Ingress
ist nur über IPv6/die DHBW-Route des Hosts erreichbar), unter Docker Desktop (macOS/Windows) wird
`localhost` durch `host.docker.internal` ersetzt.

Ein Lauf verbraucht die Sitzplätze dauerhaft - ein zweiter Lauf findet nur noch die übrig
gebliebenen freien Plätze.

**Lokal beobachtet:** bei einem Testlauf mit 30 VUs/15s liefen 878 Requests durch (694×`202`,
183×`409` durch den Hot-Seat-Mechanismus, 0 unerwartete Fehler) – `reservation-worker`
verarbeitete den entstandenen Kafka-Backlog anschließend sichtbar zeitversetzt ab (lokal nur
eine Worker-Instanz, kein KEDA) – im echten Cluster ist genau dieser Consumer-Lag das Signal,
auf das KEDA mit zusätzlichen Replicas reagiert (siehe `charts/keda`, P6).

## Ergebnisse einordnen

k6 zählt `202`/`409` als Erfolg (siehe `options.thresholds`), da ein abgelehnter Sitzplatz ein
korrektes, erwartetes Ergebnis des Flash-Sale-Szenarios ist, kein Fehler des Systems. Die
eigentliche Kapazitätsaussage (wie viele Reservierungen/Sekunde `reservation-worker` verarbeiten
kann, wie schnell KEDA hochskaliert) liest man aus den Prometheus-Metriken der Services selbst
(`reservation_requests_total`, `reservations_processed_total`), nicht aus den k6-Zahlen –
`custom.reservations_accepted`/`reservations_conflicted` in der k6-Ausgabe zeigen nur, was
`catalog-api` unmittelbar beim Annehmen der Anfrage zurückgegeben hat.
