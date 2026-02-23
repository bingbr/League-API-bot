package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/bingbr/League-API-bot/internal/app"
	"github.com/bingbr/League-API-bot/internal/config"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("config error", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := app.Run(context.Background(), cfg); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}
