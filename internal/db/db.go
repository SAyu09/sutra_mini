package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options allows configuring the pool connection behavior.
type Options struct {
	MaxConns        int32
	MaxRetries      int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	TotalTimeout    time.Duration
}

// DefaultOptions returns standard pool options suitable for Neon connections.
func DefaultOptions() Options {
	return Options{
		MaxConns:        5,
		MaxRetries:      10,
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     5 * time.Second,
		TotalTimeout:    35 * time.Second,
	}
}

// Connect creates a pgxpool.Pool with connection retry and exponential backoff
// to gracefully handle Neon/Supabase cold starts and poolers.
func Connect(ctx context.Context, connString string, opts Options) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	// Disable prepared statement cache for compatibility with PgBouncer / Supabase poolers
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	if opts.MaxConns <= 0 {
		opts.MaxConns = 5
	}
	config.MaxConns = opts.MaxConns

	timeoutCtx, cancel := context.WithTimeout(ctx, opts.TotalTimeout)
	defer cancel()

	interval := opts.InitialInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if opts.MaxInterval <= 0 {
		opts.MaxInterval = 5 * time.Second
	}

	var pool *pgxpool.Pool
	var lastErr error

	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		select {
		case <-timeoutCtx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("connecting to database timed out after retries: %w", lastErr)
			}
			return nil, timeoutCtx.Err()
		default:
		}

		pool, err = pgxpool.NewWithConfig(timeoutCtx, config)
		if err == nil {
			pingCtx, pingCancel := context.WithTimeout(timeoutCtx, 3*time.Second)
			err = pool.Ping(pingCtx)
			pingCancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}

		lastErr = err
		slog.Warn("database connection attempt failed, retrying...",
			"attempt", attempt,
			"error", err,
			"next_retry_in", interval,
		)

		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("connecting to database timed out: %w", lastErr)
		case <-time.After(interval):
		}

		interval *= 2
		if interval > opts.MaxInterval {
			interval = opts.MaxInterval
		}
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", opts.MaxRetries, lastErr)
}
