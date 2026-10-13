# Sutra Mini

> **A resilient, fault-tolerant job-work accounting integration engine for Shree Radha Studio, Delhi.**

---

## 1. Business Mission & Context

**Shree Radha Studio** is a high-volume women's ethnicwear design studio in Chandni Chowk / Delhi NCR. Fabric and semi-finished garments are dispatched to external job-workers for embroidery, zari work, dyeing, and stitching.

Fabric returns in multiple partial shipments across several days. Every partial receipt has a financial and accounting consequence: an accounting voucher must be created and posted to the studio's primary accounting software. However, the accounting system's API is notoriously slow, subject to sudden network timeouts, and intermittently fails with HTTP 500 errors.

**Sutra Mini** is the integration layer engineered to make this job-work receipt and voucher posting rock-solid, guaranteeing that vouchers are never lost, never over-received, never double-posted, and always reconciled even after server crashes.

---

## 2. Architecture & Data Flow

```mermaid
flowchart TD
    Client["Client / Web UI / ERP"] -->|POST /v1/jobwork/receipts| API["Sutra HTTP API (:8080)"]
    
    subgraph Storage["PostgreSQL Database (:5433 / Neon)"]
        Issues[("jobwork_issues\nreceived_qty <= qty")]
        Receipts[("jobwork_receipts\nidempotency_key UNIQUE")]
        Vouchers[("vouchers\nstatus: pending | pushed | failed\nclaim_token & locked_until")]
    end

    API -->|1. Atomic TX\nGuarded Counter Update\nReceipt + Voucher Creation| Storage

    subgraph Workers["Sync Worker Instances (:cmd/worker)"]
        W1["Worker Instance 1"]
        W2["Worker Instance 2"]
    end

    Storage -->|2. FOR UPDATE SKIP LOCKED\nLease: 15s| Workers
    
    subgraph Accounting["External Accounting System / Mock (:8081)"]
        MockServer["Accounting API\nPOST /v1/accounting/vouchers"]
    end

    Workers -->|3. POST voucher\nTimeout: 3s| MockServer
    MockServer -->|4. HTTP 200 OK or 409 Conflict| Workers
    Workers -->|5. Fenced Write (WHERE id=$1 AND claim_token=$2)| Storage
```

---

## 3. The Five Core Guarantees

| Guarantee | Description | Implementation Mechanism | Verification Test |
| :--- | :--- | :--- | :--- |
| **G1: No Over-Receipt** | Total received quantity across partial receipts never exceeds issued quantity. | Guarded atomic counter update (`UPDATE jobwork_issues SET received_qty = received_qty + $2 WHERE id = $1 AND received_qty + $2 <= qty`) + PostgreSQL `CHECK (received_qty <= qty)`. | **T1** (`TestConcurrentReceiptsNeverOvershoot`) |
| **G2: Atomic Voucher Creation** | A voucher is created in the exact same transaction as the receipt. A receipt never exists without a voucher. | Unified PostgreSQL transaction: issues update, receipts insert, and vouchers insert happen atomically in `receipts.CreateReceipt`. | **T1**, **T2**, **demo.sh** |
| **G3: Idempotent Replays** | Replaying an `Idempotency-Key` returns the identical result and creates no duplicate records. | Database-enforced `UNIQUE (idempotency_key)` constraint. Duplicates are caught and looked up within transaction. | **T2** (`TestIdempotencyKeyReplayCreatesNothing`) |
| **G4: No Double Post** | A voucher is never posted twice, even across network timeouts, slow calls, or process crashes. | Pre-generated `client_ref` at receipt time + claim lease + fenced writes + HTTP 409 conflict reconciliation as success. | **W1**, **W2** (`TestCrashBetweenPostAndUpdateReconcilesVia409`), **W4** |
| **G5: Concurrency & Hang Safety** | Two worker instances run safely concurrently; a 5s accounting hang never blocks other vouchers. | Atomic `FOR UPDATE SKIP LOCKED` claims + lease expiry + independent per-voucher goroutines with strict HTTP timeouts (`LEASE > 2*HTTP_TIMEOUT`). | **W1** (`TestTwoWorkersNoDoubleClaimNoDoublePost`), **M1** |

---

## 4. Prerequisites

- **Go**: 1.22+ (tested on Go 1.22+ with Go 1.22 routing syntax)
- **PostgreSQL**: 14+ (local instance or cloud database such as Neon / Supabase)
- **Node.js**: 18+ (tested on Node v22, npm 10+)
- **Bash & curl**: standard Unix utilities

---

## 5. Quickstart Guide

### Step 1: Environment Setup
Clone the repository and copy the environment template:
```bash
cp .env.example .env
```
Ensure `.env` contains valid database credentials:
```env
DATABASE_URL="postgres://apple@localhost:5433/sutra_mini?sslmode=disable"
TEST_DATABASE_URL="postgres://apple@localhost:5433/sutra_mini_test?sslmode=disable"
API_ADDR=":8080"
MOCK_ADDR=":8081"
```

### Step 2: Database Migration & Seed Data
Run database migrations and insert sample job-work issues and SKUs:
```bash
go run ./cmd/migrate up
go run ./cmd/migrate seed
```

### Step 3: Run the Services
Open separate terminal tabs or run in background:

```bash
# Terminal 1: Mock Accounting Server (Chaos server)
go run ./cmd/mock

# Terminal 2: HTTP API Service
go run ./cmd/api

# Terminal 3: Resilient Background Sync Worker
go run ./cmd/worker

# Terminal 4: Web UI Dashboard
cd web && npm run dev
```

Visit the Web UI at **`http://localhost:3000/sync`**.

---

## 6. API Reference

All endpoints accept and return `application/json`.

### `POST /v1/jobwork/receipts`
Records fabric received from a job-worker and queues an accounting voucher.
- **Headers**:
  - `Idempotency-Key` (optional, string): Unique client request token (e.g. UUID).
- **Body**:
  ```json
  {
    "issue_id": 1,
    "qty": 5,
    "received_on": "2026-10-13"
  }
  ```
- **Response (201 Created)**:
  ```json
  {
    "data": {
      "receipt": {
        "id": 12,
        "issue_id": 1,
        "qty": 5,
        "received_on": "2026-10-13"
      },
      "voucher": {
        "id": 12,
        "type": "jobwork_receipt",
        "amount_paise": 60000,
        "status": "pending",
        "client_ref": "vch-receipt-12-a1b2c3d4"
      }
    }
  }
  ```
- **Error Responses**:
  - `409 Conflict` (`OVER_RECEIPT`): If `qty` exceeds outstanding issue balance.
  - `409 Conflict` (`KEY_REUSED`): If `Idempotency-Key` was replayed with a different payload.

### `GET /v1/issues/:id`
Retrieves issue details including total issued, total received, and outstanding quantity.

### `GET /v1/vouchers?status=pending|pushed|failed`
Lists vouchers up to 200 items, ordered by creation time descending.

### `POST /v1/vouchers/:id/retry`
Resets a `failed` voucher back to `pending` with `attempts = 0` for immediate retry.
- Returns `409 Conflict` (`INVALID_STATE`) if the voucher is not in `failed` state.

---

## 7. Running Tests & Verifications

### Complete Test Suite
```bash
go test -v ./...
```

### Race Detection
Verify that no data races exist across concurrent operations:
```bash
go test -race ./...
```

### Automated Acceptance & End-to-End Demo
Run the automated end-to-end demonstration exercising all services, normal sync, idempotency, and over-receipt rejection:
```bash
bash scripts/demo.sh
```

---

## 8. Production Readiness Considerations

1. **Connection Pooling**: Uses `jackc/pgx/v5/pgxpool` with Neon cold-start exponential backoff retry. In serverless/containerized deployments, keep pool size bounded (`MaxConns = 5` to `10`) to avoid starving database connection limits.
2. **Lease Configuration Invariant**: Ensure `LEASE > 2 * HTTP_TIMEOUT`. This guarantees that slow accounting calls fail or succeed before a worker's lock expires, preventing concurrent double processing.
3. **Fenced Updates**: Never perform unconditional updates on claimed vouchers. Always use `WHERE id = $1 AND claim_token = $2::uuid` to detect lost leases.
4. **Accounting 409 Reconciliation**: When an accounting integration returns HTTP 409 with an existing external ID, always treat it as success, record the external ID, and mark the voucher pushed.
