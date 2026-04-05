---
name: max-bot project structure
description: Архитектура проекта max-bot-service и граф зависимостей кода
type: project
---

# max-bot-service — Архитектура и структура проекта

## Назначение проекта

HTTP-сервис на Go для работы с ботом в мессенджере MAX. Поддерживает два режима получения обновлений:
- **Webhook** — MAX отправляет события на публичный URL сервиса
- **Long Polling** — сервис сам опрашивает MAX API

Основные функции:
- Регистрация пользователей по номеру телефона
- Управление групповыми чатами
- Отправка уведомлений пользователям по телефону или chat_id
- Web UI для мониторинга сообщений (SSE)
- Хранение контактов и групп в SQLite

---

## Граф зависимостей модулей

```
cmd/main.go (точка входа)
    ├─> config.Load()
    ├─> storage.NewService()
    │       └─> tables.Contacts
    │       └─> tables.GroupChats
    ├─> bot.NewService(storage, config)
    │       ├─> bot.Model (API клиент MAX)
    │       ├─> bot.Service (бизнес-логика)
    │       └─> storage.Contacts
    └─> api.NewService(bot, storage, config)
            ├─> handlers.* (HTTP обработчики)
            ├─> api.Metrics
            └─> bot.Service
```

---

## Структура пакетов и ключевые файлы

### 1. **cmd/main.go** — точка входа
- Инициализирует логгер
- Создает сервисы: storage → bot → api
- Запускает все сервисы с graceful shutdown

### 2. **internal/config/config.go** — конфигурация
Загружает настройки из переменных окружения:
- `BOT_TOKEN` — токен бота MAX (обязательно)
- `WEBHOOK_URL` — URL для webhook (если пустой → Long Polling)
- `WEBHOOK_SECRET` — секрет для проверки webhook
- `MAX_API_BASE_URL` — базовый URL API MAX
- `PORT` — порт HTTP-сервера (по умолчанию 9003)
- `API_KEY` — ключ для защиты служебных эндпоинтов
- `RATE_LIMIT_PER_MINUTE` — лимит запросов (по умолчанию 60)

### 3. **internal/storage/** — работа с БД

#### storage/service.go
- Инициализирует SQLite БД в `./data/storage.db`
- Создает таблицы `contacts` и `group_chats`

#### storage/tables/contacts.go
**Таблица contacts:**
```sql
CREATE TABLE contacts (
    userID INTEGER PRIMARY KEY,
    chatID INTEGER NOT NULL,
    phone TEXT NOT NULL,
    name TEXT NOT NULL,
    emc TEXT
)
```

**Методы:**
- `Find(value)` — поиск по userID/chatID/phone
- `Save(contact)` — вставка/обновление контакта
- `UpdateByPhone(contact)` — обновление по номеру телефона
- `Delete(userID)` — удаление контакта
- `All()` — получить все контакты

**Нормализация телефона:** убирает все символы кроме цифр, заменяет 8 на 7 (для РФ)

#### storage/tables/group_chats.go
**Таблица group_chats:**
```sql
CREATE TABLE group_chats (
    chatID INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    menu_sent INTEGER DEFAULT 0
)
```

**Методы:**
- `FindByTitle(title)` — поиск по названию
- `FindByChatID(chatID)` — поиск по ID
- `Save(chat)` — вставка/обновление чата
- `SetMenuSent(chatID)` — пометить, что меню отправлено
- `All()` — получить все чаты
- `DeleteByChatID(chatID)` — удалить чат

### 4. **internal/bot/** — логика бота

#### bot/model.go — API клиент MAX
**Структура Model:**
```go
type Model struct {
    httpClient *http.Client
    apiBase    string  // https://platform-api.max.ru
    token      string  // BOT_TOKEN
    contacts   *tables.Contacts
    ID         int64   // ID бота
    Name       string  // ник бота
}
```

**Методы отправки сообщений:**
- `SendMessage(ctx, chat, thread, text, private)` — отправка по userID или phone
- `SendMessageWithKeyboard(ctx, chat, text, kb, private)` — с клавиатурой
- `SendToChatByID(ctx, chatID, text)` — отправка по chat_id
- `SendToChatByIDWithKeyboard(ctx, chatID, text, kb)` — с клавиатурой
- `GetChatMessages(ctx, chatID)` — получить историю сообщений
- `FillInfo(ctx)` — загрузить информацию о боте через `/me`

**Структуры данных:**
- `Message` — полное сообщение (Recipient, Body, Sender, Timestamp)
- `keyboard` — клавиатура с кнопками
- `keyboardButton` — кнопка (type: "message" | "request_contact")

#### bot/service.go — бизнес-логика бота
**Структура Service:**
```go
type Service struct {
    storage       *storage.Service
    BotModel      *Model
    Bot           BotClient
    webhookSecret string
    cfg           *config.Config
    NewMessages   chan *Message  // канал для SSE
}
```

**Ключевые методы:**

**Инициализация:**
- `Start(ctx)` — запускает бота:
  - Загружает информацию о боте (`FillInfo`)
  - Синхронизирует список групп (`RefreshGroupChats`)
  - Запускает Long Polling или настраивает Webhook
  - Запускает периодическую синхронизацию групп (каждые 5 минут)

**Обработка событий:**
- `WebhookHandler()` — HTTP handler для webhook
- `handleMessageCreated(ctx, raw)` — обработка входящих сообщений
- `handleMessageCallback(ctx, raw)` — обработка нажатий кнопок
- `handleBotStarted(ctx, raw)` — пользователь запустил бота
- `handleBotAdded(ctx, raw)` — бота добавили в группу

**Long Polling:**
- `runPollLoop(ctx)` — цикл опроса MAX API
- `fetchUpdates(ctx, marker)` — получение обновлений
- `unsubscribeWebhook(ctx)` — отписка от webhook

**Работа с группами:**
- `RefreshGroupChats(ctx)` — синхронизация списка групп
- `RefreshGroupChatsAndGreet(ctx)` — синхронизация + приветствие новых
- `sendBotAddedGreeting(ctx, chatID)` — приветствие в группе
- `sendGroupMenu(ctx, chatID)` — отправка меню с кнопкой "ID чата"
- `isBotAdminInChat(ctx, chatID)` — проверка прав администратора
- `getChatInfo(ctx, chatID)` — информация о чате

**Работа с контактами:**
- `saveContact(ctx, chatID, userID, phone, name, emc)` — сохранение контакта
- `deleteContact(ctx, chatID)` — удаление контакта
- `extractPhoneFromText(input)` — извлечение телефона из текста

**Меню и UI:**
- `sendMainMenuMessage(ctx, chatID)` — главное меню (запрос телефона)
- `sendNotificationSettingMessage(ctx, chatID)` — настройка уведомлений
- `sendChatIDMessage(ctx, chatID, chatID)` — показать ID чата
- `sendGroupChatsList(ctx, chatKey)` — список групп

### 5. **internal/api/** — HTTP API

#### api/service.go — HTTP сервер
**Структура Service:**
```go
type Service struct {
    Bot     *bot.Service
    Storage *storage.Service
    Cfg     *config.Config
    Metrics *Metrics
}
```

**Middleware:**
- `requireAPIKey(apiKey, next)` — проверка X-API-Key
- `rateLimit(limitPerMin, metrics, next)` — rate limiting

**Эндпоинты:**
```
GET  /                          → редирект на бота в MAX
POST /webhook                   → прием событий от MAX
POST /send-message              → отправка сообщения (rate limited)
POST /send-by-phone             → отправка по телефону (rate limited)
POST /send-to-group-by-chatid   → отправка в группу
POST /send-chat-message         → отправка + SSE broadcast
GET  /get-messages-by-chatid    → история сообщений
POST /refresh-group-chats       → обновить список групп (требует API_KEY)
GET  /group-chats               → список групп (требует API_KEY)
GET  /metrics                   → метрики (requests, errors, rate_limits)
POST /add-contact               → добавить контакт
POST /update-contact            → обновить контакт
GET  /get-contact               → получить контакт
GET  /contacts                  → список контактов
GET  /events                    → SSE stream для web UI
GET  /admin                     → web UI для мониторинга
```

#### api/handlers/ — HTTP обработчики

**handlers/send-message.go:**
- `SendMessageHandler` — отправка сообщения по chat/phone
- `SendByPhoneHandler` — отправка по телефону (ищет в contacts)
- `GetChatMessagesHandler` — получение истории сообщений

**handlers/send-to-group.go:**
- `SendToGroupByChatIdHandler` — отправка в групповой чат

**handlers/evnts.go — SSE (Server-Sent Events):**
- `EventBroker` — брокер событий для web UI
  - `broadcastLoop()` — рассылка сообщений подписчикам
  - `ServeHTTP()` — SSE endpoint с ping каждые 20 секунд
  - `BroadcastStructuredMessage(msg)` — ручная отправка события
- `SendChatMessageHandler` — отправка сообщения + broadcast в SSE

**handlers/add-contact.go:**
- `AddContactHandler` — добавление контакта
- `UpdateContactHandler` — обновление контакта по телефону

**handlers/get-contact.go:**
- `GetContactHandler` — получение контакта по userID/chatID/phone
- `ContactsHandler` — список всех контактов

**handlers/group-chats.go:**
- `GroupChatsHandler` — список групповых чатов

**handlers/refresh-group-chats.go:**
- `RefreshGroupChatsHandler` — принудительная синхронизация групп

**handlers/root.go:**
- `RootHandler` — редирект на бота в MAX

**handlers/web-ui.go:**
- `WebUIHandler` — HTML страница для мониторинга сообщений

#### api/metrics.go — метрики
```go
type Metrics struct {
    requests   int64  // общее количество запросов
    errors     int64  // количество ошибок
    rateLimits int64  // количество rate limit
}
```

---

## Граф зависимостей функций (ключевые цепочки)

### Цепочка 1: Запуск бота
```
main()
  → config.Load()
  → storage.NewService().Start()
      → tables.NewContacts(db)
      → tables.NewGroupChats(db)
  → bot.NewService(storage, config).Start()
      → bot.Model.FillInfo()  // GET /me
      → bot.Service.RefreshGroupChats()  // GET /chats
      → bot.Service.runPollLoop() [goroutine]
          → bot.Service.fetchUpdates()  // GET /updates
          → bot.Service.handleMessageCreated()
          → bot.Service.handleMessageCallback()
          → bot.Service.handleBotStarted()
  → api.NewService(bot, storage, config).Start()
      → api.Service.buildMux()
      → http.Server.ListenAndServe()
```

### Цепочка 2: Обработка входящего сообщения (Long Polling)
```
runPollLoop()
  → fetchUpdates(marker)  // GET /updates
  → handleMessageCreated(raw)
      → json.Unmarshal(raw, &messageCreatedPayload)
      → [если группа] storage.GroupChats.Save()
      → [если /start] sendMainMenuMessage()
          → bot.Model.SendMessageWithKeyboard()  // POST /messages
      → [если телефон] extractPhoneFromText()
          → saveContact()
              → storage.Contacts.Save()
              → bot.Model.SendMessageWithKeyboard()
      → [отправка в канал] NewMessages <- msg
```

### Цепочка 3: Отправка сообщения по телефону (API)
```
POST /send-by-phone
  → SendByPhoneHandler.ServeHTTP()
      → json.Decode(&SendByPhoneRequest)
      → bot.Model.SendMessage(phone, text, private=true)
          → storage.Contacts.Find(phone)
          → [нормализация] normalizePhone()
          → [отправка] POST /messages?user_id={userID}
```

### Цепочка 4: Синхронизация групповых чатов
```
RefreshGroupChatsAndGreet()
  → GET /chats (постранично с marker)
  → для каждого чата:
      → getChatInfo(chatID)  // GET /chats/{chatId}
      → [если status=active] storage.GroupChats.Save()
      → [если новый] sendBotAddedGreeting()
          → isBotAdminInChat()  // GET /chats/{chatId}/members/me
          → [если админ] sendGroupMenu()
              → bot.Model.SendToChatByID()
              → bot.Model.SendToChatByIDWithKeyboard()
              → storage.GroupChats.SetMenuSent()
  → cleanupMissingGroupChats()
      → storage.GroupChats.DeleteByChatID()
```

### Цепочка 5: SSE (Server-Sent Events) для web UI
```
GET /events
  → EventBroker.ServeHTTP()
      → [создание канала] ch := make(chan []byte, 10)
      → [регистрация клиента] clients[ch] = true
      → [цикл]:
          → [из канала] data := <-ch → fmt.Fprintf(w, "data: %s\n\n")
          → [ping] ticker.C → fmt.Fprintf(w, ": ping\n\n")
          → [закрытие] r.Context().Done()

broadcastLoop() [goroutine]
  → for msg := range NewMessages:
      → json.Marshal(msg)
      → для каждого клиента: ch <- data
```

### Цепочка 6: Обработка нажатия кнопки
```
handleMessageCallback(raw)
  → json.Unmarshal(raw, &callbackPayload)
  → [если payload="chatid:123"] sendChatIDToGroup(123)
      → bot.Model.SendToChatByID("ID этого чата:")
      → bot.Model.SendToChatByID("123")
```

---

## Быстрый поиск кода по задачам

### Нужно изменить логику регистрации пользователя?
→ `internal/bot/service.go:handleMessageCreated()` (строки 804-893)
→ `internal/bot/service.go:saveContact()` (строки 1052-1086)

### Нужно добавить новый HTTP эндпоинт?
→ `internal/api/service.go:buildMux()` (строки 94-131)
→ Создать handler в `internal/api/handlers/`

### Нужно изменить формат отправки сообщений в MAX?
→ `internal/bot/model.go:SendMessage()` (строки 165-224)
→ `internal/bot/model.go:SendMessageWithKeyboard()` (строки 227-286)

### Нужно изменить логику работы с группами?
→ `internal/bot/service.go:RefreshGroupChatsAndGreet()` (строки 1214-1322)
→ `internal/bot/service.go:sendBotAddedGreeting()` (строки 256-262)

### Нужно изменить структуру БД?
→ `internal/storage/tables/contacts.go` (схема на строках 13-23)
→ `internal/storage/tables/group_chats.go` (схема на строках 11-19)

### Нужно изменить обработку событий от MAX?
→ `internal/bot/service.go:handleMessageCreated()` (строки 804-893)
→ `internal/bot/service.go:handleBotStarted()` (строки 181-241)
→ `internal/bot/service.go:handleMessageCallback()` (строки 588-648)

### Нужно изменить web UI или SSE?
→ `internal/api/handlers/evnts.go:EventBroker` (строки 13-103)
→ `internal/api/handlers/web-ui.go`

### Нужно добавить rate limiting на новый эндпоинт?
→ `internal/api/service.go:rateLimit()` (строки 65-86)
→ Обернуть handler в `buildMux()`

---

## Ключевые структуры данных

### Message (полное сообщение)
```go
type Message struct {
    Recipient Recipient  // кому
    Timestamp int64      // когда
    Body      Body       // что
    Sender    Sender     // от кого
}
```

### Contact (контакт пользователя)
```go
type Contact struct {
    UserID int64   // ID пользователя в MAX
    ChatID int64   // ID диалога
    Phone  string  // нормализованный телефон
    Name   string  // имя
    EMC    string  // дополнительное поле
}
```

### GroupChat (групповой чат)
```go
type GroupChat struct {
    ChatID   int64   // ID чата
    Title    string  // название
    MenuSent bool    // отправлено ли меню
}
```

---

**Why:** Этот документ позволяет быстро найти нужный код для анализа и правки без необходимости читать целые файлы.

**How to apply:** При получении задачи на изменение функционала, сначала смотри в раздел "Быстрый поиск кода по задачам", затем изучай граф зависимостей для понимания контекста.
