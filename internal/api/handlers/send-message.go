// internal/api/handlers/send-message.go

package handlers

import (
	"encoding/json"
	"log"
	"max-bot-service/internal/bot"
	"net/http"
)

type SendMessageHandler struct {
	// Bot *bot.Model
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
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := handler.Bot.SendMessage(r.Context(), message.Chat, message.Thread, message.Text, message.Private); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
