// Package kafkaconsumer consumes RESERVATION_TOPIC. Offsets are committed only after both the
// booking and the result publish succeeded; redelivery is safe because booking is idempotent.
// Invalid messages are logged and committed (poison pills never block a partition).
package kafkaconsumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"

	"ticketflow/reservation-worker/internal/booking"
	"ticketflow/reservation-worker/internal/config"
	"ticketflow/reservation-worker/internal/db"
	"ticketflow/reservation-worker/internal/event"
	"ticketflow/reservation-worker/internal/metrics"
	"ticketflow/reservation-worker/internal/producer"
	"ticketflow/reservation-worker/internal/schemareg"
)

const (
	stageUnmarshal = "unmarshal"
	stageDB        = "db"
	stagePublish   = "publish"
)

type Consumer struct {
	reader     *kafka.Reader
	dialer     *kafka.Dialer
	brokers    []string
	db         *db.DB
	publisher  *producer.Producer
	schema     *schemareg.Client
	maxRetries int
	backoff    time.Duration
	logger     *slog.Logger
}

func New(cfg config.Config, database *db.DB, publisher *producer.Producer, schema *schemareg.Client, logger *slog.Logger, saslMechanism sasl.Mechanism) *Consumer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, SASLMechanism: saslMechanism}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.ConsumerGroup,
		Topic:   cfg.ReservationTopic,
		Dialer:  dialer,
	})

	return &Consumer{
		reader:     reader,
		dialer:     dialer,
		brokers:    cfg.KafkaBrokers,
		db:         database,
		publisher:  publisher,
		schema:     schema,
		maxRetries: cfg.ProcessMaxRetries,
		backoff:    cfg.ProcessRetryBackoff,
		logger:     logger,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if err := c.waitForBroker(ctx); err != nil {
		return err
	}

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("fetch message failed, retrying", "error", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil
			}
			continue
		}
		c.handleMessage(ctx, msg)
	}
}

func (c *Consumer) waitForBroker(ctx context.Context) error {
	for {
		conn, err := c.dialer.DialContext(ctx, "tcp", c.brokers[0])
		if err == nil {
			conn.Close()
			return nil
		}
		c.logger.Warn("kafka broker not yet reachable, retrying", "broker", c.brokers[0], "error", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, msg kafka.Message) {
	start := time.Now()

	payload, err := c.schema.Unwrap(msg.Value)
	var req event.Requested
	if err == nil {
		req, err = event.UnmarshalRequested(payload)
	}
	if err == nil {
		err = req.Validate()
	}
	if err != nil {
		metrics.ProcessingErrors.WithLabelValues(stageUnmarshal).Inc()
		c.logger.Error("malformed/invalid reservation request, skipping", "error", err, "offset", msg.Offset)
		c.commit(ctx, msg)
		return
	}

	var result event.Result
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(c.backoff):
			case <-ctx.Done():
				return
			}
		}

		result, lastErr = booking.Process(ctx, c.db.Pool(), req)
		if lastErr != nil {
			metrics.ProcessingErrors.WithLabelValues(stageDB).Inc()
			c.logger.Warn("booking attempt failed", "attempt", attempt+1, "reservationId", req.ReservationID, "error", lastErr)
			continue
		}

		if lastErr = c.publisher.Publish(ctx, result); lastErr != nil {
			metrics.ProcessingErrors.WithLabelValues(stagePublish).Inc()
			c.logger.Warn("result publish failed", "attempt", attempt+1, "reservationId", req.ReservationID, "error", lastErr)
			continue
		}

		metrics.ReservationsProcessed.WithLabelValues(result.Result()).Inc()
		metrics.ReservationProcessingDuration.WithLabelValues(result.Result()).Observe(time.Since(start).Seconds())
		c.logger.Info("reservation processed", "reservationId", req.ReservationID, "seatId", req.SeatID, "eventId", req.EventID, "result", result.Result())
		c.commit(ctx, msg)
		return
	}

	c.logger.Error("all booking attempts exhausted, message will be redelivered",
		"reservationId", req.ReservationID, "attempts", c.maxRetries+1, "error", lastErr)
	// No commit: redelivered later, safe because booking is idempotent.
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.logger.Error("failed to commit offset", "error", err, "offset", msg.Offset)
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
