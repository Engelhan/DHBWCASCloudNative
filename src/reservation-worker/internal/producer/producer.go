// Package producer publishes reservation-worker's booking results to
// RESULT_TOPIC.
package producer

import (
	"context"

	"github.com/segmentio/kafka-go"

	"ticketflow/reservation-worker/internal/event"
	"ticketflow/reservation-worker/internal/schemareg"
)

type Producer struct {
	writer      *kafka.Writer
	firstBroker string
	schema      *schemareg.Client
}

func New(brokers []string, topic string, schema *schemareg.Client) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:  kafka.TCP(brokers...),
			Topic: topic,
			// Key = reservation_id: jede Reservierung erzeugt genau ein
			// Result-Event, eine Ordering-Garantie zwischen verschiedenen
			// Reservierungen ist nicht erforderlich.
			Balancer:               &kafka.Hash{},
			AllowAutoTopicCreation: true, // nur lokal relevant, im Cluster legt Strimzi KafkaTopic-CRs an
		},
		firstBroker: brokers[0],
		schema:      schema,
	}
}

// Ping checks broker reachability for /readyz.
func (p *Producer) Ping(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", p.firstBroker)
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
