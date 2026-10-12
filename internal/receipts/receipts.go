package receipts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/apperr"
)

type Input struct {
	IssueID    int64  `json:"issue_id"`
	Qty        int    `json:"qty"`
	ReceivedOn string `json:"received_on"`
}

type Receipt struct {
	ID         int64  `json:"id"`
	IssueID    int64  `json:"issue_id"`
	Qty        int    `json:"qty"`
	ReceivedOn string `json:"received_on"`
}

type Voucher struct {
	ID          int64  `json:"id"`
	AmountPaise int64  `json:"amount_paise"`
	ClientRef   string `json:"client_ref"`
}

type Result struct {
	Receipt Receipt `json:"receipt"`
	Voucher Voucher `json:"voucher"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// CreateReceipt handles the atomic receipt creation and voucher generation.
// Steps 1-7 in ONE database transaction:
//
// Reasoning:
// The receipt is inserted before the counter changes so a replay never consumes quantity;
// the guarded UPDATE takes a row lock so concurrent receipts for one issue serialize and each sees the current total;
// the CHECK is a second line of defence; one transaction makes receipt and voucher atomic.
func (s *Service) CreateReceipt(ctx context.Context, in Input, key *string) (Result, bool, error) {
	// 1. Validate before opening a transaction
	if in.IssueID <= 0 {
		return Result{}, false, fmt.Errorf("%w: issue_id must be positive", apperr.ErrInvalidInput)
	}
	if in.Qty <= 0 {
		return Result{}, false, fmt.Errorf("%w: qty must be positive", apperr.ErrInvalidInput)
	}
	receivedDate, err := time.Parse("2006-01-02", strings.TrimSpace(in.ReceivedOn))
	if err != nil {
		return Result{}, false, fmt.Errorf("%w: invalid received_on date format (must be YYYY-MM-DD)", apperr.ErrInvalidInput)
	}

	var cleanedKey *string
	if key != nil {
		trimmed := strings.TrimSpace(*key)
		if len(trimmed) > 255 {
			return Result{}, false, fmt.Errorf("%w: Idempotency-Key exceeds 255 characters", apperr.ErrInvalidInput)
		}
		if trimmed != "" {
			cleanedKey = &trimmed
		}
	}

	// Begin single database transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 2. Insert receipt first
	var receiptID int64
	var receiptCreatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO jobwork_receipts (issue_id, qty, received_on, idempotency_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, created_at
	`, in.IssueID, in.Qty, receivedDate, cleanedKey).Scan(&receiptID, &receiptCreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No row returned because of ON CONFLICT DO NOTHING (key conflict)
			// Handled in step 3 below.
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return Result{}, false, fmt.Errorf("%w: issue %d not found", apperr.ErrNotFound, in.IssueID)
			}
			return Result{}, false, fmt.Errorf("insert receipt: %w", err)
		}
	}

	// 3. Replay path: key conflict occurred
	if receiptID == 0 {
		_ = tx.Rollback(ctx)

		if cleanedKey == nil {
			return Result{}, false, fmt.Errorf("unexpected conflict without idempotency key")
		}

		var storedReceiptID, storedIssueID int64
		var storedQty int
		var storedReceivedOn time.Time
		var storedVoucherID int64
		var storedAmountPaise int64
		var storedClientRef string

		row := s.pool.QueryRow(ctx, `
			SELECT r.id, r.issue_id, r.qty, r.received_on,
			       v.id, v.amount_paise, v.client_ref
			FROM jobwork_receipts r
			JOIN vouchers v ON v.source_id = r.id AND v.type = 'jobwork_receipt'
			WHERE r.idempotency_key = $1
		`, *cleanedKey)

		err = row.Scan(&storedReceiptID, &storedIssueID, &storedQty, &storedReceivedOn,
			&storedVoucherID, &storedAmountPaise, &storedClientRef)
		if err != nil {
			return Result{}, false, fmt.Errorf("load replayed receipt and voucher: %w", err)
		}

		// Compare stored payload with current input
		if storedIssueID != in.IssueID || storedQty != in.Qty || storedReceivedOn.Format("2006-01-02") != in.ReceivedOn {
			return Result{}, false, apperr.ErrKeyReused
		}

		return Result{
			Receipt: Receipt{
				ID:         storedReceiptID,
				IssueID:    storedIssueID,
				Qty:        storedQty,
				ReceivedOn: storedReceivedOn.Format("2006-01-02"),
			},
			Voucher: Voucher{
				ID:          storedVoucherID,
				AmountPaise: storedAmountPaise,
				ClientRef:   storedClientRef,
			},
		}, true, nil
	}

	// 4. Guarded atomic counter update
	var totalQty, updatedReceivedQty int
	var ratePaise int64

	err = tx.QueryRow(ctx, `
		UPDATE jobwork_issues
		SET received_qty = received_qty + $2
		WHERE id = $1 AND received_qty + $2 <= qty
		RETURNING qty, received_qty, rate_paise
	`, in.IssueID, in.Qty).Scan(&totalQty, &updatedReceivedQty, &ratePaise)

	// 5. Zero rows: over receipt or missing issue
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Roll back transaction to remove the receipt row
			_ = tx.Rollback(ctx)

			// Look up the issue to classify ErrNotFound vs ErrOverReceipt
			var issueQty, currentReceivedQty int
			errLookup := s.pool.QueryRow(ctx, `
				SELECT qty, received_qty FROM jobwork_issues WHERE id = $1
			`, in.IssueID).Scan(&issueQty, &currentReceivedQty)

			if errLookup != nil {
				if errors.Is(errLookup, pgx.ErrNoRows) {
					return Result{}, false, fmt.Errorf("%w: issue %d not found", apperr.ErrNotFound, in.IssueID)
				}
				return Result{}, false, fmt.Errorf("lookup issue after zero rows update: %w", errLookup)
			}

			outstanding := issueQty - currentReceivedQty
			return Result{}, false, apperr.ErrOverReceipt{Outstanding: outstanding}
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check constraint violation
			_ = tx.Rollback(ctx)
			return Result{}, false, apperr.ErrOverReceipt{}
		}

		return Result{}, false, fmt.Errorf("guarded counter update: %w", err)
	}

	// 6. Insert the voucher in the same transaction
	amountPaise := int64(in.Qty) * ratePaise
	clientRef := uuid.NewString()

	var voucherID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO vouchers (type, source_id, amount_paise, client_ref, status, next_attempt_at)
		VALUES ('jobwork_receipt', $1, $2, $3, 'pending', now())
		RETURNING id, amount_paise, client_ref
	`, receiptID, amountPaise, clientRef).Scan(&voucherID, &amountPaise, &clientRef)

	if err != nil {
		return Result{}, false, fmt.Errorf("insert voucher: %w", err)
	}

	// 7. Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return Result{}, false, fmt.Errorf("commit transaction: %w", err)
	}

	return Result{
		Receipt: Receipt{
			ID:         receiptID,
			IssueID:    in.IssueID,
			Qty:        in.Qty,
			ReceivedOn: in.ReceivedOn,
		},
		Voucher: Voucher{
			ID:          voucherID,
			AmountPaise: amountPaise,
			ClientRef:   clientRef,
		},
	}, false, nil
}
