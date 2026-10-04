// Package config loads and validates the environment configuration.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// CNPG provides username/password separately, so the DSN is built here (see DatabaseURL).
	DBHost     string
	DBPort     string
	DBName     string
	DBSSLMode  string
	DBUser     string
	DBPassword string

	KafkaBrokers     []string
	ReservationTopic string
	ResultTopic      string
	ConsumerGroup    string

	// Optional SCRAM-SHA-512 auth as KafkaUser reservation-worker.
	KafkaSASLEnabled  bool
	KafkaSASLUsername string
	KafkaSASLPassword string

	ProcessMaxRetries   int
	ProcessRetryBackoff time.Duration

	// RegistryURL is the Apicurio v3 REST root.
	RegistryURL     string
	RegistryGroupID string

	LogLevel string
	HTTPAddr string
}

// Load reads and validates the configuration; it fails fast on missing values.
func Load() (Config, error) {
	cfg := Config{
		DBHost:            os.Getenv("DB_HOST"),
		DBPort:            getenvDefault("DB_PORT", "5432"),
		DBName:            os.Getenv("DB_NAME"),
		DBSSLMode:         getenvDefault("DB_SSLMODE", "disable"),
		DBUser:            os.Getenv("DB_USER"),
		DBPassword:        os.Getenv("DB_PASSWORD"),
		ReservationTopic:  os.Getenv("RESERVATION_TOPIC"),
		ResultTopic:       os.Getenv("RESULT_TOPIC"),
		ConsumerGroup:     os.Getenv("CONSUMER_GROUP"),
		KafkaSASLUsername: os.Getenv("KAFKA_SASL_USERNAME"),
		KafkaSASLPassword: os.Getenv("KAFKA_SASL_PASSWORD"),
		RegistryURL:       os.Getenv("REGISTRY_URL"),
		RegistryGroupID:   getenvDefault("REGISTRY_GROUP_ID", "ticketflow"),
		LogLevel:          getenvDefault("LOG_LEVEL", "info"),
		HTTPAddr:          getenvDefault("HTTP_ADDR", ":8080"),
	}

	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		for _, b := range strings.Split(brokers, ",") {
			if b = strings.TrimSpace(b); b != "" {
				cfg.KafkaBrokers = append(cfg.KafkaBrokers, b)
			}
		}
	}

	var err error
	if cfg.KafkaSASLEnabled, err = getenvBool("KAFKA_SASL_ENABLED", false); err != nil {
		return Config{}, err
	}
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
	if c.DBHost == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	if c.DBUser == "" {
		return fmt.Errorf("DB_USER is required")
	}
	if c.DBPassword == "" {
		return fmt.Errorf("DB_PASSWORD is required")
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
	if c.RegistryURL == "" {
		return fmt.Errorf("REGISTRY_URL is required")
	}
	if c.ProcessMaxRetries < 0 {
		return fmt.Errorf("PROCESS_MAX_RETRIES must be >= 0")
	}
	if c.KafkaSASLEnabled {
		if c.KafkaSASLUsername == "" || c.KafkaSASLPassword == "" {
			return fmt.Errorf("KAFKA_SASL_USERNAME and KAFKA_SASL_PASSWORD are both required when KAFKA_SASL_ENABLED=true")
		}
	}
	return nil
}

// DatabaseURL builds the Postgres DSN from the separate fields.
func (c Config) DatabaseURL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     fmt.Sprintf("%s:%s", c.DBHost, c.DBPort),
		Path:     "/" + c.DBName,
		RawQuery: "sslmode=" + c.DBSSLMode,
	}
	return u.String()
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: invalid bool %q: %w", key, v, err)
	}
	return b, nil
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
