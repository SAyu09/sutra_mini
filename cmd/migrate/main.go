package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"sutra-mini/internal/config"
	"sutra-mini/internal/db"
	"sutra-mini/migrations"
	"sutra-mini/seed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: migrate <up|down|seed|ping>\n")
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "up", "down", "seed", "ping":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q: must be one of up, down, seed, ping\n", command)
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

	switch command {
	case "ping":
		if err := pool.Ping(ctx); err != nil {
			slog.Error("failed to ping database", "error", err)
			os.Exit(1)
		}
		fmt.Println("database connection ok")

	case "up":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			slog.Error("failed to set dialect", "error", err)
			os.Exit(1)
		}

		stdDB := stdlib.OpenDBFromPool(pool)
		defer stdDB.Close()

		if err := goose.Up(stdDB, "."); err != nil {
			slog.Error("failed to apply migrations up", "error", err)
			os.Exit(1)
		}
		fmt.Println("migrations applied successfully (up)")

	case "down":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			slog.Error("failed to set dialect", "error", err)
			os.Exit(1)
		}

		stdDB := stdlib.OpenDBFromPool(pool)
		defer stdDB.Close()

		if err := goose.Down(stdDB, "."); err != nil {
			slog.Error("failed to rollback migration (down)", "error", err)
			os.Exit(1)
		}
		fmt.Println("migration rolled back successfully (down)")

	case "seed":
		seedSQL, err := seed.FS.ReadFile("seed.sql")
		if err != nil {
			slog.Error("failed to read embedded seed.sql", "error", err)
			os.Exit(1)
		}

		if _, err := pool.Exec(ctx, string(seedSQL)); err != nil {
			slog.Error("failed to execute seed.sql", "error", err)
			os.Exit(1)
		}
		fmt.Println("seed data applied successfully")
	}
}
