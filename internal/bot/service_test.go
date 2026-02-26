package bot

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"max-bot-service/internal/storage/tables"
)

// --- мок BotClient ---

type mockBot struct {
	lastChatID string
	lastText   string
}

func (m *mockBot) SendMessageWithKeyboard(ctx context.Context, chat, text string, kb keyboard, private bool) error {
	m.lastChatID = chat
	m.lastText = text
	return nil
}

func (m *mockBot) SendMessage(ctx context.Context, chat string, thread int, text string, private bool) error {
	m.lastChatID = chat
	m.lastText = text
	return nil
}

func (m *mockBot) SendToChatByID(ctx context.Context, chatID int64, text string) error {
	m.lastChatID = strconv.FormatInt(chatID, 10)
	m.lastText = text
	return nil
}

// --- helper: in‑memory storage.Service ---

func newTestStorage(t *testing.T) *storage.Service {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	s := &storage.Service{
		Contacts: tables.NewContacts(db),
	}

	t.Cleanup(func() { _ = db.Close() })

	return s
}

// --- 1. Парсинг message_created и вызов меню ---

func TestHandleMessageCreated_StartCommand(t *testing.T) {
	stor := newTestStorage(t)
	cfg := &config.Config{}
	srv := NewService(stor, cfg)

	mb := &mockBot{}
	srv.Bot = mb // не вызываем Start, подменяем Bot вручную

	payload := messageCreatedPayload{
		Recipient: struct {
			ChatID   int64  `json:"chat_id"`
			ChatType string `json:"chat_type"`
			UserID   int64  `json:"user_id"`
		}{
			ChatID:   174132016,
			ChatType: "dialog",
			UserID:   185131477,
		},
		Sender: struct {
			UserID int64  `json:"user_id"`
			Name   string `json:"name"`
		}{
			UserID: 23718629,
			Name:   "Дмитрий",
		},
	}
	payload.Body.Text = "/start"

	raw, _ := json.Marshal(payload)

	srv.handleMessageCreated(context.Background(), raw)

	if mb.lastChatID != "23718629" {
		t.Fatalf("expected chatID=23718629, got %s", mb.lastChatID)
	}
	if mb.lastText != "Добро пожаловать!" {
		t.Fatalf("unexpected text: %q", mb.lastText)
	}
}

// --- 2. Сохранение и удаление контакта в SQLite ---

func TestSaveAndDeleteContact(t *testing.T) {
	stor := newTestStorage(t)
	cfg := &config.Config{}
	srv := NewService(stor, cfg)

	mb := &mockBot{}
	srv.Bot = mb

	ctx := context.Background()
	chatID := "23718629"

	// saveContact
	srv.saveContact(ctx, chatID, 23718629, "8(903)907-63-99")

	contact, err := srv.storage.Contacts.Find(chatID)
	if err != nil {
		t.Fatalf("Find after save error: %v", err)
	}
	if contact == nil {
		t.Fatalf("contact is nil after save")
	}
	if contact.Phone != "79039076399" {
		t.Fatalf("unexpected phone: %s", contact.Phone)
	}

	// deleteContact
	srv.deleteContact(ctx, chatID)

	contact, err = srv.storage.Contacts.Find(chatID)
	if err != nil {
		t.Fatalf("Find after delete error: %v", err)
	}
	if contact != nil {
		t.Fatalf("contact must be nil after delete, got %+v", contact)
	}
}

// --- 3. Форма HTTP‑запроса /messages в Model.SendMessageWithKeyboard ---

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendMessageWithKeyboard_RequestShape(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	contacts := tables.NewContacts(db)

	cfg := &config.Config{
		ApiBaseURL: "https://platform-api.max.ru",
		BotToken:   "TEST_TOKEN",
	}
	m := NewModel(contacts, cfg)

	var capturedReq *http.Request
	m.httpClient = &http.Client{
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			capturedReq = r
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	kb := keyboard{}
	if err := m.SendMessageWithKeyboard(context.Background(), "23718629", "test", kb, false); err != nil {
		t.Fatalf("SendMessageWithKeyboard error: %v", err)
	}

	if capturedReq == nil {
		t.Fatal("request not captured")
	}
	if capturedReq.URL.Path != "/messages" {
		t.Fatalf("expected path /messages, got %s", capturedReq.URL.Path)
	}
	if q := capturedReq.URL.Query().Get("user_id"); q != "23718629" {
		t.Fatalf("expected user_id=23718629, got %s", q)
	}

	body, _ := io.ReadAll(capturedReq.Body)
	if !bytes.Contains(body, []byte(`"text":"test"`)) {
		t.Fatalf("unexpected body: %s", string(body))
	}
}
