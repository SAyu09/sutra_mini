package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/httpapi"
	"sutra-mini/internal/issues"
	"sutra-mini/internal/receipts"
	"sutra-mini/internal/testutil"
	"sutra-mini/internal/vouchers"
)

func setupTestServer(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.SetupTestDB(t)

	receiptsSvc := receipts.NewService(pool)
	issuesSvc := issues.NewService(pool)
	vouchersSvc := vouchers.NewService(pool)

	handlers := httpapi.NewHandlers(pool, receiptsSvc, issuesSvc, vouchersSvc)
	router := httpapi.NewRouter(handlers)

	return router, pool
}

func TestHTTPAPI_CreateReceiptAndReplay(t *testing.T) {
	router, pool := setupTestServer(t)
	ctx := context.Background()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 50, 4500)
	payload := fmt.Sprintf(`{"issue_id":%d,"qty":5,"received_on":"2026-10-11"}`, issueID)

	// 1. Initial Create (201)
	req1 := httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "test-key-phase3")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec1.Header().Get("Idempotent-Replayed") != "" {
		t.Errorf("expected no Idempotent-Replayed header on first request")
	}

	var body1 struct {
		Receipt struct {
			ID         int64  `json:"id"`
			IssueID    int64  `json:"issue_id"`
			Qty        int    `json:"qty"`
			ReceivedOn string `json:"received_on"`
		} `json:"receipt"`
		Voucher struct {
			ID          int64  `json:"id"`
			AmountPaise int64  `json:"amount_paise"`
			ClientRef   string `json:"client_ref"`
		} `json:"voucher"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &body1); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body1.Receipt.Qty != 5 || body1.Voucher.AmountPaise != 5*4500 {
		t.Errorf("unexpected body content: %+v", body1)
	}

	// 2. Replay with identical key and payload (201 + Idempotent-Replayed: true)
	req2 := httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "test-key-phase3")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on replay, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("expected Idempotent-Replayed: true header on replay")
	}

	// Replay body must be identical by construction
	if strings.TrimSpace(rec1.Body.String()) != strings.TrimSpace(rec2.Body.String()) {
		t.Errorf("replay body differed from original:\nOriginal: %s\nReplay: %s", rec1.Body.String(), rec2.Body.String())
	}

	// 3. Same key with different payload -> 422 IDEMPOTENCY_KEY_REUSED
	diffPayload := fmt.Sprintf(`{"issue_id":%d,"qty":8,"received_on":"2026-10-11"}`, issueID)
	req3 := httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(diffPayload))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", "test-key-phase3")
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 for reused key with different payload, got %d: %s", rec3.Code, rec3.Body.String())
	}
	var errResp httpapi.ErrorResponse
	_ = json.Unmarshal(rec3.Body.Bytes(), &errResp)
	if errResp.Error.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Errorf("expected error code IDEMPOTENCY_KEY_REUSED, got %s", errResp.Error.Code)
	}
}

func TestHTTPAPI_ValidationAndErrors(t *testing.T) {
	router, pool := setupTestServer(t)
	ctx := context.Background()
	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 10, 1000)

	// 1. Qty = 0 -> 400 INVALID_INPUT
	badQty := fmt.Sprintf(`{"issue_id":%d,"qty":0,"received_on":"2026-10-11"}`, issueID)
	req := httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(badQty))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for qty=0, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Bad date -> 400 INVALID_INPUT
	badDate := fmt.Sprintf(`{"issue_id":%d,"qty":2,"received_on":"11-10-2026"}`, issueID)
	req = httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(badDate))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad date, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Bad JSON / Unknown fields -> 400 INVALID_INPUT
	unknownField := fmt.Sprintf(`{"issue_id":%d,"qty":2,"received_on":"2026-10-11","extra":"field"}`, issueID)
	req = httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(unknownField))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown fields, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Over receipt -> 409 OVER_RECEIPT with outstanding
	overshoot := fmt.Sprintf(`{"issue_id":%d,"qty":15,"received_on":"2026-10-11"}`, issueID)
	req = httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(overshoot))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for over receipt, got %d: %s", rec.Code, rec.Body.String())
	}
	var overResp httpapi.ErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &overResp)
	if overResp.Error.Code != "OVER_RECEIPT" {
		t.Errorf("expected error code OVER_RECEIPT, got %s", overResp.Error.Code)
	}
	if overResp.Error.Outstanding == nil || *overResp.Error.Outstanding != 10 {
		t.Errorf("expected outstanding=10, got %v", overResp.Error.Outstanding)
	}

	// 5. Unknown issue -> 404 NOT_FOUND
	req404 := httptest.NewRequest("GET", "/issues/999999", nil)
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing issue, got %d: %s", rec404.Code, rec404.Body.String())
	}

	// 6. Unknown voucher retry -> 404 NOT_FOUND
	reqRetry404 := httptest.NewRequest("POST", "/vouchers/999999/retry", nil)
	recRetry404 := httptest.NewRecorder()
	router.ServeHTTP(recRetry404, reqRetry404)
	if recRetry404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing voucher retry, got %d: %s", recRetry404.Code, recRetry404.Body.String())
	}
}

func TestHTTPAPI_IssuesAndVouchers(t *testing.T) {
	router, pool := setupTestServer(t)
	ctx := context.Background()

	issueID := testutil.CreateFixtureIssue(t, ctx, pool, 50, 4000)

	// Post receipt for 20 units
	payload := fmt.Sprintf(`{"issue_id":%d,"qty":20,"received_on":"2026-10-11"}`, issueID)
	req := httptest.NewRequest("POST", "/jobwork/receipts", bytes.NewBufferString(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create receipt: %s", rec.Body.String())
	}

	// GET /issues/{id} math
	reqGet := httptest.NewRequest("GET", fmt.Sprintf("/issues/%d", issueID), nil)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recGet.Code, recGet.Body.String())
	}
	var issueResp struct {
		Issued      int `json:"issued"`
		Received    int `json:"received"`
		Outstanding int `json:"outstanding"`
	}
	_ = json.Unmarshal(recGet.Body.Bytes(), &issueResp)
	if issueResp.Issued != 50 || issueResp.Received != 20 || issueResp.Outstanding != 30 {
		t.Errorf("issue math mismatch: expected 50/20/30, got %d/%d/%d", issueResp.Issued, issueResp.Received, issueResp.Outstanding)
	}

	// GET /vouchers?status=
	reqVouchers := httptest.NewRequest("GET", "/vouchers?status=pending", nil)
	recVouchers := httptest.NewRecorder()
	router.ServeHTTP(recVouchers, reqVouchers)
	if recVouchers.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recVouchers.Code, recVouchers.Body.String())
	}
	var vList struct {
		Vouchers []vouchers.Voucher `json:"vouchers"`
	}
	_ = json.Unmarshal(recVouchers.Body.Bytes(), &vList)
	if len(vList.Vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vList.Vouchers))
	}
	voucherID := vList.Vouchers[0].ID

	// Attempt retry on pending voucher -> 409 INVALID_STATE
	reqRetryPending := httptest.NewRequest("POST", fmt.Sprintf("/vouchers/%d/retry", voucherID), nil)
	recRetryPending := httptest.NewRecorder()
	router.ServeHTTP(recRetryPending, reqRetryPending)
	if recRetryPending.Code != http.StatusConflict {
		t.Errorf("expected 409 for retrying pending voucher, got %d: %s", recRetryPending.Code, recRetryPending.Body.String())
	}

	// Update voucher status to 'failed' using test helper
	testutil.SetVoucherStatus(t, ctx, pool, voucherID, "failed")

	// Retry on failed voucher -> 200 OK, resets to pending
	reqRetryFailed := httptest.NewRequest("POST", fmt.Sprintf("/vouchers/%d/retry", voucherID), nil)
	recRetryFailed := httptest.NewRecorder()
	router.ServeHTTP(recRetryFailed, reqRetryFailed)
	if recRetryFailed.Code != http.StatusOK {
		t.Fatalf("expected 200 for retrying failed voucher, got %d: %s", recRetryFailed.Code, recRetryFailed.Body.String())
	}
	var updatedVoucher vouchers.Voucher
	_ = json.Unmarshal(recRetryFailed.Body.Bytes(), &updatedVoucher)
	if updatedVoucher.Status != "pending" || updatedVoucher.Attempts != 0 {
		t.Errorf("expected status=pending and attempts=0 after retry, got %+v", updatedVoucher)
	}
}

func TestHTTPAPI_Healthz(t *testing.T) {
	router, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for healthz, got %d: %s", rec.Code, rec.Body.String())
	}
}
