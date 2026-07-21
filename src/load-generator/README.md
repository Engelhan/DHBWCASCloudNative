# load-generator

Erzeugt den künstlichen Lastpfad, der laut Aufgabenstellung reproduzierbar auslösbar sein muss
(Abschnitt 3) und die beiden Pflichtszenen des Screencasts bedient (Last erzeugen → Replicas
wachsen → Wirkung im Grafana-Dashboard).

## Szenario "Flash Sale"

Simuliert hunderte gleichzeitige Nutzer, die beim Vorverkaufsstart eines einzelnen, knapp
kontingentierten Events Sitzplätze reservieren wollen:

1. Ramp-up auf `VUS` virtuelle Nutzer innerhalb weniger Sekunden.
2. Jeder Nutzer wählt einen zufälligen freien Sitzplatz des Ziel-Events und ruft
   `POST /events/{id}/reservations` auf `catalog-api` auf.
3. Ein Teil der Anfragen läuft absichtlich auf denselben Sitzplatz, um Konfliktbehandlung
   (`ReservationRejected`) sichtbar zu machen.

## Werkzeug

`TODO`: [k6](https://k6.io/) als Kandidat (leichtgewichtig, gut skriptbar, Prometheus-Remote-Write
möglich, damit Lastmetriken neben App-Metriken im selben Dashboard erscheinen). Alternative:
Locust – Begründung folgt in der Root-README, sobald final entschieden.

## Verwendung (geplant)

```bash
k6 run flash-sale.js \
  --env API_URL=https://ticketflow.<cluster-domain> \
  --env EVENT_ID=demo-event \
  --env VUS=500 \
  --env DURATION=3m
```
