package config

import (
	"log"
	"os"
	"strconv"
)

const (
	DefaultApiBaseURL = "https://platform-api.max.ru"
	DefaultPort       = 9003
)

const (
	DefaultRateLimitPerMinute = 60 // 0 = отключено
)

type Config struct {
	BotToken           string
	WebhookURL         string
	WebhookSecret      string
	ApiBaseURL         string
	Port               int    // порт HTTP-сервера (PORT, по умолчанию 8080)
	APIKey             string // опционально: X-API-Key для /refresh-group-chats и /group-chats
	RateLimitPerMinute int    // лимит запросов в минуту (0 = без лимита)
	ShutdownTimeoutSec int    // таймаут завершения HTTP-сервера в секундах (0 = 10 с)
	LogLevel           string // debug|info|warn|error (LOG_LEVEL, по умолчанию info)
	LogFormat          string // json|text (LOG_FORMAT, по умолчанию text)
	UseLongPolling     bool   // true, если WEBHOOK_URL пустой

	// Эндпоинты бэкенда для управления записями
	AppointmentCancelEndpoint     string // эндпоинт для отмены записи (POST с appointment_id)
	AppointmentDoctorsEndpoint    string // эндпоинт для получения врачей по отделению
	AppointmentBackendAPIKey      string // API ключ для авторизации на бэкенде
}

func Load() *Config {
	cfg := &Config{
		BotToken:      mustEnv("BOT_TOKEN"),
		WebhookURL:    os.Getenv("WEBHOOK_URL"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		ApiBaseURL:    os.Getenv("MAX_API_BASE_URL"),
		APIKey:        os.Getenv("API_KEY"),
	}

	if cfg.ApiBaseURL == "" {
		cfg.ApiBaseURL = os.Getenv("API_BASE_URL")
	}
	if cfg.ApiBaseURL == "" {
		cfg.ApiBaseURL = DefaultApiBaseURL
	}

	if p := os.Getenv("PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			cfg.Port = port
		} else {
			cfg.Port = DefaultPort
		}
	} else {
		cfg.Port = DefaultPort
	}

	if r := os.Getenv("RATE_LIMIT_PER_MINUTE"); r != "" {
		if n, err := strconv.Atoi(r); err == nil && n >= 0 {
			cfg.RateLimitPerMinute = n
		} else {
			cfg.RateLimitPerMinute = DefaultRateLimitPerMinute
		}
	} else {
		cfg.RateLimitPerMinute = DefaultRateLimitPerMinute
	}

	if s := os.Getenv("SHUTDOWN_TIMEOUT_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.ShutdownTimeoutSec = n
		}
	}

	switch os.Getenv("LOG_LEVEL") {
	case "debug", "info", "warn", "error":
		cfg.LogLevel = os.Getenv("LOG_LEVEL")
	default:
		cfg.LogLevel = "info"
	}
	switch os.Getenv("LOG_FORMAT") {
	case "json", "text":
		cfg.LogFormat = os.Getenv("LOG_FORMAT")
	default:
		cfg.LogFormat = "text"
	}

	cfg.UseLongPolling = cfg.WebhookURL == ""

	// Эндпоинты бэкенда для записей
	cfg.AppointmentCancelEndpoint = os.Getenv("APPOINTMENT_CANCEL_ENDPOINT")
	cfg.AppointmentDoctorsEndpoint = os.Getenv("APPOINTMENT_DOCTORS_ENDPOINT")
	cfg.AppointmentBackendAPIKey = os.Getenv("APPOINTMENT_BACKEND_API_KEY")

	return cfg
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}
