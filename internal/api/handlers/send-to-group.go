// internal/api/handlers/send-to-group.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/bot"
	"net/http"
)

type SendToGroupByChatIdRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type SendToGroupByChatIdHandler struct {
	Bot bot.BotClient
}

func (h *SendToGroupByChatIdHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var req SendToGroupByChatIdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("send-to-group-by-chatid: bad json: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "invalid json: " + err.Error(),
		})
		return
	}

	if req.ChatID == 0 {
		log.Printf("send-to-group-by-chatid: chat_id is required and must be non-zero")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "chat_id is required and must be non-zero",
		})
		return
	}

	log.Printf("send-to-group-by-chatid: chat_id=%d text=%q", req.ChatID, req.Text)

	if err := h.Bot.SendToChatByID(r.Context(), req.ChatID, req.Text); err != nil {
		log.Printf("send-to-group-by-chatid: SendToChatByID error: %v", err)
		w.Header().Set("Content-Type", "application/json")
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
		"chat_id": req.ChatID,
	})
}
