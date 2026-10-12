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

---

## What the Tools Got Wrong
*(Concrete examples of AI/tool mistakes, misinterpretations, or corrections encountered during development)*
- Initially detected Go 1.21.6 in `/usr/local/go/bin/go`. Since the brief strictly requires Go 1.22+ pattern routing (`POST /jobwork/receipts`, etc.), needed to upgrade Go to 1.27 via Homebrew rather than attempting to write routers compatible with 1.21.

---

## One Thing I Caught
*(Specific bug, how it was found, the fix)*
- **Lease vs HTTP Timeout invariant**: Config validation verifies `LEASE > 2 * HTTP_TIMEOUT` at initialization time so worker instances cannot be booted with invalid lease durations that would cause premature lease expiration during slow HTTP calls.
