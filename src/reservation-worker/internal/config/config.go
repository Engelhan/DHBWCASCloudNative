// Package config loads reservation-worker's configuration from environment
// variables (injected via ConfigMap/Secret in the Helm chart, see
// charts/reservation-worker/) and validates it at startup.
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
	// DB_HOST/DB_PORT/DB_NAME/DB_SSLMODE come from a ConfigMap, DB_USER/DB_PASSWORD from a
	// Secret -- CNPG (charts/postgres-cluster) only provides a username/password Secret, not a
	// pre-built connection string, so the application builds it itself (same team decision as
	// catalog-api's buildDatabaseURL, see its main.go). Kept as separate fields + a DatabaseURL()
	// method rather than a single DATABASE_URL env var so the construction is unit-testable here.
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

	// SASL/SCRAM-SHA-512 auth against the Strimzi-managed KafkaUser "reservation-worker" (see
	// charts/kafka-cluster templates/messaging/kafka-users.yaml). Optional: local docker-compose
	// has no Strimzi listener/ACLs, so KafkaSASLEnabled defaults to false there.
	KafkaSASLEnabled  bool
	KafkaSASLUsername string
	KafkaSASLPassword string

	ProcessMaxRetries   int
	ProcessRetryBackoff time.Duration

	// RegistryURL is Apicurio Registry's v3 REST API root, e.g.
	// "http://apicurio.kafka.svc:8080/apis/registry/v3" (no trailing slash). See
	// internal/schemareg.
	RegistryURL     string
	RegistryGroupID string

	LogLevel string
	HTTPAddr string
}

// Load reads and validates configuration from the environment. It fails
// fast (returns an error, callers should exit non-zero) rather than let an
// incomplete configuration fail confusingly later at runtime.
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

// DatabaseURL builds the Postgres connection string from the separate host/port/name/user/
// password/sslmode fields. See the Config struct doc for why it's built here instead of read
// from a single DATABASE_URL env var.
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
