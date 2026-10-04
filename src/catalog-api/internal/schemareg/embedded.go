package schemareg

import _ "embed"

// RequestedSchemaJSON is a copy of
// charts/kafka-cluster/files/schemas/reservation-requested.schema.json;
// keep in sync by hand.

//go:embed schemas/reservation-requested.schema.json
var RequestedSchemaJSON []byte
