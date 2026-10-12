#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
ISSUE_ID="${1:-2}"

echo "========================================================="
echo "Concurrent Receipts Test against $API_URL for Issue #$ISSUE_ID"
echo "========================================================="

# Fetch initial state
echo "1. Checking initial state of Issue #$ISSUE_ID:"
INITIAL_STATE=$(curl -s "$API_URL/issues/$ISSUE_ID")
echo "$INITIAL_STATE"
echo ""

TOTAL_REQUESTS=10
QTY_PER_REQUEST=5

echo "2. Firing $TOTAL_REQUESTS concurrent receipts of Qty=$QTY_PER_REQUEST each..."
OUTPUT_DIR=$(mktemp -d)
trap 'rm -rf "$OUTPUT_DIR"' EXIT

for i in $(seq 1 $TOTAL_REQUESTS); do
  (
    HTTP_CODE=$(curl -s -o "$OUTPUT_DIR/body_$i.json" -w "%{http_code}" -X POST "$API_URL/jobwork/receipts" \
      -H "Content-Type: application/json" \
      -d "{\"issue_id\":$ISSUE_ID,\"qty\":$QTY_PER_REQUEST,\"received_on\":\"2026-10-11\"}")
    echo "$HTTP_CODE" > "$OUTPUT_DIR/status_$i.txt"
  ) &
done

# Wait for all background requests to complete
wait

echo "All $TOTAL_REQUESTS requests completed."
echo ""
echo "3. Analyzing response status codes:"
SUCCESS_COUNT=0
OVER_RECEIPT_COUNT=0

for i in $(seq 1 $TOTAL_REQUESTS); do
  STATUS=$(cat "$OUTPUT_DIR/status_$i.txt")
  BODY=$(cat "$OUTPUT_DIR/body_$i.json")
  if [ "$STATUS" = "201" ]; then
    SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    echo "  Request #$i: HTTP 201 Created -> $BODY"
  elif [ "$STATUS" = "409" ]; then
    OVER_RECEIPT_COUNT=$((OVER_RECEIPT_COUNT + 1))
    echo "  Request #$i: HTTP 409 Rejected -> $BODY"
  else
    echo "  Request #$i: Unexpected HTTP $STATUS -> $BODY"
  fi
done

echo ""
echo "Summary: $SUCCESS_COUNT succeeded (201), $OVER_RECEIPT_COUNT rejected (409)"
echo ""

echo "4. Checking final state of Issue #$ISSUE_ID:"
FINAL_STATE=$(curl -s "$API_URL/issues/$ISSUE_ID")
echo "$FINAL_STATE"
echo ""

echo "========================================================="
echo "Concurrent receipts verification complete."
echo "========================================================="
