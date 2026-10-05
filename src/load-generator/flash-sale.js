// Flash-sale load path (P1): many users reserving seats at sale start. setup() loads the seats
// once, so the test mostly hits POST /reservations. A fixed hot seat is chosen often on purpose
// to produce real conflicts (ReservationRejected).

import http from "k6/http";
import { check } from "k6";
import { Counter, Trend } from "k6/metrics";

const API_URL = __ENV.API_URL || "http://localhost:8080";
// "all" spreads load over every event (3x the free seats).
const EVENT_ID = __ENV.EVENT_ID || "demo-konzert";
const VUS = parseInt(__ENV.VUS || "50", 10);
const DURATION = __ENV.DURATION || "1m";
// Share of requests aimed at the hot seat.
const HOT_SEAT_RATIO = parseFloat(__ENV.HOT_SEAT_RATIO || "0.2");
// Where handleSummary writes the full JSON summary (run.sh mounts ./results there).
const SUMMARY_DIR = __ENV.SUMMARY_DIR || ".";

// 202 and 409 are expected outcomes, so http_req_failed only counts real errors (5xx, timeouts).
http.setResponseCallback(http.expectedStatuses(200, 202, 409));

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
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
  thresholds: {
    // 409 and even 5xx under extreme load are expected; only hard client errors fail the run.
    http_req_failed: ["rate<1.0"],
  },
};

const reservationsAccepted = new Counter("reservations_accepted");
const reservationsConflicted = new Counter("reservations_conflicted");
const reservationsOtherError = new Counter("reservations_other_error");
// Own trends so setup()'s seat-list GETs don't skew the reservation latency.
const reservationDuration = new Trend("reservation_duration", true);
const reservationDurationAccepted = new Trend("reservation_duration_accepted", true);
const reservationDurationConflicted = new Trend("reservation_duration_conflicted", true);
const reservationWaiting = new Trend("reservation_ttfb", true);

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

  reservationDuration.add(res.timings.duration);
  reservationWaiting.add(res.timings.waiting);
  if (res.status === 202) {
    reservationsAccepted.add(1);
    reservationDurationAccepted.add(res.timings.duration);
  } else if (res.status === 409) {
    reservationsConflicted.add(1);
    reservationDurationConflicted.add(res.timings.duration);
  } else {
    reservationsOtherError.add(1);
  }
}

// Compact load-test report on stdout plus the full k6 summary as JSON.
export function handleSummary(data) {
  const m = data.metrics;
  const count = (name) => (m[name] ? m[name].values.count : 0);
  const durationS = data.state.testRunDurationMs / 1000;

  const accepted = count("reservations_accepted");
  const conflicted = count("reservations_conflicted");
  const other = count("reservations_other_error");
  const total = accepted + conflicted + other;
  const pct = (n) => (total ? ((100 * n) / total).toFixed(1) : "0.0") + " %";

  const fmtMs = (v) =>
    v === undefined ? "-" : v < 1000 ? `${v.toFixed(1)} ms` : `${(v / 1000).toFixed(2)} s`;
  const stats = ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"];
  const pad = (v, w) => String(v).padStart(w);
  const latencyRow = (label, name) =>
    label.padEnd(22) +
    stats.map((s) => pad(m[name] ? fmtMs(m[name].values[s]) : "-", 10)).join("");

  const lines = [
    "",
    "=== Flash-sale load test ===",
    `Target           ${API_URL} (event ${EVENT_ID})`,
    `Duration         ${durationS.toFixed(1)} s, max ${m.vus_max ? m.vus_max.values.max : "-"} VUs`,
    "",
    "Throughput",
    `  Reservations   ${total} (${(total / durationS).toFixed(1)} req/s avg)`,
    `  HTTP requests  ${count("http_reqs")} incl. setup`,
    `  Data           ${(m.data_received.values.count / 1e6).toFixed(1)} MB in, ` +
      `${(m.data_sent.values.count / 1e6).toFixed(1)} MB out`,
    "",
    "Results",
    `  202 accepted   ${pad(accepted, 8)}  ${pad(pct(accepted), 8)}`,
    `  409 conflict   ${pad(conflicted, 8)}  ${pad(pct(conflicted), 8)}`,
    `  other / error  ${pad(other, 8)}  ${pad(pct(other), 8)}`,
    `  Error rate     ${((m.http_req_failed ? m.http_req_failed.values.rate : 0) * 100).toFixed(2)} % ` +
      "(5xx, timeouts, unexpected status)",
    "",
    "Latency".padEnd(22) + stats.map((s) => pad(s, 10)).join(""),
    latencyRow("  reservation (all)", "reservation_duration"),
    latencyRow("  202 accepted", "reservation_duration_accepted"),
    latencyRow("  409 conflict", "reservation_duration_conflicted"),
    latencyRow("  TTFB (server time)", "reservation_ttfb"),
    latencyRow("  connect", "http_req_connecting"),
    latencyRow("  TLS handshake", "http_req_tls_handshaking"),
    "",
  ];

  const thresholds = Object.entries(m)
    .filter(([, v]) => v.thresholds)
    .flatMap(([name, v]) =>
      Object.entries(v.thresholds).map(([t, r]) => `  ${r.ok ? "PASS" : "FAIL"}  ${name}: ${t}`)
    );
  if (thresholds.length) lines.push("Thresholds", ...thresholds, "");

  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const jsonPath = `${SUMMARY_DIR}/summary-${stamp}.json`;
  lines.push(`Full summary: ${jsonPath}`, "");

  return {
    stdout: lines.join("\n"),
    [jsonPath]: JSON.stringify(data, null, 2),
  };
}
