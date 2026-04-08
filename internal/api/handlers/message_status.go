// internal/api/handlers/message_status.go

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"time"
)

// MarkMessagesReadHandler обрабатывает POST /mark-messages-read
type MarkMessagesReadHandler struct {
	MessageStatus *tables.MessageStatus
}

type markMessagesReadRequest struct {
	ChatID      int64    `json:"chat_id"`
	MessageMids []string `json:"message_mids"`
}

func (h *MarkMessagesReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req markMessagesReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ChatID == 0 {
		http.Error(w, "chat_id required", http.StatusBadRequest)
		return
	}

	readAt := time.Now().UnixMilli()

	if len(req.MessageMids) == 0 {
		// Если не указаны конкретные сообщения, помечаем все как прочитанные
		if err := h.MessageStatus.MarkAllAsRead(req.ChatID, readAt); err != nil {
			log.Printf("MarkMessagesReadHandler: MarkAllAsRead error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Помечаем конкретные сообщения
		if err := h.MessageStatus.MarkMultipleAsRead(req.ChatID, req.MessageMids, readAt); err != nil {
			log.Printf("MarkMessagesReadHandler: MarkMultipleAsRead error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetUnreadCountHandler обрабатывает GET /unread-count?chat_id=...
type GetUnreadCountHandler struct {
	MessageStatus *tables.MessageStatus
}

func (h *GetUnreadCountHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	if chatIDStr == "" {
		http.Error(w, "chat_id required", http.StatusBadRequest)
		return
	}

	var chatID int64
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		http.Error(w, "invalid chat_id", http.StatusBadRequest)
		return
	}

	count, err := h.MessageStatus.GetUnreadCount(chatID)
	if err != nil {
		log.Printf("GetUnreadCountHandler error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"unread_count": count})
}

// GetUnreadMessagesHandler обрабатывает GET /unread-messages
type GetUnreadMessagesHandler struct {
	MessageStatus *tables.MessageStatus
}

func (h *GetUnreadMessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	messages, err := h.MessageStatus.GetUnreadMessages()
	if err != nil {
		log.Printf("GetUnreadMessagesHandler error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"messages": messages,
	})
}
