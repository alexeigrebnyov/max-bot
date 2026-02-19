package config

import (
	"log"
	"os"
)

type Config struct {
	BotToken      string
	WebhookURL    string
	WebhookSecret string
	ApiBaseURL    string
}

func Load() *Config {
	cfg := &Config{
		BotToken:      mustEnv("BOT_TOKEN"),
		WebhookURL:    os.Getenv("WEBHOOK_URL"),      // может быть пустым
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),   // может быть пустым
		ApiBaseURL:    os.Getenv("MAX_API_BASE_URL"), // опционально, по умолчанию ниже
	}

	if cfg.ApiBaseURL == "" {
		cfg.ApiBaseURL = "https://platform-api.max.ru"
	}

	return cfg
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}
