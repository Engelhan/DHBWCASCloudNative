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

## Stack

Reines HTML/CSS/JS (`index.html`, `style.css`, `app.js`), kein Build-Step, kein Framework – deckt
den kompletten Screencast-Bedarf (Sitzplan, Buchungsformular, Live-Status) ohne zusätzliche
Komplexität ab. `API_BASE_URL` wird in einer eigenen `config.js` gesetzt (kein Build-Step für
Env-Injection vorhanden) – lokal per fest eingebautem Default (`http://localhost:8080`), im
Cluster überschreibt ein ConfigMap-Mount (`charts/ticketflow-app/templates/frontend/configmap.yaml`)
diese Datei mit einem leeren String: das Ingress routet `/events` und `/reservations` bereits auf
derselben Origin an `catalog-api` weiter (siehe `templates/frontend/ingress.yaml`), ein leerer
`API_BASE_URL` macht die `fetch()`-Aufrufe damit zu relativen Same-Origin-Requests – kein CORS,
kein hartkodierter Cluster-Hostname im Image.

## Ablauf

1. `GET /events` – lädt das (einzige) Demo-Event, dann `GET /events/{id}/seats` – zeigt den
   Sitzplan als Reihen unterhalb einer „Bühne“, gruppiert nach Kategorie (`premium`/`parkett`/
   `balkon`, je mit eigenem Preis aus `price_cents`), plus einer Verfügbarkeits-Zusammenfassung
   ("x von 30 Plätzen frei").
2. Klick auf freien Sitzplatz → nativer `<dialog>` mit Sitzplatz/Kategorie/Preis-Zusammenfassung
   und Buchungsformular (Vorname/Nachname/E-Mail). Eine Session-ID (`crypto.randomUUID()`) wird
   pro Browser in `localStorage` erzeugt und bei jeder Buchung mitgeschickt, um Kunden zu
   unterscheiden.
3. `POST /events/{id}/reservations` → Statuskarte „in Bearbeitung“ (mit Spinner), danach Polling
   von `GET /reservations/{id}` alle 2s bis `confirmed` („bestätigt“, ✓) oder `rejected`
   („ausgebucht“, ✕) – oder Timeout nach 60s (z.B. wenn `reservation-worker` lokal nicht läuft).

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
