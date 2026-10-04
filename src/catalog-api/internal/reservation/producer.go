// Package reservation defines the ReservationRequested event and its Kafka producer.
package reservation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"

	"ticketflow/catalog-api/internal/schemareg"
)

// Requested is the ReservationRequested payload.
type Requested struct {
	ReservationID string    `json:"reservation_id"`
	EventID       string    `json:"event_id"`
	SeatID        string    `json:"seat_id"`
	SessionID     string    `json:"session_id"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	Email         string    `json:"email"`
	RequestedAt   time.Time `json:"requested_at"`
}

type Producer struct {
	writer      *kafka.Writer
	dialer      *kafka.Dialer
	firstBroker string
	schema      *schemareg.Client
}

// NewProducer creates the producer; a nil saslMechanism means no auth.
func NewProducer(brokers []string, topic string, schema *schemareg.Client, saslMechanism sasl.Mechanism) *Producer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, SASLMechanism: saslMechanism}
	return &Producer{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Topic:    topic,
			Balancer: &kafka.Hash{}, // key = seat_id keeps per-seat ordering
			// Requests wait for the publish; the 1s default batch timeout would add 1s per request.
			BatchTimeout: 5 * time.Millisecond,
			// acks=all (default is acks=0), matching RF 3 / min.insync 2 (P5).
			RequiredAcks:           kafka.RequireAll,
			AllowAutoTopicCreation: true, // local only; Strimzi manages topics in the cluster
			Transport:              &kafka.Transport{SASL: saslMechanism},
		},
		dialer:      dialer,
		firstBroker: brokers[0],
		schema:      schema,
	}
}

// Ping checks broker reachability for /readyz, including the SASL handshake.
func (p *Producer) Ping(ctx context.Context) error {
	conn, err := p.dialer.DialContext(ctx, "tcp", p.firstBroker)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (p *Producer) PublishReservationRequested(ctx context.Context, req Requested) error {
	value, err := json.Marshal(req)
	if err != nil {
		return err
	}
	framed, err := p.schema.Wrap(value)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(req.SeatID),
		Value: framed,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
