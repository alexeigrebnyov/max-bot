// internal/bot/model.go

package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"max-bot-service/internal/config"
	"max-bot-service/internal/storage/tables"
)

const apiHTTPTimeout = 60 * time.Second

type Model struct {
	httpClient *http.Client
	apiBase    string // https://platform-api.max.ru
	token      string // BOT_TOKEN
	contacts   *tables.Contacts
	ID         int64  // можно заполнить из /me
	Name       string // ник бота
}

type keyboardButton struct {
	Type    string `json:"type"` // "message" или "request_contact"
	Text    string `json:"text"`
	Payload string `json:"payload,omitempty"` // текст, который подставится / уйдёт
}

type keyboard struct {
	Buttons [][]keyboardButton `json:"buttons"`
}

// структура ответа /me (упрощённо)
type botInfoResponse struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type messagePeer struct {
	UserID int64  `json:"user_id,omitempty"`
	ChatID int64  `json:"chat_id,omitempty"`
	Type   string `json:"type,omitempty"` // "dialog"
}

type messageContent struct {
	Text string `json:"text"`
}

type sendMessageRequest struct {
	Peer    messagePeer    `json:"peer"`
	Content messageContent `json:"content"`
	// Keyboard добавим во второй структуре
}

type sendMessageWithKeyboardRequest struct {
	Peer     messagePeer    `json:"peer"`
	Content  messageContent `json:"content"`
	Keyboard keyboard       `json:"keyboard"`
}

type BotClient interface {
	SendMessage(ctx context.Context, chat string, thread int, text string, private bool) error
	SendMessageWithKeyboard(ctx context.Context, chat string, text string, kb keyboard, private bool) error
	SendToChatByID(ctx context.Context, chatID int64, text string) error
	SendToChatByIDWithKeyboard(ctx context.Context, chatID int64, text string, kb keyboard) error
}

func NewModel(contacts *tables.Contacts, cfg *config.Config) *Model {
	return &Model{
		httpClient: &http.Client{Timeout: apiHTTPTimeout},
		apiBase:    cfg.ApiBaseURL,
		token:      cfg.BotToken,
		contacts:   contacts,
	}
}

// SetHTTPClient подменяет HTTP-клиент (для тестов).
func (m *Model) SetHTTPClient(c *http.Client) {
	m.httpClient = c
}

// структура под реальный формат /messages
type sendMessageWithKbBody struct {
	Text        string        `json:"text"`
	Attachments []interface{} `json:"attachments,omitempty"`
}

type inlineKeyboardAttachment struct {
	Type    string   `json:"type"`    // "inline_keyboard"
	Payload keyboard `json:"payload"` // твоя структура keyboard
}

func (m *Model) SendMessage(ctx context.Context, chat string, thread int, text string, private bool) error {
	if private {
		contact, err := m.contacts.Find(chat)
		if err != nil {
			return err
		}
		if contact == nil {
			log.Printf("SendMessage: no contact found for key=%s", chat)
			return fmt.Errorf("contact not found for phone %q", chat)
		}
		originalKey := chat
		chat = strconv.FormatInt(contact.UserID, 10)
		log.Printf(
			"SendMessage: found contact key=%s -> userID=%d chatID=%d",
			originalKey, contact.UserID, contact.ChatID,
		)
	}

	chatID, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat id %q: %w", chat, err)
	}

	body := struct {
		Text string `json:"text"`
	}{
		Text: text,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendMessage body: %w", err)
	}

	log.Printf("sendMessage request: user_id=%d body=%s", chatID, string(data))

	// user_id в query, а не peer в body
	url := fmt.Sprintf("%s/messages?user_id=%d", m.apiBase, chatID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", m.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("max api error: status %d, body=%s", resp.StatusCode, string(b))
	}

	return nil
}

// SendMessageWithKeyboard - отправка сообщения с клавиатурой
func (m *Model) SendMessageWithKeyboard(ctx context.Context, chat string, text string, kb keyboard, private bool) error {
	if private {
		contact, err := m.contacts.Find(chat)
		if err != nil {
			return err
		}
		if contact == nil {
			log.Printf("SendMessageWithKeyboard: no contact found for key=%s", chat)
			return fmt.Errorf("contact not found for phone %q", chat)
		}
		chat = strconv.FormatInt(contact.UserID, 10)
	}

	chatID, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat id %q: %w", chat, err)
	}

	body := sendMessageWithKbBody{
		Text: text,
		Attachments: []interface{}{
			inlineKeyboardAttachment{
				Type:    "inline_keyboard",
				Payload: kb,
			},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendMessageWithKeyboard body: %w", err)
	}

	log.Printf("sendMessageWithKeyboard request: user_id=%d body=%s", chatID, string(data))

	url := fmt.Sprintf("%s/messages?user_id=%d", m.apiBase, chatID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", m.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	log.Printf("sendMessageWithKeyboard response: status=%d body=%s", resp.StatusCode, string(b))

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("max api error: status %d, body=%s", resp.StatusCode, string(b))
	}

	return nil
}

func (m *Model) SendToChatByIDWithKeyboard(ctx context.Context, chatID int64, text string, kb keyboard) error {
	body := sendMessageWithKbBody{
		Text: text,
		Attachments: []interface{}{
			inlineKeyboardAttachment{
				Type:    "inline_keyboard",
				Payload: kb,
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendToChatWithKeyboard body: %w", err)
	}
	log.Printf("sendToChatWithKeyboard request: chat_id=%d body=%s", chatID, string(data))
	url := fmt.Sprintf("%s/messages?chat_id=%d", m.apiBase, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", m.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("max api error: status %d, body=%s", resp.StatusCode, string(b))
	}
	return nil
}

func (m *Model) FillInfo(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.apiBase+"/me", nil)
	if err != nil {
		return fmt.Errorf("create /me request: %w", err)
	}

	req.Header.Set("Authorization", m.token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do /me request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("/me error: status %d", resp.StatusCode)
	}

	/*var info botInfoResponse
	 if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode /me response: %w", err)
	}*/
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read /me response: %w", err)
	}

	log.Printf("/me raw response: %s", string(body))

	var info botInfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("decode /me response: %w", err)
	}

	m.ID = info.UserID
	m.Name = info.Name

	return nil
}

func (m *Model) SendToChatByID(ctx context.Context, chatID int64, text string) error {
	body := struct {
		Text string `json:"text"`
	}{
		Text: text,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendToChat body: %w", err)
	}

	log.Printf("sendToChat request: chat_id=%d body=%s", chatID, string(data))

	url := fmt.Sprintf("%s/messages?chat_id=%d", m.apiBase, chatID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", m.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("max api error: status %d, body=%s", resp.StatusCode, string(b))
	}

	return nil
}
