// internal/bot/service.go

package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"strconv"
)

const (
	menuBackToMain          = "Вернуться к главному меню 🗄"
	menuNotificationSetting = "Настроить уведомления ✉"
	menuSendPhoneNumber     = "Отправить свой номер телефона ☎️"
	menuExcludePhoneNumber  = "Исключить свой номер телефона ❌"
)

type Service struct {
	storage       *storage.Service
	Bot           *Model
	webhookSecret string

	cfg *config.Config
}

func NewService(storage *storage.Service, cfg *config.Config) *Service {
	return &Service{
		storage: storage,
		cfg:     cfg,
	}
}

func (srv *Service) Start(ctx context.Context) {
	// Модель, работающая с MAX через HTTP
	srv.Bot = NewModel(srv.storage.Contacts, srv.cfg)

	// Один раз получаем информацию о боте
	if err := srv.Bot.FillInfo(ctx); err != nil {
		log.Printf("failed to load bot info: %v", err)
	} else {
		log.Printf("Bot info: ID=%d, Nick=%s", srv.Bot.ID, srv.Bot.Name)
	}
}

// WebhookHandler – приём входящих событий от MAX
// На стороне MAX при подписке на webhook указывать этот же secret,
// и платформа будет слать заголовок X-Webhook-Secret
func (srv *Service) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Проверка секрета
		if srv.cfg.WebhookSecret != "" {
			if r.Header.Get("X-Webhook-Secret") != srv.cfg.WebhookSecret {
				log.Printf("webhook: invalid secret from %s", r.RemoteAddr)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		defer r.Body.Close()

		var upd struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}

		if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
			log.Printf("webhook: decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("webhook: type=%s", upd.Type)

		switch upd.Type {
		case "message_created":
			srv.handleMessageCreated(r.Context(), upd.Payload)
		default:
			// остальные пока игнорируем
		}

		w.WriteHeader(http.StatusOK)
	}
}

// MAX: пример структуры события message_created (упрощённо)
type messageCreatedPayload struct {
	Message struct {
		ID   string `json:"id"`
		Text string `json:"text"`
		// Упрощённо: контакт и отправитель
		Contact *struct {
			PhoneNumber string `json:"phone"`
		} `json:"contact"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		NewChatMembers []struct {
			ID    int64 `json:"id"`
			IsBot bool  `json:"is_bot"`
		} `json:"new_chat_members"`
	} `json:"message"`
	Chat struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
}

// Если не заполняешь srv.Bot.ID, можно не сравнивать по ID,
// а просто считать, что если в new_chat_members есть хотя бы один бот,
// значит добавили и нашего (это корректно, если нет сценариев,
// где в чат добавляют сторонних ботов).
func (srv *Service) handleMessageCreated(ctx context.Context, raw json.RawMessage) {
	var p messageCreatedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		log.Printf("message_created: parse error: %v", err)
		return
	}

	// 0) Если бота добавили в чат (обычно group/supergroup)
	if len(p.Message.NewChatMembers) > 0 {
		for _, m := range p.Message.NewChatMembers {
			// ID бота — это srv.Bot.ID (мы можем хранить его отдельно при старте через /me, если нужно)
			if m.IsBot {
				// На всякий случай просто реагируем, когда бот среди новых участников
				text := fmt.Sprintf("Спасибо за добавление! Идентификатор чата: %s", p.Chat.ID)
				if err := srv.Bot.SendMessage(ctx, p.Chat.ID, 0, text, false); err != nil {
					log.Printf("send new member message error: %v", err)
				}
				return
			}
		}
	}

	// 1) Только приватные чаты для остальной логики
	if p.Chat.Type != "private" {
		return
	}

	// 2) Обработка текстовых команд / меню
	switch p.Message.Text {
	case "/start", menuBackToMain:
		srv.sendMainMenuMessage(ctx, p.Chat.ID)
		return

	case menuNotificationSetting:
		srv.sendNotificationSettingMessage(ctx, p.Chat.ID)
		return

	case menuExcludePhoneNumber:
		srv.deleteContact(ctx, p.Chat.ID)
		return
	}

	// 3) Обработка контакта
	if p.Message.Contact != nil {
		srv.saveContact(ctx, p.Chat.ID, p.Message.From.ID, p.Message.Contact.PhoneNumber)
		log.Printf("message_created: chatId=%s, type=%s", p.Chat.ID, p.Chat.Type)
		return
	}
}

// Главное меню – пока просто текст, без клавиатуры
func (srv *Service) sendMainMenuMessage(ctx context.Context, chatID string) {
	text := "Добро пожаловать!"

	kb := keyboard{
		Buttons: [][]keyboardButton{
			{
				{
					Type:    "message",
					Text:    menuNotificationSetting,
					Payload: menuNotificationSetting,
				},
			},
			{
				{
					Type:    "message",
					Text:    menuExcludePhoneNumber,
					Payload: menuExcludePhoneNumber,
				},
			},
			{
				{
					Type:    "message",
					Text:    menuBackToMain,
					Payload: menuBackToMain,
				},
			},
		},
	}

	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatID, text, kb, false); err != nil {
		log.Printf("sendMainMenuMessage error: %v", err)
	}
}

// Текст про настройку уведомлений – перенос из старого sendNotificationSettingMessage
func (srv *Service) sendNotificationSettingMessage(ctx context.Context, chatID string) {
	contact, _ := srv.storage.Contacts.Find(chatID)

	var text string
	var kb keyboard

	if contact == nil {
		text = "Для получения уведомлений от сервера \"АСК-Навигация\", необходимо предоставить свой номер " +
			"телефона, а также указать его в учетной записи пользователя и подтвердить его."

		kb = keyboard{
			Buttons: [][]keyboardButton{
				{
					{
						Type:    "request_contact",
						Text:    menuSendPhoneNumber,
						Payload: "request_phone",
					},
				},
				{
					{
						Type:    "message",
						Text:    menuBackToMain,
						Payload: menuBackToMain,
					},
				},
			},
		}
	} else {
		text = "Уведомления от сервера \"АСК-Навигация\" уже настроены. Если не приходят уведомления " +
			"убедитесь, что номер телефона указан в учетной записи пользователя и был подтвержден."

		kb = keyboard{
			Buttons: [][]keyboardButton{
				{
					{
						Type:    "message",
						Text:    menuExcludePhoneNumber,
						Payload: menuExcludePhoneNumber,
					},
				},
				{
					{
						Type:    "message",
						Text:    menuBackToMain,
						Payload: menuBackToMain,
					},
				},
			},
		}
	}

	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatID, text, kb, false); err != nil {
		log.Printf("sendNotificationSettingMessage error: %v", err)
	}
}

// Сохранение контакта – перенос из saveContact
func (srv *Service) saveContact(ctx context.Context, chatID string, userID int64, phone string) {
	chatIDInt, _ := strconv.ParseInt(chatID, 10, 64)

	_, err := srv.storage.Contacts.Save(&tables.Contact{
		UserID: userID,
		ChatID: chatIDInt,
		Phone:  phone,
	})

	var text string
	if err != nil {
		log.Printf("Failed to save contact: %v", err)
		text = "Не удалось сохранить номер телефона 😞"
	} else {
		text = "Номер телефона успешно сохранен!"
	}

	kb := keyboard{
		Buttons: [][]keyboardButton{
			{
				{
					Type:    "message",
					Text:    menuBackToMain,
					Payload: menuBackToMain,
				},
			},
		},
	}

	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatID, text, kb, false); err != nil {
		log.Printf("saveContact: send reply error: %v", err)
	}
}

// Удаление контакта – перенос из deleteContact
func (srv *Service) deleteContact(ctx context.Context, chatID string) {
	contact, err := srv.storage.Contacts.Find(chatID)

	var text string
	if err != nil || contact == nil {
		if err != nil {
			log.Printf("deleteContact: find error: %v", err)
		}
		text = "Не удалось удалить номер телефона 😞"
	} else {
		_, err = srv.storage.Contacts.Delete(contact.UserID)
		if err != nil {
			log.Printf("deleteContact: delete error: %v", err)
			text = "Не удалось удалить номер телефона 😞"
		} else {
			text = "Номер телефона успешно удален!"
		}
	}

	kb := keyboard{
		Buttons: [][]keyboardButton{
			{
				{
					Type:    "message",
					Text:    menuBackToMain,
					Payload: menuBackToMain,
				},
			},
		},
	}

	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatID, text, kb, false); err != nil {
		log.Printf("deleteContact: send reply error: %v", err)
	}
}
