// internal/storage/tables/contacts.go

package tables

import (
	"database/sql"
	"errors"
	"log"
	"strings"
)

const schema = `
CREATE TABLE IF NOT EXISTS contacts (
	userID INTEGER NOT NULL PRIMARY KEY,
	chatID INTEGER NOT NULL,
	phone TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS contacts_chatID ON contacts(chatID);
CREATE INDEX IF NOT EXISTS contacts_phone ON contacts(phone);`

type Contacts struct {
	database *sql.DB
}

type Contact struct {
	UserID int64
	ChatID int64
	Phone  string
}

func NewContacts(db *sql.DB) *Contacts {
	_, err := db.Exec(schema)
	if err != nil {
		log.Fatal(err)
	}

	return &Contacts{database: db}
}

func (table *Contacts) Find(value string) (*Contact, error) {
	row := table.database.QueryRow(
		"SELECT userID, chatID, phone FROM contacts WHERE userID = ? OR chatID = ? OR phone = ?",
		value, value, normalizePhone(value),
	)

	var contact Contact
	err := row.Scan(&contact.UserID, &contact.ChatID, &contact.Phone)
	if err == nil {
		return &contact, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	return nil, nil
}

func (table *Contacts) Save(contact *Contact) (bool, error) {
	phone := normalizePhone(contact.Phone)

	res, err := table.database.Exec(
		"UPDATE contacts SET chatID = ?, phone = ? WHERE userID = ?",
		contact.ChatID, phone, contact.UserID,
	)
	if err != nil {
		return false, err
	}

	count, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	if count == 0 {
		res, err = table.database.Exec(
			"INSERT INTO contacts VALUES (?, ?, ?)",
			contact.UserID, contact.ChatID, phone,
		)
		if err != nil {
			return false, err
		}

		count, err = res.RowsAffected()
		if err != nil {
			return false, err
		}

		return count > 0, nil
	}

	return false, nil
}

func (table *Contacts) Delete(userID int64) (bool, error) {
	res, err := table.database.Exec(
		"DELETE FROM contacts WHERE userID = ?",
		userID,
	)
	if err != nil {
		return false, err
	}

	count, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func normalizePhone(phone string) string {
	numbers := make([]rune, 0)
	for _, char := range phone {
		if char >= '0' && char <= '9' {
			numbers = append(numbers, char)
		}
	}

	normalized := string(numbers)
	if len([]rune(normalized)) == 11 && normalized[0] == '8' {
		normalized = strings.Replace(normalized, "8", "7", 1)
	}

	return normalized
}
