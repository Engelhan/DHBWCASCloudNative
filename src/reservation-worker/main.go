// Command reservation-worker is TicketFlow's Kafka consumer that performs
// the actual, transactional seat booking. See
// src/reservation-worker/README.md for the config contract and
// internal/booking for the seat-claim/idempotency semantics.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ticketflow/reservation-worker/internal/config"
	"ticketflow/reservation-worker/internal/db"
	"ticketflow/reservation-worker/internal/kafkaconsumer"
	"ticketflow/reservation-worker/internal/migrate"
	"ticketflow/reservation-worker/internal/producer"
	"ticketflow/reservation-worker/internal/schemareg"
	"ticketflow/reservation-worker/internal/server"
)

func main() {
	cfg, err := config.Load()
	logger := newLogger(cfg)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()

	database, err := db.Connect(connectCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect to database failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := migrate.Run(connectCtx, database.Pool()); err != nil {
		logger.Error("run database migrations failed", "error", err)
		os.Exit(1)
	}

	reqSchema, err := schemareg.New(schemareg.Config{
		BaseURL:    cfg.RegistryURL,
		GroupID:    cfg.RegistryGroupID,
		ArtifactID: "reservation-requested-value",
		Logger:     logger,
	}, schemareg.RequestedSchemaJSON)
	if err != nil {
		logger.Error("compile embedded reservation-requested schema failed", "error", err)
		os.Exit(1)
	}
	resultSchema, err := schemareg.New(schemareg.Config{
		BaseURL:    cfg.RegistryURL,
		GroupID:    cfg.RegistryGroupID,
		ArtifactID: "reservation-result-value",
		Logger:     logger,
	}, schemareg.ResultSchemaJSON)
	if err != nil {
		logger.Error("compile embedded reservation-result schema failed", "error", err)
		os.Exit(1)
	}
	if err := reqSchema.Resolve(connectCtx); err != nil {
		logger.Error("resolve reservation-requested schema globalId failed", "error", err)
		os.Exit(1)
	}
	if err := resultSchema.Resolve(connectCtx); err != nil {
		logger.Error("resolve reservation-result schema globalId failed", "error", err)
		os.Exit(1)
	}

	publisher := producer.New(cfg.KafkaBrokers, cfg.ResultTopic, resultSchema)
	defer publisher.Close()

	srv := server.New(cfg.HTTPAddr, database, publisher, reqSchema, resultSchema)
	consumer := kafkaconsumer.New(cfg, database, publisher, reqSchema, logger)

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
		logger.Info("kafka consumer starting", "topic", cfg.ReservationTopic, "group", cfg.ConsumerGroup)
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
