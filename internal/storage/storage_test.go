package storage

import (
	"context"
	"testing"

	"max-bot-service/internal/storage/tables"

	_ "modernc.org/sqlite"
)

func TestNewService(t *testing.T) {
	srv := NewService()
	if srv == nil {
		t.Fatal("NewService() returned nil")
	}
	if srv.Contacts != nil || srv.GroupChats != nil {
		t.Error("before Start(), Contacts and GroupChats should be nil")
	}
}

func TestStart_WithTempDir_InitializesTables(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewService()
	srv.DataDir = t.TempDir()
	srv.Start(ctx)

	if srv.Contacts == nil || srv.GroupChats == nil {
		t.Fatal("after Start(), Contacts and GroupChats should be set")
	}

	// Проверка работы БД: сохранение и чтение контакта
	inserted, err := srv.Contacts.Save(&tables.Contact{UserID: 1, ChatID: 2, Phone: "79001234567"})
	if err != nil {
		t.Fatalf("Contacts.Save: %v", err)
	}
	if !inserted {
		t.Error("Contacts.Save: expected inserted true")
	}
	c, err := srv.Contacts.Find("1")
	if err != nil || c == nil || c.Phone != "79001234567" {
		t.Fatalf("Contacts.Find: err=%v contact=%+v", err, c)
	}

	// Групповой чат
	_, err = srv.GroupChats.Save(&tables.GroupChat{ChatID: 100, Title: "Test", MenuSent: false})
	if err != nil {
		t.Fatalf("GroupChats.Save: %v", err)
	}
	gc, err := srv.GroupChats.FindByChatID(100)
	if err != nil || gc == nil || gc.Title != "Test" {
		t.Fatalf("GroupChats.FindByChatID: err=%v gc=%+v", err, gc)
	}
}

func TestStart_WithTempDir_ClosesDBOnContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	srv := NewService()
	srv.DataDir = t.TempDir()
	srv.Start(ctx)

	cancel() // сигнал завершения — горутина закроет db
	// Дополнительной проверки закрытия без экспорта db нет; проверяем отсутствие паники
}
