// internal/api/handlers/send-to-group.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/storage"
	"net/http"
)

type SendToGroupByTitleRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type SendToGroupByTitleHandler struct {
	Bot     bot.BotClient
	Storage *storage.Service
}

func (h *SendToGroupByTitleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var req SendToGroupByTitleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("send-to-group-by-title: bad json:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("send-to-group-by-title: incoming title=%q text=%q", req.Title, req.Text)

	// ищем ChatId по Title
	gc, err := h.Storage.GroupChats.FindByTitle(req.Title)
	if err != nil {
		log.Println("send-to-group-by-title: FindByTitle error:", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "db error",
		})
		return
	}

	if gc == nil {
		log.Printf("send-to-group-by-title: no group chat found for title=%q", req.Title)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "group chat not found",
		})
		return
	}

	// отправляем в MAX по ChatId
	if err := h.Bot.SendToChatByID(r.Context(), gc.ChatID, req.Text); err != nil {
		log.Println("send-to-group-by-title: SendToChatByID error:", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"chat_id": gc.ChatID,
		"title":   gc.Title,
	})
}
