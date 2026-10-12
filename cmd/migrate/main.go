package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"sutra-mini/internal/config"
	"sutra-mini/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: migrate <up|down|seed>\n")
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "up", "down", "seed":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q: must be one of up, down, seed\n", command)
		os.Exit(1)
	}

	cfg, err := config.LoadMigrateConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL, db.DefaultOptions())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	slog.Info("migrate configured successfully", "command", command)
}
