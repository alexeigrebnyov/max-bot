````markdown
# max.botservice — бот-сервис для MAX

HTTP‑сервис на Go для работы с ботом в мессенджере **MAX**.  
Умеет принимать webhook‑события от MAX, вести диалоги с пользователем (меню, настройка уведомлений, сохранение телефона в SQLite) и предоставляет HTTP‑endpoint для отправки уведомлений из внешних систем. [file:111]

---

## 1. Что делает сервис

- Принимает сообщения от пользователей через **webhook** `/webhook`.
- Показывает главное меню с inline‑кнопками:
  - «Настроить уведомления ✉»
  - «Отправить свой номер телефона ☎️»
  - «Исключить свой номер телефона ❌`
- Сохраняет телефон пользователя в SQLite, чтобы внешний сервер мог отправлять ему уведомления по chatId/phone. [file:64]
- Даёт внешний HTTP‑endpoint `/send-message` для отправки уведомлений в MAX‑чат.

---

## 2. Переменные окружения

Настраиваются через `.env` и/или `docker-compose.yml`.

# заполнить реальные значения BOT_TOKEN/WEBHOOK_URL/WEBHOOK_SECRET

```bash
cp .env.example .env
```

```env
BOT_TOKEN=your_max_bot_token_here           # токен бота из dev.max.ru
WEBHOOK_URL=https://your-domain.com/webhook # публичный URL вебхука
WEBHOOK_SECRET=random_secret_string         # секрет для проверки запросов
MAX_API_BASE_URL=https://platform-api.max.ru # опционально, базовый URL API MAX
```
````

- `BOT_TOKEN` — обязателен, без него сервис не стартует.
- `WEBHOOK_SECRET` — должен совпадать с тем, что укажете при подписке webhook в кабинете MAX.
- `WEBHOOK_URL` — тот же URL, который вы регистрируете в MAX для webhook.

---

## 3. Запуск локально (без Docker)

Требуется Go 1.24+.

```bash
go mod tidy
go run ./cmd
```

По умолчанию сервис слушает `:8080`. [file:38]

---

## 4. Запуск через Docker

### 4.1. Dockerfile

Сервис собирается в один бинарник:

```dockerfile
FROM golang:1.24.0-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o max-bot-service ./cmd

FROM alpine:3.19

WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/max-bot-service /app/max-bot-service
RUN mkdir -p /app/data

ENV TZ=UTC
EXPOSE 8080

ENTRYPOINT ["/app/max-bot-service"]
```

### 4.2. docker-compose.yml

Пример `docker-compose.yml` для одного сервиса:

```yaml
version: "3.8"

services:
  max-bot-service:
    image: max-bot-service:local
    container_name: max-bot-service
    restart: always
    env_file:
      - .env
    environment:
      - BOT_TOKEN=${BOT_TOKEN:?}
      - WEBHOOK_SECRET=${WEBHOOK_SECRET:?}
      - WEBHOOK_URL=${WEBHOOK_URL:?}
    volumes:
      - ./data:/data # SQLite-хранилище контактов
    ports:
      - "8080:8080" # HTTP API и /webhook
```

Сборка и запуск:

```bash
docker build -t max-bot-service:local .
docker compose up -d
```

---

## 5. HTTP API сервиса

### 5.1. Получить ссылку на бота

```http
GET /
```

Отдаёт редирект/ссылку на бота в MAX (по нику бота). [file:33]

### 5.2. Отправить уведомление

```http
POST /send-message
Content-Type: application/json
```

Тело запроса:

```json
{
  "chat": "123456789", // chatId или внутренний идентификатор
  "thread": 0, // зарезервировано, можно всегда 0
  "text": "Текст уведомления",
  "private": false // если true — ищет контакт по chat и шлёт в личку
}
```

- При `private: true` сервис ищет телефон в таблице `contacts` и отправляет сообщение напрямую пользователю. [file:68]

### 5.3. Webhook от MAX

```http
POST /webhook
X-Webhook-Secret: {WEBHOOK_SECRET из .env}
Content-Type: application/json
```

Пример тела:

```json
{
  "type": "message_created",
  "payload": {
    "message": {
      "id": "123",
      "text": "/start"
    },
    "chat": {
      "id": "987654321",
      "type": "private"
    }
  }
}
```

Сервис:

- проверяет `X-Webhook-Secret`;
- обрабатывает `message_created`, строит меню/сохраняет контакт/удаляет контакт;
- отвечает `200 OK` без тела. [file:67]

---

## 6. Отличия от Telegram-версии

- **Long Polling → Webhook**: вместо long polling используется только webhook `/webhook`. [file:111]
- **Reply Keyboard → Inline Keyboard**: меню и кнопки сделаны через inline‑клавиатуру:
  - типы кнопок `message` и `request_contact`.
- **SDK**: вместо Telegram SDK — HTTP‑клиент к `https://platform-api.max.ru/messages` и другим endpoint’ам.
- **Авторизация**: токен бота передаётся в заголовке `Authorization: <BOT_TOKEN>`.
- **Форматы событий**: обрабатывается `message_created` (при необходимости можно добавить `message_callback`, `bot_started`).

---

## 7. Минимальные шаги для запуска в MAX

1. Зарегистрировать бота на платформе MAX и получить `BOT_TOKEN`. [file:4]
2. Поднять сервис (локально или через Docker) по HTTPS‑адресу, доступному из интернета.
3. Указать `WEBHOOK_URL` и `WEBHOOK_SECRET` в `.env` и перезапустить контейнер.
4. В кабинете MAX подписать webhook:
   - URL = `WEBHOOK_URL`
   - secret = `WEBHOOK_SECRET`
5. Написать боту `/start` — должно прийти главное меню с кнопками.

---

## 8. Разработка

Для локальной разработки без Docker:

```bash
export BOT_TOKEN=...
export WEBHOOK_SECRET=dev_secret
export WEBHOOK_URL=http://localhost:8080/webhook

go run ./cmd
```

Webhook можно подать через ngrok или локальный HTTP‑тест из кабинета MAX.

```

```

---

## 9. Технический долг / TODO

- Заменить `go-sqlite3` на SQLite‑драйвер без cgo (например, modernc.org/sqlite) и убрать предупреждения `Binary was compiled with 'CGO_ENABLED=0'`.
- Перепроверить структуру ответа `/me` в API MAX и при необходимости скорректировать заполнение `Model.ID` и `Model.Name` в методе `FillInfo`.
- Добавить обработку событий `message_callback` и `bot_started` (сейчас обрабатывается только `message_created`).
- Добавить юнит‑тесты для:
  - парсинга webhook‑запросов (`message_created`),
  - сохранения/удаления контакта в SQLite,
  - отправки уведомлений через `/send-message`.
- Добавить rate limiting на HTTP‑endpoint `/send-message`, чтобы защититься от чрезмерной нагрузки со стороны внешних систем.
- Документировать пример nginx‑конфигурации для проксирования HTTPS → `max-bot-service` (если используется в продакшене).
