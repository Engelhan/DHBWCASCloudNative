// Flash-sale load path (P1): many users reserving seats at sale start. setup() loads the seats
// once, so the test mostly hits POST /reservations. A fixed hot seat is chosen often on purpose
// to produce real conflicts (ReservationRejected).

import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const API_URL = __ENV.API_URL || "http://localhost:8080";
// "all" spreads load over every event (3x the free seats).
const EVENT_ID = __ENV.EVENT_ID || "demo-konzert";
const VUS = parseInt(__ENV.VUS || "50", 10);
const DURATION = __ENV.DURATION || "1m";
// Share of requests aimed at the hot seat.
const HOT_SEAT_RATIO = parseFloat(__ENV.HOT_SEAT_RATIO || "0.2");

export const options = {
  scenarios: {
    flash_sale: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "10s", target: VUS },  // ramp-up: users arrive almost at once
        { duration: DURATION, target: VUS },  // main load
        { duration: "5s", target: 0 },
      ],
    },
  },
  thresholds: {
    // 409 and even 5xx under extreme load are expected; only hard client errors fail the run.
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
      `No free seats for event "${EVENT_ID}" - reset the demo data or choose another EVENT_ID.`
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
    "status is 202 or 409": (r) => r.status === 202 || r.status === 409,
  });

  if (res.status === 202) {
    reservationsAccepted.add(1);
  } else if (res.status === 409) {
    reservationsConflicted.add(1);
  } else {
    reservationsOtherError.add(1);
  }
}
