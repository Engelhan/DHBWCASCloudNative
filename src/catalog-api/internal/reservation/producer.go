// Package reservation enthaelt das (noch mit Carlos abzustimmende) Event-Schema fuer
// ReservationRequested sowie den Kafka-Producer, der es veroeffentlicht.
package reservation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"

	"ticketflow/catalog-api/internal/schemareg"
)

// Requested ist der Entwurf des ReservationRequested-Payloads. Noch NICHT final mit dem
// reservation-worker (Carlos) abgestimmt - siehe README ## Kafka-Event-Schema (Entwurf).
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
	firstBroker string
	schema      *schemareg.Client
}

func NewProducer(brokers []string, topic string, schema *schemareg.Client) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{}, // Key = seat_id: Nachrichten zum selben Sitzplatz bleiben geordnet
			AllowAutoTopicCreation: true,          // nur lokal relevant, im Cluster legt Strimzi KafkaTopic-CRs an
		},
		firstBroker: brokers[0],
		schema:      schema,
	}
}

// Ping prueft fuer /readyz, ob der Kafka-Broker erreichbar ist.
func (p *Producer) Ping(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", p.firstBroker)
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
