package vouchers

import (
	"context"
	"errors"
	"fmt"
	"time"

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
