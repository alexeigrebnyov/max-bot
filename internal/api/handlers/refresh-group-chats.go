// internal/api/handlers/refresh-group-chats.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/bot"
	"net/http"
)

type RefreshGroupChatsHandler struct {
	Bot *bot.Service
}

func (h *RefreshGroupChatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log.Println("refresh-group-chats: manual trigger")

	if err := h.Bot.RefreshGroupChats(ctx); err != nil {
		log.Printf("refresh-group-chats: error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
