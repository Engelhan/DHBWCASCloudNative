// Package kafkaconsumer runs notification-worker's second, independent
// Kafka consumer group off RESULT_TOPIC and implements the retry /
// dead-letter-topic / commit-only-on-success semantics described in the
// implementation plan: an offset is only committed once the notification
// was actually sent (or the message is a poison pill that could never
// succeed). A message that exhausts NOTIFY_MAX_RETRIES is published to
// DLT_TOPIC (if configured) for visibility/manual reprocessing, but its
// offset stays uncommitted -- it will be redelivered on the next
// restart/rebalance until it succeeds or lands on the DLT again. This is a
// deliberate at-least-once trade-off (documented, not silent): live
// processing never blocks on one bad message, but nothing is silently
// dropped either.
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

	"ticketflow/notification-worker/internal/config"
	"ticketflow/notification-worker/internal/event"
	"ticketflow/notification-worker/internal/metrics"
	"ticketflow/notification-worker/internal/notify"
)

const channelUnmarshal = "unmarshal"

type Consumer struct {
	reader     *kafka.Reader
	dltWriter  *kafka.Writer
	dialer     *kafka.Dialer
	brokers    []string
	dispatcher *notify.Dispatcher
	// logFallback is used as a safety net when the active dispatcher
	// channel (e.g. SMTP) exhausts all retries, so there is always at
	// least a log record of what was attempted.
	logFallback notify.Notifier
	maxRetries  int
	backoff     time.Duration
	logger      *slog.Logger
	ready       *atomic.Bool
}

func New(cfg config.Config, tlsConfig *tls.Config, dispatcher *notify.Dispatcher, logFallback notify.Notifier, logger *slog.Logger, ready *atomic.Bool) *Consumer {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, TLS: tlsConfig}

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
			Transport: &kafka.Transport{TLS: tlsConfig},
		}
	}

	return &Consumer{
		reader:      reader,
		dltWriter:   dltWriter,
		dialer:      dialer,
		brokers:     cfg.KafkaBrokers,
		dispatcher:  dispatcher,
		logFallback: logFallback,
		maxRetries:  cfg.NotifyMaxRetries,
		backoff:     cfg.NotifyRetryBackoff,
		logger:      logger,
		ready:       ready,
	}
}

// Run blocks until ctx is cancelled. Readiness (c.ready) reflects Kafka
// broker reachability, not "a message has arrived" -- there may
// legitimately be long stretches with no traffic until reservation-worker
// (the producer for RESULT_TOPIC) exists.
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
	e, err := event.Unmarshal(msg.Value)
	if err == nil {
		err = e.Validate(c.dispatcher.Channel == notify.ChannelSMTP)
	}
	if err != nil {
		metrics.DispatchErrors.WithLabelValues(channelUnmarshal).Inc()
		c.logger.Error("malformed/invalid event, sending to DLT and skipping", "error", err, "offset", msg.Offset)
		c.tryPublishToDLT(ctx, msg, err, 0)
		// A malformed message can never succeed on retry -- commit
		// regardless of DLT outcome so the consumer doesn't get stuck on a
		// poison pill forever.
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
		// Success: only now do we commit, per the "commit only on actual
		// send" requirement -- see package doc.
		c.commit(ctx, msg)
		return
	}

	c.logger.Error("all notification attempts exhausted, sending to DLT",
		"reservationId", e.ReservationID, "attempts", c.maxRetries+1, "error", lastErr)

	// Safety net: guarantee at least a log record even though the active
	// channel (e.g. SMTP) failed. Skip the duplicate if log was already the
	// active channel.
	if c.dispatcher.Channel != notify.ChannelLog {
		_ = c.logFallback.Notify(ctx, e)
	}

	c.tryPublishToDLT(ctx, msg, lastErr, c.maxRetries+1)
	// Deliberately no commit here: this message is redelivered on
	// restart/rebalance until it either succeeds or lands on the DLT
	// again -- see package doc.
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
