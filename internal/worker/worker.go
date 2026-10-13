package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sutra-mini/internal/vouchers"
)

// Options configures the sync Worker.
type Options struct {
	VoucherService *vouchers.Service
	Client         AccountingClient
	MaxAttempts    int
	BackoffBase    time.Duration
	BackoffJitter  float64
	HTTPTimeout    time.Duration
	Lease          time.Duration
	PollInterval   time.Duration
	Concurrency    int
}

// Worker is the polling daemon that claims vouchers and orchestrates processing.
type Worker struct {
	service      *vouchers.Service
	processor    *Processor
	httpTimeout  time.Duration
	lease        time.Duration
	pollInterval time.Duration
	concurrency  int
	sem          chan struct{}
	wg           sync.WaitGroup
}

// New creates a new Worker instance.
func New(opts Options) (*Worker, error) {
	if opts.VoucherService == nil {
		return nil, fmt.Errorf("VoucherService is required")
	}
	if opts.Client == nil {
		return nil, fmt.Errorf("AccountingClient is required")
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	lease := opts.Lease
	if lease <= 0 {
		lease = 15 * time.Second
	}

	httpTimeout := opts.HTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = 3 * time.Second
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 1 * time.Second
	}

	proc := NewProcessor(ProcessorOptions{
		Repo:          opts.VoucherService,
		Client:        opts.Client,
		MaxAttempts:   opts.MaxAttempts,
		BackoffBase:   opts.BackoffBase,
		BackoffJitter: opts.BackoffJitter,
	})

	return &Worker{
		service:      opts.VoucherService,
		processor:    proc,
		httpTimeout:  httpTimeout,
		lease:        lease,
		pollInterval: pollInterval,
		concurrency:  concurrency,
		sem:          make(chan struct{}, concurrency),
	}, nil
}

// RunOnce performs a single claim-and-process iteration, waiting for all spawned vouchers to complete.
// Useful for synchronous testing or single-step execution.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	claimed, err := w.service.Claim(ctx, w.concurrency, w.lease)
	if err != nil {
		return 0, fmt.Errorf("claim in RunOnce: %w", err)
	}

	if len(claimed) == 0 {
		return 0, nil
	}

	var batchWg sync.WaitGroup
	for _, v := range claimed {
		batchWg.Add(1)
		go func(item vouchers.ClaimedVoucher) {
			defer batchWg.Done()
			procCtx, cancel := context.WithTimeout(ctx, w.httpTimeout)
			defer cancel()
			_ = w.processor.Process(procCtx, item)
		}(v)
	}

	batchWg.Wait()
	return len(claimed), nil
}

// Start runs the continuous polling loop until ctx is canceled, then drains in-flight work.
// Guarantees:
// - G4: Never double-posts due to claim lease + fenced writes.
// - G5: Safe for multiple concurrent instances (FOR UPDATE SKIP LOCKED); 5s hangs never block other vouchers.
func (w *Worker) Start(ctx context.Context) {
	slog.Info("sync worker started",
		"concurrency", w.concurrency,
		"poll_interval", w.pollInterval,
		"lease", w.lease,
		"http_timeout", w.httpTimeout,
	)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	// Initial immediate poll
	w.pollAndDispatch(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("sync worker stopping, draining in-flight vouchers...")
			w.wg.Wait()
			slog.Info("sync worker stopped cleanly")
			return
		case <-ticker.C:
			w.pollAndDispatch(ctx)
		}
	}
}

// pollAndDispatch checks available concurrency slots and claims pending vouchers.
func (w *Worker) pollAndDispatch(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	// Calculate free slots
	freeSlots := w.concurrency - len(w.sem)
	if freeSlots <= 0 {
		return
	}

	claimed, err := w.service.Claim(ctx, freeSlots, w.lease)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("failed to claim vouchers in poll loop", "error", err)
		}
		return
	}

	for _, v := range claimed {
		// Acquire concurrency slot
		select {
		case w.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		w.wg.Add(1)
		go func(item vouchers.ClaimedVoucher) {
			defer func() {
				<-w.sem
				w.wg.Done()
			}()

			procCtx, cancel := context.WithTimeout(ctx, w.httpTimeout)
			defer cancel()

			_ = w.processor.Process(procCtx, item)
		}(v)
	}
}
