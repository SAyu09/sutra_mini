package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// APIConfig holds configuration for the HTTP API service.
type APIConfig struct {
	DatabaseURL string
	Addr        string
}

// WorkerConfig holds configuration for the sync worker.
type WorkerConfig struct {
	DatabaseURL   string
	MockURL       string
	MaxAttempts   int
	BackoffBase   time.Duration
	BackoffJitter float64
	HTTPTimeout   time.Duration
	Lease         time.Duration
	PollInterval  time.Duration
	Concurrency   int
}

// MockConfig holds configuration for the mock accounting server.
type MockConfig struct {
	Addr         string
	ErrorRate    float64
	HangRate     float64
	HangDuration time.Duration
}

// MigrateConfig holds configuration for running database migrations.
type MigrateConfig struct {
	DatabaseURL string
}

// AutoLoadDotEnv controls whether LoadDotEnv is automatically called by Load*Config functions.
var AutoLoadDotEnv = true

// LoadAPIConfig loads and validates configuration for the HTTP API service.
func LoadAPIConfig() (*APIConfig, error) {
	if AutoLoadDotEnv {
		LoadDotEnv()
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	addr := strings.TrimSpace(os.Getenv("API_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	return &APIConfig{
		DatabaseURL: dbURL,
		Addr:        addr,
	}, nil
}

// LoadWorkerConfig loads and validates configuration for the sync worker.
func LoadWorkerConfig() (*WorkerConfig, error) {
	if AutoLoadDotEnv {
		LoadDotEnv()
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	mockURL := strings.TrimSpace(os.Getenv("MOCK_URL"))
	if mockURL == "" {
		return nil, fmt.Errorf("MOCK_URL is required")
	}

	maxAttempts := 5
	if val := os.Getenv("MAX_ATTEMPTS"); val != "" {
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("MAX_ATTEMPTS must be an integer >= 1, got %q", val)
		}
		maxAttempts = n
	}

	backoffBase := 2 * time.Second
	if val := os.Getenv("BACKOFF_BASE"); val != "" {
		d, err := time.ParseDuration(strings.TrimSpace(val))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("BACKOFF_BASE must be a positive duration (e.g. 2s), got %q", val)
		}
		backoffBase = d
	}

	backoffJitter := 0.20
	if val := os.Getenv("BACKOFF_JITTER"); val != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return nil, fmt.Errorf("BACKOFF_JITTER must be a float between 0.0 and 1.0, got %q", val)
		}
		backoffJitter = f
	}

	httpTimeout := 3 * time.Second
	if val := os.Getenv("HTTP_TIMEOUT"); val != "" {
		d, err := time.ParseDuration(strings.TrimSpace(val))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("HTTP_TIMEOUT must be a positive duration (e.g. 3s), got %q", val)
		}
		httpTimeout = d
	}

	lease := 15 * time.Second
	if val := os.Getenv("LEASE"); val != "" {
		d, err := time.ParseDuration(strings.TrimSpace(val))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("LEASE must be a positive duration (e.g. 15s), got %q", val)
		}
		lease = d
	}

	if lease <= 2*httpTimeout {
		return nil, fmt.Errorf("LEASE (%v) must be greater than 2 x HTTP_TIMEOUT (%v)", lease, 2*httpTimeout)
	}

	pollInterval := 1 * time.Second
	if val := os.Getenv("POLL_INTERVAL"); val != "" {
		d, err := time.ParseDuration(strings.TrimSpace(val))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("POLL_INTERVAL must be a positive duration (e.g. 1s), got %q", val)
		}
		pollInterval = d
	}

	concurrency := 5
	if val := os.Getenv("CONCURRENCY"); val != "" {
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("CONCURRENCY must be an integer >= 1, got %q", val)
		}
		concurrency = n
	}

	return &WorkerConfig{
		DatabaseURL:   dbURL,
		MockURL:       mockURL,
		MaxAttempts:   maxAttempts,
		BackoffBase:   backoffBase,
		BackoffJitter: backoffJitter,
		HTTPTimeout:   httpTimeout,
		Lease:         lease,
		PollInterval:  pollInterval,
		Concurrency:   concurrency,
	}, nil
}

// LoadMockConfig loads and validates configuration for the mock accounting server.
func LoadMockConfig() (*MockConfig, error) {
	if AutoLoadDotEnv {
		LoadDotEnv()
	}

	addr := strings.TrimSpace(os.Getenv("MOCK_ADDR"))
	if addr == "" {
		addr = ":8081"
	}

	errorRate := 0.20
	if val := os.Getenv("ERROR_RATE"); val != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return nil, fmt.Errorf("ERROR_RATE must be between 0.0 and 1.0, got %q", val)
		}
		errorRate = f
	}

	hangRate := 0.10
	if val := os.Getenv("HANG_RATE"); val != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return nil, fmt.Errorf("HANG_RATE must be between 0.0 and 1.0, got %q", val)
		}
		hangRate = f
	}

	hangDuration := 5 * time.Second
	if val := os.Getenv("HANG_DURATION"); val != "" {
		d, err := time.ParseDuration(strings.TrimSpace(val))
		if err != nil || d < 0 {
			return nil, fmt.Errorf("HANG_DURATION must be a non-negative duration (e.g. 5s), got %q", val)
		}
		hangDuration = d
	}

	return &MockConfig{
		Addr:         addr,
		ErrorRate:    errorRate,
		HangRate:     hangRate,
		HangDuration: hangDuration,
	}, nil
}

// LoadMigrateConfig loads and validates configuration for migration tasks.
func LoadMigrateConfig() (*MigrateConfig, error) {
	if AutoLoadDotEnv {
		LoadDotEnv()
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return &MigrateConfig{
		DatabaseURL: dbURL,
	}, nil
}

// LoadTestDatabaseURL returns TEST_DATABASE_URL or an error if missing.
func LoadTestDatabaseURL() (string, error) {
	if AutoLoadDotEnv {
		LoadDotEnv()
	}

	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL is required for integration tests")
	}
	return url, nil
}
