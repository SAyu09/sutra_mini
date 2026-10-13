# Recording Guide: Sutra Mini Demo Video

> **Duration**: ~3 to 5 minutes  
> **Target Audience**: Technical reviewers, engineers, and stakeholders at Shree Radha Studio.

---

## Preparation Checklist
1. PostgreSQL is running on port `5433` (or configured database).
2. Four terminal windows/panes arranged side-by-side or in tabs:
   - **Terminal 1**: Mock Accounting Server (`cmd/mock`)
   - **Terminal 2**: HTTP API (`cmd/api`)
   - **Terminal 3**: Sync Worker (`cmd/worker`)
   - **Terminal 4**: CLI / Test runner
3. Browser open at `http://localhost:3000/sync`.

---

## Timed Video Script

### 00:00 - 00:45: Introduction & Problem Context
- **Screen**: Show Studio brand & Web UI (`http://localhost:3000/sync`).
- **Narration**:
  > *"Welcome to Sutra Mini. Shree Radha Studio in Delhi sends high-end fabric to external job-workers for embroidery and stitching. Because fabric returns in partial shipments over several days, each receipt generates an accounting voucher that must reach our external accounting software. However, the accounting API is notoriously slow and frequently times out. Sutra Mini guarantees zero lost vouchers, zero over-receipts, zero double posts, and automatic fault reconciliation."*

### 00:45 - 01:30: Starting the Services & Architecture
- **Screen**: Split view showing Terminals 1, 2, 3.
- **Commands**:
  ```bash
  # Terminal 1
  go run ./cmd/mock

  # Terminal 2
  go run ./cmd/api

  # Terminal 3
  go run ./cmd/worker
  ```
- **Narration**:
  > *"Here we start our three core backend services: the Chaos Accounting Mock server running with simulated latency and error rates, our HTTP API with Go 1.22 pattern routing, and our background Sync Worker utilizing Postgres SKIP LOCKED leases and fenced updates."*

### 01:30 - 02:15: Live Receipt Simulation & Real-time Sync
- **Screen**: Switch to Browser at `http://localhost:3000/sync`.
- **Action**:
  1. Click **"+ Simulate Job-Work Receipt"**.
  2. Enter Issue `#1`, Quantity `2`, click **"Create Receipt & Voucher"**.
- **Visual**:
  - Show the voucher appear in the **Pending** column with the amount in Indian Rupees (`₹240.00`).
  - Watch the live pulse indicator as the Sync Worker claims the voucher with a lease.
  - Watch the voucher seamlessly transition to the **Pushed** column with its assigned `external_ref`.
- **Narration**:
  > *"Notice how the receipt and voucher are created in a single database transaction. The worker claims the voucher using FOR UPDATE SKIP LOCKED, leases it, posts it to the accounting system, and safely marks it pushed with a fenced write."*

### 02:15 - 03:00: Fault Tolerance & 409 Reconciliation (Guarantee G4)
- **Screen**: Terminal 4 + Worker Terminal 3.
- **Action**:
  - Trigger duplicate idempotency or show reconciliation logs:
  ```bash
  # Terminal 4: Run demo script or re-send with same client_ref
  curl -s -i -X POST http://localhost:8080/v1/jobwork/receipts \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: demo-key-1" \
    -d '{"issue_id":1,"qty":2,"received_on":"2026-10-13"}'
  ```
- **Narration**:
  > *"If a worker crashes after posting but before updating the database, the voucher is reclaimed. When reposted, the accounting system returns HTTP 409 Conflict with the existing external reference. Rather than failing, Sutra Mini treats 409 as reconciliation success and marks the voucher pushed. Zero duplicate records are created."*

### 03:00 - 03:45: Permanent Failure & Interactive Retry
- **Screen**: Web UI (`http://localhost:3000/sync`).
- **Visual**:
  - Show a voucher in the **Failed** column displaying the exhausted attempts and exact error message.
  - Click the interactive **"↻ Retry"** button.
  - Observe the voucher immediately reset to `pending` and get picked up by the worker.
- **Narration**:
  > *"When an accounting endpoint fails repeatedly, exponential backoff with jitter prevents thundering herds. Once MAX_ATTEMPTS is reached, the voucher safely transitions to 'failed'. Operators can inspect the exact failure reason and trigger an immediate retry directly from the dashboard."*

### 03:45 - 04:30: Automated Verification Suite (G1-G5 Green)
- **Screen**: Terminal 4.
- **Command**:
  ```bash
  go test -race -v ./...
  ```
- **Narration**:
  > *"Finally, we run our comprehensive automated test suite with the race detector enabled. Tests T1 and T2 prove guarded balance invariants and idempotency. Tests W1 through W4 prove concurrent multi-worker safety, 409 reconciliation, and fenced writes. All guarantees G1 through G5 are mathematically verified and green."*

---

## Key Points to Emphasize
- **Amounts are in Rupees**: All amounts are formatted in ₹, never raw paise.
- **Concurrency**: 2 workers can run concurrently with zero race conditions.
- **Fenced Writes**: A worker with an expired lease can never overwrite newer state.
