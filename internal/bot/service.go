// internal/bot/service.go

package bot

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"strconv"
	"strings"
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

		defer r.Body.Close()

		body, _ := io.ReadAll(r.Body)
		log.Printf("webhook raw: %s", string(body))

		var upd webhookUpdate
		if err := json.Unmarshal(body, &upd); err != nil {
			log.Printf("webhook: decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("webhook: update_type=%s", upd.UpdateType)

		switch upd.UpdateType {
		case "message_created":
			srv.handleMessageCreated(r.Context(), upd.Message)
		default:
		}

		w.WriteHeader(http.StatusOK)
	}
}

// MAX: пример структуры события message_created (упрощённо)
type messageCreatedPayload struct {
	Recipient struct {
		ChatID   int64  `json:"chat_id"`
		ChatType string `json:"chat_type"`
		UserID   int64  `json:"user_id"`
	} `json:"recipient"`

	Body struct {
		Mid         string `json:"mid"`
		Seq         int64  `json:"seq"`
		Text        string `json:"text"`
		Attachments []struct {
			Type    string `json:"type"`
			Payload struct {
				VCFInfo string `json:"vcf_info"`
				MaxInfo struct {
					UserID int64 `json:"user_id"`
					// остальные поля не нужны
				} `json:"max_info"`
			} `json:"payload"`
		} `json:"attachments"`
	} `json:"body"`

	Sender struct {
		UserID int64  `json:"user_id"`
		Name   string `json:"name"`
		// остальные поля можно не описывать
	} `json:"sender"`
}

type webhookUpdate struct {
	Timestamp  int64           `json:"timestamp"`
	Message    json.RawMessage `json:"message"`
	UserLocale string          `json:"user_locale"`
	UpdateType string          `json:"update_type"`
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

	// Для личного диалога используем user_id отправителя
	// userId отправителя (ты)
	userID := p.Sender.UserID // 23718629
	// chatId диалога
	chatID := p.Recipient.ChatID     // 174132016
	chatType := p.Recipient.ChatType // "dialog"
	text := p.Body.Text

	log.Printf("message_created: senderUserID=%d, chatId=%d, type=%s, text=%q",
		userID, chatID, chatType, text)

	// Для дальнейшей логики будем считать chatKey = userID (для личных диалогов)
	chatKey := strconv.FormatInt(userID, 10)

	if chatType != "dialog" {
		return
	}

	// 2) Обработка текстовых команд / меню
	switch text {
	case "/start", menuBackToMain:
		srv.sendMainMenuMessage(ctx, chatKey)
		return

	case menuNotificationSetting:
		srv.sendNotificationSettingMessage(ctx, chatKey)
		return

	case menuExcludePhoneNumber:
		srv.deleteContact(ctx, chatKey)
		return
	}

	// 3) Здесь пока нет contact в payload от MAX,
	// поэтому блок с p.Message.Contact / NewChatMembers можно временно убрать
	// или потом добавить, когда увидим реальный JSON с контактами.

	if len(p.Body.Attachments) > 0 {
		for _, att := range p.Body.Attachments {
			if att.Type == "contact" {
				phone := parsePhoneFromVCF(att.Payload.VCFInfo) // напишем функцию
				if phone == "" {
					log.Printf("contact attachment without phone")
					return
				}

				// chatKey мы уже посчитали как userID отправителя
				srv.saveContact(ctx, chatKey, att.Payload.MaxInfo.UserID, phone)
				return
			}
		}
	}
}

func parsePhoneFromVCF(vcf string) string {
	lines := strings.Split(vcf, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TEL") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
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
