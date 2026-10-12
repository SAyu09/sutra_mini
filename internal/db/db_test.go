package db_test

import (
	"context"
	"testing"
	"time"

	"sutra-mini/internal/config"
	"sutra-mini/internal/db"
)

func TestDatabasePing(t *testing.T) {
	testURL, err := config.LoadTestDatabaseURL()
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not set: integration tests require a live PostgreSQL database (e.g. Neon branch). Error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, testURL, db.DefaultOptions())
	if err != nil {
		t.Fatalf("failed to connect to database at %s: %v", testURL, err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
}
