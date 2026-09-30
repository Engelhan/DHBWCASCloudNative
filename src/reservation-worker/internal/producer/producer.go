// Package producer publishes reservation-worker's booking results to
// RESULT_TOPIC.
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

// New wires up the producer. saslMechanism is nil when SASL auth is disabled -- see
// internal/kafkasasl package doc.
func New(brokers []string, topic string, schema *schemareg.Client, saslMechanism sasl.Mechanism) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:  kafka.TCP(brokers...),
			Topic: topic,
			// Key = reservation_id: jede Reservierung erzeugt genau ein
			// Result-Event, eine Ordering-Garantie zwischen verschiedenen
			// Reservierungen ist nicht erforderlich.
			Balancer: &kafka.Hash{},
			// Der Consumer verarbeitet streng eine Nachricht nach der anderen
			// und wartet synchron auf dieses Publish, bevor er den Offset
			// committet - hier liegt also nie mehr als eine Nachricht an.
			// Mit kafka-go-Defaults (BatchSize 100, BatchTimeout 1s) wartete
			// jedes Publish die volle Sekunde auf einen Batch, der nie voll
			// wird: ~1 Reservierung/s pro Pod, egal wie viele KEDA startet.
			BatchSize: 1,
			// kafka-go-Default ist RequireNone (acks=0): ein verlorenes
			// Result-Event fiele nie auf, weil der Offset danach trotzdem
			// committet wird. acks=all passt zu replicationFactor 3 /
			// min.insync.replicas 2 (P5, charts/kafka-cluster).
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: true, // nur lokal relevant, im Cluster legt Strimzi KafkaTopic-CRs an
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
