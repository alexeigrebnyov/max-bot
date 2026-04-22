package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/storage/tables"
	"net/http"
)

type AddContactHandler struct {
	Contacts *tables.Contacts
}

type UpdateContactHandler struct {
	Contacts *tables.Contacts
}

type addContactRequest struct {
	UserID    int64  `json:"user_id"`
	ChatID    int64  `json:"chat_id"`
	Phone     string `json:"phone"`
	Name      string `json:"name"`
	EMC       string `json:"emc"`
	AvatarURL string `json:"avatar_url"`
	EMCHash   string `json:"emchash"`
	Birthdate string `json:"birthdate"` // формат: dd.mm.yyyy
	Authorized bool `json:"authorized"` // формат: dd.mm.yyyy
}

func (h *AddContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req addContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("add-contact: decode error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	if req.UserID == 0 || req.Phone == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "user_id and phone are required"})
		return
	}

	contact := &tables.Contact{
		UserID:    req.UserID,
		ChatID:    req.ChatID, // будет заполнено при первом сообщении от пользователя
		Phone:     req.Phone,
		Name:      req.Name,
		EMC:       req.EMC,
		AvatarURL: req.AvatarURL,
		EMCHash:   req.EMCHash,
		Birthdate: req.Birthdate,
		Authorized: req.Authorized,
	}

	_, err := h.Contacts.Save(contact)
	if err != nil {
		log.Printf("add-contact: save error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to save contact"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *UpdateContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req addContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("add-contact: decode error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	if req.Phone == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "phone are required"})
		return
	}

	contact := &tables.Contact{
		UserID:    req.UserID,
		ChatID:    req.ChatID, // будет заполнено при первом сообщении от пользователя
		Phone:     req.Phone,
		Name:      req.Name,
		EMC:       req.EMC,
		AvatarURL: req.AvatarURL,
		EMCHash:   req.EMCHash,
		Birthdate: req.Birthdate,
	}

	cont, err := h.Contacts.UpdateByPhone(contact)
    	if err != nil {
    		log.Printf("contacts: UpdateByPhone error: %v", err)
    		w.Header().Set("Content-Type", "application/json")
    		w.WriteHeader(http.StatusInternalServerError)
    		_ = json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
    		return
    	}

    	w.Header().Set("Content-Type", "application/json")
    	_ = json.NewEncoder(w).Encode(map[string]interface{}{
    		"status":   "ok",
    		"contacts": cont,
    	})
}