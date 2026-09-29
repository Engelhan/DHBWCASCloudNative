// Flash-Sale-Lastpfad (P1: "Ein Lastpfad, den Sie kuenstlich erzeugen koennen"). Simuliert
// hunderte gleichzeitige Nutzer, die beim Vorverkaufsstart eines einzelnen Events Sitzplaetze
// reservieren wollen - siehe README ## Szenario "Flash Sale".
//
// setup() holt die Sitzplatzliste EINMAL vor dem Lasttest (nicht pro VU/Iteration) - waehrend
// des eigentlichen Tests soll fast ausschliesslich POST /events/{id}/reservations Last erzeugen,
// nicht GET-Anfragen zum Nachschauen, welche Plaetze noch frei sind (das entspricht auch eher
// dem realen Verhalten: die Sitzplan-Seite wurde schon vorher geladen, bevor der Ansturm losgeht).
//
// Ein fester "Hot Seat" (immer derselbe, seats[0]) wird bewusst ueberproportional oft gewaehlt,
// um echte Sitzplatzkonflikte zu erzeugen, die der reservation-worker als ReservationRejected
// beantworten muss (siehe README Punkt 3) - ohne das wuerden bei genug freien Sitzplaetzen kaum
// Konflikte zufaellig entstehen.

import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const API_URL = __ENV.API_URL || "http://localhost:8080";
// "all" verteilt die Last auf alle Events aus GET /events - dreimal so viele freie Sitzplaetze,
// bevor catalog-api nur noch per Vorabcheck mit 409 antwortet und nichts mehr bei Kafka ankommt.
const EVENT_ID = __ENV.EVENT_ID || "demo-konzert";
const VUS = parseInt(__ENV.VUS || "50", 10);
const DURATION = __ENV.DURATION || "1m";
// Anteil der Anfragen, die absichtlich denselben ("heissen") Sitzplatz treffen statt eines
// zufaelligen freien - siehe Datei-Kommentar oben.
const HOT_SEAT_RATIO = parseFloat(__ENV.HOT_SEAT_RATIO || "0.2");

export const options = {
  scenarios: {
    flash_sale: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "10s", target: VUS }, // Ramp-up: alle Nutzer kommen quasi gleichzeitig
        { duration: DURATION, target: VUS }, // Hauptlast
        { duration: "5s", target: 0 },
      ],
    },
  },
  thresholds: {
    // Nicht "muss alles klappen" - 409 (Sitzplatz-Konflikt) und sogar 5xx unter extremer Last
    // sind erwartete, gewollte Ergebnisse dieses Tests, siehe README. Nur harte Client-Fehler
    // (falsche URL, Verbindung abgelehnt) sollen den Testlauf als fehlgeschlagen markieren.
    http_req_failed: ["rate<1.0"],
  },
};

const reservationsAccepted = new Counter("reservations_accepted");
const reservationsConflicted = new Counter("reservations_conflicted");
const reservationsOtherError = new Counter("reservations_other_error");

export function setup() {
  let eventIds = [EVENT_ID];
  if (EVENT_ID === "all") {
    const res = http.get(`${API_URL}/events`);
    check(res, { "event list loaded": (r) => r.status === 200 });
    eventIds = res.json().map((e) => e.id);
  }

  const seats = [];
  for (const eventId of eventIds) {
    const res = http.get(`${API_URL}/events/${eventId}/seats`);
    check(res, { "seat list loaded": (r) => r.status === 200 });
    seats.push(...res.json().filter((s) => s.status === "free"));
  }
  if (seats.length === 0) {
    throw new Error(
      `Keine freien Sitzplaetze fuer Event "${EVENT_ID}" - Demo-Daten zuruecksetzen oder anderes EVENT_ID waehlen.`
    );
  }
  return { seats };
}

export default function (data) {
  const { seats } = data;
  const useHotSeat = Math.random() < HOT_SEAT_RATIO;
  const seat = useHotSeat ? seats[0] : seats[Math.floor(Math.random() * seats.length)];

  const vuId = `${__VU}-${__ITER}`;
  const payload = JSON.stringify({
    seat_id: seat.id,
    session_id: `loadtest-${vuId}`,
    first_name: "Last",
    last_name: `Test-${vuId}`,
    email: `loadtest-${vuId}@example.com`,
  });

  const res = http.post(`${API_URL}/events/${seat.event_id}/reservations`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  check(res, {
    "status ist 202 oder 409": (r) => r.status === 202 || r.status === 409,
  });

  if (res.status === 202) {
    reservationsAccepted.add(1);
  } else if (res.status === 409) {
    reservationsConflicted.add(1);
  } else {
    reservationsOtherError.add(1);
  }
}
