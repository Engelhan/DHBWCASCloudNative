// Package kafkaconsumer runs the second, independent consumer group on RESULT_TOPIC.
// Offsets are committed only after a successful send (or for poison pills). After
// NOTIFY_MAX_RETRIES a message goes to DLT_TOPIC but stays uncommitted, so it is redelivered on
// restart: at-least-once, nothing dropped silently.
package kafkaconsumer

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"

	"ticketflow/notification-worker/internal/config"
	"ticketflow/notification-worker/internal/event"
	"ticketflow/notification-worker/internal/metrics"
	"ticketflow/notification-worker/internal/notify"
	"ticketflow/notification-worker/internal/schemareg"
)

const channelUnmarshal = "unmarshal"

type Consumer struct {
	reader     *kafka.Reader
	dltWriter  *kafka.Writer
	dialer     *kafka.Dialer
	brokers    []string
	dispatcher *notify.Dispatcher
	// logFallback guarantees a log record when the active channel exhausts its retries.
	logFallback notify.Notifier
	schema      *schemareg.Client
	maxRetries  int
	backoff     time.Duration
	logger      *slog.Logger
	ready       *atomic.Bool
}

func New(cfg config.Config, tlsConfig *tls.Config, saslMechanism sasl.Mechanism, dispatcher *notify.Dispatcher, logFallback notify.Notifier, schema *schemareg.Client, logger *slog.Logger, ready *atomic.Bool) *Consumer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, TLS: tlsConfig, SASLMechanism: saslMechanism}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.ConsumerGroup,
		Topic:   cfg.ResultTopic,
		Dialer:  dialer,
	})

	var dltWriter *kafka.Writer
	if cfg.DLTTopic != "" {
		dltWriter = &kafka.Writer{
			Addr:      kafka.TCP(cfg.KafkaBrokers...),
			Topic:     cfg.DLTTopic,
			Balancer:  &kafka.LeastBytes{},
			Transport: &kafka.Transport{TLS: tlsConfig, SASL: saslMechanism},
		}
	}

	return &Consumer{
		reader:      reader,
		dltWriter:   dltWriter,
		dialer:      dialer,
		brokers:     cfg.KafkaBrokers,
		dispatcher:  dispatcher,
		logFallback: logFallback,
		schema:      schema,
		maxRetries:  cfg.NotifyMaxRetries,
		backoff:     cfg.NotifyRetryBackoff,
		logger:      logger,
		ready:       ready,
	}
}

// Run blocks until ctx is cancelled. Readiness reflects broker reachability, not traffic.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.waitForBroker(ctx); err != nil {
		return err
	}
	c.ready.Store(true)
	defer c.ready.Store(false)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.ready.Store(false)
			c.logger.Error("fetch message failed, retrying", "error", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil
			}
			// Re-probe the broker: on an idle topic readiness would otherwise wait for the next message.
			if err := c.waitForBroker(ctx); err != nil {
				return nil
			}
			c.ready.Store(true)
			continue
		}
		c.ready.Store(true)
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
	payload, err := c.schema.Unwrap(msg.Value)
	var e event.Event
	if err == nil {
		e, err = event.Unmarshal(payload)
	}
	if err == nil {
		err = e.Validate(c.dispatcher.Channel == notify.ChannelSMTP)
	}
	if err != nil {
		metrics.DispatchErrors.WithLabelValues(channelUnmarshal).Inc()
		c.logger.Error("malformed/invalid event, sending to DLT and skipping", "error", err, "offset", msg.Offset)
		c.tryPublishToDLT(ctx, msg, err, 0)
		// Poison pill: commit regardless so the consumer never gets stuck.
		c.commit(ctx, msg)
		return
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(c.backoff):
			case <-ctx.Done():
				return
			}
		}
		if err := c.dispatcher.Attempt(ctx, e); err != nil {
			lastErr = err
			c.logger.Warn("notification attempt failed", "attempt", attempt+1, "reservationId", e.ReservationID, "error", err)
			continue
		}
		// Commit only after an actual send.
		c.commit(ctx, msg)
		return
	}

	c.logger.Error("all notification attempts exhausted, sending to DLT",
		"reservationId", e.ReservationID, "attempts", c.maxRetries+1, "error", lastErr)

	// Fall back to a log record unless log was already the channel.
	if c.dispatcher.Channel != notify.ChannelLog {
		_ = c.logFallback.Notify(ctx, e)
	}

	c.tryPublishToDLT(ctx, msg, lastErr, c.maxRetries+1)
	// No commit: redelivered on restart/rebalance.
}

func (c *Consumer) tryPublishToDLT(ctx context.Context, msg kafka.Message, cause error, attempts int) {
	if c.dltWriter == nil {
		return
	}
	reason := "unknown"
	if cause != nil {
		reason = cause.Error()
	}
	err := c.dltWriter.WriteMessages(ctx, kafka.Message{
		Key:   msg.Key,
		Value: msg.Value,
		Headers: []kafka.Header{
			{Key: "x-failure-reason", Value: []byte(reason)},
			{Key: "x-attempt-count", Value: []byte(strconv.Itoa(attempts))},
			{Key: "x-original-topic", Value: []byte(msg.Topic)},
		},
	})
	if err != nil {
		c.logger.Error("failed to publish to DLT", "error", err, "offset", msg.Offset)
		return
	}
	metrics.DLTPublished.Inc()
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.logger.Error("failed to commit offset", "error", err, "offset", msg.Offset)
	}
}

func (c *Consumer) Close() error {
	err := c.reader.Close()
	if c.dltWriter != nil {
		if werr := c.dltWriter.Close(); werr != nil && err == nil {
			err = werr
		}
	}
	return err
}
