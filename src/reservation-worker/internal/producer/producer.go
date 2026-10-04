// Package producer publishes booking results to RESULT_TOPIC.
package producer

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"

	"ticketflow/reservation-worker/internal/event"
	"ticketflow/reservation-worker/internal/schemareg"
)

type Producer struct {
	writer      *kafka.Writer
	dialer      *kafka.Dialer
	firstBroker string
	schema      *schemareg.Client
}

// New creates the producer; a nil saslMechanism means no auth.
func New(brokers []string, topic string, schema *schemareg.Client, saslMechanism sasl.Mechanism) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:  kafka.TCP(brokers...),
			Topic: topic,
			// One result per reservation; no cross-reservation ordering needed.
			Balancer: &kafka.Hash{},
			// One message at a time: the 1s default batch timeout would cap each pod at ~1 msg/s.
			BatchSize: 1,
			// acks=all (default is acks=0), matching RF 3 / min.insync 2 (P5).
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: true, // local only; Strimzi manages topics in the cluster
			Transport:              &kafka.Transport{SASL: saslMechanism},
		},
		dialer:      &kafka.Dialer{Timeout: 10 * time.Second, SASLMechanism: saslMechanism},
		firstBroker: brokers[0],
		schema:      schema,
	}
}

// Ping checks broker reachability for /readyz.
func (p *Producer) Ping(ctx context.Context) error {
	conn, err := p.dialer.DialContext(ctx, "tcp", p.firstBroker)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (p *Producer) Publish(ctx context.Context, result event.Result) error {
	value, err := result.Marshal()
	if err != nil {
		return err
	}
	framed, err := p.schema.Wrap(value)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(result.ReservationID),
		Value: framed,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
