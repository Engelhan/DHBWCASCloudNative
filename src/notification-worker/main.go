// Command notification-worker sends notifications for reservation results (second consumer group).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"ticketflow/notification-worker/internal/config"
	"ticketflow/notification-worker/internal/kafkaconsumer"
	"ticketflow/notification-worker/internal/kafkasasl"
	"ticketflow/notification-worker/internal/kafkatls"
	"ticketflow/notification-worker/internal/notify"
	"ticketflow/notification-worker/internal/schemareg"
	"ticketflow/notification-worker/internal/server"
)

func main() {
	cfg, err := config.Load()
	logger := newLogger(cfg)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	tlsConfig, err := kafkatls.Build(cfg)
	if err != nil {
		logger.Error("invalid kafka TLS configuration", "error", err)
		os.Exit(1)
	}

	kafkaSASL, err := kafkasasl.Build(cfg)
	if err != nil {
		logger.Error("invalid kafka SASL configuration", "error", err)
		os.Exit(1)
	}

	logFallback := notify.LogNotifier{Logger: logger}

	var dispatcher *notify.Dispatcher
	if cfg.SMTPEnabled {
		dispatcher = notify.NewDispatcher(notify.NewSMTPNotifier(cfg), notify.ChannelSMTP)
	} else {
		dispatcher = notify.NewDispatcher(logFallback, notify.ChannelLog)
	}

	schema, err := schemareg.New(schemareg.Config{
		BaseURL:    cfg.RegistryURL,
		GroupID:    cfg.RegistryGroupID,
		ArtifactID: "reservation-result-value",
		Logger:     logger,
	}, schemareg.ResultSchemaJSON)
	if err != nil {
		logger.Error("compile embedded reservation-result schema failed", "error", err)
		os.Exit(1)
	}
	resolveCtx, cancelResolve := context.WithTimeout(context.Background(), 10*time.Second)
	if err := schema.Resolve(resolveCtx); err != nil {
		cancelResolve()
		logger.Error("resolve reservation-result schema globalId failed", "error", err)
		os.Exit(1)
	}
	cancelResolve()

	var ready atomic.Bool
	srv := server.New(cfg.HTTPAddr, &ready, schema)
	consumer := kafkaconsumer.New(cfg, tlsConfig, kafkaSASL, dispatcher, logFallback, schema, logger, &ready)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := srv.Run(); err != nil {
			logger.Error("http server failed", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		logger.Info("kafka consumer starting", "topic", cfg.ResultTopic, "group", cfg.ConsumerGroup, "smtpEnabled", cfg.SMTPEnabled)
		if err := consumer.Run(ctx); err != nil {
			logger.Error("kafka consumer stopped with error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
	if err := consumer.Close(); err != nil {
		logger.Error("kafka consumer close error", "error", err)
	}

	wg.Wait()
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
