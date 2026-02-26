// internal/bot/service.go

package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"max-bot-service/internal/storage/tables"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	menuBackToMain          = "Вернуться к главному меню 🗄"
	menuNotificationSetting = "Настроить уведомления ✉"
	menuSendPhoneNumber     = "Отправить свой номер телефона ☎️"
	menuExcludePhoneNumber  = "Исключить свой номер телефона ❌"
)

type Service struct {
	storage       *storage.Service
	BotModel      *Model
	Bot           BotClient
	webhookSecret string
	cfg           *config.Config
}

func NewService(storage *storage.Service, cfg *config.Config) *Service {
	return &Service{
		storage: storage,
		cfg:     cfg,
	}
}

func (srv *Service) Start(ctx context.Context) {
	srv.BotModel = NewModel(srv.storage.Contacts, srv.cfg)
	srv.Bot = srv.BotModel

	if err := srv.BotModel.FillInfo(ctx); err != nil {
		log.Printf("failed to load bot info: %v", err)
	} else {
		log.Printf("Bot info: ID=%d, Nick=%s", srv.BotModel.ID, srv.BotModel.Name)
	}

	if err := srv.RefreshGroupChats(ctx); err != nil {
		log.Printf("RefreshGroupChats error: %v", err)
	}

	// Периодический рефреш каждые 10 минут и синхронизирует список групп с MAX.
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("group chats refresh loop stopped: ctx done")
				return
			case <-ticker.C:
				if err := srv.RefreshGroupChats(ctx); err != nil {
					log.Printf("RefreshGroupChats error on timer: %v", err)
				}
			}
		}
	}()
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
		case "bot_started":
			srv.handleBotStarted(r.Context(), body)
		default:
		}

		w.WriteHeader(http.StatusOK)
	}
}

type botStartedPayload struct {
	User struct {
		UserID int64  `json:"user_id"`
		Name   string `json:"name"`
	} `json:"user"`
}

func (srv *Service) handleBotStarted(ctx context.Context, raw []byte) {
	var p botStartedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		log.Printf("bot_started: parse error: %v", err)
		return
	}

	chatKey := strconv.FormatInt(p.User.UserID, 10)
	log.Printf("bot_started: user_id=%d", p.User.UserID)

	srv.sendMainMenuMessage(ctx, chatKey)
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

	// 1) если это групповой чат — убеждаемся, что он есть в group_chats
	if chatType == "chat" {

		// сначала проверим в БД по chatID
		gcByID, err := srv.storage.GroupChats.FindByChatID(chatID)
		if err != nil {
			log.Printf("group_chats FindByChatID error chatID=%d: %v", chatID, err)
		}

		if gcByID == nil {
			// нет записи — подтянем info из MAX
			info, err := srv.getChatInfo(ctx, chatID)
			if err != nil {
				log.Printf("getChatInfo error chatID=%d: %v", chatID, err)
			} else if info.Type == "chat" {
				title := info.Title
				if title == "" {
					title = strconv.FormatInt(info.ChatID, 10)
				}

				if _, err := srv.storage.GroupChats.Save(&tables.GroupChat{
					ChatID: info.ChatID,
					Title:  title,
				}); err != nil {
					log.Printf("group_chats save error chatID=%d title=%q: %v", info.ChatID, title, err)
				} else {
					log.Printf("group_chats saved from message chatID=%d title=%q", info.ChatID, title)
				}
			}
		}

		// дальше можно обработать групповой message_created, если нужно
		return
	}

	// 2) Для дальнейшей логики будем считать chatKey = userID (для личных диалогов)
	chatKey := strconv.FormatInt(userID, 10)
	if chatType != "dialog" {
		return
	}

	// Обработка текстовых команд / меню
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

// структура ответа GET /chats
type chatsResponse struct {
	Chats  []chatShort `json:"chats"`
	Marker *int64      `json:"marker"`
}

type chatShort struct {
	ChatID int64  `json:"chat_id"`
	Type   string `json:"type"`  // "chat" для групп
	Title  string `json:"title"` // Nullable в API, но мы читаем как string
}

// забираем все чаты постранично и сохраняем только type=="chat"
func (srv *Service) refreshGroupChats(ctx context.Context) error {
	log.Printf("refreshGroupChats: start")
	client := http.Client{}

	var marker *int64

	for {
		url := fmt.Sprintf("%s/chats", srv.cfg.ApiBaseURL)
		if marker != nil {
			url = fmt.Sprintf("%s/chats?marker=%d", srv.cfg.ApiBaseURL, *marker)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create chats request: %w", err)
		}

		req.Header.Set("Authorization", srv.cfg.BotToken)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("do chats request: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("chats api error: status %d, body=%s", resp.StatusCode, string(body))
		}

		var cr chatsResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return fmt.Errorf("decode chats response: %w", err)
		}

		for _, ch := range cr.Chats {
			if ch.Type != "chat" {
				continue
			}
			title := ch.Title
			if title == "" {
				// fallback: если почему-то title пустой, используем ChatId как строку
				title = strconv.FormatInt(ch.ChatID, 10)
			}

			if _, err := srv.storage.GroupChats.Save(&tables.GroupChat{
				ChatID: ch.ChatID,
				Title:  title,
			}); err != nil {
				log.Printf("group_chats save error chatID=%d title=%q: %v", ch.ChatID, title, err)
			} else {
				log.Printf("group_chats saved chatID=%d title=%q", ch.ChatID, title)
			}
		}

		// если маркера нет — это последняя страница
		if cr.Marker == nil {
			break
		}
		marker = cr.Marker
	}

	log.Printf("refreshGroupChats: done")
	return nil
}

// публичный метод для хендлера
// RefreshGroupChats обновляет кеш групповых чатов бота.
// Если cleanMissing == true (по умолчанию), то после загрузки удаляет из БД те chatID,
// которых больше нет в ответе GET /chats.
func (srv *Service) RefreshGroupChats(ctx context.Context, cleanMissing ...bool) error {
	doClean := true
	if len(cleanMissing) > 0 {
		doClean = cleanMissing[0]
	}

	log.Printf("RefreshGroupChats: start (cleanMissing=%v)", doClean)

	client := http.Client{}
	seen := make(map[int64]struct{}) // chatID, которые вернул MAX сейчас

	var marker *int64

	for {
		url := fmt.Sprintf("%s/chats", srv.cfg.ApiBaseURL)
		if marker != nil {
			url = fmt.Sprintf("%s/chats?marker=%d", srv.cfg.ApiBaseURL, *marker)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create chats request: %w", err)
		}

		req.Header.Set("Authorization", srv.cfg.BotToken)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("do chats request: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("chats api error: status %d, body=%s", resp.StatusCode, string(body))
		}

		var cr chatsResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return fmt.Errorf("decode chats response: %w", err)
		}

		for _, ch := range cr.Chats {
			if ch.Type != "chat" {
				continue
			}

			title := ch.Title
			if title == "" {
				title = strconv.FormatInt(ch.ChatID, 10)
			}

			if _, err := srv.storage.GroupChats.Save(&tables.GroupChat{
				ChatID: ch.ChatID,
				Title:  title,
			}); err != nil {
				log.Printf("group_chats save error chatID=%d title=%q: %v", ch.ChatID, title, err)
			} else {
				log.Printf("group_chats saved chatID=%d title=%q", ch.ChatID, title)
			}

			seen[ch.ChatID] = struct{}{}
		}

		if cr.Marker == nil {
			break
		}
		marker = cr.Marker
	}

	// Чистка: удаляем записи, которых нет в seen
	if doClean {
		if err := srv.cleanupMissingGroupChats(seen); err != nil {
			return fmt.Errorf("cleanupMissingGroupChats: %w", err)
		}
	}

	log.Printf("RefreshGroupChats: done")
	return nil
}

// cleanupMissingGroupChats удаляет из group_chats те chatID, которых нет в seen.
func (srv *Service) cleanupMissingGroupChats(seen map[int64]struct{}) error {
	// Если seen пустой — лучше не удалять всё, а просто пропустить (может быть временная ошибка API).
	if len(seen) == 0 {
		log.Printf("cleanupMissingGroupChats: seen is empty, skip cleanup")
		return nil
	}

	// Собираем chatID, которые есть сейчас в БД
	rows, err := srv.storage.GroupChats.All()
	if err != nil {
		return fmt.Errorf("load all group_chats: %w", err)
	}

	toDelete := make([]int64, 0)
	for _, gc := range rows {
		if _, ok := seen[gc.ChatID]; !ok {
			toDelete = append(toDelete, gc.ChatID)
		}
	}

	if len(toDelete) == 0 {
		log.Printf("cleanupMissingGroupChats: nothing to delete")
		return nil
	}

	// Удаляем по одному (спокойный вариант; объём маленький)
	for _, id := range toDelete {
		if err := srv.storage.GroupChats.DeleteByChatID(id); err != nil {
			log.Printf("cleanupMissingGroupChats: delete chatID=%d error: %v", id, err)
		} else {
			log.Printf("cleanupMissingGroupChats: deleted chatID=%d", id)
		}
	}

	return nil
}

// структура ответа GET /chats/{chatId}
type chatInfo struct {
	ChatID int64  `json:"chat_id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
}

// getChatInfo загружает информацию о чате по chatId
func (srv *Service) getChatInfo(ctx context.Context, chatID int64) (*chatInfo, error) {
	client := http.Client{}

	url := fmt.Sprintf("%s/chats/%d", srv.cfg.ApiBaseURL, chatID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create chat info request: %w", err)
	}

	req.Header.Set("Authorization", srv.cfg.BotToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do chat info request: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("chat info api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var info chatInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode chat info response: %w", err)
	}

	return &info, nil
}
