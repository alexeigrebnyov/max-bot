// internal/api/handlers/appointment-reminder.go

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"strconv"
	"time"
)

// SendAppointmentReminderHandler обрабатывает POST /send-appointment-reminder
// Отправляет сообщение с кнопками Приду/Отменить/Перенести
type SendAppointmentReminderHandler struct {
	Bot          bot.BotClient
	Appointments *tables.Appointments
}

type appointmentReminderRequest struct {
	AppointmentID  int64  `json:"appointment_id"`
	Text           string `json:"text"`
	ChatID         int64  `json:"chat_id"`       // опционально: явный chat_id
	PatientPhone   string `json:"patient_phone"` // опционально: телефон для поиска контакта
}

func (h *SendAppointmentReminderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req appointmentReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("send-appointment-reminder: decode error: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.AppointmentID == 0 || req.Text == "" {
		http.Error(w, "appointment_id and text are required", http.StatusBadRequest)
		return
	}

	// Находим запись для проверки
	apt, err := h.Appointments.FindByID(req.AppointmentID)
	if err != nil {
		log.Printf("send-appointment-reminder: FindByID error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if apt == nil {
		http.Error(w, "Appointment not found", http.StatusNotFound)
		return
	}

	// Определяем chat_id для отправки
	var chatID int64
	if req.ChatID != 0 {
		chatID = req.ChatID
	} else {
		chatID = apt.PatientChatID
	}

	if chatID == 0 {
		http.Error(w, "chat_id required (not found for appointment)", http.StatusBadRequest)
		return
	}

	// Создаем клавиатуру с кнопками
	// Payload содержит команду и ID записи
	kb := bot.Keyboard{
		Buttons: [][]bot.KeyboardButton{
			{
				{Type: "message", Text: "✅ Приду", Payload: fmt.Sprintf("/appointment_confirm %d", req.AppointmentID)},
			},
			{
				{Type: "message", Text: "❌ Отменить", Payload: fmt.Sprintf("/appointment_cancel %d", req.AppointmentID)},
			},
			{
				{Type: "message", Text: "🗓 Перенести", Payload: fmt.Sprintf("/appointment_reschedule %d", req.AppointmentID)},
			},
		},
	}

	// Отправляем сообщение с клавиатурой
	ctx := r.Context()
	if err := h.Bot.SendToChatByIDWithKeyboard(ctx, chatID, req.Text, kb); err != nil {
		log.Printf("send-appointment-reminder: SendToChatByIDWithKeyboard error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Помечаем что напоминание отправлено
	if err := h.Appointments.MarkReminderSent(req.AppointmentID); err != nil {
		log.Printf("send-appointment-reminder: MarkReminderSent error: %v", err)
		// Не возвращаем ошибку, т.к. сообщение уже отправлено
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "reminder sent",
	})
}

// CreateAppointmentHandler обрабатывает POST /create-appointment
// Создает новую запись на прием
type CreateAppointmentHandler struct {
	Appointments *tables.Appointments
}

type createAppointmentRequest struct {
	PatientChatID    int64  `json:"patient_chat_id"`
	PatientName      string `json:"patient_name"`
	AppointmentTime  string `json:"appointment_time"` // RFC3339 формат
	DoctorName       string `json:"doctor_name"`
	Department       string `json:"department"`
}

func (h *CreateAppointmentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req createAppointmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("create-appointment: decode error: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.PatientChatID == 0 || req.AppointmentTime == "" {
		http.Error(w, "patient_chat_id and appointment_time are required", http.StatusBadRequest)
		return
	}

	// Парсим время
	aptTime, err := time.Parse(time.RFC3339, req.AppointmentTime)
	if err != nil {
		// Пробуем другой формат
		aptTime, err = time.Parse("2006-01-02 15:04:05", req.AppointmentTime)
		if err != nil {
			http.Error(w, "Invalid appointment_time format (use RFC3339 or YYYY-MM-DD HH:MM:SS)", http.StatusBadRequest)
			return
		}
	}

	apt := &tables.Appointment{
		PatientChatID:   req.PatientChatID,
		PatientName:     req.PatientName,
		AppointmentTime: aptTime,
		DoctorName:      req.DoctorName,
		Department:      req.Department,
		Status:          "pending",
	}

	id, err := h.Appointments.Create(apt)
	if err != nil {
		log.Printf("create-appointment: Create error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"appointment_id": id,
	})
}

// ListAppointmentsHandler обрабатывает GET /appointments
type ListAppointmentsHandler struct {
	Appointments *tables.Appointments
}

func (h *ListAppointmentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	chatIDStr := r.URL.Query().Get("chat_id")
	var appointments []*tables.Appointment
	var err error

	if chatIDStr != "" {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid chat_id", http.StatusBadRequest)
			return
		}
		appointments, err = h.Appointments.ListByChatID(chatID)
	} else {
		appointments, err = h.Appointments.ListPending(100)
	}

	if err != nil {
		log.Printf("list-appointments: error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"appointments": appointments,
	})
}
