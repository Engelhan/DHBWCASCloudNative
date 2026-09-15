package config

import "testing"

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_NAME", "ticketflow")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "pass")
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
	if cfg.DBPort != "5432" {
		t.Errorf("DBPort = %q, want %q", cfg.DBPort, "5432")
	}
	if cfg.DBSSLMode != "disable" {
		t.Errorf("DBSSLMode = %q, want %q", cfg.DBSSLMode, "disable")
	}
}

func TestDatabaseURL(t *testing.T) {
	cfg := Config{
		DBHost: "db.example", DBPort: "5432", DBName: "ticketflow",
		DBUser: "user", DBPassword: "p@ss/word", DBSSLMode: "require",
	}
	want := "postgres://user:p%40ss%2Fword@db.example:5432/ticketflow?sslmode=require"
	if got := cfg.DatabaseURL(); got != want {
		t.Errorf("DatabaseURL() = %q, want %q", got, want)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := []string{
		"DB_HOST", "DB_NAME", "DB_USER", "DB_PASSWORD", "KAFKA_BROKERS", "RESERVATION_TOPIC", "RESULT_TOPIC", "CONSUMER_GROUP", "REGISTRY_URL",
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
