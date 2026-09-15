package config

import "testing"

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/ticketflow")
	t.Setenv("KAFKA_BROKERS", "broker-1:9092,broker-2:9092")
	t.Setenv("RESERVATION_TOPIC", "reservation-requested")
	t.Setenv("RESULT_TOPIC", "reservation-result")
	t.Setenv("CONSUMER_GROUP", "reservation-worker")
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
	if cfg.ProcessMaxRetries != 3 {
		t.Errorf("ProcessMaxRetries = %d, want 3", cfg.ProcessMaxRetries)
	}
	if cfg.ProcessRetryBackoff.String() != "2s" {
		t.Errorf("ProcessRetryBackoff = %s, want 2s", cfg.ProcessRetryBackoff)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := []string{
		"DATABASE_URL", "KAFKA_BROKERS", "RESERVATION_TOPIC", "RESULT_TOPIC", "CONSUMER_GROUP", "REGISTRY_URL",
	}

	for _, missing := range tests {
		t.Run(missing, func(t *testing.T) {
			baseEnv(t)
			t.Setenv(missing, "")

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s unset: expected error, got nil", missing)
			}
		})
	}
}

func TestLoad_InvalidProcessMaxRetries(t *testing.T) {
	baseEnv(t)
	t.Setenv("PROCESS_MAX_RETRIES", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with invalid PROCESS_MAX_RETRIES: expected error, got nil")
	}
}

func TestLoad_InvalidProcessRetryBackoff(t *testing.T) {
	baseEnv(t)
	t.Setenv("PROCESS_RETRY_BACKOFF", "not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with invalid PROCESS_RETRY_BACKOFF: expected error, got nil")
	}
}
