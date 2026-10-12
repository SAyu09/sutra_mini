package main

import (
	"fmt"
	"log/slog"
	"os"

	"sutra-mini/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadMockConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("mock configured successfully",
		"addr", cfg.Addr,
		"error_rate", cfg.ErrorRate,
		"hang_rate", cfg.HangRate,
		"hang_duration", cfg.HangDuration,
	)
}
