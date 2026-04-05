// internal/api/handlers/send-message.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/bot"
	"net/http"
)

type SendMessageHandler struct {
	Bot bot.BotClient
}

func (handler *SendMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var message struct {
		Chat    string `json:"chat"`
		Text    string `json:"text"`
		Thread  int    `json:"thread"`
		Private bool   `json:"private"`
	}

	if err := decoder.Decode(&message); err != nil {
		log.Printf("send-message: decode error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "invalid json: " + err.Error(),
		})
		return
	}

	if err := handler.Bot.SendMessage(r.Context(), message.Chat, message.Thread, message.Text, message.Private); err != nil {
		log.Printf("send-message: SendMessage error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Описывает JSON‑тело запроса: ожидается объект вида
// {"phone": "79039076399", "text": "Привет"}
type SendByPhoneRequest struct {
	Phone string `json:"phone"`
	Text  string `json:"text"`
}

// Хендлер держит внутри клиента бота (bot.BotClient),
// через которого и отправляется сообщение в MAX.
type SendByPhoneHandler struct {
	Bot bot.BotClient
}

// Делает SendByPhoneHandler совместимым с интерфейсом http.Handler,
// чтобы его можно было повесить на роут "/send-by-phone" через http.Handle.
// Из любого внешнего приложения можешь сделать:
// POST /send-by-phone
// Content-Type: application/json
//
//	{
//	 "phone": "79039076399",
//	 "text": "Ваше уведомление"
//	}
func (handler *SendByPhoneHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	} // Разрешает только POST. Любой другой метод (GET/PUT/DELETE) получает 405.

	defer r.Body.Close() // Закрывает тело запроса по завершении.

	var req SendByPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("send-by-phone: bad json: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "invalid json: " + err.Error(),
		})
		return
	}
	//	Если JSON кривой или поля не совпадают — логирует ошибку и возвращает 400.

	log.Printf("send-by-phone: incoming request phone=%s text=%q", req.Phone, req.Text)

	if req.Phone == "" {
		log.Printf("send-by-phone: phone is required")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "phone is required",
		})
		return
	}

	// используем телефон как ключ, private = true
	if err := handler.Bot.SendMessage(r.Context(), req.Phone, 0, req.Text, true); err != nil {
		log.Printf("send-by-phone: SendMessage error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("send-by-phone: message sent ok for phone=%s", req.Phone)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"phone":  req.Phone,
	})
}

type GetChatMessagesHandler struct {
	Bot bot.BotClient
}

func (handler *GetChatMessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
    var chat_id = r.URL.Query().Get("chat_id")
    if chat_id == "" {
        log.Printf("get-chat-messages: chat_id is required")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{
            "status":  "error",
            "message": "chat_id is required",
        })
        return
    }

	rows, err := handler.Bot.GetChatMessages(r.Context(), chat_id);
	if err != nil {
		log.Printf("get-chat-messages: error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

		log.Printf("get-chat-messages ok for chat_id=%s", chat_id)

    	w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(http.StatusOK)
    	_ = json.NewEncoder(w).Encode(rows)
}