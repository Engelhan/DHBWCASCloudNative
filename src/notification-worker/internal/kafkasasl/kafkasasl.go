// Package kafkasasl builds the SCRAM-SHA-512 SASL mechanism shared by the
// Kafka consumer and the dead-letter-topic producer.
package kafkasasl

import (
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/scram"

	"ticketflow/notification-worker/internal/config"
)

// Build returns a SCRAM-SHA-512 mechanism authenticating as the
// Strimzi-managed KafkaUser "notification-worker" (see charts/kafka-cluster
// templates/messaging/kafka-users.yaml), or (nil, nil) when
// KAFKA_SASL_ENABLED is false -- kafka-go treats a nil sasl.Mechanism as "no
// auth", the current default for local docker-compose (no Strimzi
// listener/ACLs there).
func Build(cfg config.Config) (sasl.Mechanism, error) {
	if !cfg.KafkaSASLEnabled {
		return nil, nil
	}
	return scram.Mechanism(scram.SHA512, cfg.KafkaSASLUsername, cfg.KafkaSASLPassword)
}
