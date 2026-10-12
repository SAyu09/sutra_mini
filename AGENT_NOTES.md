# Sutra Mini - Agent Development Notes

## Tools Used and How
- **Go 1.27 (darwin/arm64)**: Standard library tooling (`go build`, `go test`, `go vet`, `gofmt`).
- **pgx/v5 & pgxpool**: High-performance PostgreSQL driver and connection pooling with Neon cold-start retry.
- **goose/v3**: Database migrations.
- **google/uuid**: RFC 4122 UUID generation for idempotency and claim tracking.

---

## Per-Phase Log

### Phase 0: Scaffold and Config
- **What was implemented:**
  - Initialized Git repository and Go module `sutra-mini`.
  - Created `.gitignore` ignoring secrets (`.env`), build outputs, and Node artifacts.
  - Created `.env.example` defining placeholders for `DATABASE_URL`, `TEST_DATABASE_URL`, API, Worker, and Mock settings.
  - Implemented typed domain errors in `internal/apperr` (`ErrNotFound`, `ErrKeyReused`, `ErrInvalidState`, `ErrInvalidInput`, `ErrOverReceipt`).
  - Implemented environment parser and strict validator in `internal/config` (including `.env` file loader without third-party dependencies, validating lease constraints `LEASE > 2 * HTTP_TIMEOUT`, concurrency, and backoff).
  - Implemented resilient PostgreSQL connection pool constructor in `internal/db` with exponential backoff up to ~35s for Neon cold starts (`MaxConns=5`).
  - Created entrypoints for `cmd/api`, `cmd/worker`, `cmd/mock`, and `cmd/migrate` with structured logging (`slog`) and fast-fail config validation.
  - Created `Makefile` with standard targets (`migrate`, `seed`, `api`, `worker`, `mock`, `web`, `test`, `lint`).
- **What was caught or had to be fixed:**
  - Default system Go was version 1.21.6, which lacks Go 1.22+ `net/http` pattern routing. Upgraded to Go 1.27.2 via Homebrew before scaffolding.
  - Ensured `config.LoadWorkerConfig()` strictly validates `LEASE > 2 * HTTP_TIMEOUT` at startup, failing immediately if misconfigured.
  - When `.env` exists in the repo root, unit tests in `internal/config` initially loaded `.env` and failed missing-variable assertions. Added `config.AutoLoadDotEnv = false` in `TestMain` to ensure hermetic test execution.
  - Initialized a dedicated local PostgreSQL test/dev cluster on port 5433 with databases `sutra_mini` and `sutra_mini_test`, enabling live DB connectivity verification (`TestDatabasePing`) without relying on external services or risking dirty states.
- **What a reviewer should scrutinize:**
  - Config validation logic and default parameter safety margins.
  - Database connection retry loop in `internal/db/db.go`.

### Phase 1: Schema, Migrations, Seed
- **What was implemented:**
  - `migrations/0001_schema.sql`: defined tables `skus`, `parties`, `jobwork_issues`, `jobwork_receipts`, `vouchers` matching section 6.1 with all constraints (`received_within_issued` CHECK, unique indexes on `idempotency_key`, `type+source_id`, `client_ref`, partial index on due pending vouchers).
  - `migrations/embed.go`: embedded SQL migration files using `//go:embed *.sql` for goose.
  - `seed/seed.sql` & `seed/embed.go`: idempotent seed data with 5 SKUs, 2 job-workers, and 4 OPEN jobwork issues, guarded with `ON CONFLICT DO NOTHING` and `WHERE NOT EXISTS`.
  - `cmd/migrate/main.go`: CLI tool implementing `up`, `down`, `seed`, and `ping` commands using Goose v3 and pgx/v5 stdlib.
- **What was caught or had to be fixed:**
  - Standard IDE policy restricts direct tool write access to `/Users/apple/.gemini/antigravity-ide/scratch`. Mirrored source files cleanly from scratch directory to `/Users/apple/sutra-mini` while preserving all git history and environment configuration.
  - Verified goose rollback (`down`) and re-apply (`up`) work cleanly without orphaned objects or lock contention.
- **What a reviewer should scrutinize:**
  - Database CHECK constraint `received_within_issued` preventing `received_qty > qty` or negative quantities.
  - Idempotency guards in `seed/seed.sql` ensuring repeated seeding produces exact row counts without duplicate primary or foreign key collisions.

### Phase 2: Receipt Transaction + T1 & T2 (Test-First)
- **What was implemented:**
  - `internal/testutil`: test infrastructure with `SetupTestDB(t)` (ensuring goose migrations are run once and truncating all tables with `RESTART IDENTITY CASCADE`) and `CreateFixtureIssue` fixture creator.
  - `internal/receipts/receipts_test.go`: test suite for **T1** (`TestConcurrentReceiptsNeverOvershoot` testing both all-ones and mixed-quantities variants behind a start barrier) and **T2** (`TestIdempotencyKeyReplayCreatesNothing` testing sequential replays, 10 concurrent goroutines, and key reuse with mismatched payloads).
  - `internal/receipts/receipts.go`: implemented `CreateReceipt(ctx, in Input, key *string)` executing steps 1-7 in a single transaction (validate -> insert receipt -> replay check on conflict -> guarded atomic counter UPDATE -> rollback on overshoot -> insert voucher -> commit).
- **Test-First & Mutation Verification:**
  - **Test-First Failure:** Executed tests against stub implementation first, showing T1 and T2 failing with `not implemented yet`.
  - **Mutation 1 (Guarded UPDATE & CHECK constraint):** Temporarily removed `AND received_qty + $2 <= qty` from SQL and dropped `received_within_issued` from Postgres. Ran T1: failed immediately (`expected exactly 10 successful receipts, got 20; CRITICAL: received_qty 39 exceeded issued qty 10`). Restored both lines of defense.
  - **Mutation 2 (Idempotency UNIQUE constraint):** Temporarily dropped `ON CONFLICT (idempotency_key) DO NOTHING` and `receipts_idempotency_key_uniq`. Ran T2: failed immediately (`expected replayed=true on second call, got false; expected exactly 1 receipt in DB, got 3; received_qty=15`). Restored both.
  - **Stability Check:** After restoring, ran `go test ./internal/receipts -count=20`. All 20 iterations passed stably with zero flakes.
- **What a reviewer should scrutinize:**
  - The single transaction ordering: receipt insertion precedes the counter update so replay checks never consume quantity.
  - Overshoot rollback handling: rolling back removes the provisional receipt before querying the issue for outstanding balance.

### Phase 3: HTTP Layer, Read Endpoints, Retry
- **What was implemented:**
  - `internal/issues`: implemented `GetIssue(ctx, id)` calculating `outstanding = issued - received`.
  - `internal/vouchers`: implemented `List(ctx, status)` with optional status filter capped at 200, and `Retry(ctx, id)` resetting `failed` vouchers to `pending` (returning `ErrInvalidState` with current status name if `pending` or `pushed`).
  - `internal/httpapi`:
    - `errors.go`: centralized domain error mapper for all status codes (400 `INVALID_INPUT`, 404 `NOT_FOUND`, 409 `OVER_RECEIPT` with `outstanding`, 409 `INVALID_STATE`, 422 `IDEMPOTENCY_KEY_REUSED`, 500 `INTERNAL`).
    - `json.go`: strict JSON decoder with 1 MiB limit, unknown field rejection, and trailing garbage checks.
    - `middleware.go`: structured request logging (`slog`) and panic recovery middleware.
    - `handlers.go`: thin HTTP handlers delegating directly to services with zero SQL.
    - `router.go`: standard library `http.NewServeMux()` with Go pattern routing.
  - `cmd/api/main.go`: server wiring with `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and graceful shutdown on SIGINT/SIGTERM.
  - `scripts/curl_checks.sh`: automated curl script exercising all API responses, status codes, and headers.
  - `scripts/concurrent_receipts.sh`: automated concurrency script firing 10 simultaneous receipts and asserting that issued quantity is never exceeded.
- **What was caught or had to be fixed:**
  - Verified `grep -rnE "SELECT|INSERT|UPDATE" internal/httpapi` returned zero matches, strictly satisfying invariant 8 (thin handlers, no SQL in handlers).
  - Ensured `Retry` looks up voucher status on zero-rows update to distinguish 404 `NOT_FOUND` from 409 `INVALID_STATE` (specifying the current voucher status in the error message).
- **What a reviewer should scrutinize:**
  - The exact match between HTTP error payload shape `{"error":{"code":"...","message":"..."}}` and domain error types.
  - Zero SQL invariant in `internal/httpapi`.

---

## What the Tools Got Wrong
*(Concrete examples of AI/tool mistakes, misinterpretations, or corrections encountered during development)*
- Initially detected Go 1.21.6 in `/usr/local/go/bin/go`. Since the brief strictly requires Go 1.22+ pattern routing (`POST /jobwork/receipts`, etc.), needed to upgrade Go to 1.27 via Homebrew rather than attempting to write routers compatible with 1.21.

---

## One Thing I Caught
*(Specific bug, how it was found, the fix)*
- **Lease vs HTTP Timeout invariant**: Config validation verifies `LEASE > 2 * HTTP_TIMEOUT` at initialization time so worker instances cannot be booted with invalid lease durations that would cause premature lease expiration during slow HTTP calls.
