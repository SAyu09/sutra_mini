package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"sutra-mini/internal/vouchers"
)

// VoucherRepository defines the persistence operations required to update voucher status.
type VoucherRepository interface {
	MarkPushed(ctx context.Context, id int64, claimToken string, externalRef string) (bool, error)
	MarkFailed(ctx context.Context, id int64, claimToken string, attempts int, lastError string) (bool, error)
	ScheduleRetry(ctx context.Context, id int64, claimToken string, attempts int, lastError string, nextAttemptAt time.Time) (bool, error)
}

// ProcessorOptions configures the Processor.
type ProcessorOptions struct {
	Repo          VoucherRepository
	Client        AccountingClient
	MaxAttempts   int
	BackoffBase   time.Duration
	BackoffJitter float64
	NowFunc       func() time.Time
}

// Processor coordinates processing a single claimed voucher against the accounting service.
type Processor struct {
	repo          VoucherRepository
	client        AccountingClient
	maxAttempts   int
	backoffBase   time.Duration
	backoffJitter float64
	nowFunc       func() time.Time

	randMu sync.Mutex
	rng    *rand.Rand
}

// NewProcessor creates a new Processor with the given options.
func NewProcessor(opts ProcessorOptions) *Processor {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	backoffBase := opts.BackoffBase
	if backoffBase <= 0 {
		backoffBase = 2 * time.Second
	}
	nowFunc := opts.NowFunc
	if nowFunc == nil {
		nowFunc = time.Now
	}

	return &Processor{
		repo:          opts.Repo,
		client:        opts.Client,
		maxAttempts:   maxAttempts,
		backoffBase:   backoffBase,
		backoffJitter: opts.BackoffJitter,
		nowFunc:       nowFunc,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// randFloat returns a random float in [0.0, 1.0) protected by a mutex.
func (p *Processor) randFloat() float64 {
	p.randMu.Lock()
	defer p.randMu.Unlock()
	return p.rng.Float64()
}

// Process processes a single claimed voucher.
// Invariant G4: Fenced writes protect against dual updates if lease was lost.
func (p *Processor) Process(ctx context.Context, v vouchers.ClaimedVoucher) error {
	postReq := PostVoucherRequest{
		ClientRef:   v.ClientRef,
		AmountPaise: v.AmountPaise,
		Type:        v.Type,
		SourceID:    v.SourceID,
	}

	result, postErr := p.client.PostVoucher(ctx, postReq)

	// Case 1: 200 OK or 409 Conflict (Reconciliation) -> Mark Pushed
	if result.Status == PostStatusOK || result.Status == PostStatusConflictReconciled {
		reconciled := (result.Status == PostStatusConflictReconciled)
		if reconciled {
			slog.Info("voucher 409 reconciled with accounting",
				"voucher_id", v.ID,
				"client_ref", v.ClientRef,
				"external_ref", result.ExternalRef,
			)
		}

		updated, err := p.repo.MarkPushed(ctx, v.ID, v.ClaimToken, result.ExternalRef)
		if err != nil {
			slog.Error("failed to mark voucher pushed in db", "voucher_id", v.ID, "error", err)
			return fmt.Errorf("mark pushed: %w", err)
		}
		if !updated {
			slog.Warn("fenced write rejected: lost lease on voucher (already claimed/pushed by another worker)",
				"voucher_id", v.ID,
				"claim_token", v.ClaimToken,
			)
			return nil
		}

		slog.Info("voucher marked pushed",
			"voucher_id", v.ID,
			"client_ref", v.ClientRef,
			"external_ref", result.ExternalRef,
			"reconciled", reconciled,
		)
		return nil
	}

	// Case 2: Transient or persistent failure
	errMsg := result.ErrorMessage
	if errMsg == "" && postErr != nil {
		errMsg = postErr.Error()
	}
	if errMsg == "" {
		errMsg = "unknown accounting error"
	}

	newAttempts := v.Attempts + 1
	if newAttempts >= p.maxAttempts {
		// Terminal failure
		updated, err := p.repo.MarkFailed(ctx, v.ID, v.ClaimToken, newAttempts, errMsg)
		if err != nil {
			slog.Error("failed to mark voucher failed in db", "voucher_id", v.ID, "error", err)
			return fmt.Errorf("mark failed: %w", err)
		}
		if !updated {
			slog.Warn("fenced write rejected: lost lease on voucher mark failed",
				"voucher_id", v.ID,
				"claim_token", v.ClaimToken,
			)
			return nil
		}

		slog.Warn("voucher reached max attempts, marked failed",
			"voucher_id", v.ID,
			"client_ref", v.ClientRef,
			"attempts", newAttempts,
			"error", errMsg,
		)
		return nil
	}

	// Schedule retry with exponential backoff + jitter
	// backoff = BACKOFF_BASE * (2 ^ (newAttempts - 1))
	// jitter = backoff * random(-BACKOFF_JITTER, +BACKOFF_JITTER)
	backoffFactor := math.Pow(2, float64(newAttempts-1))
	backoffSec := p.backoffBase.Seconds() * backoffFactor

	jitterFactor := (p.randFloat()*2.0 - 1.0) * p.backoffJitter
	totalDelaySec := backoffSec + (backoffSec * jitterFactor)
	if totalDelaySec < 0 {
		totalDelaySec = 0
	}
	delay := time.Duration(totalDelaySec * float64(time.Second))
	nextAttemptAt := p.nowFunc().Add(delay)

	updated, err := p.repo.ScheduleRetry(ctx, v.ID, v.ClaimToken, newAttempts, errMsg, nextAttemptAt)
	if err != nil {
		slog.Error("failed to schedule voucher retry in db", "voucher_id", v.ID, "error", err)
		return fmt.Errorf("schedule retry: %w", err)
	}
	if !updated {
		slog.Warn("fenced write rejected: lost lease on voucher schedule retry",
			"voucher_id", v.ID,
			"claim_token", v.ClaimToken,
		)
		return nil
	}

	slog.Info("voucher scheduled for retry",
		"voucher_id", v.ID,
		"client_ref", v.ClientRef,
		"attempts", newAttempts,
		"next_attempt_at", nextAttemptAt,
		"delay", delay,
		"error", errMsg,
	)
	return nil
}
