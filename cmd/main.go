// cmd/main.go

package main

import (
	"context"
	"max-bot-service/internal/api"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"os"
	"os/signal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := config.Load()

	storeService := storage.NewService()
	storeService.Start(ctx)

	botService := bot.NewService(storeService, cfg)
	botService.Start(ctx)

	apiService := api.NewService(botService, storeService)
	apiService.Start(ctx)

	<-ctx.Done()
}
