package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"sutra-mini/internal/config"
	"sutra-mini/internal/db"
	"sutra-mini/migrations"
)

var migrateOnce sync.Once

// SetupTestDB connects to TEST_DATABASE_URL, ensures migrations are up,
// and truncates tables between tests. It fails the test immediately if TEST_DATABASE_URL is unset.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	testURL, err := config.LoadTestDatabaseURL()
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not set: integration tests require a live PostgreSQL database (rule 2.8). Error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, testURL, db.DefaultOptions())
	if err != nil {
		t.Fatalf("failed to connect to test database at %s: %v", testURL, err)
	}

	// Run migrations once per test binary execution
	var migrateErr error
	migrateOnce.Do(func() {
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			migrateErr = fmt.Errorf("set dialect: %w", err)
			return
		}
		stdDB := stdlib.OpenDBFromPool(pool)
		defer stdDB.Close()
		if err := goose.Up(stdDB, "."); err != nil {
			migrateErr = fmt.Errorf("apply migrations: %w", err)
			return
		}
	})
	if migrateErr != nil {
		pool.Close()
		t.Fatalf("migration failed: %v", migrateErr)
	}

	// Acquire session-level advisory lock so concurrent test packages serialize safely
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to acquire connection for advisory lock: %v", err)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(987654321);"); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("failed to acquire test advisory lock: %v", err)
	}

	// Truncate tables and restart identity cascade
	truncateQuery := `TRUNCATE TABLE vouchers, jobwork_receipts, jobwork_issues, parties, skus RESTART IDENTITY CASCADE;`
	if _, err := conn.Exec(ctx, truncateQuery); err != nil {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(987654321);")
		conn.Release()
		pool.Close()
		t.Fatalf("failed to truncate test tables: %v", err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(987654321);")
		conn.Release()
		pool.Close()
	})

	return pool
}

// CreateFixtureIssue inserts a test SKU, party, and an open jobwork issue with the given qty and rate.
func CreateFixtureIssue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, qty int, ratePaise int64) int64 {
	t.Helper()

	var skuID int64
	skuCode := fmt.Sprintf("SKU-TEST-%d", time.Now().UnixNano())
	err := pool.QueryRow(ctx, `
		INSERT INTO skus (code, name, image_url)
		VALUES ($1, 'Test SKU', 'https://example.com/test.jpg')
		RETURNING id
	`, skuCode).Scan(&skuID)
	if err != nil {
		t.Fatalf("failed to insert fixture sku: %v", err)
	}

	var partyID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO parties (name, phone)
		VALUES ('Test Jobworker', '+91 99999 88888')
		RETURNING id
	`).Scan(&partyID)
	if err != nil {
		t.Fatalf("failed to insert fixture party: %v", err)
	}

	var issueID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO jobwork_issues (party_id, sku_id, qty, received_qty, rate_paise, issued_on)
		VALUES ($1, $2, $3, 0, $4, '2026-10-10')
		RETURNING id
	`, partyID, skuID, qty, ratePaise).Scan(&issueID)
	if err != nil {
		t.Fatalf("failed to insert fixture issue: %v", err)
	}

	return issueID
}

// SetVoucherStatus updates the status of a voucher for testing purposes.
func SetVoucherStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, status string) {
	t.Helper()
	query := `UPDATE vouchers SET status = $1 WHERE id = $2`
	if _, err := pool.Exec(ctx, query, status, id); err != nil {
		t.Fatalf("failed to update voucher status: %v", err)
	}
}
