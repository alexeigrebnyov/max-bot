// internal/api/service.go

package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"max-bot-service/internal/api/handlers"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type Service struct {
	Bot     *bot.Service
	Storage *storage.Service
	Cfg     *config.Config
	Metrics *Metrics // счётчики запросов/ошибок/rate limit; nil — метрики не собираются
}

func NewService(botSrv *bot.Service, stor *storage.Service, cfg *config.Config) *Service {
	return &Service{
		Bot:     botSrv,
		Storage: stor,
		Cfg:     cfg,
		Metrics: NewMetrics(),
	}
}

// Таймаут ожидания корректного завершения HTTP-сервера при остановке.
const defaultShutdownTimeout = 10 * time.Second

// requireAPIKey при заданном apiKey возвращает обёртку, проверяющую заголовок X-API-Key.
// Если apiKey пустой — возвращает next без проверки.
func requireAPIKey(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(got), []byte(apiKey)) != 1 {
			slog.Warn("api: missing or invalid X-API-Key", "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "missing or invalid X-API-Key",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit при limit > 0 оборачивает next лимитером (запросов в минуту). При превышении — 429.
func rateLimit(limitPerMin int, metrics *Metrics, next http.Handler) http.Handler {
	if limitPerMin <= 0 {
		return next
	}
	limiter := rate.NewLimiter(rate.Limit(limitPerMin)/60, limitPerMin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			if metrics != nil {
				metrics.IncRateLimit()
			}
			slog.Warn("api: rate limit exceeded", "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "rate limit exceeded",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler возвращает HTTP-обработчик для тестирования без запуска сервера.
// Handler возвращает HTTP-обработчик для тестирования без запуска сервера.
func (srv *Service) Handler() http.Handler {
	return srv.buildMux()
}

func (srv *Service) buildMux() http.Handler {
	mux := http.NewServeMux()
	broker := handlers.NewEventBroker(srv.Bot)
	if !srv.Bot.UseLongPolling() {
		mux.HandleFunc("/webhook", srv.Bot.WebhookHandler())
	}
// 	webUIHandler, err := handlers.WebUIHandler()
//     if err != nil {
//         slog.Error("failed to init web UI", "error", err)
//     } else {
//         mux.Handle("/admin", webUIHandler)
//     }
	mux.Handle("/", &handlers.RootHandler{Bot: srv.Bot.BotModel})
	mux.Handle("/send-message", rateLimit(srv.Cfg.RateLimitPerMinute, srv.Metrics, &handlers.SendMessageHandler{Bot: srv.Bot.BotModel}))
	mux.Handle("/send-by-phone", rateLimit(srv.Cfg.RateLimitPerMinute, srv.Metrics, &handlers.SendByPhoneHandler{Bot: srv.Bot.BotModel}))
	mux.Handle("/send-to-group-by-chatid", &handlers.SendToGroupByChatIdHandler{Bot: srv.Bot.BotModel})
	mux.Handle("/get-messages-by-chatid", &handlers.GetChatMessagesHandler{Bot: srv.Bot.BotModel})
	mux.Handle("/refresh-group-chats", requireAPIKey(srv.Cfg.APIKey, &handlers.RefreshGroupChatsHandler{Bot: srv.Bot}))
	mux.Handle("/group-chats", requireAPIKey(srv.Cfg.APIKey, &handlers.GroupChatsHandler{Storage: srv.Storage}))
	mux.HandleFunc("/metrics", srv.serveMetrics)
	mux.Handle("/add-contact", &handlers.AddContactHandler{Contacts: srv.Storage.Contacts})
	mux.Handle("/update-contact", &handlers.UpdateContactHandler{Contacts: srv.Storage.Contacts})
    mux.Handle("/get-contact", &handlers.GetContactHandler{Contacts: srv.Storage.Contacts})
    mux.Handle("/contacts", &handlers.ContactsHandler{Contacts: srv.Storage.Contacts})
    // Новые:
    mux.Handle("/events", broker)
    mux.Handle("/send-chat-message", &handlers.SendChatMessageHandler{
        Bot: srv.Bot.BotModel,
        Events: broker,
        })
    mux.Handle("/admin", &handlers.WebUIHandler{})

	var h http.Handler = mux
	if srv.Metrics != nil {
		h = srv.Metrics.metricsMiddleware(mux)
	}
	return h
}

func (srv *Service) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if srv.Metrics == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(srv.Metrics.Snapshot())
}

func (srv *Service) Start(ctx context.Context) {
	port := srv.Cfg.Port
	slog.Info("http server starting", "port", port)
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: srv.buildMux()}

	go func() {
		<-ctx.Done()
		timeout := defaultShutdownTimeout
		if srv.Cfg.ShutdownTimeoutSec > 0 {
			timeout = time.Duration(srv.Cfg.ShutdownTimeoutSec) * time.Second
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
}
