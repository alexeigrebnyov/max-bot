// internal/storage/tables/group_chats.go

package tables

import (
	"database/sql"
	"errors"
	"log"
)

const groupChatsSchema = `
CREATE TABLE IF NOT EXISTS group_chats (
    chatID INTEGER NOT NULL PRIMARY KEY,
    title  TEXT    NOT NULL,
    menu_sent INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS group_chats_title ON group_chats(title);
`

type GroupChats struct {
	database *sql.DB
}

type GroupChat struct {
	ChatID   int64
	Title    string
	MenuSent bool
}

func NewGroupChats(db *sql.DB) *GroupChats {
	if _, err := db.Exec(groupChatsSchema); err != nil {
		log.Fatal(err)
	}
	// Миграция: добавить колонку menu_sent в существующие БД (для новых она уже в CREATE TABLE).
	_, _ = db.Exec("ALTER TABLE group_chats ADD COLUMN menu_sent INTEGER NOT NULL DEFAULT 0")
	// Игнорируем ошибку "duplicate column name" (таблица уже с menu_sent).
	return &GroupChats{database: db}
}

// FindByTitle ищет чат по title (точное совпадение строки)
func (table *GroupChats) FindByTitle(title string) (*GroupChat, error) {
	row := table.database.QueryRow(
		"SELECT chatID, title, menu_sent FROM group_chats WHERE title = ?",
		title,
	)

	var chat GroupChat
	var menuSent int
	err := row.Scan(&chat.ChatID, &chat.Title, &menuSent)
	if err == nil {
		chat.MenuSent = menuSent != 0
		return &chat, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	return nil, nil
}

// Save вставляет или обновляет запись по chatID (menu_sent при вставке = 0, при обновлении не меняется).
func (table *GroupChats) Save(chat *GroupChat) (bool, error) {
	res, err := table.database.Exec(
		"UPDATE group_chats SET title = ? WHERE chatID = ?",
		chat.Title, chat.ChatID,
	)
	if err != nil {
		return false, err
	}

	count, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	if count == 0 {
		menuSent := 0
		if chat.MenuSent {
			menuSent = 1
		}
		res, err = table.database.Exec(
			"INSERT INTO group_chats(chatID, title, menu_sent) VALUES (?, ?, ?)",
			chat.ChatID, chat.Title, menuSent,
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

// SetMenuSent помечает, что меню с кнопкой «Покажи ID чата» уже отправлено в этот чат.
func (table *GroupChats) SetMenuSent(chatID int64) error {
	_, err := table.database.Exec(
		"UPDATE group_chats SET menu_sent = 1 WHERE chatID = ?",
		chatID,
	)
	return err
}

func (table *GroupChats) FindByChatID(chatID int64) (*GroupChat, error) {
	row := table.database.QueryRow(
		"SELECT chatID, title, COALESCE(menu_sent, 0) FROM group_chats WHERE chatID = ?",
		chatID,
	)

	var chat GroupChat
	var menuSent int
	err := row.Scan(&chat.ChatID, &chat.Title, &menuSent)
	if err == nil {
		chat.MenuSent = menuSent != 0
		return &chat, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	return nil, nil
}

// All - вернуть все записи (для отладки / UI)
func (table *GroupChats) All() ([]GroupChat, error) {
	rows, err := table.database.Query(
		"SELECT chatID, title, COALESCE(menu_sent, 0) FROM group_chats ORDER BY title",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]GroupChat, 0)
	for rows.Next() {
		var gc GroupChat
		var menuSent int
		if err := rows.Scan(&gc.ChatID, &gc.Title, &menuSent); err != nil {
			return nil, err
		}
		gc.MenuSent = menuSent != 0
		res = append(res, gc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// DeleteByChatID — для точечной чистки
func (table *GroupChats) DeleteByChatID(chatID int64) error {
	_, err := table.database.Exec(
		"DELETE FROM group_chats WHERE chatID = ?",
		chatID,
	)
	return err
}
