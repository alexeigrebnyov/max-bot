// internal/api/service.go

package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"max-bot-service/internal/api/handlers"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/storage"
	"net/http"
)

const port = 8080

type Service struct {
	Bot     *bot.Service
	Storage *storage.Service
}

func NewService(botSrv *bot.Service, stor *storage.Service) *Service {
	return &Service{
		Bot:     botSrv,
		Storage: stor,
	}
}

func (srv *Service) Start(ctx context.Context) {
	log.Printf("Listening on port %d", port)

	// Webhook endpoint для MAX
	http.HandleFunc("/webhook", srv.Bot.WebhookHandler())

	// API endpoints
	http.Handle("/", &handlers.RootHandler{Bot: srv.Bot.BotModel})
	http.Handle("/send-message", &handlers.SendMessageHandler{Bot: srv.Bot.BotModel})
	http.Handle("/send-by-phone", &handlers.SendByPhoneHandler{Bot: srv.Bot.BotModel})
	http.Handle("/send-to-group-by-title", &handlers.SendToGroupByTitleHandler{
		Bot:     srv.Bot.BotModel,
		Storage: srv.Storage,
	})
	http.Handle("/refresh-group-chats", &handlers.RefreshGroupChatsHandler{
		Bot: srv.Bot,
	})
	http.Handle("/group-chats", &handlers.GroupChatsHandler{
		Storage: srv.Storage,
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: nil}

	go func() {
		<-ctx.Done()
		if err := server.Shutdown(ctx); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
}
