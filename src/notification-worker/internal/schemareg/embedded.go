package schemareg

import _ "embed"

// ResultSchemaJSON is a copy of charts/kafka-cluster/files/schemas/reservation-result.schema.json;
// keep in sync by hand.

//go:embed schemas/reservation-result.schema.json
var ResultSchemaJSON []byte
