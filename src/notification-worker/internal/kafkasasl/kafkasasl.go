// Package kafkasasl builds the SCRAM-SHA-512 mechanism for consumer and DLT producer.
package kafkasasl

import (
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/scram"

	"ticketflow/notification-worker/internal/config"
)

// Build returns the mechanism, or nil when KAFKA_SASL_ENABLED is false.
func Build(cfg config.Config) (sasl.Mechanism, error) {
	if !cfg.KafkaSASLEnabled {
		return nil, nil
	}
	return scram.Mechanism(scram.SHA512, cfg.KafkaSASLUsername, cfg.KafkaSASLPassword)
}
