# max.botservice — бот-сервис для MAX

HTTP‑сервис на Go для работы с ботом в мессенджере **MAX**.  
Умеет получать события от MAX через **Webhook** или **Long Polling**, вести диалоги с пользователем (меню, настройка уведомлений, сохранение телефона в SQLite) и предоставляет HTTP‑endpoint для отправки уведомлений из внешних систем.

---

## 1. Что делает сервис

- Принимает сообщения от пользователей через **Webhook** (`POST /webhook`) или через **Long Polling** (GET `/updates` к API MAX). Режим задаётся переменной `WEBHOOK_URL`: если она **пустая** — используется Long Polling; если указана — Webhook.
- В **личном диалоге** показывает главное меню с inline‑кнопками (соответствуют константам в `internal/bot/service.go`):
  - «ID этого чата 🆔»
  - «Список групп 💬» — список групповых чатов бота и их ChatID
  - «Настроить уведомления 🔔»
  - «Искл. свой # телефона 📵»
  - «В главное меню ⬅️»  
  В экране уведомлений также доступна кнопка «Отпр. свой # телефона 📞».
- **Реагирование на START:** в личном диалоге при нажатии кнопки «НАЧАТЬ» или при сообщении **«/start»** / **«start»** бот показывает главное меню. В группе (если бот администратор) при сообщении **«start»**, **«/start»** или, например, **«@бот start»** бот присылает в группу приветствие и меню с кнопкой «ID этого чата 🆔».
- Сохраняет телефон пользователя в SQLite, чтобы внешний сервер мог отправлять ему уведомления по chatId/phone.
- Даёт внешний HTTP‑endpoint `/send-message` для отправки уведомлений в MAX‑чат.

### 1.1. Поведение в групповом чате

- **Меню с кнопкой «ID этого чата 🆔»** в группе показывается **только если бот является администратором** (или владельцем) группы. Проверка выполняется через GET `/chats/{chatId}/members/me` (поле `is_admin` / `is_owner`).
- **При добавлении бота в группу не как администратора** — приветствие и меню **не отправляются**.
- **Если бота позже назначают администратором** — при следующем обновлении списка чатов (по таймеру или при событии `bot_added`) сервис обнаруживает, что бот стал админом и меню ещё не отправлялось, и тогда отправляет приветствие и сообщение с кнопкой «ID этого чата 🆔» в эту группу. Флаг «меню отправлено» хранится в БД (`group_chats.menu_sent`), чтобы не дублировать отправку.
- **Запрос меню в группе по тексту:** если в группе, где бот уже администратор, кто‑то пишет **«start»**, **«/start»** или, например, **«@бот start»**, бот присылает в группу приветствие и меню с кнопкой «ID этого чата 🆔».
- Нажатие кнопки «ID этого чата 🆔» в группе отправляет в чат два сообщения: подпись «ID этого чата:» и значение ChatID.

---

## 2. Переменные окружения

Настраиваются через `.env` и/или `docker-compose.yml`.

Заполнить реальные значения (минимум — `BOT_TOKEN`):

```bash
cp .env.example .env
```

| Переменная | Обязательность | Описание |
|------------|----------------|----------|
| `BOT_TOKEN` | обязательно | Токен бота из кабинета MAX (Чат-боты → Интеграция). |
| `WEBHOOK_URL` | опционально | Публичный HTTPS-URL для Webhook (например `https://your-domain.com/webhook`). **Если не задан или пустой — сервис работает в режиме Long Polling** (сам опрашивает GET `/updates`). |
| `WEBHOOK_SECRET` | для Webhook | Секрет, совпадающий с указанным при подписке Webhook в кабинете MAX. |
| `MAX_API_BASE_URL` | опционально | Базовый URL API MAX, по умолчанию `https://platform-api.max.ru`. |

**Webhook:** при подписке на обновления (POST `/subscriptions` или настройка в кабинете MAX) обязательно укажите в **update_types** тип **`message_callback`** — иначе нажатия на кнопки (например «ID этого чата 🆔» в группе) не будут приходить на сервис. Минимум: `message_created`, `message_callback`, `bot_started`, `bot_added` ([документация MAX](https://dev.max.ru/docs-api/methods/POST/subscriptions)).

**События из группы:** по документации MAX события из группового чата или канала приходят боту **только если бот назначен администратором** этой группы/канала. Если нажатие кнопки в группе не даёт логов — проверьте: 1) бот администратор группы; 2) при Webhook в подписке указаны `message_created` и `message_callback`; 3) в логах при нажатии появляется строка `webhook: POST received` (если её нет — запрос от MAX не доходит до сервера).

**Long Polling:** при пустом `WEBHOOK_URL` при старте бот сам снимает подписки на Webhook через API (GET/DELETE `/subscriptions`), чтобы получать события через GET `/updates`. Ручная отписка в кабинете MAX не обязательна.

---

## 3. Запуск локально (без Docker)

Требуется Go 1.24+.

```bash
go mod tidy
go run ./cmd
```

По умолчанию сервис слушает `:8080`.

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

Отдаёт редирект/ссылку на бота в MAX (по нику бота).

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

- При `private: true` сервис ищет телефон в таблице `contacts` и отправляет сообщение напрямую пользователю.

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
- обрабатывает `message_created`, `message_callback` (нажатия кнопок), `bot_started`, `bot_added`;
- для нажатий кнопок в группах при подписке на Webhook в **update_types** должен быть указан **message_callback** ([POST /subscriptions](https://dev.max.ru/docs-api/methods/POST/subscriptions));
- отвечает `200 OK` без тела.

---

## 6. Отличия от Telegram-версии

- **Webhook или Long Polling**: поддерживаются оба режима. Если задан `WEBHOOK_URL` — используется Webhook; если пустой — Long Polling (GET `/updates`).
- **Reply Keyboard → Inline Keyboard**: меню и кнопки сделаны через inline‑клавиатуру:
  - типы кнопок `message` и `request_contact`.
- **SDK**: вместо Telegram SDK — HTTP‑клиент к `https://platform-api.max.ru/messages` и другим endpoint’ам.
- **Авторизация**: токен бота передаётся в заголовке `Authorization: <BOT_TOKEN>`.
- **Форматы событий**: обрабатываются `message_created`, `message_callback`, `bot_started`, `bot_added`.

---

## 7. Минимальные шаги для запуска в MAX

### Вариант A: Long Polling (проще для разработки и production без своего HTTPS)

1. Зарегистрировать бота на платформе MAX и получить `BOT_TOKEN`.
2. В `.env` задать только `BOT_TOKEN`; `WEBHOOK_URL` не задавать (или оставить пустым).
3. Запустить сервис. Бот при старте сам снимет подписки на Webhook через API. В логах: `updates: using Long Polling (GET /updates)` (при наличии подписок — `unsubscribeWebhook: removed subscription url=...`).
4. Написать боту `/start` — должно прийти главное меню с кнопками.

### Вариант B: Webhook

1. Зарегистрировать бота и получить `BOT_TOKEN`.
2. Поднять сервис по HTTPS‑адресу, доступному из интернета.
3. Указать `WEBHOOK_URL` и `WEBHOOK_SECRET` в `.env` и перезапустить.
4. В кабинете MAX подписать Webhook: URL = `WEBHOOK_URL`, secret = `WEBHOOK_SECRET`.
5. Написать боту `/start` — должно прийти главное меню.

---

## 8. Разработка

**Локально с Long Polling** (без ngrok и без настройки Webhook в кабинете):

```bash
export BOT_TOKEN=...
# WEBHOOK_URL не задаём — будет Long Polling
go run ./cmd
```

В кабинете MAX при этом не должна быть подписка на Webhook.

**Локально с Webhook** (например через ngrok):

```bash
export BOT_TOKEN=...
export WEBHOOK_SECRET=dev_secret
export WEBHOOK_URL=https://your-ngrok-url.ngrok.io/webhook
go run ./cmd
```

Webhook можно подать через ngrok или локальный HTTPS‑тест из кабинета MAX.

## 9. Тестирование

Тесты Go запускаются стандартной командой в корне модуля.

В твоём случае:

```bash
cd E:\MaxCode\max.botservice
go test ./...
```

Если хочешь гонять только тесты пакета `internal/bot`:

```bash
go test ./internal/bot
```

Чтобы видеть подробный вывод по каждому тесту:

```bash
go test -v ./internal/bot
```

## 10. Технический долг / TODO

- Добавить обработку событий `message_callback` (сейчас обрабатывается только `message_created` и `bot_started`).
- Добавить rate limiting на HTTP‑endpoint `/send-message`, чтобы защититься от чрезмерной нагрузки со стороны внешних систем.

## 11. Компилчяция

В текущем проекте max.botservice сделать:
docker build -t docker.dev.ask-glonass.ru/max-bot-service:local .

И далее в max.botservice.deploy:
docker-compose down
docker-compose up -d
docker logs -f botserver

## 12. Обновление и просмотр списка снаружи

Ручной рефреш (если нужно):

```bash
curl -X POST https://maksik.ask-gps.ru/refresh-group-chats
```

Просмотр кеша снаружи (.NET, браузер, curl):

```bash
curl https://maksik.ask-gps.ru/group-chats
```

Ответ:

json
[
  { "ChatID": 174132016, "Title": "СГТ-Тревоги-Скорость" },
  { "ChatID": 200500300, "Title": "Тест-группа" }
]
