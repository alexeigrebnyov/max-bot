// internal/storage/service.go

package storage

import (
	"context"
	"database/sql"
	"log"
	"max-bot-service/internal/storage/tables"
	"os"
	"path"

	_ "modernc.org/sqlite"
)

const (
	defaultDirectory = "./data/"
	file             = "storage.db"
)

type Service struct {
	Contacts      *tables.Contacts
	GroupChats    *tables.GroupChats
	AuthSessions  *tables.AuthSessions
	MessageStatus *tables.MessageStatus
	// DataDir задаёт каталог для SQLite (для тестов — временная директория). Пустой — defaultDirectory.
	DataDir string
}

func (srv *Service) dataDir() string {
	if srv.DataDir != "" {
		return srv.DataDir
	}
	return defaultDirectory
}

func NewService() *Service {
	return &Service{}
}

func (srv *Service) Start(ctx context.Context) {
	dir := srv.dataDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	db, err := sql.Open("sqlite", path.Join(dir, file))
	if err != nil {
		log.Fatal(err)
	}

	// Настройка SQLite для лучшей конкурентности
	db.SetMaxOpenConns(1) // SQLite поддерживает только одно соединение для записи
	db.SetMaxIdleConns(1)

	// Включаем WAL mode для лучшей конкурентности
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("Warning: failed to enable WAL mode: %v", err)
	}

	// Увеличиваем таймаут для занятой БД
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		log.Printf("Warning: failed to set busy_timeout: %v", err)
	}

	srv.Contacts = tables.NewContacts(db)
	srv.GroupChats = tables.NewGroupChats(db)
	srv.AuthSessions = tables.NewAuthSessions(db)
	srv.MessageStatus = tables.NewMessageStatus(db)

	go func() {
		<-ctx.Done()
		_ = db.Close()
	}()
}
