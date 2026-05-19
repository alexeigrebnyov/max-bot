# Навигационная карта проекта MAX Bot

> Последнее обновление: 2026-05-04

## Оглавление
- [Архитектура проекта](#архитектура-проекта)
- [Основные компоненты](#основные-компоненты)
- [Навигация по функциональности](#навигация-по-функциональности)
- [База данных](#база-данных)
- [API эндпоинты](#api-эндпоинты)
- [Фронтенд](#фронтенд)

---

## Архитектура проекта

```
max-bot/
├── cmd/max-bot/          # Точка входа приложения
│   └── main.go           # Инициализация и запуск сервисов
├── internal/
│   ├── config/           # Конфигурация
│   │   └── config.go     # Загрузка переменных окружения
│   ├── storage/          # Слой работы с БД
│   │   ├── service.go    # Инициализация БД, WAL mode, connection pool
│   │   └── tables/       # Модели таблиц
│   │       ├── contacts.go        # Таблица контактов
│   │       ├── message_status.go  # Статусы прочитанности сообщений
│   │       ├── auth_sessions.go   # Сессии авторизации
│   │       └── group_chats.go     # Групповые чаты
│   ├── bot/              # Логика бота
│   │   ├── service.go    # Основная логика, обработка событий
│   │   └── model.go      # Модель бота, API клиент MAX
│   └── api/              # HTTP API
│       ├── service.go    # Регистрация роутов, middleware
│       └── handlers/     # HTTP handlers
│           ├── evnts.go           # EventSource (SSE)
│           ├── message_status.go  # Статусы сообщений
│           ├── add-contact.go     # Управление контактами
│           └── ...
└── web/static/           # Фронтенд
    ├── index.html        # Главная страница (чаты)
    └── messages.html     # Страница сообщений

```

---

## Основные компоненты

### 1. Точка входа
**Файл:** `cmd/max-bot/main.go`
- Загрузка конфигурации
- Инициализация БД
- Запуск bot service
- Запуск API service

### 2. Конфигурация
**Файл:** `internal/config/config.go`
- Переменные окружения
- Настройки MAX API
- Настройки webhook/long polling

### 3. База данных
**Файл:** `internal/storage/service.go`
- Инициализация SQLite
- WAL mode для конкурентности
- Connection pool (MaxOpenConns=1, MaxIdleConns=1)
- Busy timeout 5000ms

### 4. Бот сервис
**Файл:** `internal/bot/service.go`
- Обработка событий от MAX
- Авторизация пользователей
- Отправка сообщений
- Управление контактами

### 5. API сервис
**Файл:** `internal/api/service.go`
- HTTP роуты
- Middleware (rate limit, API key)
- EventSource для real-time обновлений

---

## Навигация по функциональности

### 🔐 Авторизация пользователей

#### Где искать:
- **Основная логика:** `internal/bot/service.go`
  - `handleAuthSession()` - роутинг по типу авторизации
  - `handleAwaitingPhoneEMCHash()` - авторизация по EMC + телефон (строка 994)
  - `handleAwaitingBirthdate()` - авторизация по дате рождения (строка 1120)
  - `saveContactByEMCHash()` - авторизация по EMC hash из payload (строка 1400)

#### Таблицы БД:
- `internal/storage/tables/auth_sessions.go` - временные сессии авторизации
- `internal/storage/tables/contacts.go` - сохранение авторизованных контактов

#### Поток авторизации:
1. Пользователь отправляет `/start` с payload или без
2. Создается сессия в `auth_sessions`
3. Бот запрашивает телефон/дату рождения
4. После проверки устанавливается `authorized = true`
5. Контакт сохраняется в `contacts`

---

### 💬 Сообщения

#### Отправка сообщений:
- **API handler:** `internal/api/handlers/send-message.go`
  - POST `/send-message` - отправка по chat_id
  - POST `/send-by-phone` - отправка по телефону
  - POST `/send-chat-message` - отправка с EventSource уведомлением

- **Бот клиент:** `internal/bot/model.go`
  - `SendMessage()` - базовый метод отправки
  - `SendMessageWithKeyboard()` - с клавиатурой

#### Получение сообщений:
- **API handler:** `internal/api/handlers/get-messages.go`
  - GET `/get-messages-by-chatid?chat_id=X` - все сообщения чата

#### Обработка входящих:
- **Webhook:** `internal/bot/service.go`
  - `WebhookHandler()` - прием от MAX (строка 100)
  - `handleUpdate()` - обработка события (строка 200)
  - `handleMessageCreated()` - новое сообщение (строка 700)

---

### 📊 Статусы прочитанности

#### Где искать:
- **Таблица:** `internal/storage/tables/message_status.go`
  - `CreateUnread()` - создать запись о непрочитанном
  - `MarkAsRead()` - удалить запись (= пометить прочитанным)
  - `MarkMultipleAsRead()` - удалить несколько записей
  - `MarkAllAsRead()` - удалить все записи чата
  - `GetUnreadCount()` - количество непрочитанных
  - `GetUnreadMessages()` - список непрочитанных

- **API handlers:** `internal/api/handlers/message_status.go`
  - POST `/mark-messages-read` - пометить прочитанными
  - GET `/unread-count?chat_id=X` - счетчик
  - GET `/unread-messages` - список всех непрочитанных

#### Логика:
- При получении сообщения от пользователя → `CreateUnread()`
- При открытии чата → `MarkAllAsRead()`
- Прочитанные сообщения **удаляются** из таблицы (не обновляются)

---

### 👥 Контакты

#### Где искать:
- **Таблица:** `internal/storage/tables/contacts.go`
  - `Find()` - поиск по userID/chatID/phone
  - `FindByEMCHash()` - поиск по EMC hash
  - `Save()` - создание/обновление
  - `UpdateByPhone()` - обновление по телефону
  - `All()` - **только авторизованные** контакты
  - `Delete()` - удаление

- **API handlers:** `internal/api/handlers/add-contact.go`, `internal/api/handlers/get-contact.go`
  - POST `/add-contact` - добавить/обновить контакт
  - POST `/update-contact` - обновить контакт по телефону
  - GET `/get-contact?phone=X&user_id=Y` - получить контакт
  - GET `/contacts` - список авторизованных контактов
  - GET `/all-contacts` - список всех контактов

#### Поля контакта:
```go
type Contact struct {
    UserID     int64  // ID пользователя в MAX
    ChatID     int64  // ID чата
    Phone      string // Телефон
    Name       string // Имя
    EMC        string // EMC код
    AvatarURL  string // URL аватара
    EMCHash    string // Hash для авторизации
    Birthdate  string // Дата рождения (dd.mm.yyyy)
    Authorized bool   // Флаг авторизации
}
```

#### Важно:
- `All()` возвращает **только** `authorized = true`
- При авторизации устанавливается `Authorized = true`

---

### 🔄 Real-time обновления (EventSource)

#### Где искать:
- **Backend:** `internal/api/handlers/evnts.go`
  - `EventBroker` - управление подключениями
  - `broadcastMessagesLoop()` - рассылка новых сообщений
  - `broadcastContactsLoop()` - рассылка обновлений контактов
  - `broadcast()` - общая функция рассылки

- **Frontend:** `web/static/index.html`
  - `eventSource = new EventSource('/events')` - подключение
  - `handleMessageEvent()` - обработка нового сообщения
  - `handleContactUpdateEvent()` - обработка обновления контакта

#### Каналы:
- `bot.Service.NewMessages` - канал новых сообщений (буфер 100)
- `bot.Service.NewContacts` - канал обновлений контактов (буфер 100)

#### Формат событий:
```json
{
  "type": "message",
  "data": { /* Message */ }
}

{
  "type": "contact_update",
  "data": { /* Contact */ }
}
```

---

### 🎨 Фронтенд

#### Главная страница (чаты)
**Файл:** `web/static/index.html`

**Основные функции:**
- `loadContacts()` - загрузка списка контактов
- `openChat(chatId)` - открытие чата
- `loadMessages(chatId)` - загрузка сообщений чата
- `sendMessage()` - отправка сообщения
- `markAllAsRead(chatId)` - пометить все прочитанными
- `scrollToMessage(mid)` - скролл к сообщению с подсветкой
- `handleMessageEvent()` - обработка нового сообщения из EventSource
- `handleContactUpdateEvent()` - обработка обновления контакта

**EventSource:**
- Автоматическое обновление контактов
- Автоматическое добавление новых сообщений
- Обновление счетчиков непрочитанных

#### Страница сообщений
**Файл:** `web/static/messages.html`

**Основные функции:**
- `loadContacts()` - загрузка контактов
- `loadAllMessages()` - загрузка всех сообщений
- `loadUnreadMessages()` - загрузка непрочитанных из БД
- `renderMessages()` - отрисовка с фильтрацией
- `openChat(chatId, mid)` - переход к чату с фокусом на сообщении

**Фильтры:**
- "Все" - все сообщения из MAX
- "Непрочитанные" - из `/unread-messages` (БД бота)
- "Отправленные" - фильтр по `sender.is_bot`

---

## База данных

### Таблица: `contacts`
**Файл:** `internal/storage/tables/contacts.go`

```sql
CREATE TABLE contacts (
    userID INTEGER PRIMARY KEY,
    chatID INTEGER NOT NULL,
    phone TEXT NOT NULL,
    name TEXT NOT NULL,
    emc TEXT,
    avatar_url TEXT,
    emchash TEXT,
    birthdate TEXT,
    authorized INTEGER NOT NULL DEFAULT 0
);
```

**Индексы:**
- `contacts_chatID` - по chatID
- `contacts_phone` - по phone
- `contacts_emchash` - по emchash
- `contacts_authorized` - по authorized

---

### Таблица: `message_status`
**Файл:** `internal/storage/tables/message_status.go`

```sql
CREATE TABLE message_status (
    chat_id INTEGER NOT NULL,
    message_mid TEXT NOT NULL,
    is_read INTEGER NOT NULL DEFAULT 0,
    read_at INTEGER,
    PRIMARY KEY (chat_id, message_mid)
);
```

**Индексы:**
- `message_status_chat_id` - по chat_id
- `message_status_is_read` - по is_read

**Важно:** Прочитанные сообщения **удаляются** из таблицы!

---

### Таблица: `appointments`
**Файл:** `internal/storage/tables/appointments.go`

```sql
CREATE TABLE appointments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    patient_chat_id INTEGER NOT NULL,
    patient_name TEXT,
    appointment_time INTEGER NOT NULL,
    doctor_name TEXT,
    department TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    reminder_sent INTEGER DEFAULT 0,
    created_at INTEGER DEFAULT (strftime('%s','now')),
    updated_at INTEGER
);
```

**Статусы:** `pending` → `confirmed` | `cancelled` | `rescheduled`

**Индексы:**
- `appointments_chat_id` - по patient_chat_id
- `appointments_status` - по status
- `appointments_time` - по appointment_time

---

### Таблица: `reschedule_sessions`
**Файл:** `internal/storage/tables/reschedule_sessions.go`

Хранит состояние пошагового переноса записи.

```sql
CREATE TABLE reschedule_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    appointment_id INTEGER NOT NULL,
    step TEXT NOT NULL DEFAULT 'week',
    selected_week TEXT,
    selected_day TEXT,
    selected_date TEXT,
    selected_doctor TEXT,
    selected_time TEXT,
    available_doctors TEXT,
    available_times TEXT,
    created_at INTEGER DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL
);
```

**Шаги:** `week` → `day` → `doctor` → `time` → `confirm`

**Индексы:**
- `reschedule_sessions_user_id` - по user_id
- `reschedule_sessions_appointment_id` - по appointment_id
- `reschedule_sessions_expires` - по expires_at

**Flow переноса:**
1. Пользователь нажимает "Перенести" → создается сессия
2. Выбор недели (текущая/следующая)
3. Выбор дня недели (Пн-Пт) с конкретной датой
4. Выбор врача (список из бэкенда по отделению)
5. Выбор времени
6. Подтверждение

---

### Таблица: `auth_sessions`
**Файл:** `internal/storage/tables/auth_sessions.go`

```sql
CREATE TABLE auth_sessions (
    user_id INTEGER PRIMARY KEY,
    state TEXT NOT NULL,
    emchash TEXT,
    phone TEXT,
    attempts INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    avatar TEXT
);
```

**Состояния:**
- `awaiting_phone_emchash` - ожидание телефона (есть EMC hash)
- `awaiting_phone_empty` - ожидание телефона (нет payload)
- `awaiting_birthdate` - ожидание даты рождения

---

### Таблица: `group_chats`
**Файл:** `internal/storage/tables/group_chats.go`

```sql
CREATE TABLE group_chats (
    chat_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    last_updated INTEGER NOT NULL
);
```

---

## API эндпоинты

### Сообщения
```
POST   /send-message              - отправить сообщение по chat_id
POST   /send-by-phone             - отправить сообщение по телефону
POST   /send-to-group-by-chatid   - отправить в группу
POST   /send-chat-message         - отправить и транслировать в EventSource
GET    /get-messages-by-chatid    - получить сообщения чата с статусами прочтения
```

### Статусы сообщений
```
POST   /mark-messages-read        - пометить прочитанными
GET    /unread-count              - счетчик непрочитанных
GET    /unread-messages           - список непрочитанных
```

### Записи на приём к врачу
```
POST   /create-appointment        - создать запись
POST   /send-appointment-reminder - отправить напоминание с кнопками
GET    /appointments              - список записей
```

**Обработка кнопок:**
- Кнопки отправляют команды: `/appointment_confirm <id>`, `/appointment_cancel <id>`, `/appointment_reschedule <id>`
- Обработка в `internal/bot/service.go` → `handleAppointmentCommand()`
- Уведомление бэкенда через `notifyBackendAppointmentStatus()`

### Контакты
```
POST   /add-contact               - добавить/обновить контакт
POST   /update-contact            - обновить контакт по телефону
GET    /get-contact               - получить контакт (по phone или user_id)
GET    /contacts                  - список авторизованных контактов
GET    /all-contacts              - список всех контактов
```

### Групповые чаты
```
POST   /refresh-group-chats       - обновить список групп (требует API key)
GET    /group-chats               - получить список групп (требует API key)
```

### Real-time
```
GET    /events                    - EventSource (SSE)
```

### UI
```
GET    /admin                     - главная страница (чаты)
GET    /messages                  - страница сообщений
```

### Служебные
```
POST   /webhook                   - webhook от MAX
GET    /metrics                   - метрики сервиса
GET    /                          - информация о боте
```

---

## Быстрый поиск по задачам

### Нужно добавить новое поле в контакт?
1. `internal/storage/tables/contacts.go` - добавить в схему и структуру
2. Обновить все SQL запросы в методах таблицы
3. `internal/api/handlers/add-contact.go` - добавить в request struct
4. Миграция: `ALTER TABLE contacts ADD COLUMN ...`

### Нужно добавить новый API эндпоинт?
1. Создать handler в `internal/api/handlers/`
2. Зарегистрировать в `internal/api/service.go` → `buildMux()`
3. При необходимости добавить middleware (rate limit, API key)

### Нужно изменить логику авторизации?
1. `internal/bot/service.go` → `handleAuthSession()`
2. Найти нужный handler (`handleAwaitingPhoneEMCHash`, `handleAwaitingBirthdate`)
3. `internal/storage/tables/auth_sessions.go` - если нужно изменить состояния

### Нужно добавить новый тип события от MAX?
1. `internal/bot/service.go` → `handleUpdate()`
2. Добавить case для нового типа события
3. Создать handler функцию

### Нужно изменить фронтенд?
- **Чаты:** `web/static/index.html`
- **Сообщения:** `web/static/messages.html`
- Оба файла содержат HTML + CSS + JavaScript в одном файле

---

## Полезные команды

### Поиск по коду
```bash
# Найти все места где используется функция
grep -r "functionName" internal/

# Найти определение структуры
grep -r "type StructName" internal/

# Найти все SQL запросы
grep -r "SELECT\|INSERT\|UPDATE\|DELETE" internal/storage/
```

### Работа с БД
```bash
# Открыть БД
sqlite3 bot.db

# Посмотреть схему таблицы
.schema contacts

# Проверить авторизованных пользователей
SELECT * FROM contacts WHERE authorized = 1;

# Проверить непрочитанные сообщения
SELECT * FROM message_status;
```

---

## Changelog

### 2026-05-07
- **Интеграция с бэкендом для записей**
  - Таблица `reschedule_sessions` для пошагового переноса записи
  - Переменные окружения для интеграции:
    - `APPOINTMENT_CANCEL_ENDPOINT` - POST-запрос для отмены записи
    - `APPOINTMENT_DOCTORS_ENDPOINT` - GET-запрос для получения врачей по отделению
    - `APPOINTMENT_BACKEND_API_KEY` - API ключ для авторизации
  - Многоэтапный flow переноса: неделя → день → врач → время → подтверждение
  - Методы: `requestBackendCancel()`, `requestBackendDoctors()`

### 2026-05-04
- **Новый функционал: Записи на приём к врачу**
  - Таблица `appointments` для хранения записей
  - API: `POST /create-appointment` - создание записи
  - API: `POST /send-appointment-reminder` - отправка с кнопками (Приду/Отменить/Перенести)
  - API: `GET /appointments` - список записей
  - Обработка команд: `/appointment_confirm|cancel|reschedule <id>`
  - Уведомление бэкенда через переменную `APPOINTMENT_BACKEND_URL`
  - Экспорт типов `Keyboard`, `KeyboardButton` из `internal/bot/model.go`
- Обновлена документация README.md (API эндпоинты)
- Добавлен `/all-contacts` - список всех контактов
- Уточнен порт по умолчанию: 9003

### 2026-04-08
- Добавлено поле `authorized` в таблицу `contacts`
- Изменена логика статусов: прочитанные сообщения удаляются
- Добавлен API `/unread-messages`
- Фильтр "непрочитанные" теперь использует БД бота
- Обновления контактов через EventSource
- Фокус на сообщении при переходе из раздела "Сообщения"

---

**Совет:** При работе с проектом всегда начинайте с этой карты, чтобы быстро найти нужный файл и функцию!
