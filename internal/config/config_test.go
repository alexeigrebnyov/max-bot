package config

import (
	"os"
	"testing"
)

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7")
	defer func() {
		_ = os.Unsetenv("MAX_API_BASE_URL")
		_ = os.Unsetenv("API_BASE_URL")
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("RATE_LIMIT_PER_MINUTE")
		_ = os.Unsetenv("SHUTDOWN_TIMEOUT_SEC")
	}()

	os.Unsetenv("MAX_API_BASE_URL")
	os.Unsetenv("API_BASE_URL")
	os.Unsetenv("PORT")
	os.Unsetenv("RATE_LIMIT_PER_MINUTE")
	os.Unsetenv("SHUTDOWN_TIMEOUT_SEC")

	cfg := Load()

	if cfg.BotToken != "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7" {
		t.Errorf("BotToken: got %q", cfg.BotToken)
	}
	if cfg.ApiBaseURL != DefaultApiBaseURL {
		t.Errorf("ApiBaseURL: got %q, want %q", cfg.ApiBaseURL, DefaultApiBaseURL)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port: got %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.RateLimitPerMinute != DefaultRateLimitPerMinute {
		t.Errorf("RateLimitPerMinute: got %d, want %d", cfg.RateLimitPerMinute, DefaultRateLimitPerMinute)
	}
	if cfg.ShutdownTimeoutSec != 0 {
		t.Errorf("ShutdownTimeoutSec: got %d, want 0 (use default)", cfg.ShutdownTimeoutSec)
	}
}

func TestLoad_PortFromEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7")
	t.Setenv("PORT", "9090")
	defer os.Unsetenv("PORT")

	cfg := Load()
	if cfg.Port != 9090 {
		t.Errorf("Port: got %d, want 9090", cfg.Port)
	}
}

func TestLoad_ApiBaseURLFromEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7")
	t.Setenv("MAX_API_BASE_URL", "https://custom.max.ru")
	defer os.Unsetenv("MAX_API_BASE_URL")

	cfg := Load()
	if cfg.ApiBaseURL != "https://custom.max.ru" {
		t.Errorf("ApiBaseURL: got %q", cfg.ApiBaseURL)
	}
}

func TestLoad_RateLimitAndShutdownFromEnv(t *testing.T) {
	t.Setenv("BOT_TOKEN", "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "120")
	t.Setenv("SHUTDOWN_TIMEOUT_SEC", "15")
	defer func() {
		_ = os.Unsetenv("RATE_LIMIT_PER_MINUTE")
		_ = os.Unsetenv("SHUTDOWN_TIMEOUT_SEC")
	}()

	cfg := Load()
	if cfg.RateLimitPerMinute != 120 {
		t.Errorf("RateLimitPerMinute: got %d, want 120", cfg.RateLimitPerMinute)
	}
	if cfg.ShutdownTimeoutSec != 15 {
		t.Errorf("ShutdownTimeoutSec: got %d, want 15", cfg.ShutdownTimeoutSec)
	}
}

func TestLoad_UseLongPollingWhenWebhookURLEmpty(t *testing.T) {
	t.Setenv("BOT_TOKEN", "f9LHodD0cOL7q8e32yXdHIzR1UW2QaU5OvhRliDTjfqpfT28tLqH3C35MxwcZp5yoUfB3XpQk8Xli5M-eHd7")
	os.Unsetenv("WEBHOOK_URL")
	cfg := Load()
	if !cfg.UseLongPolling {
		t.Error("UseLongPolling: expected true when WEBHOOK_URL empty")
	}

	t.Setenv("WEBHOOK_URL", "https://example.com/webhook")
	cfg2 := Load()
	if cfg2.UseLongPolling {
		t.Error("UseLongPolling: expected false when WEBHOOK_URL set")
	}
}
