package worker_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/mock"
	"sutra-mini/internal/testutil"
	"sutra-mini/internal/vouchers"
	"sutra-mini/internal/worker"
)

// helper to insert a valid test receipt and voucher
func createTestVoucher(t *testing.T, ctx context.Context, pool *pgxpool.Pool, issueID int64, clientRef string, status string, attempts int) int64 {
	t.Helper()

	var receiptID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO jobwork_receipts (issue_id, qty, received_on)
		VALUES ($1, 1, '2026-10-12')
		RETURNING id
	`, issueID).Scan(&receiptID)
	if err != nil {
		t.Fatalf("failed to insert test receipt: %v", err)
	}

	var voucherID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO vouchers (type, source_id, amount_paise, status, attempts, client_ref, next_attempt_at)
		VALUES ('jobwork_receipt', $1, 5000, $2, $3, $4, now())
		RETURNING id
	`, receiptID, status, attempts, clientRef).Scan(&voucherID)
	if err != nil {
		t.Fatalf("failed to insert test voucher: %v", err)
	}

	return voucherID
}

// W1: TestTwoWorkersNoDoubleClaimNoDoublePost
// Two worker instances running concurrently against the same DB and mock.
// 10 vouchers inserted into vouchers (status 'pending').
// Both workers start and run until all vouchers are processed (or timeout).
// Enforced by: FOR UPDATE SKIP LOCKED + Lease + Fenced Writes (G4, G5).
func TestTwoWorkersNoDoubleClaimNoDoublePost(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mockServer := mock.NewServer(mock.Options{
		ErrorRate: 0.0,
		HangRate:  0.0,
	})
	httpServer := httptest.NewServer(mockServer.Handler())
	defer httpServer.Close()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 50, 5000)

	const totalVouchers = 10
	for i := 0; i < totalVouchers; i++ {
		clientRef := fmt.Sprintf("W1-REF-%d-%s", i, uuid.NewString())
		createTestVoucher(t, ctx, pool, issueID, clientRef, "pending", 0)
	}

	voucherSvc := vouchers.NewService(pool)
	client := worker.NewHTTPAccountingClient(httpServer.URL, 3*time.Second)

	w1, err := worker.New(worker.Options{
		VoucherService: voucherSvc,
		Client:         client,
		MaxAttempts:    3,
		BackoffBase:    100 * time.Millisecond,
		HTTPTimeout:    2 * time.Second,
		Lease:          5 * time.Second,
		PollInterval:   50 * time.Millisecond,
		Concurrency:    3,
	})
	if err != nil {
		t.Fatalf("failed to create worker 1: %v", err)
	}

	w2, err := worker.New(worker.Options{
		VoucherService: voucherSvc,
		Client:         client,
		MaxAttempts:    3,
		BackoffBase:    100 * time.Millisecond,
		HTTPTimeout:    2 * time.Second,
		Lease:          5 * time.Second,
		PollInterval:   50 * time.Millisecond,
		Concurrency:    3,
	})
	if err != nil {
		t.Fatalf("failed to create worker 2: %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		w1.Start(workerCtx)
	}()
	go func() {
		defer wg.Done()
		w2.Start(workerCtx)
	}()

	// Wait until all 10 vouchers reach 'pushed' status
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var pushedCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers WHERE status = 'pushed'").Scan(&pushedCount)
		if pushedCount == totalVouchers {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Stop workers
	workerCancel()
	wg.Wait()

	// Verify vouchers in database
	var pushedCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers WHERE status = 'pushed' AND external_ref IS NOT NULL").Scan(&pushedCount)
	if err != nil {
		t.Fatalf("query vouchers: %v", err)
	}
	if pushedCount != totalVouchers {
		t.Fatalf("expected %d vouchers to be 'pushed', got %d", totalVouchers, pushedCount)
	}

	// Verify mock accounting server received exactly 10 distinct client_refs
	if regCount := mockServer.RegistryCount(); regCount != totalVouchers {
		t.Fatalf("expected mock server to record exactly %d distinct vouchers, got %d", totalVouchers, regCount)
	}
}

// W2: TestCrashBetweenPostAndUpdateReconcilesVia409
// Simulates a crash:
// 1. Voucher is claimed.
// 2. POST to mock accounting succeeds (returns 200 and stores EXT-...).
// 3. Worker crashes BEFORE updating voucher in DB (voucher remains 'pending').
// 4. Second attempt claims the voucher and posts to mock again.
// 5. Mock returns 409 with stored external_ref.
// 6. Worker treats 409 as success, marks voucher 'pushed' with that external_ref.
func TestCrashBetweenPostAndUpdateReconcilesVia409(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	mockServer := mock.NewServer(mock.Options{
		ErrorRate: 0.0,
		HangRate:  0.0,
	})
	httpServer := httptest.NewServer(mockServer.Handler())
	defer httpServer.Close()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 10, 5000)
	clientRef := fmt.Sprintf("W2-RECON-%s", uuid.NewString())
	voucherID := createTestVoucher(t, ctx, pool, issueID, clientRef, "pending", 0)

	client := worker.NewHTTPAccountingClient(httpServer.URL, 3*time.Second)

	// Step 1 & 2: Simulate initial post to mock accounting (succeeds)
	initRes, err := client.PostVoucher(ctx, worker.PostVoucherRequest{
		ClientRef:   clientRef,
		AmountPaise: 5000,
		Type:        "jobwork_receipt",
		SourceID:    voucherID,
	})
	if err != nil {
		t.Fatalf("initial post failed: %v", err)
	}
	if initRes.Status != worker.PostStatusOK || initRes.ExternalRef == "" {
		t.Fatalf("expected 200 OK with external_ref, got status %v, ref %q", initRes.Status, initRes.ExternalRef)
	}

	// Step 3: Worker crashes before updating DB. DB row remains 'pending', external_ref is NULL.
	var dbStatus string
	_ = pool.QueryRow(ctx, "SELECT status FROM vouchers WHERE id = $1", voucherID).Scan(&dbStatus)
	if dbStatus != "pending" {
		t.Fatalf("expected voucher to still be pending, got %q", dbStatus)
	}

	// Step 4 & 5 & 6: Second worker attempts processing
	voucherSvc := vouchers.NewService(pool)
	w, err := worker.New(worker.Options{
		VoucherService: voucherSvc,
		Client:         client,
		MaxAttempts:    3,
		BackoffBase:    100 * time.Millisecond,
		HTTPTimeout:    2 * time.Second,
		Lease:          5 * time.Second,
		Concurrency:    1,
	})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	// Run single iteration
	claimedCount, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if claimedCount != 1 {
		t.Fatalf("expected 1 voucher claimed, got %d", claimedCount)
	}

	// Verify DB state: marked 'pushed' with the original external_ref
	var finalStatus string
	var finalExtRef *string
	err = pool.QueryRow(ctx, "SELECT status, external_ref FROM vouchers WHERE id = $1", voucherID).Scan(&finalStatus, &finalExtRef)
	if err != nil {
		t.Fatalf("failed to query voucher: %v", err)
	}
	if finalStatus != "pushed" {
		t.Errorf("expected final status 'pushed', got %q", finalStatus)
	}
	if finalExtRef == nil || *finalExtRef != initRes.ExternalRef {
		t.Errorf("expected external_ref to match %q, got %v", initRes.ExternalRef, finalExtRef)
	}

	// Verify mock server registry has exactly 1 entry (no duplicate records)
	if mockServer.RegistryCount() != 1 {
		t.Errorf("expected mock registry count 1, got %d", mockServer.RegistryCount())
	}
}

// W3: TestPermanentFailureEndsFailedWithReason
// Configure mock to always return 500 for a voucher.
// Configure worker with MAX_ATTEMPTS = 3.
// Voucher is retried up to MAX_ATTEMPTS.
// After 3 attempts: status is 'failed', attempts = 3, last_error is non-empty.
func TestPermanentFailureEndsFailedWithReason(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	// Mock server always returns 500
	mockServer := mock.NewServer(mock.Options{
		ErrorRate: 1.0,
		HangRate:  0.0,
	})
	httpServer := httptest.NewServer(mockServer.Handler())
	defer httpServer.Close()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 10, 5000)
	clientRef := fmt.Sprintf("W3-FAIL-%s", uuid.NewString())
	voucherID := createTestVoucher(t, ctx, pool, issueID, clientRef, "pending", 0)

	voucherSvc := vouchers.NewService(pool)
	client := worker.NewHTTPAccountingClient(httpServer.URL, 2*time.Second)

	w, err := worker.New(worker.Options{
		VoucherService: voucherSvc,
		Client:         client,
		MaxAttempts:    3,
		BackoffBase:    10 * time.Millisecond,
		BackoffJitter:  0.0,
		HTTPTimeout:    2 * time.Second,
		Lease:          5 * time.Second,
		Concurrency:    1,
	})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	// Attempt 1
	n, err := w.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("attempt 1 failed: n=%d, err=%v", n, err)
	}

	// Advance next_attempt_at so it is immediately eligible for attempt 2
	_, _ = pool.Exec(ctx, "UPDATE vouchers SET next_attempt_at = now() - interval '1 second' WHERE id = $1", voucherID)

	// Attempt 2
	n, err = w.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("attempt 2 failed: n=%d, err=%v", n, err)
	}

	// Advance next_attempt_at for attempt 3
	_, _ = pool.Exec(ctx, "UPDATE vouchers SET next_attempt_at = now() - interval '1 second' WHERE id = $1", voucherID)

	// Attempt 3 (terminal failure)
	n, err = w.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("attempt 3 failed: n=%d, err=%v", n, err)
	}

	// Verify terminal failure state in database
	var status string
	var attempts int
	var lastError *string
	err = pool.QueryRow(ctx, "SELECT status, attempts, last_error FROM vouchers WHERE id = $1", voucherID).Scan(&status, &attempts, &lastError)
	if err != nil {
		t.Fatalf("failed to query voucher: %v", err)
	}

	if status != "failed" {
		t.Errorf("expected status 'failed', got %q", status)
	}
	if attempts != 3 {
		t.Errorf("expected attempts 3, got %d", attempts)
	}
	if lastError == nil || !strings.Contains(*lastError, "500") {
		t.Errorf("expected last_error containing '500', got %v", lastError)
	}

	// Attempt 4: Should not claim failed voucher
	n, err = w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("attempt 4 query failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 vouchers claimed after permanent failure, got %d", n)
	}
}

// W4: TestStaleClaimCannotOverwrite (Fenced write)
// Voucher is claimed by Worker 1 (claim_token = tok1).
// Lease expires or another process clears/steals the claim (claim_token changes to tok2).
// Worker 1 finishes its work and tries to update voucher with claim_token = tok1.
// Update affects 0 rows (fenced write rejects it).
// Worker 1 detects 0 rows affected, does not overwrite state.
func TestStaleClaimCannotOverwrite(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 10, 5000)
	clientRef := fmt.Sprintf("W4-FENCE-%s", uuid.NewString())
	voucherID := createTestVoucher(t, ctx, pool, issueID, clientRef, "pending", 0)

	voucherSvc := vouchers.NewService(pool)

	// Worker 1 claims voucher
	claimed, err := voucherSvc.Claim(ctx, 1, 10*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("failed to claim voucher: %v, len=%d", err, len(claimed))
	}
	tok1 := claimed[0].ClaimToken

	// Worker 2 steals voucher (simulated by updating claim_token to tok2 and marking pushed)
	tok2 := uuid.NewString()
	_, err = pool.Exec(ctx, `
		UPDATE vouchers
		SET claim_token = $1::uuid,
		    status = 'pushed',
		    external_ref = 'EXT-WORKER-2-WINNER',
		    updated_at = now()
		WHERE id = $2
	`, tok2, voucherID)
	if err != nil {
		t.Fatalf("failed to simulate worker 2 steal: %v", err)
	}

	// Worker 1 attempts to mark pushed using stale tok1
	updated, err := voucherSvc.MarkPushed(ctx, voucherID, tok1, "EXT-WORKER-1-STALE")
	if err != nil {
		t.Fatalf("MarkPushed returned unexpected error: %v", err)
	}
	if updated {
		t.Errorf("CRITICAL INVARIANT VIOLATION: fenced write succeeded with stale claim_token!")
	}

	// Verify DB state is still from Worker 2
	var finalStatus string
	var finalExtRef *string
	err = pool.QueryRow(ctx, "SELECT status, external_ref FROM vouchers WHERE id = $1", voucherID).Scan(&finalStatus, &finalExtRef)
	if err != nil {
		t.Fatalf("failed to query voucher: %v", err)
	}
	if finalStatus != "pushed" {
		t.Errorf("expected status 'pushed', got %q", finalStatus)
	}
	if finalExtRef == nil || *finalExtRef != "EXT-WORKER-2-WINNER" {
		t.Errorf("expected external_ref 'EXT-WORKER-2-WINNER', got %v", finalExtRef)
	}
}
