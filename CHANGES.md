# Изменения: Добавление поддержки avatar_url и emchash

## Дата: 2026-04-05

## Описание
Добавлена поддержка сохранения URL аватара пользователя и emchash для регистрации через payload в команде `/start`.

## Изменённые файлы

### 1. `internal/storage/tables/contacts.go`
**Изменения в БД:**
- Добавлены поля `avatar_url TEXT` и `emchash TEXT` в таблицу `contacts`
- Добавлен индекс `contacts_emchash` для быстрого поиска по emchash
- Добавлена миграция для существующих БД (ALTER TABLE)

**Изменения в структуре Contact:**
```go
type Contact struct {
    UserID    int64
    ChatID    int64
    Phone     string
    Name      string
    EMC       string
    AvatarURL string  // новое поле
    EMCHash   string  // новое поле
}
```

**Новые методы:**
- `FindByEMCHash(emchash string) (*Contact, error)` — поиск контакта по emchash

**Обновлённые методы:**
- `Find()` — теперь возвращает avatar_url и emchash
- `Save()` — сохраняет avatar_url и emchash
- `UpdateByPhone()` — обновляет avatar_url и emchash
- `All()` — возвращает avatar_url и emchash

---

### 2. `internal/bot/service.go`
**Изменения в структуре messageCreatedPayload:**
```go
type messageCreatedPayload struct {
    // ...
    Sender struct {
        UserID int64  `json:"user_id"`
        Name   string `json:"name"`
        Avatar string `json:"avatar_url"`  // новое поле
    } `json:"sender"`
    
    Payload string `json:"payload"`  // новое поле на верхнем уровне
}
```

**Изменения в логике `/start`:**
- Если `Payload` содержит `emchash_{value}` → сразу сохраняется контакт по emchash
- Если `Payload` пустой → текущая логика (запрос телефона через кнопку)

**Новая функция:**
```go
func (srv *Service) saveContactByEMCHash(ctx, chatID, userID, emchash, name, avatar)
```
- Проверяет наличие контакта с таким emchash
- Если есть — обновляет (userID, chatID, name, avatar)
- Если нет — создаёт новый контакт без телефона

**Обновлённая функция:**
```go
func (srv *Service) saveContact(ctx, chatID, userID, phone, name, emc, avatar)
```
- Добавлен параметр `avatar` для сохранения URL аватара
- При сохранении по телефону `emchash` остаётся пустым

---

### 3. `internal/api/handlers/add-contact.go`
**Изменения в структуре запроса:**
```go
type addContactRequest struct {
    UserID    int64  `json:"user_id"`
    ChatID    int64  `json:"chat_id"`
    Phone     string `json:"phone"`
    Name      string `json:"name"`
    EMC       string `json:"emc"`
    AvatarURL string `json:"avatar_url"`  // новое поле
    EMCHash   string `json:"emchash"`     // новое поле
}
```

**Обновлённые handlers:**
- `AddContactHandler` — теперь принимает и сохраняет avatar_url и emchash
- `UpdateContactHandler` — обновляет avatar_url и emchash

---

### 4. `internal/bot/service_test.go`
**Обновлён тест:**
- Вызов `saveContact()` обновлён с новыми параметрами (name, emc, avatar)

---

## Сценарии использования

### Сценарий 1: Регистрация через emchash (новый)
1. Пользователь переходит по ссылке с payload: `https://max.ru/bot?payload=emchash_abc123`
2. MAX отправляет событие `bot_started` или `message_created` с `payload="emchash_abc123"`
3. Бот извлекает emchash и сохраняет контакт:
   ```
   userID: 12345
   chatID: 67890
   phone: "" (пока неизвестен)
   name: "Иван Иванов"
   emchash: "abc123"
   avatar_url: "https://..."
   ```
4. Пользователь получает сообщение: "Контакт успешно создан! Теперь вы можете получать уведомления."

### Сценарий 2: Регистрация через телефон (существующий)
1. Пользователь отправляет `/start` без payload
2. Бот запрашивает телефон (кнопка или текст)
3. Пользователь отправляет телефон
4. Бот сохраняет контакт с телефоном и avatar_url (emchash пустой)

### Сценарий 3: Обновление контакта через API
```bash
POST /add-contact
{
  "user_id": 12345,
  "chat_id": 67890,
  "phone": "79039076399",
  "name": "Иван Иванов",
  "emc": "EMC001",
  "avatar_url": "https://cdn.max.ru/avatars/12345.jpg",
  "emchash": "abc123"
}
```

---

## Миграция БД

При первом запуске обновлённого кода автоматически выполняются:
```sql
ALTER TABLE contacts ADD COLUMN avatar_url TEXT;
ALTER TABLE contacts ADD COLUMN emchash TEXT;
CREATE INDEX IF NOT EXISTS contacts_emchash ON contacts(emchash);
```

Ошибки "duplicate column name" игнорируются (для повторных запусков).

---

## Обратная совместимость

✅ Все изменения обратно совместимы:
- Существующие контакты продолжают работать (новые поля NULL)
- API принимает запросы без новых полей
- Старая логика `/start` (без payload) работает как прежде

---

## Тестирование

Для проверки изменений:
1. Запустить тесты: `go test ./...`
2. Проверить миграцию БД: удалить `./data/storage.db` и запустить сервис
3. Протестировать `/start` с payload: `emchash_test123`
4. Протестировать `/start` без payload (обычная регистрация)
5. Проверить API: `POST /add-contact` с новыми полями

---

## Следующие шаги

- [ ] Добавить валидацию формата emchash
- [ ] Добавить endpoint для получения контакта по emchash: `GET /contact-by-emchash?emchash=...`
- [ ] Обновить документацию API
- [ ] Добавить логирование для отладки payload
