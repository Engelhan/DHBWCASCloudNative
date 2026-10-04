package schemareg

import _ "embed"

// Copies of charts/kafka-cluster/files/schemas/*.schema.json; keep in sync by hand.

//go:embed schemas/reservation-requested.schema.json
var RequestedSchemaJSON []byte

//go:embed schemas/reservation-result.schema.json
var ResultSchemaJSON []byte
