# Документация: Записи на приём к врачу

> Дата создания: 2026-05-19
> Описывает логику работы с записями в расписаниях, эндпоинты и интеграцию с бэкендом

---

## Содержание

1. [Общая архитектура](#общая-архитектура)
2. [Структуры данных](#структуры-данных)
3. [API Эндпоинты](#api-эндпоинты)
4. [Обработка команд от пользователя](#обработка-команд-от-пользователя)
5. [Интеграция с бэкендом](#интеграция-с-бэкендом)
6. [Пошаговый flow переноса записи](#пошаговый-flow-переноса-записи)
7. [Конфигурация](#конфигурация)

---

## Общая архитектура

### Компоненты системы записей

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MAX Bot Service                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────────────────────────┐│
│  │   HTTP API   │  │   Bot Core   │  │         Database (SQLite)           ││
│  │  (internal/  │  │  (internal/  │  │                                     ││
│  │   api/handlers│  │  bot/service)│  │  • appointments                    ││
│  │              │  │              │  │  • reschedule_sessions             ││
│  └──────┬───────┘  └──────┬───────┘  └─────────────────────────────────────┘│
│         │                 │                                                │
│         ▼                 ▼                                                │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                      Backend Integration                              │  │
│  │  • POST /cancel - отмена записи                                      │  │
│  │  • GET /doctors?department=X - список врачей                         │  │
│  │  • POST /webhook - уведомление о статусе                             │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Расположение файлов

| Компонент | Путь к файлу | Описание |
|-----------|-------------|----------|
| **Таблица записей** | `internal/storage/tables/appointments.go` | CRUD операции с записями |
| **Таблица сессий переноса** | `internal/storage/tables/reschedule_sessions.go` | Состояние пошагового переноса |
| **API хендлеры** | `internal/api/handlers/appointment-reminder.go` | HTTP эндпоинты |
| **Регистрация роутов** | `internal/api/service.go` | Маршрутизация |
| **Бизнес-логика** | `internal/bot/service.go` | Обработка команд, flow переноса |
| **Конфигурация** | `internal/config/config.go` | Переменные окружения |

---

## Структуры данных

### Таблица `appointments`

**Расположение:** `internal/storage/tables/appointments.go`

**SQL Schema:**
```sql
CREATE TABLE appointments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    patient_chat_id INTEGER NOT NULL,      -- ID чата пациента
    patient_name TEXT,                      -- Имя пациента
    appointment_time INTEGER NOT NULL,      -- Unix timestamp
    doctor_name TEXT,                       -- ФИО врача
    department TEXT,                        -- Отделение
    status TEXT NOT NULL DEFAULT 'pending', -- pending|confirmed|cancelled|rescheduled
    reminder_sent INTEGER DEFAULT 0,        -- Флаг отправки напоминания
    created_at INTEGER DEFAULT (strftime('%s','now')),
    updated_at INTEGER
);

-- Индексы
CREATE INDEX appointments_chat_id ON appointments(patient_chat_id);
CREATE INDEX appointments_status ON appointments(status);
CREATE INDEX appointments_time ON appointments(appointment_time);
```

**Go структура:**
```go
type Appointment struct {
    ID               int64
    PatientChatID    int64
    PatientName      string
    AppointmentTime  time.Time
    DoctorName       string
    Department       string
    Status           string    // "pending", "confirmed", "cancelled", "rescheduled"
    ReminderSent     bool
    CreatedAt        time.Time
    UpdatedAt        *time.Time // nullable
}
```

**Методы таблицы:**

| Метод | Сигнатура | Описание |
|-------|-----------|----------|
| `Create` | `Create(apt *Appointment) (int64, error)` | Создаёт новую запись, возвращает ID |
| `FindByID` | `FindByID(id int64) (*Appointment, error)` | Находит запись по ID |
| `UpdateStatus` | `UpdateStatus(id int64, status string) error` | Обновляет статус |
| `UpdateReschedule` | `UpdateReschedule(id int64, newTime time.Time, newDoctor string) error` | Обновляет время и врача при переносе |
| `MarkReminderSent` | `MarkReminderSent(id int64) error` | Помечает что напоминание отправлено |
| `ListPending` | `ListPending(limit int) ([]*Appointment, error)` | Список записей для напоминаний |
| `ListByChatID` | `ListByChatID(chatID int64) ([]*Appointment, error)` | Записи конкретного пациента |

---

### Таблица `reschedule_sessions`

**Расположение:** `internal/storage/tables/reschedule_sessions.go`

Хранит состояние пошагового переноса записи. Сессия живёт 30 минут.

**SQL Schema:**
```sql
CREATE TABLE reschedule_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    appointment_id INTEGER NOT NULL,
    step TEXT NOT NULL DEFAULT 'week',  -- week|day|doctor|time|confirm
    selected_week TEXT,                  -- 'current' или 'next'
    selected_day TEXT,                   -- 'mon'|'tue'|'wed'|'thu'|'fri'
    selected_date TEXT,                  -- DD.MM.YYYY
    selected_doctor TEXT,
    selected_time TEXT,                  -- HH:MM
    available_doctors TEXT,              -- JSON массив
    available_times TEXT,                -- JSON массив
    created_at INTEGER DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL          -- Unix timestamp
);
```

**Go структура:**
```go
type RescheduleSession struct {
    ID               int64
    UserID           int64
    AppointmentID    int64
    Step             string   // week, day, doctor, time, confirm
    SelectedWeek     string   // 'current' или 'next'
    SelectedDay      string   // 'mon', 'tue', 'wed', 'thu', 'fri'
    SelectedDate     string   // DD.MM.YYYY
    SelectedDoctor   string
    SelectedTime     string   // HH:MM
    AvailableDoctors []string // из JSON
    AvailableTimes   []string // из JSON
    CreatedAt        time.Time
    ExpiresAt        time.Time
}
```

**Методы таблицы:**

| Метод | Описание |
|-------|----------|
| `Create(userID, appointmentID int64)` | Создаёт сессию с expires_at = now + 30 min |
| `FindByUserID(userID int64)` | Находит активную (не протухшую) сессию |
| `UpdateStepWeek(id, week)` | Сохраняет выбор недели, переходит на шаг 'day' |
| `UpdateStepDay(id, day)` | Сохраняет день, переходит на 'doctor' |
| `UpdateStepDate(id, date, doctors)` | Сохраняет дату и список врачей (JSON) |
| `UpdateStepDoctor(id, doctor, times)` | Сохраняет врача и время (JSON) |
| `UpdateStepTime(id, time)` | Сохраняет время, переходит на 'confirm' |
| `Delete(id)` | Удаляет сессию |
| `CleanupExpired()` | Удаляет протухшие сессии |

---

## API Эндпоинты

**Регистрация в:** `internal/api/service.go:133-135`

### 1. Создание записи

```
POST /create-appointment
Content-Type: application/json
```

**Request:**
```go
type createAppointmentRequest struct {
    PatientChatID    int64  `json:"patient_chat_id"`  // обязательно
    PatientName      string `json:"patient_name"`
    AppointmentTime  string `json:"appointment_time"` // RFC3339 или "2006-01-02 15:04:05"
    DoctorName       string `json:"doctor_name"`
    Department       string `json:"department"`
}
```

**Пример запроса:**
```json
{
  "patient_chat_id": 123456789,
  "patient_name": "Иванов Иван",
  "appointment_time": "2026-05-20T10:00:00+03:00",
  "doctor_name": "Петров А.В.",
  "department": "Терапевтическое"
}
```

**Response (200 OK):**
```json
{
  "status": "ok",
  "appointment_id": 42
}
```

**Response (400 Bad Request):**
```json
{
  "error": "patient_chat_id and appointment_time are required"
}
```

**Реализация:** `internal/api/handlers/appointment-reminder.go:112-174`

---

### 2. Отправка напоминания

```
POST /send-appointment-reminder
Content-Type: application/json
```

Отправляет сообщение с тремя кнопками: Приду / Отменить / Перенести.

**Request:**
```go
type appointmentReminderRequest struct {
    AppointmentID  int64  `json:"appointment_id"`  // обязательно
    Text           string `json:"text"`            // обязательно, текст сообщения
    ChatID         int64  `json:"chat_id"`         // опционально, явный chat_id
    PatientPhone   string `json:"patient_phone"`   // опционально, для поиска контакта
}
```

**Пример запроса:**
```json
{
  "appointment_id": 42,
  "text": "Напоминание о записи к врачу Петрову А.В. на 20.05.2026 в 10:00. Отделение: Терапевтическое."
}
```

**Response (200 OK):**
```json
{
  "status": "ok",
  "message": "reminder sent"
}
```

**Response (404 Not Found):**
```json
{
  "error": "Appointment not found"
}
```

**Клавиатура в сообщении:**
```
[✅ Приду]          → payload: "/appointment_confirm 42"
[❌ Отменить]       → payload: "/appointment_cancel 42"
[🗓 Перенести]      → payload: "/appointment_reschedule 42"
```

**Реализация:** `internal/api/handlers/appointment-reminder.go:16-108`

---

### 3. Список записей

```
GET /appointments?chat_id={optional}
```

**Параметры:**
- `chat_id` (optional) - если указан, возвращает записи конкретного пациента
- без параметра - возвращает `pending` записи (limit 100)

**Response (200 OK):**
```json
{
  "status": "ok",
  "appointments": [
    {
      "ID": 42,
      "PatientChatID": 123456789,
      "PatientName": "Иванов Иван",
      "AppointmentTime": "2026-05-20T10:00:00+03:00",
      "DoctorName": "Петров А.В.",
      "Department": "Терапевтическое",
      "Status": "pending",
      "ReminderSent": false,
      "CreatedAt": "2026-05-18T09:00:00+03:00",
      "UpdatedAt": null
    }
  ]
}
```

**Реализация:** `internal/api/handlers/appointment-reminder.go:176-213`

---

## Обработка команд от пользователя

### Маршрутизация команд

**Расположение:** `internal/bot/service.go:909-920`

```go
// Обработка команд записи на приём к врачу
if strings.HasPrefix(trimmedText, "/appointment_confirm") ||
   strings.HasPrefix(trimmedText, "/appointment_cancel") ||
   strings.HasPrefix(trimmedText, "/appointment_reschedule") {
    srv.handleAppointmentCommand(ctx, userID, chatKey, trimmedText)
    return
}

// Обработка шагов переноса (дата → врач → время)
if strings.HasPrefix(trimmedText, "/reschedule_") {
    srv.handleRescheduleStep(ctx, userID, chatKey, trimmedText)
    return
}
```

### Команды записи

**Метод:** `internal/bot/service.go:1957-2031`

| Команда | Описание | Действие |
|---------|----------|----------|
| `/appointment_confirm {id}` | Подтверждение записи | Статус → `confirmed`, уведомление бэкенд |
| `/appointment_cancel {id}` | Отмена записи | POST на бэкенд (отмена), статус → `cancelled` |
| `/appointment_reschedule {id}` | Начать перенос | Запуск flow переноса |

**Получение команды:** через кнопки в напоминании (payload содержит команду + ID).

---

### Команды переноса (step-by-step)

**Метод:** `internal/bot/service.go:2140-2198`

| Команда | Шаг | Описание |
|---------|-----|----------|
| `/reschedule_week {current\|next}` | 1 | Выбор недели |
| `/reschedule_day {mon\|tue\|wed\|thu\|fri} {DD.MM.YYYY}` | 2 | Выбор дня и даты |
| `/reschedule_date {DD.MM.YYYY}` | 3 | Запрос врачей на дату |
| `/reschedule_doctor {ФИО}` | 4 | Выбор врача, получение времени |
| `/reschedule_time {HH:MM}` | 5 | Выбор времени, показ подтверждения |
| `/reschedule_confirm` | 6 | Финальное подтверждение |
| `/reschedule_cancel` | - | Отмена сессии |
| `/reschedule_restart` | - | Начать заново |
| `/reschedule_back_to_*` | - | Навигация назад |

---

## Интеграция с бэкендом

### Уведомление о статусе записи

**Метод:** `internal/bot/service.go:2033-2070`

При каждом изменении статуса (confirm, cancel, reschedule) отправляется webhook на бэкенд:

```
POST {APPOINTMENT_BACKEND_URL}
Content-Type: application/json
```

**Payload:**
```json
{
  "appointment_id": 42,
  "status": "confirmed",
  "user_id": 123456789,
  "timestamp": 1716123456
}
```

Если `APPOINTMENT_BACKEND_URL` не настроен — уведомление пропускается (логируется).

---

### Отмена записи через бэкенд

**Метод:** `internal/bot/service.go:2501-2543`

```
POST {APPOINTMENT_CANCEL_ENDPOINT}
Content-Type: application/json
X-API-Key: {APPOINTMENT_BACKEND_API_KEY}  // если настроен
```

**Request:**
```json
{
  "appointment_id": 42,
  "timestamp": 1716123456
}
```

**Логика:**
1. Если endpoint не настроен — пропускаем, локальная отмена продолжается
2. При ошибке бэкенда — логируем, но всё равно отменяем локально
3. Успешный ответ 2xx — продолжаем

---

### Получение списка врачей

**Метод:** `internal/bot/service.go:2545-2597`

```
GET {APPOINTMENT_DOCTORS_ENDPOINT}?department={department}
X-API-Key: {APPOINTMENT_BACKEND_API_KEY}  // если настроен
```

**Response ожидаемый:**
```json
{
  "doctors": ["Петров А.В.", "Сидоров М.Б.", "Кузнецова Е.В."]
}
```

**Fallback:** если endpoint не настроен или вернул ошибку — используется локальный список:
```go
[]string{
    "Петров А.В.",
    "Сидоров М.Б.",
    "Кузнецова Е.В.",
    "Новикова О.П.",
}
```

---

## Пошаговый flow переноса записи

### Диаграмма состояний

```
┌─────────────┐
│   Начало    │  Пользователь нажимает "Перенести"
└──────┬──────┘
       ▼
┌─────────────┐     ┌─────────────┐
│   Создание  │────►│   Шаг 1:    │
│   сессии    │     │  Выбор недели │
└─────────────┘     └──────┬──────┘
                           │ /reschedule_week {current|next}
                           ▼
                    ┌─────────────┐
                    │   Шаг 2:    │
                    │ Выбор дня   │
                    └──────┬──────┘
                           │ /reschedule_day {day} {date}
                           ▼
                    ┌─────────────┐
                    │   Шаг 3:    │
                    │ Запрос      │◄──── GET /doctors?department=X
                    │ врачей      │       (из бэкенда или fallback)
                    └──────┬──────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   Шаг 4:    │
                    │ Выбор врача │
                    └──────┬──────┘
                           │ /reschedule_doctor {ФИО}
                           ▼
                    ┌─────────────┐
                    │   Шаг 5:    │
                    │  Получение  │◄──── getAvailableTimes() (заглушка)
                    │   времени   │
                    └──────┬──────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   Шаг 6:    │
                    │ Выбор времени│
                    └──────┬──────┘
                           │ /reschedule_time {HH:MM}
                           ▼
                    ┌─────────────┐
                    │   Шаг 7:    │
                    │ Подтверждение│
                    └──────┬──────┘
                           │ /reschedule_confirm
                           ▼
                    ┌─────────────┐
                    │  Финализация │────► UpdateReschedule() в БД
                    │  + уведомление│────► notifyBackendAppointmentStatus()
                    └─────────────┘      ► Удаление сессии
```

### Пример сессии в БД

```
id: 15
user_id: 123456789
appointment_id: 42
step: "confirm"                    -- текущий шаг
selected_week: "current"
selected_day: "tue"
selected_date: "20.05.2026"
selected_doctor: "Петров А.В."
selected_time: "14:30"
available_doctors: '["Петров А.В.","Сидоров М.Б."]'
available_times: '["09:00","09:30","14:30"]'
expires_at: 1716125256             -- +30 мин от создания
```

---

## Конфигурация

### Переменные окружения

**Расположение:** `internal/config/config.go:31-34`

| Переменная | Тип | Описание |
|------------|-----|----------|
| `APPOINTMENT_CANCEL_ENDPOINT` | URL (POST) | Эндпоинт для отмены записи на бэкенде |
| `APPOINTMENT_DOCTORS_ENDPOINT` | URL (GET) | Эндпоинт для получения врачей по отделению |
| `APPOINTMENT_BACKEND_API_KEY` | string | API ключ для авторизации на бэкенде |
| `APPOINTMENT_BACKEND_URL` | URL (POST) | Вебхук для уведомлений о статусе |

### Пример .env

```bash
# Основные настройки бота
BOT_TOKEN=your_bot_token
WEBHOOK_URL=https://your-domain.com/webhook

# Эндпоинты бэкенда для записей
APPOINTMENT_CANCEL_ENDPOINT=https://hospital-api.example.com/appointments/cancel
APPOINTMENT_DOCTORS_ENDPOINT=https://hospital-api.example.com/doctors
APPOINTMENT_BACKEND_API_KEY=your_backend_api_key
APPOINTMENT_BACKEND_URL=https://hospital-api.example.com/webhook/status
```

### Загрузка конфигурации

```go
// internal/config/config.go:94-97
cfg.AppointmentCancelEndpoint = os.Getenv("APPOINTMENT_CANCEL_ENDPOINT")
cfg.AppointmentDoctorsEndpoint = os.Getenv("APPOINTMENT_DOCTORS_ENDPOINT")
cfg.AppointmentBackendAPIKey = os.Getenv("APPOINTMENT_BACKEND_API_KEY")
```

**Использование в сервисе:**
```go
// internal/bot/service.go:38
cfg           *config.Config

// Доступ к эндпоинтам через srv.cfg.AppointmentCancelEndpoint
```

---

## Частые сценарии

### Сценарий 1: Создание записи и отправка напоминания

```bash
# 1. Создаём запись
curl -X POST http://localhost:9003/create-appointment \
  -H "Content-Type: application/json" \
  -d '{
    "patient_chat_id": 123456789,
    "patient_name": "Иванов Иван",
    "appointment_time": "2026-05-20T10:00:00+03:00",
    "doctor_name": "Петров А.В.",
    "department": "Терапевтическое"
  }'
# Ответ: {"status": "ok", "appointment_id": 42}

# 2. Отправляем напоминание
curl -X POST http://localhost:9003/send-appointment-reminder \
  -H "Content-Type: application/json" \
  -d '{
    "appointment_id": 42,
    "text": "Напоминание о записи к врачу Петрову А.В. на завтра в 10:00"
  }'
```

### Сценарий 2: Пользователь переносит запись

1. Пользователь получает напоминание с кнопками
2. Нажимает "Перенести" → бот вызывает `startRescheduleFlow()`
3. Пошаговый выбор через кнопки:
   - Выбор недели → Выбор дня → Выбор врача → Выбор времени → Подтверждение
4. При каждом шаге обновляется `reschedule_sessions`
5. При подтверждении:
   - Обновляется `appointments` (новое время, врач, статус="rescheduled")
   - Отправляется уведомление на бэкенд
   - Сессия удаляется

### Сценарий 3: Отмена через бэкенд

1. Пользователь нажимает "Отменить"
2. Бот вызывает `requestBackendCancel(appointmentID)`
   - POST на `APPOINTMENT_CANCEL_ENDPOINT`
3. Независимо от результата бэкенда:
   - Обновляем статус в локальной БД → `cancelled`
   - Отправляем уведомление на webhook
   - Отправляем подтверждение пользователю

---

## Сводка по файлам и методам

| Функциональность | Файл | Методы/структуры |
|-----------------|------|-----------------|
| **БД - Записи** | `internal/storage/tables/appointments.go` | `Appointment struct`, `Create()`, `FindByID()`, `UpdateStatus()`, `UpdateReschedule()`, `MarkReminderSent()`, `ListPending()`, `ListByChatID()` |
| **БД - Сессии** | `internal/storage/tables/reschedule_sessions.go` | `RescheduleSession struct`, `Create()`, `FindByUserID()`, `UpdateStep*()`, `Delete()`, `CleanupExpired()` |
| **API - Создание** | `internal/api/handlers/appointment-reminder.go:112` | `CreateAppointmentHandler.ServeHTTP()` |
| **API - Напоминание** | `internal/api/handlers/appointment-reminder.go:30` | `SendAppointmentReminderHandler.ServeHTTP()` |
| **API - Список** | `internal/api/handlers/appointment-reminder.go:181` | `ListAppointmentsHandler.ServeHTTP()` |
| **API - Роуты** | `internal/api/service.go:133-135` | `mux.Handle("/create-appointment", ...)`, `/send-appointment-reminder`, `/appointments` |
| **Бот - Команды** | `internal/bot/service.go:1957` | `handleAppointmentCommand()` |
| **Бот - Flow переноса** | `internal/bot/service.go:2072` | `startRescheduleFlow()`, `showWeekSelection()`, `handleRescheduleStep()`, `handleRescheduleWeek()`, `handleRescheduleDay()`, `handleRescheduleDate()`, `handleRescheduleDoctor()`, `handleRescheduleTime()`, `finalizeReschedule()` |
| **Бот - Бэкенд API** | `internal/bot/service.go:2501` | `requestBackendCancel()`, `requestBackendDoctors()`, `notifyBackendAppointmentStatus()` |
| **Бот - Fallback** | `internal/bot/service.go:2599` | `getFallbackDoctors()`, `getAvailableTimes()`, `getAvailableDoctors()` |
| **Конфиг** | `internal/config/config.go:31` | `AppointmentCancelEndpoint`, `AppointmentDoctorsEndpoint`, `AppointmentBackendAPIKey` |

---

## Важные замечания

1. **Сессии переноса** живут 30 минут. После истечения пользователь должен начать заново.

2. **Fallback на врачей** — если бэкенд недоступен или не настроен, используется фиксированный список врачей.

3. **Отмена всегда локальная** — даже если бэкенд вернул ошибку, запись отменяется в локальной БД.

4. **Уведомления webhook** — отправляются при любом изменении статуса, но не блокируют операцию при ошибке.

5. **Авторизация на бэкенде** — через заголовок `X-API-Key` (если настроен `APPOINTMENT_BACKEND_API_KEY`).
