// internal/storage/tables/appointments.go

package tables

import (
	"database/sql"
	"fmt"
	"time"
)

const appointmentsSchema = `
CREATE TABLE IF NOT EXISTS appointments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	patient_chat_id INTEGER NOT NULL,
	patient_name TEXT,
	appointment_time INTEGER NOT NULL, -- unix timestamp
	doctor_name TEXT,
	department TEXT,
	status TEXT NOT NULL DEFAULT 'pending', -- pending, confirmed, cancelled, rescheduled
	reminder_sent INTEGER DEFAULT 0,
	created_at INTEGER DEFAULT (strftime('%s','now')),
	updated_at INTEGER
);

CREATE INDEX IF NOT EXISTS appointments_chat_id ON appointments(patient_chat_id);
CREATE INDEX IF NOT EXISTS appointments_status ON appointments(status);
CREATE INDEX IF NOT EXISTS appointments_time ON appointments(appointment_time);`

type Appointment struct {
	ID               int64
	PatientChatID    int64
	PatientName      string
	AppointmentTime  time.Time
	DoctorName       string
	Department       string
	Status           string
	ReminderSent     bool
	CreatedAt        time.Time
	UpdatedAt        *time.Time
}

type Appointments struct {
	database *sql.DB
}

func NewAppointments(db *sql.DB) (*Appointments, error) {
	_, err := db.Exec(appointmentsSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to create appointments schema: %w", err)
	}
	return &Appointments{database: db}, nil
}

// Create создает новую запись на прием
func (table *Appointments) Create(apt *Appointment) (int64, error) {
	res, err := table.database.Exec(
		"INSERT INTO appointments (patient_chat_id, patient_name, appointment_time, doctor_name, department, status) VALUES (?, ?, ?, ?, ?, ?)",
		apt.PatientChatID, apt.PatientName, apt.AppointmentTime.Unix(), apt.DoctorName, apt.Department, apt.Status,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FindByID находит запись по ID
func (table *Appointments) FindByID(id int64) (*Appointment, error) {
	row := table.database.QueryRow(
		"SELECT id, patient_chat_id, patient_name, appointment_time, doctor_name, department, status, reminder_sent, created_at, updated_at FROM appointments WHERE id = ?",
		id,
	)

	var apt Appointment
	var aptTimeUnix, createdAtUnix int64
	var updatedAtUnix sql.NullInt64
	var reminderSent int

	err := row.Scan(&apt.ID, &apt.PatientChatID, &apt.PatientName, &aptTimeUnix, &apt.DoctorName, &apt.Department, &apt.Status, &reminderSent, &createdAtUnix, &updatedAtUnix)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	apt.AppointmentTime = time.Unix(aptTimeUnix, 0)
	apt.CreatedAt = time.Unix(createdAtUnix, 0)
	apt.ReminderSent = reminderSent == 1
	if updatedAtUnix.Valid {
		t := time.Unix(updatedAtUnix.Int64, 0)
		apt.UpdatedAt = &t
	}

	return &apt, nil
}

// UpdateStatus обновляет статус записи
func (table *Appointments) UpdateStatus(id int64, status string) error {
	_, err := table.database.Exec(
		"UPDATE appointments SET status = ?, updated_at = strftime('%s','now') WHERE id = ?",
		status, id,
	)
	return err
}

// MarkReminderSent помечает что напоминание отправлено
func (table *Appointments) MarkReminderSent(id int64) error {
	_, err := table.database.Exec(
		"UPDATE appointments SET reminder_sent = 1 WHERE id = ?",
		id,
	)
	return err
}

// ListPending возвращает ожидающие записи для отправки напоминаний
func (table *Appointments) ListPending(limit int) ([]*Appointment, error) {
	rows, err := table.database.Query(
		"SELECT id, patient_chat_id, patient_name, appointment_time, doctor_name, department, status, reminder_sent, created_at FROM appointments WHERE status = 'pending' AND reminder_sent = 0 LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []*Appointment
	for rows.Next() {
		var apt Appointment
		var aptTimeUnix, createdAtUnix int64
		var reminderSent int

		err := rows.Scan(&apt.ID, &apt.PatientChatID, &apt.PatientName, &aptTimeUnix, &apt.DoctorName, &apt.Department, &apt.Status, &reminderSent, &createdAtUnix)
		if err != nil {
			return nil, err
		}

		apt.AppointmentTime = time.Unix(aptTimeUnix, 0)
		apt.CreatedAt = time.Unix(createdAtUnix, 0)
		apt.ReminderSent = reminderSent == 1
		appointments = append(appointments, &apt)
	}

	return appointments, rows.Err()
}

// UpdateReschedule обновляет запись при переносе (новое время и врач)
func (table *Appointments) UpdateReschedule(id int64, newTime time.Time, newDoctor string) error {
	_, err := table.database.Exec(
		"UPDATE appointments SET appointment_time = ?, doctor_name = ?, status = 'rescheduled', updated_at = strftime('%s','now') WHERE id = ?",
		newTime.Unix(), newDoctor, id,
	)
	return err
}

// ListByChatID возвращает записи пациента
func (table *Appointments) ListByChatID(chatID int64) ([]*Appointment, error) {
	rows, err := table.database.Query(
		"SELECT id, patient_chat_id, patient_name, appointment_time, doctor_name, department, status, reminder_sent, created_at, updated_at FROM appointments WHERE patient_chat_id = ? ORDER BY appointment_time",
		chatID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appointments []*Appointment
	for rows.Next() {
		var apt Appointment
		var aptTimeUnix, createdAtUnix int64
		var updatedAtUnix sql.NullInt64
		var reminderSent int

		err := rows.Scan(&apt.ID, &apt.PatientChatID, &apt.PatientName, &aptTimeUnix, &apt.DoctorName, &apt.Department, &apt.Status, &reminderSent, &createdAtUnix, &updatedAtUnix)
		if err != nil {
			return nil, err
		}

		apt.AppointmentTime = time.Unix(aptTimeUnix, 0)
		apt.CreatedAt = time.Unix(createdAtUnix, 0)
		apt.ReminderSent = reminderSent == 1
		if updatedAtUnix.Valid {
			t := time.Unix(updatedAtUnix.Int64, 0)
			apt.UpdatedAt = &t
		}
		appointments = append(appointments, &apt)
	}

	return appointments, rows.Err()
}

