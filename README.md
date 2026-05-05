<!-- markdownlint-disable MD041 -->

## max.botservice — бот-сервис для MAX

HTTP‑сервис на Go для работы с ботом в MAX. Поддерживает **Webhook** и **Long Polling**, хранит контакты/группы в SQLite и предоставляет HTTP API для отправки сообщений.

### Требования

- **Go**: 1.25+
- **Docker**: опционально (для контейнерного запуска)

---

## Назначение и режимы работы

- **Webhook**: MAX делает `POST /webhook` на ваш публичный URL.
- **Long Polling**: сервис сам опрашивает MAX (`GET /updates`), если `WEBHOOK_URL` пустой.

---

## Конфигурация (env)

Скопируйте пример и заполните минимум `BOT_TOKEN`:

```bash
cp .env.example .env
```

### Обязательное

- **`BOT_TOKEN`**: токен бота MAX.

### Режим обновлений

- **`WEBHOOK_URL`**: если задан — включается Webhook.
- **`WEBHOOK_SECRET`**: если задан — входящие Webhook **обязаны** содержать заголовок `X-Webhook-Secret` с этим значением; иначе `401`.

### Сеть/сервер

- **`PORT`**: порт HTTP‑сервера (по умолчанию 8080).
- **`MAX_API_BASE_URL`**: базовый URL API MAX (по умолчанию `https://platform-api.max.ru`).

### Доступ к служебным эндпоинтам

- **`API_KEY`**: если задан — эндпоинты `/refresh-group-chats` и `/group-chats` требуют заголовок `X-API-Key`.

### Надёжность

- **`RATE_LIMIT_PER_MINUTE`**: лимит запросов в минуту для `/send-message` и `/send-by-phone` (по умолчанию 60, `0` = отключено). При превышении — `429`.
- **`SHUTDOWN_TIMEOUT_SEC`**: таймаут корректного завершения сервера (по умолчанию 10 секунд).

### Логирование

- **`LOG_LEVEL`**: `debug|info|warn|error` (по умолчанию `info`)
- **`LOG_FORMAT`**: `text|json` (по умолчанию `text`)

---

## Запуск

### Локально

```bash
go run ./cmd
```

### Docker Compose

Используйте `docker-compose.yml` в корне репозитория:

```bash
docker compose up -d --build
```

---

## HTTP API

### `GET /`

Редирект на страницу бота в MAX по нику.

### `POST /send-message`

- **Назначение**: отправить сообщение пользователю.
- **Rate limit**: да (см. `RATE_LIMIT_PER_MINUTE`).

Пример:

```bash
curl -X POST http://localhost:9003/send-message \
  -H "Content-Type: application/json" \
  -d '{"chat":"23718629","thread":0,"text":"Hello","private":false}'
```

### `POST /send-by-phone`

- **Назначение**: отправить сообщение по номеру телефона (ищется в таблице `contacts`).
- **Rate limit**: да.

### `POST /send-to-group-by-chatid`

- **Назначение**: отправить сообщение в групповой чат по `chat_id`.

### `POST /send-chat-message`

- **Назначение**: отправить сообщение в чат и транслировать его в EventSource.
- **Тело**: `{"chat_id": 123, "text": "сообщение"}`

### `GET /get-messages-by-chatid`

- **Назначение**: получить сообщения чата по `chat_id` с статусами прочтения.
- **Параметр**: `chat_id` (query string)

---

## Управление контактами

### `POST /add-contact`

- **Назначение**: добавить или обновить контакт.
- **Тело**: `{"user_id": 123, "chat_id": 456, "phone": "79001234567", "name": "Имя", "emc": "...", "avatar_url": "...", "emchash": "...", "birthdate": "01.01.1990", "authorized": true}`

### `POST /update-contact`

- **Назначение**: обновить контакт по номеру телефона.
- **Тело**: аналогично `add-contact`, обновляет поля по `phone`.

### `GET /get-contact`

- **Назначение**: получить контакт по `phone` или `user_id`.
- **Параметры**: `phone` или `user_id` (query string)

### `GET /contacts`

- **Назначение**: получить всех **авторизованных** контактов.

### `GET /all-contacts`

- **Назначение**: получить **все** контакты (включая неавторизованных).

---

## Статусы сообщений

### `POST /mark-messages-read`

- **Назначение**: пометить сообщения как прочитанные.
- **Тело**: `{"chat_id": 123, "message_mids": ["mid1", "mid2"]}` (если `message_mids` пуст — помечает все)

### `GET /unread-count`

- **Назначение**: получить количество непрочитанных сообщений в чате.
- **Параметр**: `chat_id` (query string)

### `GET /unread-messages`

- **Назначение**: получить список всех непрочитанных сообщений.

---

## Записи на приём к врачу

### `POST /create-appointment`

- **Назначение**: создать новую запись на приём.
- **Тело**: `{"patient_chat_id": 123, "patient_name": "Иван", "appointment_time": "2026-05-10T14:00:00Z", "doctor_name": "Петров", "department": "Терапия"}`
- **Ответ**: `{"appointment_id": 42}`

### `POST /send-appointment-reminder`

- **Назначение**: отправить пациенту напоминание о записи с кнопками (Приду/Отменить/Перенести).
- **Тело**: `{"appointment_id": 42, "text": "Напоминание о приёме..."}`
- **Кнопки**: При нажатии отправляют команды `/appointment_confirm <id>`, `/appointment_cancel <id>`, `/appointment_reschedule <id>`

### `GET /appointments`

- **Назначение**: получить список записей.
- **Параметры**: `chat_id` (опционально) — фильтр по пациенту
- **Ответ**: список записей со статусами (pending, confirmed, cancelled, rescheduled)

**Обработка ответов пациента:**
- При нажатии кнопки бот автоматически:
  1. Обновляет статус записи в БД
  2. Отправляет подтверждение пациенту
  3. Уведомляет бэкенд (если настроен `APPOINTMENT_BACKEND_URL`)

---

## Web UI и EventSource

### `GET /admin`

- **Назначение**: веб-интерфейс управления ботом (HTML страница).

### `GET /messages`

- **Назначение**: страница просмотра сообщений (HTML страница).

### `GET /events`

- **Назначение**: EventSource (SSE) для получения событий в реальном времени.
- **События**: `message` — новое сообщение, `contact_update` — обновление контакта.

---

## Служебные эндпоинты

### `POST /refresh-group-chats`

- **Назначение**: принудительно обновить кеш групп из MAX.
- **Защита**: если `API_KEY` задан — требуется `X-API-Key`.

### `GET /group-chats`

- **Назначение**: вернуть кеш групповых чатов из SQLite.
- **Защита**: если `API_KEY` задан — требуется `X-API-Key`.

### `POST /webhook`

- **Назначение**: приём событий от MAX (Webhook mode).
- **Защита**: если `WEBHOOK_SECRET` задан — требуется `X-Webhook-Secret`.

### `GET /metrics`

JSON‑счётчики: `requests`, `errors`, `rate_limits`.

---

## Тестирование

```bash
go test ./...
```

Если в вашей среде Windows запуск части тестов блокируется политикой (Application Control), это не связано с кодом. В таком случае запускать тесты в Docker/CI.

