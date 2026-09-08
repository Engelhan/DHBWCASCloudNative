const API_BASE_URL = window.API_BASE_URL;
const SESSION_ID_KEY = "ticketflow_session_id";

const CATEGORY_LABELS = { premium: "Premium", parkett: "Parkett", balkon: "Balkon" };
const priceFormatter = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" });

const eventNameEl = document.getElementById("event-name");
const eventMetaEl = document.getElementById("event-meta");
const hallEl = document.getElementById("hall");
const availabilitySummaryEl = document.getElementById("availability-summary");
const categoryLegendEl = document.getElementById("category-legend");

const bookingDialogEl = document.getElementById("booking-dialog");
const bookingFormEl = document.getElementById("booking-form");
const dialogSeatLabelEl = document.getElementById("dialog-seat-label");
const dialogSeatCategoryEl = document.getElementById("dialog-seat-category");
const dialogSeatPriceEl = document.getElementById("dialog-seat-price");
const dialogCancelEl = document.getElementById("dialog-cancel");

const statusCardEl = document.getElementById("status-card");
const statusIconEl = document.getElementById("status-icon");
const statusTitleEl = document.getElementById("status-title");
const statusMessageEl = document.getElementById("status-message");

const errorBannerEl = document.getElementById("error-banner");

let currentEvent = null;
let selectedSeat = null;
let pollTimer = null;

function seatLabel(seat) {
  return `Reihe ${seat.row_number}, Platz ${seat.seat_number}`;
}

function getSessionId() {
  let sessionId = localStorage.getItem(SESSION_ID_KEY);
  if (!sessionId) {
    sessionId = crypto.randomUUID();
    localStorage.setItem(SESSION_ID_KEY, sessionId);
  }
  return sessionId;
}

function showError(message) {
  errorBannerEl.textContent = message;
  errorBannerEl.hidden = false;
}

function clearError() {
  errorBannerEl.hidden = true;
}

async function loadEvent() {
  try {
    const response = await fetch(`${API_BASE_URL}/events`);
    if (!response.ok) throw new Error(`GET /events fehlgeschlagen (${response.status})`);
    const events = await response.json();
    if (!events || events.length === 0) {
      eventNameEl.textContent = "Kein Event verfügbar";
      return;
    }
    currentEvent = events[0];
    const startsAt = new Date(currentEvent.starts_at).toLocaleString("de-DE", {
      dateStyle: "full",
      timeStyle: "short",
    });
    eventNameEl.textContent = currentEvent.name;
    eventMetaEl.textContent = `${currentEvent.venue} · ${startsAt}`;
    await loadSeats();
  } catch (err) {
    showError(`Event konnte nicht geladen werden: ${err.message}`);
  }
}

async function loadSeats() {
  try {
    const response = await fetch(`${API_BASE_URL}/events/${currentEvent.id}/seats`);
    if (!response.ok) throw new Error(`GET /events/{id}/seats fehlgeschlagen (${response.status})`);
    const seats = await response.json();
    renderHall(seats);
  } catch (err) {
    showError(`Sitzplätze konnten nicht geladen werden: ${err.message}`);
  }
}

function renderHall(seats) {
  const freeCount = seats.filter((s) => s.status === "free").length;
  availabilitySummaryEl.textContent = `${freeCount} von ${seats.length} Plätzen frei`;

  const categoriesSeen = new Map();
  const rows = new Map();
  for (const seat of seats) {
    if (!rows.has(seat.row_number)) rows.set(seat.row_number, []);
    rows.get(seat.row_number).push(seat);
    if (!categoriesSeen.has(seat.category)) categoriesSeen.set(seat.category, seat.price_cents);
  }

  hallEl.innerHTML = "";
  for (const rowNumber of [...rows.keys()].sort((a, b) => a - b)) {
    const rowSeats = rows.get(rowNumber).sort((a, b) => a.seat_number - b.seat_number);
    const category = rowSeats[0].category;

    const rowEl = document.createElement("div");
    rowEl.className = "row";

    const labelEl = document.createElement("div");
    labelEl.className = "row-label";
    labelEl.innerHTML = `<span class="cat-dot ${category}"></span> Reihe ${rowNumber} · ${CATEGORY_LABELS[category] ?? category} · ${priceFormatter.format(rowSeats[0].price_cents / 100)}`;
    rowEl.appendChild(labelEl);

    const seatsEl = document.createElement("div");
    seatsEl.className = "row-seats";
    rowSeats.forEach((seat, index) => {
      if (index === Math.ceil(rowSeats.length / 2)) {
        const aisle = document.createElement("span");
        aisle.className = "aisle";
        seatsEl.appendChild(aisle);
      }
      const button = document.createElement("button");
      button.type = "button";
      button.className = `seat ${seat.status}`;
      button.textContent = seat.seat_number;
      button.title = seatLabel(seat);
      button.disabled = seat.status !== "free";
      button.addEventListener("click", () => openBookingDialog(seat, button));
      seatsEl.appendChild(button);
    });
    rowEl.appendChild(seatsEl);
    hallEl.appendChild(rowEl);
  }

  categoryLegendEl.innerHTML = "";
  for (const [category, priceCents] of categoriesSeen) {
    const item = document.createElement("li");
    item.innerHTML = `<span class="cat-dot ${category}"></span> ${CATEGORY_LABELS[category] ?? category} — ${priceFormatter.format(priceCents / 100)}`;
    categoryLegendEl.appendChild(item);
  }
}

function openBookingDialog(seat, button) {
  clearError();
  selectedSeat = seat;

  for (const el of hallEl.querySelectorAll(".seat.selected")) {
    el.classList.remove("selected");
  }
  button.classList.add("selected");

  dialogSeatLabelEl.textContent = seatLabel(seat);
  dialogSeatCategoryEl.textContent = CATEGORY_LABELS[seat.category] ?? seat.category;
  dialogSeatPriceEl.textContent = priceFormatter.format(seat.price_cents / 100);
  bookingFormEl.reset();
  bookingDialogEl.showModal();
}

dialogCancelEl.addEventListener("click", () => {
  bookingDialogEl.close();
});

bookingDialogEl.addEventListener("close", () => {
  if (bookingDialogEl.returnValue !== "submit") {
    for (const el of hallEl.querySelectorAll(".seat.selected")) {
      el.classList.remove("selected");
    }
    selectedSeat = null;
  }
});

bookingFormEl.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!selectedSeat) return;
  clearError();

  const formData = new FormData(bookingFormEl);
  const body = {
    seat_id: selectedSeat.id,
    session_id: getSessionId(),
    first_name: formData.get("first_name"),
    last_name: formData.get("last_name"),
    email: formData.get("email"),
  };

  try {
    const response = await fetch(
      `${API_BASE_URL}/events/${currentEvent.id}/reservations`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }
    );
    if (response.status === 409) {
      bookingDialogEl.close();
      showError("Dieser Sitzplatz ist inzwischen ausgebucht. Bitte anderen Platz wählen.");
      await loadSeats();
      return;
    }
    if (!response.ok) throw new Error(`POST reservation fehlgeschlagen (${response.status})`);

    const { reservation_id: reservationId } = await response.json();
    bookingDialogEl.returnValue = "submit";
    bookingDialogEl.close();
    setStatus("pending", "In Bearbeitung", "Deine Buchung wird verarbeitet…");
    startPolling(reservationId);
  } catch (err) {
    showError(`Buchung fehlgeschlagen: ${err.message}`);
  }
});

function setStatus(status, title, detail) {
  statusCardEl.hidden = false;
  statusIconEl.className = `status-icon ${status}`;
  statusIconEl.textContent = status === "confirmed" ? "✓" : status === "rejected" ? "✕" : "";
  statusTitleEl.textContent = title;
  statusMessageEl.textContent = detail;
}

function startPolling(reservationId) {
  stopPolling();
  const startedAt = Date.now();
  const POLL_INTERVAL_MS = 2000;
  const TIMEOUT_MS = 60000;

  pollTimer = setInterval(async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/reservations/${reservationId}`);
      if (!response.ok) throw new Error(`GET /reservations/{id} fehlgeschlagen (${response.status})`);
      const reservation = await response.json();

      if (reservation.status === "confirmed") {
        setStatus("confirmed", "Bestätigt", "Dein Sitzplatz ist reserviert.");
        stopPolling();
        return;
      }
      if (reservation.status === "rejected") {
        setStatus("rejected", "Ausgebucht", "Dieser Sitzplatz konnte nicht reserviert werden.");
        stopPolling();
        await loadSeats();
        return;
      }

      if (Date.now() - startedAt > TIMEOUT_MS) {
        setStatus("pending", "In Bearbeitung", "Dauert länger als erwartet (reservation-worker läuft nicht?).");
        stopPolling();
      }
    } catch (err) {
      showError(`Status konnte nicht abgefragt werden: ${err.message}`);
      stopPolling();
    }
  }, POLL_INTERVAL_MS);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

loadEvent();
