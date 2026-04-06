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
	phone TEXT NOT NULL,
	name TEXT NOT NULL,
	emc TEXT,
	avatar_url TEXT,
	emchash TEXT,
	birthdate TEXT
);

CREATE INDEX IF NOT EXISTS contacts_chatID ON contacts(chatID);
CREATE INDEX IF NOT EXISTS contacts_phone ON contacts(phone);
CREATE INDEX IF NOT EXISTS contacts_emchash ON contacts(emchash);`

type Contacts struct {
	database *sql.DB
}

type Contact struct {
	UserID    int64
	ChatID    int64
	Phone     string
	Name      string
	EMC       string
	AvatarURL string
	EMCHash   string
	Birthdate string // формат: dd.mm.yyyy
}

func NewContacts(db *sql.DB) *Contacts {
	_, err := db.Exec(schema)
	if err != nil {
		log.Fatal(err)
	}

	// Миграция: добавить колонки avatar_url, emchash и birthdate в существующие БД
	_, _ = db.Exec("ALTER TABLE contacts ADD COLUMN avatar_url TEXT")
	_, _ = db.Exec("ALTER TABLE contacts ADD COLUMN emchash TEXT")
	_, _ = db.Exec("ALTER TABLE contacts ADD COLUMN birthdate TEXT")
	// Игнорируем ошибку "duplicate column name" (таблица уже с этими полями)

	return &Contacts{database: db}
}

func (table *Contacts) Find(value string) (*Contact, error) {
	row := table.database.QueryRow(
		"SELECT userID, chatID, phone, name, COALESCE(emc, ''), COALESCE(avatar_url, ''), COALESCE(emchash, ''), COALESCE(birthdate, '') FROM contacts WHERE userID = ? OR chatID = ? OR phone = ?",
		value, value, normalizePhone(value),
	)

	var contact Contact
	err := row.Scan(&contact.UserID, &contact.ChatID, &contact.Phone, &contact.Name, &contact.EMC, &contact.AvatarURL, &contact.EMCHash, &contact.Birthdate)
	if err == nil {
		return &contact, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	return nil, nil
}

// FindByEMCHash ищет контакт по emchash
func (table *Contacts) FindByEMCHash(emchash string) (*Contact, error) {
	if emchash == "" {
		return nil, nil
	}

	row := table.database.QueryRow(
		"SELECT userID, chatID, phone, name, COALESCE(emc, ''), COALESCE(avatar_url, ''), COALESCE(emchash, ''), COALESCE(birthdate, '') FROM contacts WHERE emchash = ?",
		emchash,
	)

	var contact Contact
	err := row.Scan(&contact.UserID, &contact.ChatID, &contact.Phone, &contact.Name, &contact.EMC, &contact.AvatarURL, &contact.EMCHash, &contact.Birthdate)
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
		"UPDATE contacts SET userID = ?, chatID = ?, avatar_url = ? WHERE phone = ? ",
		contact.UserID, contact.ChatID,  contact.AvatarURL, phone,
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
			"INSERT INTO contacts VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			contact.UserID, contact.ChatID, phone, contact.Name, contact.EMC, contact.AvatarURL, contact.EMCHash, contact.Birthdate,
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

func (table *Contacts) UpdateByPhone(contact *Contact) (*Contact, error) {
	phone := normalizePhone(contact.Phone)

	rows, err := table.database.Query(
		"SELECT userID, chatID, phone, name, COALESCE(emc, ''), COALESCE(avatar_url, ''), COALESCE(emchash, ''), COALESCE(birthdate, '') FROM contacts WHERE phone = ?",
		phone,
	)

	if err != nil {
        		return nil, err
        	}

	defer rows.Close()

    	var contacts []*Contact
    	for rows.Next() {
    		var newcontact Contact
    		if err := rows.Scan(&newcontact.UserID, &newcontact.ChatID, &newcontact.Phone, &newcontact.Name, &newcontact.EMC, &newcontact.AvatarURL, &newcontact.EMCHash, &newcontact.Birthdate); err != nil {
    			return nil, err
    		}
    		contacts = append(contacts, &newcontact)
    	}
    	if err := rows.Err(); err != nil {
    		return nil, err
    	}



	count := len(contacts)

	if count > 1 {
	err := errors.New("Найдено более одного совпадения для номера телефона")
	    return nil, err
	} else if count==0  {
      		return nil, sql.ErrNoRows
      		}

	if count > 0 {
		res, err := table.database.Exec(
			"UPDATE contacts SET name=?, emc=?, avatar_url=?, emchash=?, birthdate=? WHERE phone = ?",
			contact.Name, contact.EMC, contact.AvatarURL, contact.EMCHash, contact.Birthdate, phone,
		)
		if err != nil {
			return nil, err
		}

		count, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}

		if count > 0 {

		    return contacts[0], nil

		}

		return nil, nil
	}

	return nil, nil
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

func NormalizePhone(phone string) string {
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

func normalizePhone(phone string) string {
	return NormalizePhone(phone)
}

// All возвращает все контакты из таблицы.
func (table *Contacts) All() ([]*Contact, error) {
	rows, err := table.database.Query("SELECT userID, chatID, phone, name, COALESCE(emc, ''), COALESCE(avatar_url, ''), COALESCE(emchash, ''), COALESCE(birthdate, '') FROM contacts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.UserID, &c.ChatID, &c.Phone, &c.Name, &c.EMC, &c.AvatarURL, &c.EMCHash, &c.Birthdate); err != nil {
			return nil, err
		}
		contacts = append(contacts, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contacts, nil
}
