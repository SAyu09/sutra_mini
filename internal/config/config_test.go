package config_test

import (
	"os"
	"testing"
	"time"

	"sutra-mini/internal/config"
)

func TestMain(m *testing.M) {
	config.AutoLoadDotEnv = false
	os.Exit(m.Run())
}

func clearEnv() {
	vars := []string{
		"DATABASE_URL", "TEST_DATABASE_URL", "API_ADDR", "API_URL",
		"MOCK_ADDR", "MOCK_URL", "ERROR_RATE", "HANG_RATE", "HANG_DURATION",
		"MAX_ATTEMPTS", "BACKOFF_BASE", "BACKOFF_JITTER", "HTTP_TIMEOUT",
		"LEASE", "POLL_INTERVAL", "CONCURRENCY",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

func TestLoadAPIConfig_MissingDBURL(t *testing.T) {
	clearEnv()
	_, err := config.LoadAPIConfig()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}

func TestLoadAPIConfig_Valid(t *testing.T) {
	clearEnv()
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost:5432/db" {
		t.Errorf("expected DATABASE_URL to match, got %s", cfg.DatabaseURL)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("expected default Addr :8080, got %s", cfg.Addr)
	}
}

func TestLoadWorkerConfig_MissingRequired(t *testing.T) {
	clearEnv()
	_, err := config.LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}

	os.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	_, err = config.LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error when MOCK_URL is missing")
	}
}

func TestLoadWorkerConfig_LeaseValidation(t *testing.T) {
	clearEnv()
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	os.Setenv("MOCK_URL", "http://localhost:8081")
	os.Setenv("HTTP_TIMEOUT", "10s")
	os.Setenv("LEASE", "15s") // 15s is not > 2 * 10s (20s)

	_, err := config.LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error when LEASE <= 2 * HTTP_TIMEOUT")
	}
}

func TestLoadWorkerConfig_Defaults(t *testing.T) {
	clearEnv()
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	os.Setenv("MOCK_URL", "http://localhost:8081")

	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", cfg.MaxAttempts)
	}
	if cfg.BackoffBase != 2*time.Second {
		t.Errorf("expected BackoffBase=2s, got %v", cfg.BackoffBase)
	}
	if cfg.BackoffJitter != 0.20 {
		t.Errorf("expected BackoffJitter=0.2, got %v", cfg.BackoffJitter)
	}
	if cfg.HTTPTimeout != 3*time.Second {
		t.Errorf("expected HTTPTimeout=3s, got %v", cfg.HTTPTimeout)
	}
	if cfg.Lease != 15*time.Second {
		t.Errorf("expected Lease=15s, got %v", cfg.Lease)
	}
	if cfg.PollInterval != 1*time.Second {
		t.Errorf("expected PollInterval=1s, got %v", cfg.PollInterval)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("expected Concurrency=5, got %d", cfg.Concurrency)
	}
}

func TestLoadMockConfig_Defaults(t *testing.T) {
	clearEnv()
	cfg, err := config.LoadMockConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":8081" {
		t.Errorf("expected default Addr=:8081, got %s", cfg.Addr)
	}
	if cfg.ErrorRate != 0.20 {
		t.Errorf("expected ErrorRate=0.20, got %v", cfg.ErrorRate)
	}
	if cfg.HangRate != 0.10 {
		t.Errorf("expected HangRate=0.10, got %v", cfg.HangRate)
	}
	if cfg.HangDuration != 5*time.Second {
		t.Errorf("expected HangDuration=5s, got %v", cfg.HangDuration)
	}
}

func TestLoadDotEnv(t *testing.T) {
	clearEnv()
	tmpFile, err := os.CreateTemp("", "test_dotenv_*.env")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "DATABASE_URL=\"postgres://test-user:pass@host/db\"\n# A comment\nAPI_ADDR=:9090\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	config.LoadDotEnv(tmpFile.Name())

	if os.Getenv("DATABASE_URL") != "postgres://test-user:pass@host/db" {
		t.Errorf("expected DATABASE_URL to be set, got %q", os.Getenv("DATABASE_URL"))
	}
	if os.Getenv("API_ADDR") != ":9090" {
		t.Errorf("expected API_ADDR to be :9090, got %q", os.Getenv("API_ADDR"))
	}
}
