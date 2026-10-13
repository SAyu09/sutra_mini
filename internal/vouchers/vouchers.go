package vouchers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/apperr"
)

type Voucher struct {
	ID            int64     `json:"id"`
	Type          string    `json:"type"`
	SourceID      int64     `json:"source_id"`
	AmountPaise   int64     `json:"amount_paise"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	LastError     *string   `json:"last_error"`
	ExternalRef   *string   `json:"external_ref"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// List returns vouchers, optionally filtered by status (pending|pushed|failed),
// ordered by created_at DESC, id DESC, capped at 200.
func (s *Service) List(ctx context.Context, status *string) ([]Voucher, error) {
	if status != nil {
		switch *status {
		case "pending", "pushed", "failed":
		default:
			return nil, fmt.Errorf("%w: invalid status filter %q (must be pending, pushed, or failed)", apperr.ErrInvalidInput, *status)
		}
	}

	var query string
	var args []any

	if status != nil {
		query = `
			SELECT id, type, source_id, amount_paise, status, attempts,
			       last_error, external_ref, next_attempt_at, created_at
			FROM vouchers
			WHERE status = $1
			ORDER BY created_at DESC, id DESC
			LIMIT 200
		`
		args = append(args, *status)
	} else {
		query = `
			SELECT id, type, source_id, amount_paise, status, attempts,
			       last_error, external_ref, next_attempt_at, created_at
			FROM vouchers
			ORDER BY created_at DESC, id DESC
			LIMIT 200
		`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list vouchers: %w", err)
	}
	defer rows.Close()

	list := make([]Voucher, 0)
	for rows.Next() {
		var v Voucher
		err := rows.Scan(
			&v.ID, &v.Type, &v.SourceID, &v.AmountPaise, &v.Status, &v.Attempts,
			&v.LastError, &v.ExternalRef, &v.NextAttemptAt, &v.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan voucher: %w", err)
		}
		list = append(list, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vouchers: %w", err)
	}

	return list, nil
}

// Retry resets a failed voucher to pending for immediate re-attempt.
// Only vouchers in 'failed' status may be retried.
func (s *Service) Retry(ctx context.Context, id int64) (*Voucher, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: voucher id must be positive", apperr.ErrInvalidInput)
	}

	query := `
		UPDATE vouchers
		SET status='pending', attempts=0, last_error=NULL, next_attempt_at=now(),
		    locked_until=NULL, claim_token=NULL, updated_at=now()
		WHERE id=$1 AND status='failed'
		RETURNING id, type, source_id, amount_paise, status, attempts,
		          last_error, external_ref, next_attempt_at, created_at
	`

	var v Voucher
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&v.ID, &v.Type, &v.SourceID, &v.AmountPaise, &v.Status, &v.Attempts,
		&v.LastError, &v.ExternalRef, &v.NextAttemptAt, &v.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Zero rows updated: inspect voucher status to distinguish 404 from 409
			var currentStatus string
			errLookup := s.pool.QueryRow(ctx, `SELECT status FROM vouchers WHERE id = $1`, id).Scan(&currentStatus)
			if errLookup != nil {
				if errors.Is(errLookup, pgx.ErrNoRows) {
					return nil, fmt.Errorf("%w: voucher %d not found", apperr.ErrNotFound, id)
				}
				return nil, fmt.Errorf("lookup voucher status: %w", errLookup)
			}

			return nil, fmt.Errorf("%w: cannot retry voucher in %s state, only failed vouchers can be retried", apperr.ErrInvalidState, currentStatus)
		}
		return nil, fmt.Errorf("update voucher for retry: %w", err)
	}

	return &v, nil
}

// ClaimedVoucher represents a voucher atomically claimed by a worker with an active lease.
type ClaimedVoucher struct {
	ID          int64
	SourceID    int64
	Type        string
	AmountPaise int64
	ClientRef   string
	Attempts    int
	Status      string
	CreatedAt   time.Time
	ClaimToken  string
	LockedUntil time.Time
}

// Claim atomically selects and leases up to limit pending vouchers using FOR UPDATE SKIP LOCKED.
func (s *Service) Claim(ctx context.Context, limit int, lease time.Duration) ([]ClaimedVoucher, error) {
	if limit <= 0 {
		return nil, nil
	}

	leaseInterval := fmt.Sprintf("%d seconds", int(lease.Seconds()))
	if lease.Seconds() <= 0 {
		leaseInterval = "15 seconds"
	}

	query := `
		WITH claimable AS (
			SELECT id
			FROM vouchers
			WHERE status = 'pending'
			  AND next_attempt_at <= now()
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE vouchers v
		SET locked_until = now() + $2::interval,
		    claim_token = gen_random_uuid(),
		    updated_at = now()
		FROM claimable
		WHERE v.id = claimable.id
		RETURNING v.id, v.source_id, v.type, v.amount_paise, v.client_ref, v.attempts, v.status, v.created_at, v.claim_token, v.locked_until;
	`

	rows, err := s.pool.Query(ctx, query, limit, leaseInterval)
	if err != nil {
		return nil, fmt.Errorf("claim vouchers: %w", err)
	}
	defer rows.Close()

	var claimed []ClaimedVoucher
	for rows.Next() {
		var v ClaimedVoucher
		var claimToken uuid.UUID
		if err := rows.Scan(
			&v.ID, &v.SourceID, &v.Type, &v.AmountPaise, &v.ClientRef,
			&v.Attempts, &v.Status, &v.CreatedAt, &claimToken, &v.LockedUntil,
		); err != nil {
			return nil, fmt.Errorf("scan claimed voucher: %w", err)
		}
		v.ClaimToken = claimToken.String()
		claimed = append(claimed, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed vouchers: %w", err)
	}

	return claimed, nil
}

// MarkPushed performs a fenced write updating the voucher to 'pushed' with the given external_ref.
// Returns (true, nil) if successfully updated, or (false, nil) if the fenced write affected 0 rows (lease lost).
func (s *Service) MarkPushed(ctx context.Context, id int64, claimToken string, externalRef string) (bool, error) {
	query := `
		UPDATE vouchers
		SET status = 'pushed',
		    external_ref = $3,
		    attempts = attempts + 1,
		    last_error = NULL,
		    locked_until = NULL,
		    claim_token = NULL,
		    updated_at = now()
		WHERE id = $1 AND claim_token = $2::uuid;
	`

	cmd, err := s.pool.Exec(ctx, query, id, claimToken, externalRef)
	if err != nil {
		return false, fmt.Errorf("fenced mark pushed: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}

// MarkFailed performs a fenced write updating the voucher to 'failed' with the last error.
// Returns (true, nil) if successfully updated, or (false, nil) if lease was lost.
func (s *Service) MarkFailed(ctx context.Context, id int64, claimToken string, attempts int, lastError string) (bool, error) {
	query := `
		UPDATE vouchers
		SET status = 'failed',
		    attempts = $3,
		    last_error = $4,
		    locked_until = NULL,
		    claim_token = NULL,
		    updated_at = now()
		WHERE id = $1 AND claim_token = $2::uuid;
	`

	cmd, err := s.pool.Exec(ctx, query, id, claimToken, attempts, lastError)
	if err != nil {
		return false, fmt.Errorf("fenced mark failed: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}

// ScheduleRetry performs a fenced write rescheduling the voucher with backoff and jitter.
// Returns (true, nil) if successfully updated, or (false, nil) if lease was lost.
func (s *Service) ScheduleRetry(ctx context.Context, id int64, claimToken string, attempts int, lastError string, nextAttemptAt time.Time) (bool, error) {
	query := `
		UPDATE vouchers
		SET status = 'pending',
		    attempts = $3,
		    last_error = $4,
		    next_attempt_at = $5,
		    locked_until = NULL,
		    claim_token = NULL,
		    updated_at = now()
		WHERE id = $1 AND claim_token = $2::uuid;
	`

	cmd, err := s.pool.Exec(ctx, query, id, claimToken, attempts, lastError, nextAttemptAt)
	if err != nil {
		return false, fmt.Errorf("fenced schedule retry: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}
