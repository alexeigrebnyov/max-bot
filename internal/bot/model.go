// internal/bot/model.go

package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"max-bot-service/internal/config"
	"max-bot-service/internal/storage/tables"
)

type Model struct {
	httpClient *http.Client
	apiBase    string // https://platform-api.max.ru
	token      string // BOT_TOKEN
	contacts   *tables.Contacts
	ID         int64  // можно заполнить из /me
	Name       string // ник бота
}

// Вспомогательная структура для отправки сообщения в MAX
type sendMessageRequest struct {
	ChatID string `json:"chatId"`
	Text   string `json:"text"`
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
	ID   int64  `json:"id"`
	Nick string `json:"nick"`
}

func NewModel(contacts *tables.Contacts, cfg *config.Config) *Model {
	return &Model{
		httpClient: &http.Client{},
		apiBase:    cfg.ApiBaseURL,
		token:      cfg.BotToken,
		contacts:   contacts,
	}
}

func (m *Model) SendMessage(ctx context.Context, chat string, thread int, text string, private bool) error {
	if private {
		contact, err := m.contacts.Find(chat)
		if err != nil {
			return err
		}
		if contact != nil {
			chat = strconv.FormatInt(contact.UserID, 10)
		}
	}

	body := sendMessageRequest{
		ChatID: chat,
		Text:   text,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendMessage body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.apiBase+"/messages", bytes.NewReader(data))
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
		return fmt.Errorf("max api error: status %d", resp.StatusCode)
	}

	return nil
}

// Новый метод: отправка сообщения с клавиатурой
func (m *Model) SendMessageWithKeyboard(ctx context.Context, chat string, text string, kb keyboard, private bool) error {
	if private {
		contact, err := m.contacts.Find(chat)
		if err != nil {
			return err
		}
		if contact != nil {
			chat = strconv.FormatInt(contact.UserID, 10)
		}
	}

	body := struct {
		ChatID   string   `json:"chatId"`
		Text     string   `json:"text"`
		Keyboard keyboard `json:"keyboard"`
	}{
		ChatID:   chat,
		Text:     text,
		Keyboard: kb,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendMessageWithKeyboard body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.apiBase+"/messages", bytes.NewReader(data))
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
		return fmt.Errorf("max api error: status %d", resp.StatusCode)
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

	var info botInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode /me response: %w", err)
	}

	m.ID = info.ID
	m.Name = info.Nick

	return nil
}
