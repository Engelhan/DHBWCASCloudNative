// Package config loads reservation-worker's configuration from environment
// variables (injected via ConfigMap/Secret in the Helm chart, see
// charts/reservation-worker/) and validates it at startup.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string

	KafkaBrokers     []string
	ReservationTopic string
	ResultTopic      string
	ConsumerGroup    string

	ProcessMaxRetries   int
	ProcessRetryBackoff time.Duration

	LogLevel string
	HTTPAddr string
}

// Load reads and validates configuration from the environment. It fails
// fast (returns an error, callers should exit non-zero) rather than let an
// incomplete configuration fail confusingly later at runtime.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		ReservationTopic: os.Getenv("RESERVATION_TOPIC"),
		ResultTopic:      os.Getenv("RESULT_TOPIC"),
		ConsumerGroup:    os.Getenv("CONSUMER_GROUP"),
		LogLevel:         getenvDefault("LOG_LEVEL", "info"),
		HTTPAddr:         getenvDefault("HTTP_ADDR", ":8080"),
	}

	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		for _, b := range strings.Split(brokers, ",") {
			if b = strings.TrimSpace(b); b != "" {
				cfg.KafkaBrokers = append(cfg.KafkaBrokers, b)
			}
		}
	}

	var err error
	if cfg.ProcessMaxRetries, err = getenvInt("PROCESS_MAX_RETRIES", 3); err != nil {
		return Config{}, err
	}
	if cfg.ProcessRetryBackoff, err = getenvDuration("PROCESS_RETRY_BACKOFF", 2*time.Second); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if len(c.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required")
	}
	if c.ReservationTopic == "" {
		return fmt.Errorf("RESERVATION_TOPIC is required")
	}
	if c.ResultTopic == "" {
		return fmt.Errorf("RESULT_TOPIC is required")
	}
	if c.ConsumerGroup == "" {
		return fmt.Errorf("CONSUMER_GROUP is required")
	}
	if c.ProcessMaxRetries < 0 {
		return fmt.Errorf("PROCESS_MAX_RETRIES must be >= 0")
	}
	return nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid int %q: %w", key, v, err)
	}
	return n, nil
}

func getenvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, v, err)
	}
	return d, nil
}
