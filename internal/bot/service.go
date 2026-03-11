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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	menuBackToMain          = "В главное меню ⬅️"
	menuNotificationSetting = "Настроить уведомления 🔔"
	menuSendPhoneNumber     = "Отпр. свой # телефона 📞"
	menuExcludePhoneNumber  = "Искл. свой # телефона 📵"
	menuShowChatID          = "ID этого чата 🆔"
	menuListChats           = "Список групп 💬"
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

// UseLongPolling возвращает true, если обновления получаем через GET /updates (WEBHOOK_URL пустой).
func (srv *Service) UseLongPolling() bool {
	return srv.cfg.UseLongPolling
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

	if srv.cfg.UseLongPolling {
		if err := srv.unsubscribeWebhook(ctx); err != nil {
			log.Printf("unsubscribeWebhook (non-fatal): %v", err)
		}
		log.Printf("updates: using Long Polling (GET /updates)")
		go srv.runPollLoop(ctx)
	}

	// Периодический рефреш синхронизирует список групп с MAX.
	go func() {
		// Делать чаще, чтобы приветствие приходило быстрее и независимо от bot_added (который приходит без chat_id).
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("group chats refresh loop stopped: ctx done")
				return
			case <-ticker.C:
				if err := srv.RefreshGroupChatsAndGreet(ctx); err != nil {
					log.Printf("RefreshGroupChatsAndGreet error on timer: %v", err)
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
		log.Printf("webhook: POST received")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()

		body, _ := io.ReadAll(r.Body)
		log.Printf("webhook: body_len=%d raw=%s", len(body), string(body))

		var upd webhookUpdate
		if err := json.Unmarshal(body, &upd); err != nil {
			log.Printf("webhook: decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("webhook: update_type=%q message_len=%d callback_len=%d", upd.UpdateType, len(upd.Message), len(upd.Callback))
		if upd.UpdateType == "" {
			var m map[string]interface{}
			if json.Unmarshal(body, &m) == nil {
				var keys []string
				for k := range m {
					keys = append(keys, k)
				}
				log.Printf("webhook: update_type пустой; ключи в body: %v", keys)
			}
		}

		switch upd.UpdateType {
		case "message_created":
			srv.handleMessageCreated(r.Context(), upd.Message)
		case "message_callback":
			raw := upd.Callback
			if len(raw) == 0 {
				raw = upd.Message
			}
			srv.handleMessageCallback(r.Context(), raw)
		case "bot_started":
			// У bot_started payload на верхнем уровне update (recipient, user), а не в message.
			srv.handleBotStarted(r.Context(), body)
		case "bot_added":
			srv.handleBotAdded(r.Context(), body)
		default:
			log.Printf("webhook: unhandled update_type=%s", upd.UpdateType)
		}

		w.WriteHeader(http.StatusOK)
	}
}

type botStartedPayload struct {
	Recipient struct {
		ChatID   int64  `json:"chat_id"`
		ChatType string `json:"chat_type"`
		UserID   int64  `json:"user_id"`
	} `json:"recipient"`

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

	// Если бота добавили в групповой чат — сохраняем чат в БД и показываем меню с кнопкой (если бот администратор).
	if p.Recipient.ChatType == "chat" && p.Recipient.ChatID != 0 {
		chatID := p.Recipient.ChatID
		log.Printf("bot_started: group chat_id=%d", chatID)

		gcByID, err := srv.storage.GroupChats.FindByChatID(chatID)
		if err != nil {
			log.Printf("bot_started: group_chats FindByChatID error chatID=%d: %v", chatID, err)
		}
		if gcByID == nil {
			info, err := srv.getChatInfo(ctx, chatID)
			if err != nil {
				log.Printf("bot_started: getChatInfo error chatID=%d: %v", chatID, err)
			} else if info.Type == "chat" {
				title := info.Title
				if title == "" {
					title = strconv.FormatInt(info.ChatID, 10)
				}
				if _, err := srv.storage.GroupChats.Save(&tables.GroupChat{
					ChatID: info.ChatID,
					Title:  title,
				}); err != nil {
					log.Printf("bot_started: group_chats save error chatID=%d title=%q: %v", info.ChatID, title, err)
				}
			}
		}

		srv.sendBotAddedGreeting(ctx, chatID)
		return
	}

	// Личный диалог: показываем меню. Источник идентификатора чата может быть разным в зависимости от payload MAX.
	var chatKey string
	if p.User.UserID != 0 {
		chatKey = strconv.FormatInt(p.User.UserID, 10)
		log.Printf("bot_started: user_id=%d (from user)", p.User.UserID)
		srv.sendMainMenuMessage(ctx, chatKey)
		return
	}
	if p.Recipient.ChatType == "dialog" && p.Recipient.ChatID != 0 {
		log.Printf("bot_started: chat_id=%d (from recipient, dialog)", p.Recipient.ChatID)
		srv.sendMainMenuMessageByChatID(ctx, p.Recipient.ChatID)
		return
	}
	if p.Recipient.UserID != 0 {
		chatKey = strconv.FormatInt(p.Recipient.UserID, 10)
		log.Printf("bot_started: user_id=%d (from recipient)", p.Recipient.UserID)
		srv.sendMainMenuMessage(ctx, chatKey)
		return
	}
	log.Printf("bot_started: no user_id or chat_id in payload (user=%+v recipient=%+v)", p.User, p.Recipient)
}

func (srv *Service) handleBotAdded(ctx context.Context, raw []byte) {
	// MAX присылает bot_added без chat_id (message=null), поэтому используем событие как триггер
	// синхронизации списка чатов и приветствия новых чатов.
	if len(raw) == 0 {
		log.Printf("bot_added: message is empty (expected), syncing group chats")
	} else {
		log.Printf("bot_added: message len=%d, syncing group chats", len(raw))
	}
	if err := srv.RefreshGroupChatsAndGreet(ctx); err != nil {
		log.Printf("bot_added: RefreshGroupChatsAndGreet error: %v", err)
	}
}

func (srv *Service) sendBotAddedGreeting(ctx context.Context, chatID int64) {
	if !srv.isBotAdminInChat(ctx, chatID) {
		log.Printf("bot_added: chat_id=%d — бот не администратор, меню в группе не показываем", chatID)
		return
	}
	srv.sendGroupMenu(ctx, chatID)
}

// sendGroupMenu отправляет в групповой чат приветствие и меню с одной кнопкой «Покажи ID чата».
// Вызывать только когда бот уже проверен как администратор (isBotAdminInChat).
func (srv *Service) sendGroupMenu(ctx context.Context, chatID int64) {
	text := fmt.Sprintf("Добро пожаловать! ID этого чата: %d.", chatID)
	if err := srv.Bot.SendToChatByID(ctx, chatID, text); err != nil {
		if srv.isChatDeniedError(err) {
			srv.storage.GroupChats.DeleteByChatID(chatID)
			log.Printf("sendGroupMenu: chat_id=%d denied/closed, removed from DB", chatID)
		} else {
			log.Printf("sendGroupMenu: greeting chat_id=%d error: %v", chatID, err)
		}
		return
	}
	const payloadPrefix = "chatid:"
	btnPayload := payloadPrefix + strconv.FormatInt(chatID, 10)
	btnText := menuShowChatID + " " + strconv.FormatInt(chatID, 10)
	groupMenuKb := keyboard{
		Buttons: [][]keyboardButton{
			{{Type: "message", Text: btnText, Payload: btnPayload}},
		},
	}
	if err := srv.Bot.SendToChatByIDWithKeyboard(ctx, chatID, "ID чата", groupMenuKb); err != nil {
		if srv.isChatDeniedError(err) {
			srv.storage.GroupChats.DeleteByChatID(chatID)
			log.Printf("sendGroupMenu: chat_id=%d denied/closed (menu), removed from DB", chatID)
		} else {
			log.Printf("sendGroupMenu: menu chat_id=%d error: %v", chatID, err)
		}
		return
	}
	if err := srv.storage.GroupChats.SetMenuSent(chatID); err != nil {
		log.Printf("sendGroupMenu: SetMenuSent chat_id=%d error: %v", chatID, err)
	}
}

func (srv *Service) isChatDeniedError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "403") && strings.Contains(s, "chat.denied")
}

// isGroupStartTrigger возвращает true, если текст похож на запрос меню: "start", "/start" или "@bot start".
func isGroupStartTrigger(trimmed string) bool {
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return false
	}
	if trimmed == "start" || trimmed == "/start" {
		return true
	}
	for _, w := range strings.Fields(trimmed) {
		w = strings.TrimSpace(w)
		if w == "start" || w == "/start" {
			return true
		}
	}
	return false
}

// sendChatIDToGroup отправляет в групповой чат два сообщения: подпись и ID чата.
func (srv *Service) sendChatIDToGroup(ctx context.Context, chatID int64) {
	msg1, msg2 := "ID этого чата:", strconv.FormatInt(chatID, 10)
	log.Printf("sendChatIDToGroup: chat_id=%d -> sending msg1=%q msg2=%q", chatID, msg1, msg2)
	if err := srv.Bot.SendToChatByID(ctx, chatID, msg1); err != nil {
		log.Printf("sendChatIDToGroup: caption chat_id=%d error: %v", chatID, err)
		return
	}
	if err := srv.Bot.SendToChatByID(ctx, chatID, msg2); err != nil {
		log.Printf("sendChatIDToGroup: id chat_id=%d error: %v", chatID, err)
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
		Payload     string `json:"payload"` // при нажатии кнопки типа "message" MAX может присылать payload сюда
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

// webhookUpdate — структура Update из MAX (Webhook POST и GET /updates).
// Для message_callback данные приходят в поле callback, а не message (см. dev.max.ru/docs-api/methods/POST/answers).
type webhookUpdate struct {
	Timestamp  int64           `json:"timestamp"`
	Message    json.RawMessage `json:"message"`
	Callback   json.RawMessage `json:"callback"` // для update_type=message_callback
	UserLocale string          `json:"user_locale"`
	UpdateType string          `json:"update_type"`
}

// updatesResponse — ответ GET /updates (Long Polling). Updates как RawMessage, чтобы для bot_started
// передавать полный объект (recipient, user на верхнем уровне, не в message).
type updatesResponse struct {
	Updates []json.RawMessage `json:"updates"`
	Marker  *int64            `json:"marker"`
}

const pollTimeoutSec = 30
const pollLimit = 100

func (srv *Service) fetchUpdates(ctx context.Context, marker *int64) ([]json.RawMessage, *int64, error) {
	// types=message_created,message_callback,... — иначе нажатия кнопок (message_callback) не приходят (см. GET /updates в документации MAX).
	url := fmt.Sprintf("%s/updates?timeout=%d&limit=%d&types=message_created,message_callback,bot_started,bot_added", srv.cfg.ApiBaseURL, pollTimeoutSec, pollLimit)
	if marker != nil {
		url = fmt.Sprintf("%s/updates?timeout=%d&limit=%d&types=message_created,message_callback,bot_started,bot_added&marker=%d", srv.cfg.ApiBaseURL, pollTimeoutSec, pollLimit, *marker)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create updates request: %w", err)
	}
	req.Header.Set("Authorization", srv.cfg.BotToken)

	client := &http.Client{Timeout: 65 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("do updates request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, nil, fmt.Errorf("updates api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var ur updatesResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return nil, nil, fmt.Errorf("decode updates response: %w", err)
	}
	return ur.Updates, ur.Marker, nil
}

func (srv *Service) runPollLoop(ctx context.Context) {
	var marker *int64
	for {
		select {
		case <-ctx.Done():
			log.Printf("long polling loop stopped: ctx done")
			return
		default:
		}

		updates, nextMarker, err := srv.fetchUpdates(ctx, marker)
		if err != nil {
			log.Printf("fetchUpdates error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		if nextMarker != nil {
			marker = nextMarker
		}

		if len(updates) > 0 {
			log.Printf("long poll: received %d update(s)", len(updates))
		}

		for _, rawUpd := range updates {
			var upd webhookUpdate
			if err := json.Unmarshal(rawUpd, &upd); err != nil {
				log.Printf("long poll: decode update error: %v", err)
				continue
			}
			preview := upd.Message
			if len(upd.Callback) > 0 {
				preview = upd.Callback
			}
			msgPreview := string(preview)
			if len(msgPreview) > 500 {
				msgPreview = msgPreview[:500] + "..."
			}
			log.Printf("long poll: update_type=%s message_len=%d callback_len=%d preview=%s", upd.UpdateType, len(upd.Message), len(upd.Callback), msgPreview)
			switch upd.UpdateType {
			case "message_created":
				srv.handleMessageCreated(ctx, upd.Message)
			case "message_callback":
				raw := upd.Callback
				if len(raw) == 0 {
					raw = upd.Message
				}
				srv.handleMessageCallback(ctx, raw)
			case "bot_started":
				// У bot_started payload на верхнем уровне (recipient, user), не в message.
				srv.handleBotStarted(ctx, rawUpd)
			case "bot_added":
				srv.handleBotAdded(ctx, rawUpd)
			default:
				log.Printf("long poll: unhandled update_type=%s", upd.UpdateType)
			}
		}
	}
}

// subscriptionRecord — элемент ответа GET /subscriptions (MAX API).
type subscriptionRecord struct {
	URL string `json:"url"`
}

type subscriptionsListResponse struct {
	Subscriptions []subscriptionRecord `json:"subscriptions"`
}

// unsubscribeWebhook снимает все подписки на Webhook через API, чтобы Long Polling получал события.
func (srv *Service) unsubscribeWebhook(ctx context.Context) error {
	client := &http.Client{Timeout: 25 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.cfg.ApiBaseURL+"/subscriptions", nil)
	if err != nil {
		return fmt.Errorf("create GET subscriptions request: %w", err)
	}
	req.Header.Set("Authorization", srv.cfg.BotToken)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET subscriptions: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("GET subscriptions: status %d, body=%s", resp.StatusCode, string(body))
	}

	var list subscriptionsListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("decode GET subscriptions response: %w", err)
	}

	if len(list.Subscriptions) == 0 {
		log.Printf("unsubscribeWebhook: no active subscriptions")
		return nil
	}

	for _, sub := range list.Subscriptions {
		if sub.URL == "" {
			continue
		}
		delURL := srv.cfg.ApiBaseURL + "/subscriptions?url=" + url.QueryEscape(sub.URL)
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if err != nil {
			log.Printf("unsubscribeWebhook: create DELETE request for %q: %v", sub.URL, err)
			continue
		}
		delReq.Header.Set("Authorization", srv.cfg.BotToken)

		delResp, err := client.Do(delReq)
		if err != nil {
			log.Printf("unsubscribeWebhook: DELETE %q: %v", sub.URL, err)
			continue
		}
		delBody, _ := io.ReadAll(delResp.Body)
		delResp.Body.Close()

		if delResp.StatusCode >= http.StatusBadRequest {
			log.Printf("unsubscribeWebhook: DELETE %q status %d body=%s", sub.URL, delResp.StatusCode, string(delBody))
			continue
		}
		log.Printf("unsubscribeWebhook: removed subscription url=%s", sub.URL)
	}

	return nil
}

// messageCallbackPayload — событие нажатия inline-кнопки (message_callback).
// Вариант 1: когда callback приходит как message (body.payload, recipient).
type messageCallbackPayload struct {
	Recipient struct {
		ChatID   int64  `json:"chat_id"`
		ChatType string `json:"chat_type"`
		UserID   int64  `json:"user_id"`
	} `json:"recipient"`
	ChatID int64 `json:"chat_id"`
	Body   struct {
		Payload string `json:"payload"`
		Text    string `json:"text"`
		Mid     string `json:"mid"`
	} `json:"body"`
}

// callbackPayload — формат объекта callback в Update (см. dev.max.ru: updates[i].callback.callback_id).
// Данные приходят в поле callback, не message.
type callbackPayload struct {
	CallbackID string `json:"callback_id"`
	Payload    string `json:"payload"`
	Text       string `json:"text"`
	Recipient  struct {
		ChatID   int64  `json:"chat_id"`
		ChatType string `json:"chat_type"`
		UserID   int64  `json:"user_id"`
	} `json:"recipient"`
	ChatID int64 `json:"chat_id"`
}

func (srv *Service) handleMessageCallback(ctx context.Context, raw []byte) {
	log.Printf("message_callback: entered, len(raw)=%d", len(raw))
	if len(raw) == 0 {
		log.Printf("message_callback: empty callback/message (проверьте: 1) подписка webhook должна включать update_types message_callback; 2) для message_callback MAX присылает данные в поле callback, не message)")
		return
	}
	log.Printf("message_callback RAW: %s", string(raw))

	var payloadStr string
	var chatID int64

	// Пробуем формат callback (поле callback в Update по документации MAX).
	var cb callbackPayload
	if err := json.Unmarshal(raw, &cb); err == nil && (cb.CallbackID != "" || cb.Payload != "" || cb.Recipient.ChatID != 0) {
		payloadStr = strings.TrimSpace(cb.Payload)
		if payloadStr == "" {
			payloadStr = strings.TrimSpace(cb.Text)
		}
		chatID = cb.Recipient.ChatID
		if chatID == 0 {
			chatID = cb.ChatID
		}
		log.Printf("message_callback: parsed as callback payload=%q chat_id=%d callback_id=%s", payloadStr, chatID, cb.CallbackID)
	} else {
		// Формат message (body.payload, recipient).
		var p messageCallbackPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Printf("message_callback: parse error: %v", err)
			return
		}
		payloadStr = strings.TrimSpace(p.Body.Payload)
		if payloadStr == "" {
			payloadStr = strings.TrimSpace(p.Body.Text)
		}
		chatID = p.Recipient.ChatID
		if chatID == 0 {
			chatID = p.ChatID
		}
	}

	const payloadPrefix = "chatid:"
	if strings.HasPrefix(payloadStr, payloadPrefix) {
		id, err := strconv.ParseInt(strings.TrimSpace(payloadStr[len(payloadPrefix):]), 10, 64)
		if err != nil {
			log.Printf("message_callback: invalid chatid in payload=%q: %v", payloadStr, err)
			return
		}
		chatID = id
	} else if payloadStr == menuShowChatID {
		// chatID уже из recipient
	} else if payloadStr != "" {
		log.Printf("message_callback: skip payload=%q", payloadStr)
		return
	}

	if chatID == 0 {
		log.Printf("message_callback: no chat_id")
		return
	}
	log.Printf("message_callback: chat_id=%d -> sendChatIDToGroup", chatID)
	srv.sendChatIDToGroup(ctx, chatID)
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
	if text == "" && p.Body.Payload != "" {
		text = p.Body.Payload // при нажатии кнопки MAX может присылать только payload
	}

	log.Printf("message_created: senderUserID=%d, chatId=%d, type=%s, text=%q, payload=%q",
		userID, chatID, chatType, text, p.Body.Payload)

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
					// Первый раз увидели этот групповой чат — отправим приветственное сообщение.
					srv.sendBotAddedGreeting(ctx, info.ChatID)
				}
			}
		}

		// Кнопка «Покажи ID чата» в группе: payload "chatid:{id}", или текст "Покажи ID чата" / "Покажи ID чата -123".
		trimmed := strings.TrimSpace(text)
		log.Printf("message_created GROUP: chat_id=%d text=%q", chatID, text)
		if strings.HasPrefix(trimmed, "chatid:") {
			idStr := strings.TrimSpace(trimmed[len("chatid:"):])
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				log.Printf("GROUP BUTTON: payload chatid -> sendChatIDToGroup chat_id=%d", id)
				srv.sendChatIDToGroup(ctx, id)
				return
			}
		}
		if trimmed == menuShowChatID {
			log.Printf("GROUP BUTTON: text match -> sendChatIDToGroup chat_id=%d", chatID)
			srv.sendChatIDToGroup(ctx, chatID)
			return
		}
		if strings.HasPrefix(trimmed, menuShowChatID) {
			rest := strings.TrimSpace(trimmed[len(menuShowChatID):])
			if id, err := strconv.ParseInt(rest, 10, 64); err == nil {
				log.Printf("GROUP BUTTON: text with ID -> sendChatIDToGroup chat_id=%d", id)
				srv.sendChatIDToGroup(ctx, id)
				return
			}
		}
		// Запрос меню в группе: «start», «/start» или «@bot start» — показываем меню с кнопкой, если бот администратор.
		if isGroupStartTrigger(trimmed) && srv.isBotAdminInChat(ctx, chatID) {
			log.Printf("message_created GROUP: start trigger, bot is admin -> sendGroupMenu chat_id=%d", chatID)
			srv.sendGroupMenu(ctx, chatID)
		}
		return
	}

	// 2) Для дальнейшей логики будем считать chatKey = userID (для личных диалогов)
	chatKey := strconv.FormatInt(userID, 10)
	if chatType != "dialog" {
		return
	}

	// Обработка текстовых команд / меню (кнопка «НАЧАТЬ» может прийти как /start или start)
	trimmedText := strings.TrimSpace(text)
	switch trimmedText {
	case "/start", "start", menuBackToMain:
		srv.sendMainMenuMessage(ctx, chatKey)
		return

	case menuNotificationSetting:
		srv.sendNotificationSettingMessage(ctx, chatKey)
		return

	case menuExcludePhoneNumber:
		srv.deleteContact(ctx, chatKey)
		return

	case menuShowChatID:
		srv.sendChatIDMessage(ctx, chatKey, p.Recipient.ChatID)
		return

	case menuListChats:
		srv.sendGroupChatsList(ctx, chatKey)
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
				{Type: "message", Text: menuShowChatID, Payload: menuShowChatID},
			},
			{
				{Type: "message", Text: menuListChats, Payload: menuListChats},
			},
			{
				{Type: "message", Text: menuNotificationSetting, Payload: menuNotificationSetting},
			},
			{
				{Type: "message", Text: menuExcludePhoneNumber, Payload: menuExcludePhoneNumber},
			},
			{
				{Type: "message", Text: menuBackToMain, Payload: menuBackToMain},
			},
		},
	}

	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatID, text, kb, false); err != nil {
		log.Printf("sendMainMenuMessage error: %v", err)
	}
}

// sendMainMenuMessageByChatID отправляет главное меню в чат по chat_id (для диалога при bot_started, когда передан recipient.chat_id).
func (srv *Service) sendMainMenuMessageByChatID(ctx context.Context, chatID int64) {
	text := "Добро пожаловать!"
	kb := keyboard{
		Buttons: [][]keyboardButton{
			{{Type: "message", Text: menuShowChatID, Payload: menuShowChatID}},
			{{Type: "message", Text: menuListChats, Payload: menuListChats}},
			{{Type: "message", Text: menuNotificationSetting, Payload: menuNotificationSetting}},
			{{Type: "message", Text: menuExcludePhoneNumber, Payload: menuExcludePhoneNumber}},
			{{Type: "message", Text: menuBackToMain, Payload: menuBackToMain}},
		},
	}
	if err := srv.Bot.SendToChatByIDWithKeyboard(ctx, chatID, text, kb); err != nil {
		log.Printf("sendMainMenuMessageByChatID error: %v", err)
	}
}

// sendGroupChatsList отправляет в личный диалог список групповых чатов (название и ChatID) с клавиатурой главного меню.
func (srv *Service) sendGroupChatsList(ctx context.Context, chatKey string) {
	rows, err := srv.storage.GroupChats.All()
	if err != nil {
		log.Printf("sendGroupChatsList: GroupChats.All error: %v", err)
		srv.sendMainMenuMessage(ctx, chatKey)
		return
	}
	var b strings.Builder
	b.WriteString("Список групповых чатов (название — ChatID):\n\n")
	if len(rows) == 0 {
		b.WriteString("Нет сохранённых групповых чатов. Добавьте бота в группу и нажмите «Покажи ID чата» в группе.")
	} else {
		for _, gc := range rows {
			b.WriteString("• ")
			b.WriteString(gc.Title)
			b.WriteString(" — ")
			b.WriteString(strconv.FormatInt(gc.ChatID, 10))
			b.WriteString("\n")
		}
	}
	text := b.String()
	kb := keyboard{
		Buttons: [][]keyboardButton{
			{{Type: "message", Text: menuShowChatID, Payload: menuShowChatID}},
			{{Type: "message", Text: menuListChats, Payload: menuListChats}},
			{{Type: "message", Text: menuNotificationSetting, Payload: menuNotificationSetting}},
			{{Type: "message", Text: menuExcludePhoneNumber, Payload: menuExcludePhoneNumber}},
			{{Type: "message", Text: menuBackToMain, Payload: menuBackToMain}},
		},
	}
	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatKey, text, kb, false); err != nil {
		log.Printf("sendGroupChatsList error: %v", err)
	}
}

// sendChatIDMessage отправляет в чат (личный диалог) сообщение с ID чата и клавиатурой главного меню.
// Два сообщения: подпись и отдельно число, чтобы длинный ID не обрезался интерфейсом.
func (srv *Service) sendChatIDMessage(ctx context.Context, chatKey string, chatID int64) {
	if err := srv.Bot.SendMessage(ctx, chatKey, 0, "ID этого чата:", false); err != nil {
		log.Printf("sendChatIDMessage (caption) error: %v", err)
		return
	}
	kb := keyboard{
		Buttons: [][]keyboardButton{
			{{Type: "message", Text: menuShowChatID, Payload: menuShowChatID}},
			{{Type: "message", Text: menuNotificationSetting, Payload: menuNotificationSetting}},
			{{Type: "message", Text: menuExcludePhoneNumber, Payload: menuExcludePhoneNumber}},
			{{Type: "message", Text: menuBackToMain, Payload: menuBackToMain}},
		},
	}
	idText := strconv.FormatInt(chatID, 10)
	if err := srv.Bot.SendMessageWithKeyboard(ctx, chatKey, idText, kb, false); err != nil {
		log.Printf("sendChatIDMessage (id) error: %v", err)
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
	return srv.refreshGroupChatsInternal(ctx, false, cleanMissing...)
}

// RefreshGroupChatsAndGreet обновляет кеш групп и отправляет приветствие
// только для реально новых чатов (которые впервые вставились в БД).
func (srv *Service) RefreshGroupChatsAndGreet(ctx context.Context, cleanMissing ...bool) error {
	return srv.refreshGroupChatsInternal(ctx, true, cleanMissing...)
}

func (srv *Service) refreshGroupChatsInternal(ctx context.Context, greetNew bool, cleanMissing ...bool) error {
	doClean := true
	if len(cleanMissing) > 0 {
		doClean = cleanMissing[0]
	}

	log.Printf("RefreshGroupChats: start (cleanMissing=%v, greetNew=%v)", doClean, greetNew)

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
			// GET /chats/{chatId} возвращает status: active | removed | left | closed — только active считаем актуальным.
			info, err := srv.getChatInfo(ctx, ch.ChatID)
			if err != nil {
				log.Printf("RefreshGroupChats: skip chat_id=%d (getChatInfo: %v)", ch.ChatID, err)
				continue
			}
			if info.Type != "chat" || info.Status != "active" {
				if info.Status != "" && info.Status != "active" {
					log.Printf("RefreshGroupChats: skip chat_id=%d status=%q", ch.ChatID, info.Status)
				}
				continue
			}
			title := info.Title
			if title == "" {
				title = ch.Title
			}
			if title == "" {
				title = strconv.FormatInt(ch.ChatID, 10)
			}

			inserted, err := srv.storage.GroupChats.Save(&tables.GroupChat{
				ChatID: ch.ChatID,
				Title:  title,
			})
			if err != nil {
				log.Printf("group_chats save error chatID=%d title=%q: %v", ch.ChatID, title, err)
			} else {
				log.Printf("group_chats saved chatID=%d title=%q", ch.ChatID, title)
				if greetNew && inserted {
					srv.sendBotAddedGreeting(ctx, ch.ChatID)
				} else if greetNew && !inserted {
					// Чат уже был в БД — проверяем, не стал ли бот администратором: тогда отправим меню, если ещё не отправляли.
					gc, _ := srv.storage.GroupChats.FindByChatID(ch.ChatID)
					if gc != nil && !gc.MenuSent && srv.isBotAdminInChat(ctx, ch.ChatID) {
						log.Printf("RefreshGroupChats: chat_id=%d — бот администратор, меню ещё не отправлялось, отправляем меню", ch.ChatID)
						srv.sendGroupMenu(ctx, ch.ChatID)
					}
				}
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

// структура ответа GET /chats/{chatId} (см. https://dev.max.ru/docs-api/methods/GET/chats/-chatId-)
type chatInfo struct {
	ChatID int64  `json:"chat_id"`
	Type   string `json:"type"`
	Status string `json:"status"` // active | removed | left | closed
	Title  string `json:"title"`
}

// chatMembershipMe — ответ GET /chats/{chatId}/members/me (членство бота в групповом чате).
type chatMembershipMe struct {
	IsAdmin bool `json:"is_admin"`
	IsOwner bool `json:"is_owner"`
}

// isBotAdminInChat возвращает true, если бот является администратором или владельцем группового чата.
func (srv *Service) isBotAdminInChat(ctx context.Context, chatID int64) bool {
	url := fmt.Sprintf("%s/chats/%d/members/me", srv.cfg.ApiBaseURL, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("isBotAdminInChat: create request chat_id=%d: %v", chatID, err)
		return false
	}
	req.Header.Set("Authorization", srv.cfg.BotToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("isBotAdminInChat: request chat_id=%d: %v", chatID, err)
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("isBotAdminInChat: chat_id=%d status=%d body=%s", chatID, resp.StatusCode, string(body))
		return false
	}
	var m chatMembershipMe
	if err := json.Unmarshal(body, &m); err != nil {
		log.Printf("isBotAdminInChat: decode chat_id=%d: %v", chatID, err)
		return false
	}
	return m.IsAdmin || m.IsOwner
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
