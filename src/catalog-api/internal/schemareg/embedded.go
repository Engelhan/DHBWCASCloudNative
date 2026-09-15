package schemareg

import _ "embed"

// RequestedSchemaJSON is a copy of
// charts/ticketflow/files/schemas/reservation-requested.schema.json, duplicated here (not shared
// via a Go module) -- same convention as reservation-worker/notification-worker's internal/event
// packages. Keep in sync by hand when the canonical schema changes.

//go:embed schemas/reservation-requested.schema.json
var RequestedSchemaJSON []byte
