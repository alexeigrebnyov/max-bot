// cmd/main.go

package main

import (
	"context"
	"log/slog"
	"max-bot-service/internal/api"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"os"
	"os/signal"
)

func initLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := config.Load()
	initLogger(cfg)

	storeService := storage.NewService()
	storeService.Start(ctx)

	botService := bot.NewService(storeService, cfg)
	botService.Start(ctx)

	apiService := api.NewService(botService, storeService, cfg)
	apiService.Start(ctx)

	<-ctx.Done()
}
