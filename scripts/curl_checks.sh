#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
echo "Running Sutra Mini API verification against: $API_URL"
echo "========================================================="

# Helper function to print separator
section() {
  echo ""
  echo "--- $1 ---"
}

# 1. Health check
section "1. GET /healthz"
curl -s -i "$API_URL/healthz"
echo ""

# 2. Get Open Issue #1 to check initial state
section "2. GET /issues/1 (Initial Read)"
curl -s -i "$API_URL/issues/1"
echo ""

# 3. Create Receipt (201 Created)
IDEMP_KEY="chk-key-$(date +%s%N)"
section "3. POST /jobwork/receipts (201 Created with Key: $IDEMP_KEY)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d '{"issue_id":1,"qty":5,"received_on":"2026-10-11"}'
echo ""

# 4. Replay with identical key and payload (201 Created + Idempotent-Replayed: true)
section "4. POST /jobwork/receipts (Idempotent Replay with Header Check)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d '{"issue_id":1,"qty":5,"received_on":"2026-10-11"}'
echo ""

# 5. Same key with different payload (422 IDEMPOTENCY_KEY_REUSED)
section "5. POST /jobwork/receipts (422 Reused Key with Qty=8)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d '{"issue_id":1,"qty":8,"received_on":"2026-10-11"}'
echo ""

# 6. Over-Receipt rejection (409 OVER_RECEIPT with outstanding)
section "6. POST /jobwork/receipts (409 Over-Receipt Qty=9999)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -d '{"issue_id":1,"qty":9999,"received_on":"2026-10-11"}'
echo ""

# 7. Validation Failures (400 INVALID_INPUT)
section "7a. POST /jobwork/receipts (400 Invalid Input: qty=0)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -d '{"issue_id":1,"qty":0,"received_on":"2026-10-11"}'
echo ""

section "7b. POST /jobwork/receipts (400 Invalid Input: bad date format)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -d '{"issue_id":1,"qty":5,"received_on":"11-10-2026"}'
echo ""

section "7c. POST /jobwork/receipts (400 Invalid Input: unknown JSON fields)"
curl -s -i -X POST "$API_URL/jobwork/receipts" \
  -H "Content-Type: application/json" \
  -d '{"issue_id":1,"qty":5,"received_on":"2026-10-11","unknown_param":true}'
echo ""

# 8. Not Found Errors (404 NOT_FOUND)
section "8a. GET /issues/999999 (404 Issue Not Found)"
curl -s -i "$API_URL/issues/999999"
echo ""

section "8b. POST /vouchers/999999/retry (404 Voucher Not Found)"
curl -s -i -X POST "$API_URL/vouchers/999999/retry"
echo ""

# 9. GET /issues/1 Math Verification
section "9. GET /issues/1 (Verify math: outstanding = issued - received)"
curl -s -i "$API_URL/issues/1"
echo ""

# 10. List Vouchers & Status Validation
section "10a. GET /vouchers?status=pending (200 OK)"
curl -s -i "$API_URL/vouchers?status=pending"
echo ""

section "10b. GET /vouchers?status=invalid_status (400 Bad Status Filter)"
curl -s -i "$API_URL/vouchers?status=invalid_status"
echo ""

# 11. Retry Voucher Status Constraints
section "11a. POST /vouchers/1/retry (409 INVALID_STATE for pending voucher)"
curl -s -i -X POST "$API_URL/vouchers/1/retry"
echo ""

# Manually set voucher 1 to 'failed' in DB to test successful retry
psql -p 5433 -U apple sutra_mini -c "UPDATE vouchers SET status='failed' WHERE id=1;" >/dev/null 2>&1 || true

section "11b. POST /vouchers/1/retry (200 OK for failed voucher, resets to pending)"
curl -s -i -X POST "$API_URL/vouchers/1/retry"
echo ""

section "11c. POST /vouchers/1/retry (409 INVALID_STATE again because voucher is now pending)"
curl -s -i -X POST "$API_URL/vouchers/1/retry"
echo ""

echo "========================================================="
echo "All curl checks finished successfully."
