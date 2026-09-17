# frontend

Minimales Web-UI für Demo/Screencast: Veranstaltungsauswahl, Sitzplatzauswahl, Reservierungs-Button,
Live-Status ("in Bearbeitung" → "bestätigt"/"abgelehnt"). Kein Pflichtbestandteil der
Aufgabenstellung (P1 verlangt zwei *Backend*-Services), aber hilfreich, um die Pflichtszenen des
Screencasts (Last erzeugen, Pod löschen) an einer sichtbaren Oberfläche statt nur im Terminal zu
zeigen.

## Zweck

- Spricht ausschließlich mit [`catalog-api`](../catalog-api/) über HTTP – keine eigene
  Business-Logik, kein direkter DB-/Kafka-Zugriff.
- Wird als statisches Build in einem eigenen, schlanken Container (z.B. `nginx` oder
  `caddy`) ausgeliefert.

## Stack

Reines HTML/CSS/JS (`index.html`, `style.css`, `app.js`), kein Build-Step, kein Framework – deckt
den kompletten Screencast-Bedarf (Veranstaltungsauswahl, Sitzplan, Buchungsformular, Live-Status)
ohne zusätzliche Komplexität ab. `API_BASE_URL` wird in einer eigenen `config.js` gesetzt (kein Build-Step für
Env-Injection vorhanden) – lokal per fest eingebautem Default (`http://localhost:8080`), im
Cluster überschreibt ein ConfigMap-Mount (`charts/ticketflow-app/templates/frontend/configmap.yaml`)
diese Datei mit einem leeren String: das Ingress routet `/events` und `/reservations` bereits auf
derselben Origin an `catalog-api` weiter (siehe `templates/frontend/ingress.yaml`), ein leerer
`API_BASE_URL` macht die `fetch()`-Aufrufe damit zu relativen Same-Origin-Requests – kein CORS,
kein hartkodierter Cluster-Hostname im Image.

## Ablauf

1. `GET /events` – lädt **alle** Veranstaltungen (aktuell drei, siehe
   `src/catalog-api/internal/db/migrate.go`) und füllt damit die Auswahlliste im Kopfbereich.
   Vorausgewählt ist die zuletzt gewählte Veranstaltung aus `localStorage` – sofern die API sie
   noch kennt, sonst `events[0]`, also die zeitlich nächste (`ORDER BY starts_at`). Bei nur einer
   Veranstaltung blendet sich die Auswahl aus. Danach `GET /events/{id}/seats` – zeigt den
   Sitzplan als Reihen unterhalb einer „Bühne“, gruppiert nach Kategorie (`premium`/`parkett`/
   `balkon`, je mit eigenem Preis aus `price_cents`), plus einer Verfügbarkeits-Zusammenfassung
   ("x von 1000 Plätzen frei"). Ein Wechsel der Veranstaltung lädt den Sitzplan neu und setzt
   Auswahl und Statuskarte zurück.
2. Klick auf freien Sitzplatz → nativer `<dialog>` mit Sitzplatz/Kategorie/Preis-Zusammenfassung
   und Buchungsformular (Vorname/Nachname/E-Mail). Eine Session-ID (`crypto.randomUUID()`) wird
   pro Browser in `localStorage` erzeugt und bei jeder Buchung mitgeschickt, um Kunden zu
   unterscheiden.
3. `POST /events/{id}/reservations` → Statuskarte „in Bearbeitung“ (mit Spinner), danach Polling
   von `GET /reservations/{id}` alle 2s bis `confirmed` („bestätigt“, ✓) oder `rejected`
   („ausgebucht“, ✕) – oder Timeout nach 60s (z.B. wenn `reservation-worker` lokal nicht läuft).
   Danach wird der Sitzplan neu geladen, aber nur, wenn noch dieselbe Veranstaltung ausgewählt
   ist wie beim Absenden.

## Darstellung von 1000 Plätzen

Eine Halle hat 40 Reihen à 25 Plätze. Zwei Dinge, die daran hängen und beim Ändern des Sitzplans
zu beachten sind:

- **Ein einziger Klick-Listener** liegt per Delegation auf `#hall`; die Sitz-ID steht in
  `data-seat-id`. 1000 einzelne `addEventListener` wären hier der naheliegende, aber falsche Weg.
  Der Sitzplan wird in einem `DocumentFragment` aufgebaut und genau einmal eingehängt.
- **Eine volle Reihe ist breiter als die Textspalte.** Sitzmaße und Spaltenbreiten liegen als
  CSS-Variablen (`--seat-w`, `--seat-gap`, `--aisle-w`, `--row-label-w`, `--content-max`) am
  `:root` in `style.css`; `.hall-scroll` scrollt horizontal, sobald eine Reihe nicht mehr passt,
  damit nie die *Seite* seitlich scrollt. Die Reihenbeschriftung ist `position: sticky` und
  bleibt dabei stehen. Sie nennt bewusst nur Reihe und Preis – der Kategoriename wiederholt sich
  über 40 Reihen und sprengte die Spalte; ihn tragen der farbige Punkt, die Legende und der
  `title`.

## Lokal starten

Über das repo-weite `docker-compose.yml` (siehe Root-README `## Betrieb`):

```bash
docker compose up catalog-api frontend
```

Frontend läuft dann auf `http://localhost:8081`, catalog-api auf `http://localhost:8080` (in
`index.html` als `API_BASE_URL` hinterlegt).

## Image bauen

```bash
docker build -t ticketflow/frontend:<git-sha> src/frontend
```
