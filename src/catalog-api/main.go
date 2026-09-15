package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"ticketflow/catalog-api/internal/api"
	"ticketflow/catalog-api/internal/db"
	"ticketflow/catalog-api/internal/reservation"
	"ticketflow/catalog-api/internal/schemareg"
)

func main() {
	setupLogger(getenv("LOG_LEVEL", "info"))

	databaseURL := buildDatabaseURL()
	kafkaBrokers := strings.Split(requireEnv("KAFKA_BROKERS"), ",")
	reservationTopic := requireEnv("RESERVATION_TOPIC")
	registryURL := requireEnv("REGISTRY_URL")
	registryGroupID := getenv("REGISTRY_GROUP_ID", "ticketflow")
	corsOrigin := getenv("CORS_ALLOWED_ORIGIN", "*")
	port := getenv("PORT", "8080")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	database, err := db.Connect(ctx, databaseURL)
	if err != nil {
		slog.Error("connect to database failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	schema, err := schemareg.New(schemareg.Config{
		BaseURL:    registryURL,
		GroupID:    registryGroupID,
		ArtifactID: "reservation-requested-value",
		Logger:     slog.Default(),
	}, schemareg.RequestedSchemaJSON)
	if err != nil {
		slog.Error("compile embedded reservation-requested schema failed", "error", err)
		os.Exit(1)
	}
	if err := schema.Resolve(ctx); err != nil {
		slog.Error("resolve reservation-requested schema globalId failed", "error", err)
		os.Exit(1)
	}

	producer := reservation.NewProducer(kafkaBrokers, reservationTopic, schema)
	if err := database.Migrate(ctx); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	defer producer.Close()

	a := api.New(database, producer, schema, corsOrigin)

	router := a.Routes()
	router.Handle("/metrics", promhttp.Handler())

	slog.Info("catalog-api starting", "port", port, "reservation_topic", reservationTopic)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required environment variable", "key", key)
		os.Exit(1)
	}
	return v
}

// buildDatabaseURL setzt den Connection-String aus Einzelteilen zusammen, statt einen fertigen
// DATABASE_URL-String zu erwarten - CNPG-Cluster (siehe charts/postgres-cluster) liefern nur ein
// Secret mit getrennten username/password-Feldern, kein zusammengesetztes Connection-String-Secret
// (Team-Entscheidung: das baut sich jede Anwendung selbst, da das Format je Anwendung/Treiber
// unterschiedlich sein kann). DB_USER/DB_PASSWORD kommen aus einem Secret, DB_HOST/DB_PORT/
// DB_NAME/DB_SSLMODE aus einer ConfigMap (kein Geheimnis).
func buildDatabaseURL() string {
	host := requireEnv("DB_HOST")
	port := getenv("DB_PORT", "5432")
	name := requireEnv("DB_NAME")
	user := requireEnv("DB_USER")
	password := requireEnv("DB_PASSWORD")
	sslMode := getenv("DB_SSLMODE", "disable")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     fmt.Sprintf("%s:%s", host, port),
		Path:     "/" + name,
		RawQuery: "sslmode=" + sslMode,
	}
	return u.String()
}
