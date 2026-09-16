package config

import "testing"

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("KAFKA_BROKERS", "broker-1:9092,broker-2:9092")
	t.Setenv("RESULT_TOPIC", "reservation-results")
	t.Setenv("CONSUMER_GROUP", "notification-worker")
	t.Setenv("REGISTRY_URL", "http://apicurio.kafka.svc:8080/apis/registry/v3")
}

func TestLoad_Defaults(t *testing.T) {
	baseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if got, want := len(cfg.KafkaBrokers), 2; got != want {
		t.Errorf("KafkaBrokers len = %d, want %d", got, want)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.SMTPTLSMode != TLSModeStartTLS {
		t.Errorf("SMTPTLSMode = %q, want %q", cfg.SMTPTLSMode, TLSModeStartTLS)
	}
	if cfg.SMTPPort != 587 {
		t.Errorf("SMTPPort = %d, want 587", cfg.SMTPPort)
	}
	if cfg.NotifyMaxRetries != 3 {
		t.Errorf("NotifyMaxRetries = %d, want 3", cfg.NotifyMaxRetries)
	}
	if cfg.SMTPEnabled {
		t.Error("SMTPEnabled = true, want false by default")
	}
	if cfg.KafkaTLSEnabled {
		t.Error("KafkaTLSEnabled = true, want false by default")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	if _, err := Load(); err == nil {
		t.Fatal("Load() with no env vars set: expected error, got nil")
	}
}

func TestLoad_SMTPEnabledRequiresHostAndFrom(t *testing.T) {
	baseEnv(t)
	t.Setenv("SMTP_ENABLED", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when SMTP_ENABLED=true without SMTP_HOST/SMTP_FROM")
	}

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() unexpected error once host/from set: %v", err)
	}
}

func TestLoad_PlaintextSMTPRejectsCredentials(t *testing.T) {
	baseEnv(t)
	t.Setenv("SMTP_TLS_MODE", "none")
	t.Setenv("SMTP_USERNAME", "someone")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when SMTP_TLS_MODE=none with SMTP_USERNAME set")
	}
}

func TestLoad_KafkaTLSEnabledRequiresAllThreeCerts(t *testing.T) {
	baseEnv(t)
	t.Setenv("KAFKA_TLS_ENABLED", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when KAFKA_TLS_ENABLED=true without CA/client cert/key")
	}

	t.Setenv("KAFKA_TLS_CA_CERT", "ca-pem")
	t.Setenv("KAFKA_TLS_CLIENT_CERT", "cert-pem")
	t.Setenv("KAFKA_TLS_CLIENT_KEY", "key-pem")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() unexpected error once all three set: %v", err)
	}
}

func TestLoad_KafkaSASLEnabledRequiresUsernameAndPassword(t *testing.T) {
	baseEnv(t)
	t.Setenv("KAFKA_SASL_ENABLED", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when KAFKA_SASL_ENABLED=true without username/password")
	}

	t.Setenv("KAFKA_SASL_USERNAME", "notification-worker")
	t.Setenv("KAFKA_SASL_PASSWORD", "secret")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() unexpected error once username/password set: %v", err)
	}
}

func TestLoad_InvalidTLSMode(t *testing.T) {
	baseEnv(t)
	t.Setenv("SMTP_TLS_MODE", "bogus")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid SMTP_TLS_MODE")
	}
}
