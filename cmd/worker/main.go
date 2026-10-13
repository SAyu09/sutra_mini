package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutra-mini/internal/config"
	"sutra-mini/internal/db"
	"sutra-mini/internal/vouchers"
	"sutra-mini/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, db.DefaultOptions())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	voucherSvc := vouchers.NewService(pool)
	accountingClient := worker.NewHTTPAccountingClient(cfg.MockURL, cfg.HTTPTimeout)

	w, err := worker.New(worker.Options{
		VoucherService: voucherSvc,
		Client:         accountingClient,
		MaxAttempts:    cfg.MaxAttempts,
		BackoffBase:    cfg.BackoffBase,
		BackoffJitter:  cfg.BackoffJitter,
		HTTPTimeout:    cfg.HTTPTimeout,
		Lease:          cfg.Lease,
		PollInterval:   cfg.PollInterval,
		Concurrency:    cfg.Concurrency,
	})
	if err != nil {
		slog.Error("failed to initialize worker", "error", err)
		os.Exit(1)
	}

	// Trap termination signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	workerDone := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(workerDone)
	}()

	slog.Info("sync worker daemon running",
		"mock_url", cfg.MockURL,
		"max_attempts", cfg.MaxAttempts,
		"lease", cfg.Lease,
		"poll_interval", cfg.PollInterval,
		"concurrency", cfg.Concurrency,
	)

	sig := <-sigChan
	slog.Info("shutdown signal received, canceling context...", "signal", sig.String())
	cancel()

	// Wait up to LEASE + 5s for clean drain
	shutdownTimeout := cfg.Lease + 5*time.Second
	select {
	case <-workerDone:
		slog.Info("sync worker finished draining successfully")
	case <-time.After(shutdownTimeout):
		slog.Warn("shutdown timed out waiting for worker goroutines to exit", "timeout", shutdownTimeout)
	}

	slog.Info("sync worker process exiting")
}
