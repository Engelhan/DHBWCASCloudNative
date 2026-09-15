package schemareg

import _ "embed"

// RequestedSchemaJSON/ResultSchemaJSON are copies of
// charts/ticketflow/files/schemas/*.schema.json, duplicated here (not shared via a Go module) --
// same convention as internal/event, see that package's doc comment. Keep both in sync by hand
// when the canonical schema changes.

//go:embed schemas/reservation-requested.schema.json
var RequestedSchemaJSON []byte

//go:embed schemas/reservation-result.schema.json
var ResultSchemaJSON []byte
