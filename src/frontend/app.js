const API_BASE_URL = window.API_BASE_URL;
const SESSION_ID_KEY = "ticketflow_session_id";
const SELECTED_EVENT_KEY = "ticketflow_selected_event";

const CATEGORY_LABELS = { premium: "Premium", parkett: "Parkett", balkon: "Balkon" };
const priceFormatter = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" });
const dateFormatter = new Intl.DateTimeFormat("de-DE", { dateStyle: "full", timeStyle: "short" });
const shortDateFormatter = new Intl.DateTimeFormat("de-DE", { dateStyle: "medium", timeStyle: "short" });

const eventNameEl = document.getElementById("event-name");
const eventMetaEl = document.getElementById("event-meta");
const eventPickerEl = document.getElementById("event-picker");
const eventSelectEl = document.getElementById("event-select");
const hallEl = document.getElementById("hall");
const availabilitySummaryEl = document.getElementById("availability-summary");
const categoryLegendEl = document.getElementById("category-legend");

const bookingDialogEl = document.getElementById("booking-dialog");
const bookingFormEl = document.getElementById("booking-form");
const dialogEventLabelEl = document.getElementById("dialog-event-label");
const dialogSeatLabelEl = document.getElementById("dialog-seat-label");
const dialogSeatCategoryEl = document.getElementById("dialog-seat-category");
const dialogSeatPriceEl = document.getElementById("dialog-seat-price");
const dialogCancelEl = document.getElementById("dialog-cancel");

const statusCardEl = document.getElementById("status-card");
const statusIconEl = document.getElementById("status-icon");
const statusTitleEl = document.getElementById("status-title");
const statusMessageEl = document.getElementById("status-message");

const errorBannerEl = document.getElementById("error-banner");

let events = [];
let currentEvent = null;
// Seats of the current hall by id (one delegated click listener instead of 1000).
let seatsById = new Map();
let selectedSeat = null;
let pollTimer = null;
// Increasing counter: a slow response for a deselected event must not overwrite the current one.
let loadToken = 0;

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

async function loadEvents() {
  try {
    const response = await fetch(`${API_BASE_URL}/events`);
    if (!response.ok) throw new Error(`GET /events fehlgeschlagen (${response.status})`);
    events = (await response.json()) ?? [];
    if (events.length === 0) {
      eventNameEl.textContent = "Kein Event verfügbar";
      eventMetaEl.textContent = "";
      hallEl.innerHTML = "";
      availabilitySummaryEl.textContent = "";
      return;
    }

    renderEventPicker();
    // Restore the last event only if the API still knows it, else events[0].
    const remembered = localStorage.getItem(SELECTED_EVENT_KEY);
    const initial = events.find((e) => e.id === remembered) ?? events[0];
    await selectEvent(initial.id);
  } catch (err) {
    showError(`Events konnten nicht geladen werden: ${err.message}`);
  }
}

function renderEventPicker() {
  eventSelectEl.innerHTML = "";
  for (const event of events) {
    const option = document.createElement("option");
    option.value = event.id;
    option.textContent = `${event.name} · ${event.venue} · ${shortDateFormatter.format(new Date(event.starts_at))}`;
    eventSelectEl.appendChild(option);
  }
  // Hide the picker for a single event.
  eventPickerEl.hidden = events.length < 2;
}

async function selectEvent(eventId) {
  const event = events.find((e) => e.id === eventId);
  if (!event) return;

  currentEvent = event;
  localStorage.setItem(SELECTED_EVENT_KEY, event.id);
  eventSelectEl.value = event.id;

  // Switching events resets the booking in progress.
  stopPolling();
  statusCardEl.hidden = true;
  selectedSeat = null;
  clearError();

  eventNameEl.textContent = event.name;
  eventMetaEl.textContent = `${event.venue} · ${dateFormatter.format(new Date(event.starts_at))}`;

  await loadSeats();
}

async function loadSeats() {
  const token = ++loadToken;
  hallEl.replaceChildren(Object.assign(document.createElement("p"), {
    className: "loading",
    textContent: "Lade Sitzplätze…",
  }));
  availabilitySummaryEl.textContent = "";

  try {
    const response = await fetch(`${API_BASE_URL}/events/${currentEvent.id}/seats`);
    if (!response.ok) throw new Error(`GET /events/{id}/seats fehlgeschlagen (${response.status})`);
    const seats = await response.json();
    if (token !== loadToken) return;
    renderHall(seats);
  } catch (err) {
    if (token !== loadToken) return;
    showError(`Sitzplätze konnten nicht geladen werden: ${err.message}`);
  }
}

function renderHall(seats) {
  const freeCount = seats.filter((s) => s.status === "free").length;
  availabilitySummaryEl.textContent = `${freeCount} von ${seats.length} Plätzen frei`;

  seatsById = new Map(seats.map((seat) => [seat.id, seat]));

  const categoriesSeen = new Map();
  const rows = new Map();
  for (const seat of seats) {
    if (!rows.has(seat.row_number)) rows.set(seat.row_number, []);
    rows.get(seat.row_number).push(seat);
    if (!categoriesSeen.has(seat.category)) categoriesSeen.set(seat.category, seat.price_cents);
  }

  // 1000 seats: build in a fragment and attach once.
  const fragment = document.createDocumentFragment();
  for (const rowNumber of [...rows.keys()].sort((a, b) => a - b)) {
    const rowSeats = rows.get(rowNumber).sort((a, b) => a.seat_number - b.seat_number);
    const category = rowSeats[0].category;

    const rowEl = document.createElement("div");
    rowEl.className = "row";

    const categoryLabel = CATEGORY_LABELS[category] ?? category;
    const price = priceFormatter.format(rowSeats[0].price_cents / 100);
    const labelEl = document.createElement("div");
    labelEl.className = "row-label";
    // No category name per row (too wide); the dot color and title carry it.
    labelEl.title = `Reihe ${rowNumber} · ${categoryLabel} · ${price}`;
    labelEl.innerHTML = `<span class="cat-dot ${category}"></span> Reihe ${rowNumber} · ${price}`;
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
      button.dataset.seatId = seat.id;
      button.textContent = seat.seat_number;
      button.title = seatLabel(seat);
      button.disabled = seat.status !== "free";
      seatsEl.appendChild(button);
    });
    rowEl.appendChild(seatsEl);
    fragment.appendChild(rowEl);
  }
  hallEl.replaceChildren(fragment);

  categoryLegendEl.innerHTML = "";
  for (const [category, priceCents] of categoriesSeen) {
    const item = document.createElement("li");
    item.innerHTML = `<span class="cat-dot ${category}"></span> ${CATEGORY_LABELS[category] ?? category} — ${priceFormatter.format(priceCents / 100)}`;
    categoryLegendEl.appendChild(item);
  }
}

eventSelectEl.addEventListener("change", (event) => {
  selectEvent(event.target.value);
});

hallEl.addEventListener("click", (event) => {
  const button = event.target.closest("button.seat");
  if (!button || button.disabled) return;
  const seat = seatsById.get(button.dataset.seatId);
  if (seat) openBookingDialog(seat, button);
});

function openBookingDialog(seat, button) {
  clearError();
  selectedSeat = seat;

  for (const el of hallEl.querySelectorAll(".seat.selected")) {
    el.classList.remove("selected");
  }
  button.classList.add("selected");

  dialogEventLabelEl.textContent = `${currentEvent.name} · ${currentEvent.venue}`;
  dialogSeatLabelEl.textContent = seatLabel(seat);
  dialogSeatCategoryEl.textContent = CATEGORY_LABELS[seat.category] ?? seat.category;
  dialogSeatPriceEl.textContent = priceFormatter.format(seat.price_cents / 100);
  bookingFormEl.reset();
  bookingDialogEl.returnValue = "";
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
  // Remember the booked event; the user may switch while polling.
  const bookedEventId = currentEvent.id;

  try {
    const response = await fetch(
      `${API_BASE_URL}/events/${bookedEventId}/reservations`,
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
    startPolling(reservationId, bookedEventId);
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

function startPolling(reservationId, bookedEventId) {
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
        if (currentEvent.id === bookedEventId) await loadSeats();
        return;
      }
      if (reservation.status === "rejected") {
        setStatus("rejected", "Ausgebucht", "Dieser Sitzplatz konnte nicht reserviert werden.");
        stopPolling();
        if (currentEvent.id === bookedEventId) await loadSeats();
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

loadEvents();
