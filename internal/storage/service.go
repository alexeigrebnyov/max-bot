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
	Contacts   *tables.Contacts
	GroupChats *tables.GroupChats
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

	srv.Contacts = tables.NewContacts(db)
	srv.GroupChats = tables.NewGroupChats(db)

	go func() {
		<-ctx.Done()
		_ = db.Close()
	}()
}
