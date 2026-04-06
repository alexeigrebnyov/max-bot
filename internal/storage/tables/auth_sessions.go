// internal/storage/tables/auth_sessions.go

package tables

import (
	"database/sql"
	"errors"
	"log"
	"time"
)

const authSessionsSchema = `
CREATE TABLE IF NOT EXISTS auth_sessions (
    user_id INTEGER NOT NULL PRIMARY KEY,
    state TEXT NOT NULL,
    emchash TEXT,
    phone TEXT,
    avatar TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS auth_sessions_state ON auth_sessions(state);
CREATE INDEX IF NOT EXISTS auth_sessions_created_at ON auth_sessions(created_at);
`

type AuthSessions struct {
	database *sql.DB
}

type AuthSession struct {
	UserID    int64
	State     string // "awaiting_phone_emchash", "awaiting_phone_empty", "awaiting_birthdate"
	EMCHash   string // для варианта с emchash
	Phone     string // для варианта с телефоном
	Avatar     string // для варианта с телефоном
	Attempts  int    // количество попыток (максимум 3)
	CreatedAt int64  // timestamp создания
	UpdatedAt int64  // timestamp последнего обновления
}

func NewAuthSessions(db *sql.DB) *AuthSessions {
	if _, err := db.Exec(authSessionsSchema); err != nil {
		log.Fatal(err)
	}
	return &AuthSessions{database: db}
}

// Get возвращает сессию авторизации по userID
func (table *AuthSessions) Get(userID int64) (*AuthSession, error) {
	row := table.database.QueryRow(
		"SELECT user_id, state, COALESCE(emchash, ''), COALESCE(phone, ''), COALESCE(avatar, ''), attempts, created_at, updated_at FROM auth_sessions WHERE user_id = ?",
		userID,
	)

	var session AuthSession
	err := row.Scan(&session.UserID, &session.State, &session.EMCHash, &session.Phone, &session.Avatar, &session.Attempts, &session.CreatedAt, &session.UpdatedAt)
	if err == nil {
		return &session, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	return nil, nil
}

// Save создаёт или обновляет сессию авторизации
func (table *AuthSessions) Save(session *AuthSession) error {
	now := time.Now().Unix()
	if session.CreatedAt == 0 {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	res, err := table.database.Exec(
		"UPDATE auth_sessions SET state = ?, emchash = ?, phone = ?, avatar = ?, attempts = ?, updated_at = ? WHERE user_id = ?",
		session.State, session.EMCHash, session.Phone, session.Avatar, session.Attempts, session.UpdatedAt, session.UserID,
	)
	if err != nil {
		return err
	}

	count, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		_, err = table.database.Exec(
			"INSERT INTO auth_sessions VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			session.UserID, session.State, session.EMCHash, session.Phone, session.Avatar, session.Attempts, session.CreatedAt, session.UpdatedAt,
		)
		return err
	}

	return nil
}

// Delete удаляет сессию авторизации
func (table *AuthSessions) Delete(userID int64) error {
	_, err := table.database.Exec("DELETE FROM auth_sessions WHERE user_id = ?", userID)
	return err
}

// CleanupOld удаляет старые сессии (старше 1 часа)
func (table *AuthSessions) CleanupOld() error {
	oneHourAgo := time.Now().Add(-1 * time.Hour).Unix()
	_, err := table.database.Exec("DELETE FROM auth_sessions WHERE created_at < ?", oneHourAgo)
	return err
}
