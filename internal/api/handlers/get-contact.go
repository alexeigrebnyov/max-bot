package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/storage/tables"
	"net/http"
)

type GetContactHandler struct {
	Contacts *tables.Contacts
}

func (h *GetContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	phone := r.URL.Query().Get("phone")
	userIDStr := r.URL.Query().Get("user_id")
	if phone == "" && userIDStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "missing phone or user_id parameter",
		})
		return
	}

	var contact *tables.Contact
	var err error
	if phone != "" {
		contact, err = h.Contacts.Find(phone)
	} else {
		// user_id provided as string; Find will search by userID, chatID, phone
		contact, err = h.Contacts.Find(userIDStr)
	}

	if err != nil {
		log.Printf("get-contact: find error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	if contact == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "contact not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"contact": contact,
	})
}

type ContactsHandler struct {
	Contacts *tables.Contacts
}

func (h *ContactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	contacts, err := h.Contacts.All()
	if err != nil {
		log.Printf("contacts: All error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"contacts": contacts,
	})
}