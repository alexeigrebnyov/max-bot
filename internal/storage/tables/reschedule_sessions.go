// internal/storage/tables/reschedule_sessions.go

package tables

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const rescheduleSessionsSchema = `
CREATE TABLE IF NOT EXISTS reschedule_sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	appointment_id INTEGER NOT NULL,
	step TEXT NOT NULL DEFAULT 'date', -- date, doctor, time, confirm
	selected_date TEXT,
	selected_doctor TEXT,
	selected_time TEXT,
	available_doctors TEXT, -- JSON массив
	available_times TEXT,   -- JSON массив
	created_at INTEGER DEFAULT (strftime('%s','now')),
	expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS reschedule_sessions_user_id ON reschedule_sessions(user_id);
CREATE INDEX IF NOT EXISTS reschedule_sessions_appointment_id ON reschedule_sessions(appointment_id);
CREATE INDEX IF NOT EXISTS reschedule_sessions_expires ON reschedule_sessions(expires_at);`

// RescheduleSession хранит состояние пошагового выбора для переноса
type RescheduleSession struct {
	ID               int64
	UserID           int64
	AppointmentID    int64
	Step             string // date, doctor, time, confirm
	SelectedDate     string // YYYY-MM-DD
	SelectedDoctor   string
	SelectedTime     string // HH:MM
	AvailableDoctors []string
	AvailableTimes   []string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type RescheduleSessions struct {
	database *sql.DB
}

func NewRescheduleSessions(db *sql.DB) (*RescheduleSessions, error) {
	_, err := db.Exec(rescheduleSessionsSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to create reschedule_sessions schema: %w", err)
	}
	return &RescheduleSessions{database: db}, nil
}

// Create создает новую сессию переноса
func (table *RescheduleSessions) Create(userID, appointmentID int64) (int64, error) {
	// Сессия живет 30 минут
	expiresAt := time.Now().Add(30 * time.Minute).Unix()

	res, err := table.database.Exec(
		"INSERT INTO reschedule_sessions (user_id, appointment_id, step, expires_at) VALUES (?, ?, 'date', ?)",
		userID, appointmentID, expiresAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FindByUserID находит активную сессию пользователя
func (table *RescheduleSessions) FindByUserID(userID int64) (*RescheduleSession, error) {
	now := time.Now().Unix()
	row := table.database.QueryRow(
		"SELECT id, user_id, appointment_id, step, selected_date, selected_doctor, selected_time, available_doctors, available_times, created_at, expires_at FROM reschedule_sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC LIMIT 1",
		userID, now,
	)

	var session RescheduleSession
	var createdAtUnix, expiresAtUnix int64
	var availableDoctorsJSON, availableTimesJSON sql.NullString

	err := row.Scan(&session.ID, &session.UserID, &session.AppointmentID, &session.Step, &session.SelectedDate, &session.SelectedDoctor, &session.SelectedTime, &availableDoctorsJSON, &availableTimesJSON, &createdAtUnix, &expiresAtUnix)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session.CreatedAt = time.Unix(createdAtUnix, 0)
	session.ExpiresAt = time.Unix(expiresAtUnix, 0)

	if availableDoctorsJSON.Valid && availableDoctorsJSON.String != "" {
		if err := json.Unmarshal([]byte(availableDoctorsJSON.String), &session.AvailableDoctors); err != nil {
			log.Printf("FindByUserID: failed to unmarshal available_doctors: %v", err)
		}
	}
	if availableTimesJSON.Valid && availableTimesJSON.String != "" {
		if err := json.Unmarshal([]byte(availableTimesJSON.String), &session.AvailableTimes); err != nil {
			log.Printf("FindByUserID: failed to unmarshal available_times: %v", err)
		}
	}

	return &session, nil
}

// UpdateStepDate обновляет шаг и выбранную дату
func (table *RescheduleSessions) UpdateStepDate(id int64, date string, doctors []string) error {
	doctorsJSON, err := json.Marshal(doctors)
	if err != nil {
		return fmt.Errorf("failed to marshal doctors: %w", err)
	}
	_, err = table.database.Exec(
		"UPDATE reschedule_sessions SET step = 'doctor', selected_date = ?, available_doctors = ? WHERE id = ?",
		date, string(doctorsJSON), id,
	)
	return err
}

// UpdateStepDoctor обновляет шаг и выбранного врача
func (table *RescheduleSessions) UpdateStepDoctor(id int64, doctor string, times []string) error {
	timesJSON, err := json.Marshal(times)
	if err != nil {
		return fmt.Errorf("failed to marshal times: %w", err)
	}
	_, err = table.database.Exec(
		"UPDATE reschedule_sessions SET step = 'time', selected_doctor = ?, available_times = ? WHERE id = ?",
		doctor, string(timesJSON), id,
	)
	return err
}

// UpdateStepTime обновляет шаг и выбранное время
func (table *RescheduleSessions) UpdateStepTime(id int64, timeStr string) error {
	_, err := table.database.Exec(
		"UPDATE reschedule_sessions SET step = 'confirm', selected_time = ? WHERE id = ?",
		timeStr, id,
	)
	return err
}

// Delete удаляет сессию
func (table *RescheduleSessions) Delete(id int64) error {
	_, err := table.database.Exec("DELETE FROM reschedule_sessions WHERE id = ?", id)
	return err
}

// CleanupExpired удаляет протухшие сессии
func (table *RescheduleSessions) CleanupExpired() error {
	now := time.Now().Unix()
	_, err := table.database.Exec("DELETE FROM reschedule_sessions WHERE expires_at <= ?", now)
	return err
}
