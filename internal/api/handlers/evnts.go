package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"max-bot-service/internal/bot"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"sync"
	"time" // Не забудьте импортировать time
)

type EventBroker struct {
	mu          sync.Mutex
	clients     map[chan []byte]bool
	newMessages chan *bot.Message
	newContacts chan *tables.Contact
	botSrv      *bot.Service
}

func NewEventBroker(botSrv *bot.Service) *EventBroker {
	b := &EventBroker{
		clients:     make(map[chan []byte]bool),
		newMessages: botSrv.NewMessages,
		newContacts: botSrv.NewContacts,
		botSrv:      botSrv,
	}
	if b.newMessages != nil {
		go b.broadcastMessagesLoop()
	}
	if b.newContacts != nil {
		go b.broadcastContactsLoop()
	}
	return b
}

// broadcastMessagesLoop рассылает сообщения по каналам
func (b *EventBroker) broadcastMessagesLoop() {
	for msg := range b.newMessages {
		data, err := json.Marshal(map[string]interface{}{
			"type": "message",
			"data": msg,
		})
		if err != nil {
			log.Printf("broadcastMessagesLoop: marshal error: %v", err)
			continue
		}
		b.broadcast(data)
	}
}

// broadcastContactsLoop рассылает обновления контактов по каналам
func (b *EventBroker) broadcastContactsLoop() {
	for contact := range b.newContacts {
		data, err := json.Marshal(map[string]interface{}{
			"type": "contact_update",
			"data": contact,
		})
		if err != nil {
			log.Printf("broadcastContactsLoop: marshal error: %v", err)
			continue
		}
		b.broadcast(data)
	}
}

// broadcast отправляет данные всем подключенным клиентам
func (b *EventBroker) broadcast(data []byte) {
	b.mu.Lock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// Если клиент не успевает читать, удаляем его
			close(ch)
			delete(b.clients, ch)
		}
	}
	b.mu.Unlock()
}

// ServeHTTP — вот здесь мы добавляем цикл с тикером
func (b *EventBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("EventBroker: new client connected")

	// Проверяем, поддерживает ли ResponseWriter flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("EventBroker: ResponseWriter does not support flushing")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Создаем канал для конкретного браузера
	ch := make(chan []byte, 10)

	b.mu.Lock()
	b.clients[ch] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		log.Printf("EventBroker: client disconnected")
	}()

	// Отправляем начальный flush
	flusher.Flush()

	// Инициализируем тикер для пинга
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	// ГЛАВНЫЙ ЦИКЛ ОБРАБОТКИ
	for {
		select {
		case data := <-ch:
			// Здесь 'data' — это то, что пришло из broadcastLoop
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()

		case <-ticker.C:
			// Отправляем пустой комментарий (ping), чтобы браузер не закрыл соединение
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			// Если пользователь закрыл вкладку браузера
			return
		}
	}
}

// Метод для ручной отправки сообщения в канал SSE
func (b *EventBroker) BroadcastStructuredMessage(msg *bot.Message) {
    if b == nil { return }

    data, err := json.Marshal(msg)
    if err != nil {
        log.Printf("BroadcastStructuredMessage: marshal error: %v", err)
        return
    }

    b.mu.Lock()
    defer b.mu.Unlock()
    for ch := range b.clients {
        select {
        case ch <- data:
        default:
            close(ch)
            delete(b.clients, ch)
        }
    }
}

// SendChatMessageHandler обрабатывает POST /send-chat-message
type SendChatMessageHandler struct {
    Bot bot.BotClient
    Events *EventBroker
}

type sendChatMessageRequest struct {
    ChatID int64  `json:"chat_id"`
    Text   string `json:"text"`
}

func (h *SendChatMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    var req sendChatMessageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if req.ChatID == 0 || req.Text == "" {
        http.Error(w, "chat_id and text required", http.StatusBadRequest)
        return
    }

    if err := h.Bot.SendToChatByID(r.Context(), req.ChatID, req.Text); err != nil {
        log.Printf("SendChatMessageHandler error: %v", err)
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Отправляем исходящее сообщение в EventSource
    if h.Events != nil {
        msg := h.Bot.GetStructuredMessage(req.ChatID, req.Text)
        h.Events.BroadcastStructuredMessage(msg)
    }

    w.WriteHeader(http.StatusNoContent)
}
