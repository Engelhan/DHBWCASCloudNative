package schemareg

import _ "embed"

// ResultSchemaJSON is a copy of
// charts/ticketflow/files/schemas/reservation-result.schema.json, duplicated here (not shared via
// a Go module) -- same convention as this service's own internal/event package. Keep in sync by
// hand when the canonical schema changes.

//go:embed schemas/reservation-result.schema.json
var ResultSchemaJSON []byte
