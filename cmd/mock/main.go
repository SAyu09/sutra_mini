package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutra-mini/internal/config"
	"sutra-mini/internal/mock"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadMockConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	mockServer := mock.NewServer(mock.Options{
		ErrorRate:    cfg.ErrorRate,
		HangRate:     cfg.HangRate,
		HangDuration: cfg.HangDuration,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mockServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("mock accounting server listening",
			"addr", cfg.Addr,
			"error_rate", cfg.ErrorRate,
			"hang_rate", cfg.HangRate,
			"hang_duration", cfg.HangDuration,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("mock server failed", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		slog.Info("shutting down mock server...", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mock server graceful shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("mock server stopped cleanly")
}
