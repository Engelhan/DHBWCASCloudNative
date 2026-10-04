// Package config loads and validates the environment configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// TLSMode selects how the SMTP client secures its connection to the relay.
type TLSMode string

const (
	TLSModeNone     TLSMode = "none"
	TLSModeStartTLS TLSMode = "starttls"
	TLSModeTLS      TLSMode = "tls"
)

type Config struct {
	KafkaBrokers  []string
	ResultTopic   string
	ConsumerGroup string
	// Empty DLTTopic disables dead-letter publishing; undeliverable messages are never committed
	// either way.
	DLTTopic string

	NotifyMaxRetries   int
	NotifyRetryBackoff time.Duration

	// RegistryURL is the Apicurio v3 REST root.
	RegistryURL     string
	RegistryGroupID string

	LogLevel string
	HTTPAddr string

	KafkaTLSEnabled    bool
	KafkaTLSCACert     string
	KafkaTLSClientCert string
	KafkaTLSClientKey  string

	// Optional SCRAM-SHA-512 auth as KafkaUser notification-worker (independent of mTLS).
	KafkaSASLEnabled  bool
	KafkaSASLUsername string
	KafkaSASLPassword string

	SMTPEnabled  bool
	SMTPHost     string
	SMTPPort     int
	SMTPFrom     string
	SMTPTLSMode  TLSMode
	SMTPUsername string
	SMTPPassword string
}

// Load reads and validates the configuration; it fails fast on insecure or incomplete settings.
func Load() (Config, error) {
	cfg := Config{
		ResultTopic:        os.Getenv("RESULT_TOPIC"),
		ConsumerGroup:      os.Getenv("CONSUMER_GROUP"),
		DLTTopic:           os.Getenv("DLT_TOPIC"),
		RegistryURL:        os.Getenv("REGISTRY_URL"),
		RegistryGroupID:    getenvDefault("REGISTRY_GROUP_ID", "ticketflow"),
		LogLevel:           getenvDefault("LOG_LEVEL", "info"),
		HTTPAddr:           getenvDefault("HTTP_ADDR", ":8080"),
		KafkaTLSCACert:     os.Getenv("KAFKA_TLS_CA_CERT"),
		KafkaTLSClientCert: os.Getenv("KAFKA_TLS_CLIENT_CERT"),
		KafkaTLSClientKey:  os.Getenv("KAFKA_TLS_CLIENT_KEY"),
		KafkaSASLUsername:  os.Getenv("KAFKA_SASL_USERNAME"),
		KafkaSASLPassword:  os.Getenv("KAFKA_SASL_PASSWORD"),
		SMTPTLSMode:        TLSMode(getenvDefault("SMTP_TLS_MODE", string(TLSModeStartTLS))),
		SMTPHost:           os.Getenv("SMTP_HOST"),
		SMTPFrom:           os.Getenv("SMTP_FROM"),
		SMTPUsername:       os.Getenv("SMTP_USERNAME"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
	}

	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		for _, b := range strings.Split(brokers, ",") {
			if b = strings.TrimSpace(b); b != "" {
				cfg.KafkaBrokers = append(cfg.KafkaBrokers, b)
			}
		}
	}

	var err error
	if cfg.SMTPEnabled, err = getenvBool("SMTP_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.KafkaTLSEnabled, err = getenvBool("KAFKA_TLS_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.KafkaSASLEnabled, err = getenvBool("KAFKA_SASL_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.SMTPPort, err = getenvInt("SMTP_PORT", 587); err != nil {
		return Config{}, err
	}
	if cfg.NotifyMaxRetries, err = getenvInt("NOTIFY_MAX_RETRIES", 3); err != nil {
		return Config{}, err
	}
	if cfg.NotifyRetryBackoff, err = getenvDuration("NOTIFY_RETRY_BACKOFF", 2*time.Second); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if len(c.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required")
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
	if c.NotifyMaxRetries < 0 {
		return fmt.Errorf("NOTIFY_MAX_RETRIES must be >= 0")
	}

	switch c.SMTPTLSMode {
	case TLSModeNone, TLSModeStartTLS, TLSModeTLS:
	default:
		return fmt.Errorf("SMTP_TLS_MODE must be one of none|starttls|tls, got %q", c.SMTPTLSMode)
	}
	if c.SMTPEnabled {
		if c.SMTPHost == "" {
			return fmt.Errorf("SMTP_HOST is required when SMTP_ENABLED=true")
		}
		if c.SMTPFrom == "" {
			return fmt.Errorf("SMTP_FROM is required when SMTP_ENABLED=true")
		}
	}
	if c.SMTPTLSMode == TLSModeNone && c.SMTPUsername != "" {
		return fmt.Errorf("SMTP_USERNAME must not be set when SMTP_TLS_MODE=none (refusing to send credentials over an unencrypted connection)")
	}

	if c.KafkaTLSEnabled {
		if c.KafkaTLSCACert == "" || c.KafkaTLSClientCert == "" || c.KafkaTLSClientKey == "" {
			return fmt.Errorf("KAFKA_TLS_CA_CERT, KAFKA_TLS_CLIENT_CERT and KAFKA_TLS_CLIENT_KEY are all required when KAFKA_TLS_ENABLED=true")
		}
	}
	if c.KafkaSASLEnabled {
		if c.KafkaSASLUsername == "" || c.KafkaSASLPassword == "" {
			return fmt.Errorf("KAFKA_SASL_USERNAME and KAFKA_SASL_PASSWORD are both required when KAFKA_SASL_ENABLED=true")
		}
	}

	return nil
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
