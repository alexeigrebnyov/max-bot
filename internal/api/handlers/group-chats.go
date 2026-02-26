// internal/api/handlers/group-chats.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/storage"
	"net/http"
)

type GroupChatsHandler struct {
	Storage *storage.Service
}

func (h *GroupChatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rows, err := h.Storage.GroupChats.All()
	if err != nil {
		log.Printf("group-chats: All error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "db error",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rows)
}
