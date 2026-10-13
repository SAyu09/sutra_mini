package receipts_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sutra-mini/internal/apperr"
	"sutra-mini/internal/receipts"
	"sutra-mini/internal/testutil"
)

// T1: TestConcurrentReceiptsNeverOvershoot verifies that concurrent receipts never exceed issued quantity.
// Enforced by: Guarded atomic UPDATE + CHECK constraint (G1).
func TestConcurrentReceiptsNeverOvershoot(t *testing.T) {
	t.Run("AllOnes_20ConcurrentRequestsForQty10", func(t *testing.T) {
		pool := testutil.SetupTestDB(t)
		ctx := context.Background()
		svc := receipts.NewService(pool)

		const totalQty = 10
		const ratePaise int64 = 1000
		issueID := testutil.CreateFixtureIssue(t, ctx, pool, totalQty, ratePaise)

		const concurrentCount = 20
		startBarrier := make(chan struct{})
		var wg sync.WaitGroup

		type resultItem struct {
			res      receipts.Result
			replayed bool
			err      error
		}
		results := make([]resultItem, concurrentCount)

		for i := 0; i < concurrentCount; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-startBarrier

				reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				in := receipts.Input{
					IssueID:    issueID,
					Qty:        1,
					ReceivedOn: "2026-10-11",
				}
				res, replayed, err := svc.CreateReceipt(reqCtx, in, nil)
				results[idx] = resultItem{res: res, replayed: replayed, err: err}
			}(i)
		}

		// Release all goroutines simultaneously
		close(startBarrier)
		wg.Wait()

		var successCount int
		var overReceiptCount int

		for _, r := range results {
			if r.err == nil {
				successCount++
			} else {
				var overErr apperr.ErrOverReceipt
				if errors.As(r.err, &overErr) {
					overReceiptCount++
				} else {
					t.Errorf("unexpected error type for rejected receipt: %v", r.err)
				}
			}
		}

		if successCount != totalQty {
			t.Fatalf("expected exactly %d successful receipts, got %d", totalQty, successCount)
		}
		if overReceiptCount != concurrentCount-totalQty {
			t.Fatalf("expected exactly %d ErrOverReceipt rejections, got %d", concurrentCount-totalQty, overReceiptCount)
		}

		// Verify database state: jobwork_issues.received_qty == totalQty
		var issueReceivedQty, issueTotalQty int
		err := pool.QueryRow(ctx, "SELECT received_qty, qty FROM jobwork_issues WHERE id = $1", issueID).Scan(&issueReceivedQty, &issueTotalQty)
		if err != nil {
			t.Fatalf("failed to query issue: %v", err)
		}
		if issueReceivedQty != totalQty {
			t.Errorf("expected issue.received_qty to be %d, got %d", totalQty, issueReceivedQty)
		}

		// Verify sum of jobwork_receipts.qty == totalQty
		var receiptsTotalQty int
		var receiptsCount int
		err = pool.QueryRow(ctx, "SELECT count(*), coalesce(sum(qty), 0) FROM jobwork_receipts WHERE issue_id = $1", issueID).Scan(&receiptsCount, &receiptsTotalQty)
		if err != nil {
			t.Fatalf("failed to query receipts: %v", err)
		}
		if receiptsCount != totalQty {
			t.Errorf("expected %d receipt rows, got %d", totalQty, receiptsCount)
		}
		if receiptsTotalQty != totalQty {
			t.Errorf("expected sum(qty) in receipts to be %d, got %d", totalQty, receiptsTotalQty)
		}

		// Verify vouchers count and amounts
		var voucherCount int
		var totalVoucherAmount int64
		err = pool.QueryRow(ctx, `
			SELECT count(*), coalesce(sum(amount_paise), 0)
			FROM vouchers v
			JOIN jobwork_receipts r ON v.source_id = r.id AND v.type = 'jobwork_receipt'
			WHERE r.issue_id = $1
		`, issueID).Scan(&voucherCount, &totalVoucherAmount)
		if err != nil {
			t.Fatalf("failed to query vouchers: %v", err)
		}
		if voucherCount != totalQty {
			t.Errorf("expected %d voucher rows, got %d", totalQty, voucherCount)
		}
		expectedAmount := int64(totalQty) * ratePaise
		if totalVoucherAmount != expectedAmount {
			t.Errorf("expected total voucher amount %d paise, got %d", expectedAmount, totalVoucherAmount)
		}
	})

	t.Run("MixedQuantities_CannotOvershoot", func(t *testing.T) {
		pool := testutil.SetupTestDB(t)
		ctx := context.Background()
		svc := receipts.NewService(pool)

		const totalQty = 10
		const ratePaise int64 = 5000
		issueID := testutil.CreateFixtureIssue(t, ctx, pool, totalQty, ratePaise)

		// 10 goroutines requesting 2 units, 10 requesting 3 units
		const count = 20
		startBarrier := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < count; i++ {
			wg.Add(1)
			requestedQty := (i % 3) + 1 // requests 1, 2, or 3
			go func(qty int) {
				defer wg.Done()
				<-startBarrier

				reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				in := receipts.Input{
					IssueID:    issueID,
					Qty:        qty,
					ReceivedOn: "2026-10-11",
				}
				_, _, _ = svc.CreateReceipt(reqCtx, in, nil)
			}(requestedQty)
		}

		close(startBarrier)
		wg.Wait()

		var receivedQty, issueQty int
		err := pool.QueryRow(ctx, "SELECT received_qty, qty FROM jobwork_issues WHERE id = $1", issueID).Scan(&receivedQty, &issueQty)
		if err != nil {
			t.Fatalf("failed to query issue: %v", err)
		}
		if receivedQty > totalQty {
			t.Fatalf("CRITICAL: received_qty %d exceeded issued qty %d", receivedQty, totalQty)
		}

		var sumReceiptsQty int
		var receiptCount int
		err = pool.QueryRow(ctx, "SELECT count(*), coalesce(sum(qty), 0) FROM jobwork_receipts WHERE issue_id = $1", issueID).Scan(&receiptCount, &sumReceiptsQty)
		if err != nil {
			t.Fatalf("failed to query receipts: %v", err)
		}
		if sumReceiptsQty != receivedQty {
			t.Errorf("receipts sum %d != issue.received_qty %d", sumReceiptsQty, receivedQty)
		}

		var voucherCount int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers WHERE type = 'jobwork_receipt'").Scan(&voucherCount)
		if err != nil {
			t.Fatalf("failed to query vouchers: %v", err)
		}
		if voucherCount != receiptCount {
			t.Errorf("voucher count %d != receipt count %d", voucherCount, receiptCount)
		}
	})
}

// T2: TestIdempotencyKeyReplayCreatesNothing verifies that replaying an Idempotency-Key
// returns the identical result and creates no duplicate records.
// Enforced by: UNIQUE constraint in Postgres + one DB transaction (G2, G3).
func TestIdempotencyKeyReplayCreatesNothing(t *testing.T) {
	t.Run("SequentialReplays", func(t *testing.T) {
		pool := testutil.SetupTestDB(t)
		ctx := context.Background()
		svc := receipts.NewService(pool)

		issueID := testutil.CreateFixtureIssue(t, ctx, pool, 20, 3000)
		idempKey := "idemp-seq-key-12345"

		in := receipts.Input{
			IssueID:    issueID,
			Qty:        5,
			ReceivedOn: "2026-10-11",
		}

		// First call
		res1, replayed1, err := svc.CreateReceipt(ctx, in, &idempKey)
		if err != nil {
			t.Fatalf("initial receipt creation failed: %v", err)
		}
		if replayed1 {
			t.Errorf("expected replayed=false on first call, got true")
		}

		// Replay 1
		res2, replayed2, err := svc.CreateReceipt(ctx, in, &idempKey)
		if err != nil {
			t.Fatalf("first replay failed: %v", err)
		}
		if !replayed2 {
			t.Errorf("expected replayed=true on second call, got false")
		}

		// Replay 2
		res3, replayed3, err := svc.CreateReceipt(ctx, in, &idempKey)
		if err != nil {
			t.Fatalf("second replay failed: %v", err)
		}
		if !replayed3 {
			t.Errorf("expected replayed=true on third call, got false")
		}

		// Assert identical results
		if res1.Receipt.ID != res2.Receipt.ID || res1.Receipt.ID != res3.Receipt.ID {
			t.Errorf("receipt IDs differ across replays: %d vs %d vs %d", res1.Receipt.ID, res2.Receipt.ID, res3.Receipt.ID)
		}
		if res1.Voucher.ID != res2.Voucher.ID || res1.Voucher.ID != res3.Voucher.ID {
			t.Errorf("voucher IDs differ across replays: %d vs %d vs %d", res1.Voucher.ID, res2.Voucher.ID, res3.Voucher.ID)
		}
		if res1.Voucher.ClientRef != res2.Voucher.ClientRef || res1.Voucher.ClientRef != res3.Voucher.ClientRef {
			t.Errorf("voucher client_ref differs across replays: %s vs %s vs %s", res1.Voucher.ClientRef, res2.Voucher.ClientRef, res3.Voucher.ClientRef)
		}
		if res1.Voucher.AmountPaise != 5*3000 {
			t.Errorf("expected amount_paise %d, got %d", 5*3000, res1.Voucher.AmountPaise)
		}

		// Verify database row counts
		var receiptCount, voucherCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM jobwork_receipts WHERE issue_id = $1", issueID).Scan(&receiptCount)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers").Scan(&voucherCount)
		if receiptCount != 1 {
			t.Errorf("expected exactly 1 receipt in DB, got %d", receiptCount)
		}
		if voucherCount != 1 {
			t.Errorf("expected exactly 1 voucher in DB, got %d", voucherCount)
		}

		var receivedQty int
		_ = pool.QueryRow(ctx, "SELECT received_qty FROM jobwork_issues WHERE id = $1", issueID).Scan(&receivedQty)
		if receivedQty != 5 {
			t.Errorf("expected received_qty=5, got %d", receivedQty)
		}
	})

	t.Run("ConcurrentReplays_10Goroutines", func(t *testing.T) {
		pool := testutil.SetupTestDB(t)
		ctx := context.Background()
		svc := receipts.NewService(pool)

		issueID := testutil.CreateFixtureIssue(t, ctx, pool, 30, 4000)
		idempKey := "idemp-concurrent-key-999"

		in := receipts.Input{
			IssueID:    issueID,
			Qty:        10,
			ReceivedOn: "2026-10-11",
		}

		const goroutines = 10
		startBarrier := make(chan struct{})
		var wg sync.WaitGroup

		type callResult struct {
			res      receipts.Result
			replayed bool
			err      error
		}
		results := make([]callResult, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-startBarrier

				reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				res, replayed, err := svc.CreateReceipt(reqCtx, in, &idempKey)
				results[idx] = callResult{res: res, replayed: replayed, err: err}
			}(i)
		}

		close(startBarrier)
		wg.Wait()

		var nonReplayedCount int
		var replayedCount int
		var canonicalRes receipts.Result

		for i, r := range results {
			if r.err != nil {
				t.Fatalf("concurrent replay call %d failed with error: %v", i, r.err)
			}
			if !r.replayed {
				nonReplayedCount++
				canonicalRes = r.res
			} else {
				replayedCount++
			}
		}

		if nonReplayedCount != 1 {
			t.Errorf("expected exactly 1 non-replayed response, got %d", nonReplayedCount)
		}
		if replayedCount != goroutines-1 {
			t.Errorf("expected %d replayed responses, got %d", goroutines-1, replayedCount)
		}

		// Verify every response had identical IDs
		for i, r := range results {
			if r.res.Receipt.ID != canonicalRes.Receipt.ID {
				t.Errorf("goroutine %d returned receipt ID %d, expected %d", i, r.res.Receipt.ID, canonicalRes.Receipt.ID)
			}
			if r.res.Voucher.ID != canonicalRes.Voucher.ID {
				t.Errorf("goroutine %d returned voucher ID %d, expected %d", i, r.res.Voucher.ID, canonicalRes.Voucher.ID)
			}
			if r.res.Voucher.ClientRef != canonicalRes.Voucher.ClientRef {
				t.Errorf("goroutine %d returned voucher client_ref %s, expected %s", i, r.res.Voucher.ClientRef, canonicalRes.Voucher.ClientRef)
			}
		}

		// Check database row counts: exactly 1 receipt and 1 voucher
		var rc, vc, recQty int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM jobwork_receipts WHERE issue_id = $1", issueID).Scan(&rc)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers").Scan(&vc)
		_ = pool.QueryRow(ctx, "SELECT received_qty FROM jobwork_issues WHERE id = $1", issueID).Scan(&recQty)

		if rc != 1 {
			t.Errorf("expected 1 receipt row in DB, got %d", rc)
		}
		if vc != 1 {
			t.Errorf("expected 1 voucher row in DB, got %d", vc)
		}
		if recQty != 10 {
			t.Errorf("expected received_qty=10, got %d", recQty)
		}
	})

	t.Run("ReusedKeyWithDifferentPayload_ReturnsErrKeyReused", func(t *testing.T) {
		pool := testutil.SetupTestDB(t)
		ctx := context.Background()
		svc := receipts.NewService(pool)

		issueID1 := testutil.CreateFixtureIssue(t, ctx, pool, 20, 2000)
		issueID2 := testutil.CreateFixtureIssue(t, ctx, pool, 20, 2000)
		idempKey := "idemp-reuse-different-payload"

		in1 := receipts.Input{
			IssueID:    issueID1,
			Qty:        5,
			ReceivedOn: "2026-10-11",
		}

		_, replayed, err := svc.CreateReceipt(ctx, in1, &idempKey)
		if err != nil || replayed {
			t.Fatalf("first creation failed: %v", err)
		}

		// Attempt 1: different qty
		diffQty := receipts.Input{
			IssueID:    issueID1,
			Qty:        8, // different!
			ReceivedOn: "2026-10-11",
		}
		_, _, err = svc.CreateReceipt(ctx, diffQty, &idempKey)
		if !errors.Is(err, apperr.ErrKeyReused) {
			t.Fatalf("expected ErrKeyReused for different qty, got %v", err)
		}

		// Attempt 2: different issue_id
		diffIssue := receipts.Input{
			IssueID:    issueID2, // different!
			Qty:        5,
			ReceivedOn: "2026-10-11",
		}
		_, _, err = svc.CreateReceipt(ctx, diffIssue, &idempKey)
		if !errors.Is(err, apperr.ErrKeyReused) {
			t.Fatalf("expected ErrKeyReused for different issue_id, got %v", err)
		}

		// Attempt 3: different date
		diffDate := receipts.Input{
			IssueID:    issueID1,
			Qty:        5,
			ReceivedOn: "2026-10-12", // different!
		}
		_, _, err = svc.CreateReceipt(ctx, diffDate, &idempKey)
		if !errors.Is(err, apperr.ErrKeyReused) {
			t.Fatalf("expected ErrKeyReused for different received_on, got %v", err)
		}

		// Verify no extra receipts or vouchers were created
		var rc, vc int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM jobwork_receipts").Scan(&rc)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM vouchers").Scan(&vc)
		if rc != 1 || vc != 1 {
			t.Errorf("expected 1 receipt and 1 voucher, got %d receipts and %d vouchers", rc, vc)
		}
	})
}
