// Package kafkaconsumer runs reservation-worker's consumer group on
// RESERVATION_TOPIC. A message's offset is committed only once both the
// booking transaction (internal/booking) and the RESULT_TOPIC publish
// (internal/producer) have succeeded. Since booking.Process is idempotent on
// reservation_id, an uncommitted message being redelivered after a partial
// failure (e.g. the process crashes after the DB commit but before the
// offset commit) is safe -- it re-derives the same result and republishes
// rather than claiming a seat twice. This is the same "commit only on
// verified completion" philosophy as
// src/notification-worker/internal/kafkaconsumer, adapted for two side
// effects (DB write, Kafka publish) instead of one.
//
// A malformed/structurally invalid message can never succeed on retry, so
// it's logged and its offset committed immediately (skip, don't block the
// partition on a poison pill) rather than retried.
package kafkaconsumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"

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

func New(cfg config.Config, database *db.DB, publisher *producer.Producer, schema *schemareg.Client, logger *slog.Logger) *Consumer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second}

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
		c.commit(ctx, msg)
		return
	}

	c.logger.Error("all booking attempts exhausted, message will be redelivered",
		"reservationId", req.ReservationID, "attempts", c.maxRetries+1, "error", lastErr)
	// Deliberately no commit: redelivered on restart/rebalance, which is safe
	// because booking.Process is idempotent on reservation_id -- see package doc.
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.logger.Error("failed to commit offset", "error", err, "offset", msg.Offset)
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
