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
	directory = "./data/"
	file      = "storage.db"
)

type Service struct {
	Contacts *tables.Contacts
}

func NewService() *Service {
	return &Service{}
}

func (srv *Service) Start(ctx context.Context) {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		if err = os.Mkdir(directory, 0755); err != nil {
			log.Fatal(err)
		}
	}

	db, err := sql.Open("sqlite", path.Join(directory, file))
	if err != nil {
		log.Fatal(err)
	}

	srv.Contacts = tables.NewContacts(db)

	go func() {
		<-ctx.Done()
		_ = db.Close()
	}()
}
