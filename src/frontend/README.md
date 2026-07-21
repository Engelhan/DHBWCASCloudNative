# frontend

Minimales Web-UI für Demo/Screencast: Event-Liste, Sitzplatzauswahl, Reservierungs-Button,
Live-Status ("in Bearbeitung" → "bestätigt"/"abgelehnt"). Kein Pflichtbestandteil der
Aufgabenstellung (P1 verlangt zwei *Backend*-Services), aber hilfreich, um die Pflichtszenen des
Screencasts (Last erzeugen, Pod löschen) an einer sichtbaren Oberfläche statt nur im Terminal zu
zeigen.

## Zweck

- Spricht ausschließlich mit [`catalog-api`](../catalog-api/) über HTTP – keine eigene
  Business-Logik, kein direkter DB-/Kafka-Zugriff.
- Wird als statisches Build in einem eigenen, schlanken Container (z.B. `nginx` oder
  `caddy`) ausgeliefert.

## Geplanter Stack

`TODO`: einfaches SPA-Framework oder sogar reines HTML/JS, um den Umfang klein zu halten – die
Bewertung honoriert keine zusätzliche Komplexität, die aus der Domäne nicht folgt.

## Image bauen

```bash
docker build -t ticketflow/frontend:<git-sha> src/frontend
```
