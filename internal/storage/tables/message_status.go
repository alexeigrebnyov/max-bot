// internal/storage/tables/message_status.go

package tables

import (
	"database/sql"
	"log"
)

const messageStatusSchema = `
CREATE TABLE IF NOT EXISTS message_status (
	chat_id INTEGER NOT NULL,
	message_mid TEXT NOT NULL,
	is_read INTEGER NOT NULL DEFAULT 0,
	read_at INTEGER,
	PRIMARY KEY (chat_id, message_mid)
);

CREATE INDEX IF NOT EXISTS message_status_chat_id ON message_status(chat_id);
CREATE INDEX IF NOT EXISTS message_status_is_read ON message_status(is_read);`

type MessageStatus struct {
	database *sql.DB
}

type MessageReadStatus struct {
	ChatID     int64
	MessageMid string
	IsRead     bool
	ReadAt     int64 // timestamp в миллисекундах
}

func NewMessageStatus(db *sql.DB) *MessageStatus {
	_, err := db.Exec(messageStatusSchema)
	if err != nil {
		log.Fatal(err)
	}
	return &MessageStatus{database: db}
}

// MarkAsRead помечает сообщение как прочитанное
func (table *MessageStatus) MarkAsRead(chatID int64, messageMid string, readAt int64) error {
	_, err := table.database.Exec(
		"INSERT OR REPLACE INTO message_status (chat_id, message_mid, is_read, read_at) VALUES (?, ?, 1, ?)",
		chatID, messageMid, readAt,
	)
	return err
}

// CreateUnread создаёт запись о непрочитанном сообщении
func (table *MessageStatus) CreateUnread(chatID int64, messageMid string) error {
	_, err := table.database.Exec(
		"INSERT OR IGNORE INTO message_status (chat_id, message_mid, is_read, read_at) VALUES (?, ?, 0, NULL)",
		chatID, messageMid,
	)
	return err
}

// MarkMultipleAsRead помечает несколько сообщений как прочитанные
func (table *MessageStatus) MarkMultipleAsRead(chatID int64, messageMids []string, readAt int64) error {
	tx, err := table.database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR REPLACE INTO message_status (chat_id, message_mid, is_read, read_at) VALUES (?, ?, 1, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, mid := range messageMids {
		if _, err := stmt.Exec(chatID, mid, readAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetStatus возвращает статус сообщения
func (table *MessageStatus) GetStatus(chatID int64, messageMid string) (*MessageReadStatus, error) {
	row := table.database.QueryRow(
		"SELECT chat_id, message_mid, is_read, COALESCE(read_at, 0) FROM message_status WHERE chat_id = ? AND message_mid = ?",
		chatID, messageMid,
	)

	var status MessageReadStatus
	var isRead int
	err := row.Scan(&status.ChatID, &status.MessageMid, &isRead, &status.ReadAt)
	if err == sql.ErrNoRows {
		// Если записи нет, считаем сообщение непрочитанным
		return &MessageReadStatus{
			ChatID:     chatID,
			MessageMid: messageMid,
			IsRead:     false,
			ReadAt:     0,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	status.IsRead = isRead == 1
	return &status, nil
}

// GetUnreadCount возвращает количество непрочитанных сообщений для чата
func (table *MessageStatus) GetUnreadCount(chatID int64) (int, error) {
	row := table.database.QueryRow(
		"SELECT COUNT(*) FROM message_status WHERE chat_id = ? AND is_read = 0",
		chatID,
	)

	var count int
	err := row.Scan(&count)
	return count, err
}

// MarkAllAsRead помечает все сообщения чата как прочитанные
func (table *MessageStatus) MarkAllAsRead(chatID int64, readAt int64) error {
	_, err := table.database.Exec(
		"UPDATE message_status SET is_read = 1, read_at = ? WHERE chat_id = ? AND is_read = 0",
		readAt, chatID,
	)
	return err
}
