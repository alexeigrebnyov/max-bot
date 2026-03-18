package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"max-bot-service/internal/bot"
	"max-bot-service/internal/config"
	"max-bot-service/internal/storage"
	"max-bot-service/internal/storage/tables"

	_ "modernc.org/sqlite"
)

func testStorage(t *testing.T) *storage.Service {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &storage.Service{
		Contacts:   tables.NewContacts(db),
		GroupChats: tables.NewGroupChats(db),
	}
}

// mockTransport возвращает 200 OK для тестов отправки сообщений.
type mockTransport struct{}

func (mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

func testService(t *testing.T, cfg *config.Config) *Service {
	t.Helper()
	stor := testStorage(t)
	if cfg == nil {
		cfg = &config.Config{
			ApiBaseURL: config.DefaultApiBaseURL,
			BotToken:   "test",
			Port:       config.DefaultPort,
		}
	}
	botSrv := bot.NewService(stor, cfg)
	model := bot.NewModel(stor.Contacts, cfg)
	model.SetHTTPClient(&http.Client{Transport: &mockTransport{}})
	model.Name = "testbot"
	botSrv.BotModel = model
	botSrv.Bot = &bot.MockClient{}
	return NewService(botSrv, stor, cfg)
}

func TestSendMessageHandler_BadJSON_Returns400WithBody(t *testing.T) {
	srv := testService(t, nil)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/send-message", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "error" || body["message"] == "" {
		t.Errorf("expected status=error and non-empty message, got %+v", body)
	}
}

func TestSendMessageHandler_WrongMethod_Returns405(t *testing.T) {
	srv := testService(t, nil)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/send-message", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestProtectedEndpoints_RequireAPIKeyWhenSet(t *testing.T) {
	cfg := &config.Config{
		ApiBaseURL: config.DefaultApiBaseURL,
		BotToken:   "test",
		Port:       config.DefaultPort,
		APIKey:     "test-api-key-456",
	}
	srv := testService(t, cfg)
	handler := srv.Handler()

	t.Run("group_chats_without_key_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/group-chats", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		var body map[string]string
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if body["message"] != "missing or invalid X-API-Key" {
			t.Errorf("unexpected message: %s", body["message"])
		}
	})

	t.Run("group_chats_with_wrong_key_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/group-chats", nil)
		req.Header.Set("X-API-Key", "wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("group_chats_with_correct_key_returns_200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/group-chats", nil)
		req.Header.Set("X-API-Key", "test-api-key-456")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("refresh_group_chats_without_key_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/refresh-group-chats", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestRateLimit_Returns429WhenExceeded(t *testing.T) {
	cfg := &config.Config{
		ApiBaseURL:         config.DefaultApiBaseURL,
		BotToken:           "test",
		Port:               config.DefaultPort,
		RateLimitPerMinute: 2, // лимит 2 запроса в минуту
	}
	srv := testService(t, cfg)
	handler := srv.Handler()

	body := []byte(`{"chat":"123","text":"hi","thread":0,"private":false}`)
	doPost := func() int {
		req := httptest.NewRequest(http.MethodPost, "/send-message", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := doPost(); code != http.StatusNoContent {
		t.Errorf("first request: got %d, want 204", code)
	}
	if code := doPost(); code != http.StatusNoContent {
		t.Errorf("second request: got %d, want 204", code)
	}
	if code := doPost(); code != http.StatusTooManyRequests {
		t.Errorf("third request: got %d, want 429", code)
	}
}

func TestRateLimit_DisabledWhenZero(t *testing.T) {
	cfg := &config.Config{
		ApiBaseURL:         config.DefaultApiBaseURL,
		BotToken:           "test",
		Port:               config.DefaultPort,
		RateLimitPerMinute: 0,
	}
	srv := testService(t, cfg)
	handler := srv.Handler()
	body := []byte(`{"chat":"123","text":"hi","thread":0,"private":false}`)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/send-message", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("request %d: got %d, want 204 (rate limit disabled)", i+1, rec.Code)
		}
	}
}

func TestProtectedEndpoints_NoAPIKey_AllowsAccess(t *testing.T) {
	cfg := &config.Config{
		ApiBaseURL: config.DefaultApiBaseURL,
		BotToken:   "test",
		Port:       config.DefaultPort,
		APIKey:     "", // не задан
	}
	srv := testService(t, cfg)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/group-chats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 when API_KEY not set, got %d", rec.Code)
	}
}

// --- Интеграционные тесты: API + БД ---

func TestIntegration_SendByPhone_WithContactInDB(t *testing.T) {
	srv := testService(t, nil)
	// Предзаполняем контакт в той же БД, которую использует сервис
	_, err := srv.Storage.Contacts.Save(&tables.Contact{
		UserID: 999,
		ChatID: 888,
		Phone:  "79001112233",
	})
	if err != nil {
		t.Fatalf("pre-insert contact: %v", err)
	}

	handler := srv.Handler()
	body := []byte(`{"phone":"79001112233","text":"Интеграционный тест"}`)
	req := httptest.NewRequest(http.MethodPost, "/send-by-phone", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /send-by-phone with existing contact: got %d, want 200", rec.Code)
	}
	var res map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res["status"] != "ok" || res["phone"] != "79001112233" {
		t.Errorf("unexpected response: %+v", res)
	}
}

func TestIntegration_GroupChats_ReturnsJSONArray(t *testing.T) {
	srv := testService(t, nil)
	_, _ = srv.Storage.GroupChats.Save(&tables.GroupChat{ChatID: 1, Title: "Чат 1", MenuSent: false})
	_, _ = srv.Storage.GroupChats.Save(&tables.GroupChat{ChatID: 2, Title: "Чат 2", MenuSent: true})

	handler := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/group-chats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /group-chats: got %d", rec.Code)
	}
	var list []struct {
		ChatID   int64  `json:"ChatID"`
		Title    string `json:"Title"`
		MenuSent bool   `json:"MenuSent"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode group-chats: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 group chats, got %d", len(list))
	}
}

func TestMetrics_ReturnsJSON(t *testing.T) {
	srv := testService(t, nil)
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /metrics: got %d", rec.Code)
	}
	var m map[string]int64
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode /metrics: %v", err)
	}
	if _, ok := m["requests"]; !ok {
		t.Error("expected metrics to contain 'requests'")
	}
	if _, ok := m["errors"]; !ok {
		t.Error("expected metrics to contain 'errors'")
	}
	if _, ok := m["rate_limits"]; !ok {
		t.Error("expected metrics to contain 'rate_limits'")
	}
}
