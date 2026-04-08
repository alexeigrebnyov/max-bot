# План изучения Go для Java-разработчика

> **Студент:** Grebnev_A  
> **Опыт:** Java developer, начинающий в Go  
> **Проект:** MAX Bot (Telegram-подобный бот на Go)  
> **Цель:** Подготовка к собеседованию по Go  
> **Старт:** 2026-04-08

---

## 🎯 Философия обучения

Вместо классического пути "от Hello World к продакшену" мы идём **от работающего проекта в разные стороны**:
- Разбираем код, который уже написан
- Понимаем паттерны и идиомы Go
- Сравниваем с Java подходами
- Углубляемся в теорию по мере необходимости
- Документируем прогресс и инсайты

---

## 📊 Структура плана

### Уровень 1: Основы (1-2 недели)
Понимание базового синтаксиса через призму существующего кода

### Уровень 2: Средний (2-3 недели)
Конкурентность, интерфейсы, обработка ошибок

### Уровень 3: Продвинутый (2-3 недели)
Производительность, тестирование, best practices

### Уровень 4: Подготовка к собеседованию (1-2 недели)
Типовые вопросы, задачи, code review

---

## 📚 Уровень 1: Основы Go через MAX Bot

### Модуль 1.1: Типы данных и структуры (2-3 дня)

#### Что изучаем:
- Базовые типы (int, string, bool)
- Структуры (struct) vs Java классы
- Указатели vs ссылки в Java
- Слайсы vs ArrayList
- Мапы vs HashMap

#### Практика в проекте:
```go
// internal/storage/tables/contacts.go
type Contact struct {
    UserID     int64   // примитивный тип
    ChatID     int64
    Phone      string  // строки в Go
    Name       string
    Authorized bool    // булевы значения
}
```

**Задания:**
1. ✅ Изучить структуру `Contact` - какие типы используются?
2. ✅ Сравнить со структурой `Message` в `internal/bot/model.go`
3. ✅ Понять разницу между `*Contact` и `Contact`
4. ✅ Найти все места где используются слайсы `[]Contact`

**Вопросы для самопроверки:**
- Чем отличается `var x int` от `x := 0`?
- Когда нужно использовать указатель `*Contact`?
- Как работает `make()` для слайсов и мап?

**Ресурсы:**
- [A Tour of Go - Basics](https://go.dev/tour/basics/1)
- [Go by Example - Structs](https://gobyexample.com/structs)

---

### Модуль 1.2: Функции и методы (2-3 дня)

#### Что изучаем:
- Функции vs методы
- Receiver (value vs pointer)
- Множественные возвращаемые значения
- Defer, panic, recover

#### Практика в проекте:
```go
// internal/storage/tables/contacts.go

// Метод с pointer receiver
func (table *Contacts) Save(contact *Contact) (bool, error) {
    // Множественное возвращаемое значение
    // ...
}

// Метод с value receiver (если бы был)
func (c Contact) String() string {
    return c.Name
}
```

**Задания:**
1. ✅ Найти все методы с pointer receiver в `contacts.go`
2. ✅ Найти все функции, возвращающие `(result, error)`
3. ✅ Изучить использование `defer` в `internal/storage/tables/contacts.go:146`
4. ✅ Написать свой метод `IsAuthorized()` для `Contact`

**Сравнение с Java:**
```java
// Java
public boolean save(Contact contact) throws Exception {
    // один return, исключения
}

// Go
func (table *Contacts) Save(contact *Contact) (bool, error) {
    // два return, явная обработка ошибок
}
```

**Вопросы для самопроверки:**
- Когда использовать value receiver, а когда pointer?
- Почему в Go нет исключений?
- Как работает `defer` и когда он выполняется?

---

### Модуль 1.3: Пакеты и импорты (1-2 дня)

#### Что изучаем:
- Структура проекта (internal, cmd, pkg)
- Видимость (exported vs unexported)
- Импорты и зависимости
- go.mod и go.sum

#### Практика в проекте:
```go
// internal/bot/service.go
package bot  // имя пакета

import (
    "context"  // стандартная библиотека
    "log"
    "max-bot-service/internal/storage"  // внутренний пакет
    "max-bot-service/internal/storage/tables"
)

// Exported (публичный)
type Service struct { ... }

// unexported (приватный)
func normalizePhone(phone string) string { ... }
```

**Задания:**
1. ✅ Изучить структуру директорий проекта
2. ✅ Понять почему `Service` экспортируется, а `normalizePhone` нет
3. ✅ Найти все импорты в `cmd/max-bot/main.go`
4. ✅ Изучить `go.mod` - какие зависимости используются?

**Сравнение с Java:**
- `package` в Go ≈ `package` в Java
- Exported (заглавная буква) ≈ `public` в Java
- unexported (строчная буква) ≈ `private/package-private` в Java
- `internal/` директория - специальное правило Go

---

### Модуль 1.4: Обработка ошибок (2-3 дня)

#### Что изучаем:
- Паттерн `if err != nil`
- Создание ошибок (`errors.New`, `fmt.Errorf`)
- Обёртывание ошибок (`%w`)
- `errors.Is` и `errors.As`

#### Практика в проекте:
```go
// internal/storage/tables/contacts.go

func (table *Contacts) Find(value string) (*Contact, error) {
    // ...
    err := row.Scan(&contact.UserID, ...)
    if err == nil {
        return &contact, nil
    } else if !errors.Is(err, sql.ErrNoRows) {
        return nil, err  // пробрасываем ошибку
    }
    return nil, nil  // не найдено - не ошибка
}
```

**Задания:**
1. ✅ Найти все места с `if err != nil` в проекте
2. ✅ Изучить как создаются ошибки в `internal/bot/service.go`
3. ✅ Понять разницу между `return nil, err` и `return nil, fmt.Errorf("context: %w", err)`
4. ✅ Написать функцию с правильной обработкой ошибок

**Сравнение с Java:**
```java
// Java - исключения
try {
    contact = findContact(id);
} catch (SQLException e) {
    throw new RuntimeException("Failed to find", e);
}

// Go - явная обработка
contact, err := table.Find(id)
if err != nil {
    return fmt.Errorf("failed to find: %w", err)
}
```

**Вопросы для самопроверки:**
- Почему в Go нет try-catch?
- Когда использовать `%w` в `fmt.Errorf`?
- Что такое sentinel errors?

---

## 📚 Уровень 2: Средний уровень

### Модуль 2.1: Интерфейсы (3-4 дня)

#### Что изучаем:
- Неявная реализация интерфейсов
- Пустой интерфейс `interface{}`
- Type assertions и type switches
- Композиция интерфейсов

#### Практика в проекте:
```go
// internal/bot/service.go

type BotClient interface {
    SendMessage(ctx context.Context, chatID string, replyTo int64, text string, silent bool) error
    SendMessageWithKeyboard(ctx context.Context, chatID string, text string, kb keyboard, silent bool) error
    // ...
}

// Model неявно реализует BotClient
type Model struct { ... }

func (m *Model) SendMessage(...) error { ... }
```

**Задания:**
1. ✅ Найти все интерфейсы в проекте
2. ✅ Понять как `Model` реализует `BotClient`
3. ✅ Изучить использование `interface{}` в `internal/api/handlers/message_status.go:25`
4. ✅ Создать свой интерфейс `ContactRepository`

**Сравнение с Java:**
```java
// Java - явная реализация
public class Model implements BotClient {
    @Override
    public void sendMessage(...) { }
}

// Go - неявная реализация
type Model struct { }
func (m *Model) SendMessage(...) error { }
// Model автоматически реализует BotClient!
```

**Вопросы для самопроверки:**
- Как проверить реализует ли тип интерфейс?
- Что такое "accept interfaces, return structs"?
- Когда использовать `interface{}`?

---

### Модуль 2.2: Конкурентность - Горутины и каналы (5-7 дней)

#### Что изучаем:
- Горутины vs потоки в Java
- Каналы (channels)
- Select statement
- Sync пакет (Mutex, WaitGroup, Once)
- Context для отмены

#### Практика в проекте:
```go
// internal/bot/service.go

type Service struct {
    NewMessages chan *Message  // буферизованный канал
    NewContacts chan *tables.Contact
}

// Запуск горутины
go srv.runPollLoop(ctx)

// Чтение из канала
case msg := <-srv.NewMessages:
    broker.broadcast(data)
```

**Задания:**
1. ✅ Найти все `go` ключевые слова (запуск горутин)
2. ✅ Изучить каналы `NewMessages` и `NewContacts`
3. ✅ Понять как работает `select` в `internal/api/handlers/evnts.go`
4. ✅ Изучить использование `context.Context` для отмены
5. ✅ Найти все `sync.Mutex` в проекте

**Реальный пример из проекта:**
```go
// internal/api/handlers/evnts.go

func (broker *EventBroker) broadcastMessagesLoop() {
    for {
        select {
        case msg := <-broker.newMessages:
            // Получили сообщение из канала
            data, _ := json.Marshal(...)
            broker.broadcast(data)
        }
    }
}
```

**Сравнение с Java:**
```java
// Java
ExecutorService executor = Executors.newCachedThreadPool();
executor.submit(() -> {
    // код в отдельном потоке
});

BlockingQueue<Message> queue = new LinkedBlockingQueue<>();

// Go
go func() {
    // код в горутине
}()

messages := make(chan *Message, 100)
```

**Вопросы для самопроверки:**
- Чем горутина отличается от потока?
- Когда использовать буферизованный канал?
- Как избежать deadlock?
- Зачем нужен `context.Context`?

---

### Модуль 2.3: HTTP и REST API (3-4 дня)

#### Что изучаем:
- `net/http` пакет
- Handlers и ServeMux
- Middleware pattern
- JSON encoding/decoding
- Context в HTTP

#### Практика в проекте:
```go
// internal/api/service.go

func (srv *Service) buildMux() http.Handler {
    mux := http.NewServeMux()
    
    // Регистрация handlers
    mux.Handle("/contacts", &handlers.ContactsHandler{...})
    mux.Handle("/send-message", rateLimit(..., &handlers.SendMessageHandler{...}))
    
    return mux
}

// internal/api/handlers/add-contact.go
func (h *AddContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Декодирование JSON
    var req addContactRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // Ответ
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
```

**Задания:**
1. ✅ Изучить все handlers в `internal/api/handlers/`
2. ✅ Понять как работает middleware `rateLimit`
3. ✅ Написать свой простой handler
4. ✅ Изучить как работает `http.ResponseWriter`

**Сравнение с Java (Spring Boot):**
```java
// Java Spring
@RestController
@RequestMapping("/api")
public class ContactController {
    @PostMapping("/contacts")
    public ResponseEntity<Contact> addContact(@RequestBody ContactRequest req) {
        // ...
    }
}

// Go
type AddContactHandler struct { }
func (h *AddContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // более низкоуровневый подход
}
```

---

### Модуль 2.4: Работа с БД (3-4 дня)

#### Что изучаем:
- `database/sql` пакет
- Prepared statements
- Transactions
- Connection pooling
- SQLite специфика

#### Практика в проекте:
```go
// internal/storage/service.go

func NewService(dbPath string) (*Service, error) {
    db, err := sql.Open("sqlite3", dbPath)
    
    // Connection pool
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    
    // WAL mode для конкурентности
    db.Exec("PRAGMA journal_mode=WAL")
    db.Exec("PRAGMA busy_timeout=5000")
}

// internal/storage/tables/contacts.go

func (table *Contacts) Save(contact *Contact) (bool, error) {
    // Prepared statement
    res, err := table.database.Exec(
        "UPDATE contacts SET userID = ?, chatID = ? WHERE phone = ?",
        contact.UserID, contact.ChatID, phone,
    )
    
    // Проверка результата
    count, _ := res.RowsAffected()
}
```

**Задания:**
1. ✅ Изучить инициализацию БД в `internal/storage/service.go`
2. ✅ Понять зачем WAL mode и busy_timeout
3. ✅ Найти все транзакции в проекте
4. ✅ Изучить как работает `sql.ErrNoRows`

**Проблемы которые были решены:**
- **SQLITE_BUSY** - решено через WAL mode и busy_timeout
- **Connection pool** - MaxOpenConns=1 для SQLite

---

## 📚 Уровень 3: Продвинутый

### Модуль 3.1: Тестирование (4-5 дней)

#### Что изучаем:
- `testing` пакет
- Table-driven tests
- Mocking и интерфейсы
- Benchmarks
- Coverage

#### Практика:
```go
// internal/bot/service_test.go (создадим)

func TestNormalizePhone(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"Russian mobile", "89001234567", "79001234567"},
        {"With plus", "+79001234567", "79001234567"},
        {"With spaces", "8 900 123 45 67", "79001234567"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := normalizePhone(tt.input)
            if result != tt.expected {
                t.Errorf("got %s, want %s", result, tt.expected)
            }
        })
    }
}
```

**Задания:**
1. ✅ Написать тесты для `normalizePhone`
2. ✅ Написать тесты для `Contact.Save`
3. ✅ Создать mock для `BotClient`
4. ✅ Написать benchmark для критичных функций

---

### Модуль 3.2: Производительность и оптимизация (3-4 дня)

#### Что изучаем:
- Профилирование (pprof)
- Memory allocations
- Escape analysis
- Оптимизация строк и слайсов
- Sync.Pool

#### Практика в проекте:
```go
// Проблемы которые были:
// 1. Периодические запросы контактов каждые 5 секунд
//    Решение: EventSource для real-time обновлений

// 2. Фильтрация непрочитанных на клиенте
//    Решение: API /unread-messages с фильтрацией в БД

// 3. Хранение прочитанных сообщений
//    Решение: Удаление вместо обновления статуса
```

**Задания:**
1. ✅ Запустить pprof на проекте
2. ✅ Найти места с большими аллокациями
3. ✅ Оптимизировать критичные участки
4. ✅ Измерить улучшения

---

### Модуль 3.3: Best Practices и идиомы Go (3-4 дня)

#### Что изучаем:
- Effective Go
- Code Review Comments
- Именование
- Организация кода
- Обработка ошибок (продвинутая)

#### Примеры из проекта:
```go
// ✅ Хорошо: короткие имена в локальном scope
func (table *Contacts) Find(value string) (*Contact, error) {
    row := table.database.QueryRow(...)
    var c Contact  // короткое имя для локальной переменной
    err := row.Scan(&c.UserID, ...)
}

// ✅ Хорошо: guard clauses
if err != nil {
    return nil, err
}
// основная логика

// ❌ Плохо: вложенные if
if err == nil {
    if contact != nil {
        // основная логика
    }
}
```

**Задания:**
1. ✅ Code review существующего кода
2. ✅ Рефакторинг по best practices
3. ✅ Изучить effective Go
4. ✅ Применить линтеры (golangci-lint)

---

## 📚 Уровень 4: Подготовка к собеседованию

### Модуль 4.1: Типовые вопросы (1 неделя)

#### Базовые вопросы:
1. Чем отличается `make` от `new`?
2. Что такое zero value?
3. Как работают defer, panic, recover?
4. Чем slice отличается от array?
5. Как работает garbage collector в Go?

#### Вопросы по конкурентности:
1. Чем горутина отличается от потока?
2. Как избежать race condition?
3. Что такое channel и когда его использовать?
4. Объясните select statement
5. Зачем нужен context.Context?

#### Вопросы по интерфейсам:
1. Как работают интерфейсы в Go?
2. Что такое пустой интерфейс?
3. Когда использовать pointer receiver?
4. Что такое type assertion?

#### Вопросы по производительности:
1. Как профилировать Go приложение?
2. Что такое escape analysis?
3. Как оптимизировать работу со строками?
4. Когда использовать sync.Pool?

---

### Модуль 4.2: Практические задачи (1 неделя)

#### Задачи на собеседованиях:

**Задача 1: Конкурентная обработка**
```go
// Обработать слайс URL конкурентно, вернуть результаты
func FetchURLs(urls []string) ([]Result, error) {
    // Ваша реализация
}
```

**Задача 2: Rate Limiter**
```go
// Реализовать rate limiter на каналах
type RateLimiter struct {
    // ...
}

func (rl *RateLimiter) Allow() bool {
    // Ваша реализация
}
```

**Задача 3: LRU Cache**
```go
// Реализовать потокобезопасный LRU cache
type LRUCache struct {
    // ...
}
```

---

### Модуль 4.3: Code Review и рефакторинг (3-4 дня)

#### Задания:
1. ✅ Code review всего проекта MAX Bot
2. ✅ Найти места для улучшения
3. ✅ Предложить рефакторинг
4. ✅ Написать документацию к коду

---

## 📈 Трекинг прогресса

### Чек-лист по модулям

#### Уровень 1: Основы
- [ ] 1.1 Типы данных и структуры
- [ ] 1.2 Функции и методы
- [ ] 1.3 Пакеты и импорты
- [ ] 1.4 Обработка ошибок

#### Уровень 2: Средний
- [ ] 2.1 Интерфейсы
- [ ] 2.2 Конкурентность
- [ ] 2.3 HTTP и REST API
- [ ] 2.4 Работа с БД

#### Уровень 3: Продвинутый
- [ ] 3.1 Тестирование
- [ ] 3.2 Производительность
- [ ] 3.3 Best Practices

#### Уровень 4: Собеседование
- [ ] 4.1 Типовые вопросы
- [ ] 4.2 Практические задачи
- [ ] 4.3 Code Review

---

## 📝 Журнал обучения

### Формат записи:
```markdown
## [Дата] - Модуль X.Y: Название

### Что изучил:
- Пункт 1
- Пункт 2

### Инсайты:
- Интересное наблюдение 1
- Сравнение с Java

### Вопросы:
- Непонятный момент 1

### Практика:
- Что сделал в коде
```

---

## 🎯 Цели по неделям

### Неделя 1-2: Основы
- Понимание базового синтаксиса
- Уверенное чтение кода проекта
- Написание простых функций

### Неделя 3-4: Средний уровень
- Понимание конкурентности
- Работа с интерфейсами
- HTTP handlers

### Неделя 5-6: Продвинутый
- Тестирование
- Оптимизация
- Best practices

### Неделя 7-8: Собеседование
- Ответы на типовые вопросы
- Решение задач
- Уверенность в коде

---

## 📚 Ресурсы

### Обязательные:
1. [A Tour of Go](https://go.dev/tour/) - интерактивный туториал
2. [Effective Go](https://go.dev/doc/effective_go) - официальный гайд
3. [Go by Example](https://gobyexample.com/) - примеры кода
4. [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Книги:
1. "The Go Programming Language" (Donovan & Kernighan)
2. "Concurrency in Go" (Katherine Cox-Buday)
3. "Learning Go" (Jon Bodner)

### Для Java разработчиков:
1. [Go for Java Programmers](https://yourbasic.org/golang/go-java-tutorial/)
2. [From Java to Go](https://www.youtube.com/watch?v=1MXIGYrMk80)

### Практика:
1. [Exercism Go Track](https://exercism.org/tracks/go)
2. [LeetCode Go](https://leetcode.com/)
3. [Go Playground](https://go.dev/play/)

---

## 🤝 Формат работы с наставником (Claude)

### Каждая сессия:
1. Обзор прогресса с прошлой сессии
2. Разбор непонятных моментов
3. Изучение нового модуля
4. Практические задания
5. Обновление журнала обучения

### Что я (Claude) буду делать:
- Объяснять концепции через примеры из проекта
- Сравнивать с Java подходами
- Давать практические задания
- Проверять код
- Готовить к собеседованию

### Что нужно от тебя:
- Регулярная практика (хотя бы 1-2 часа в день)
- Ведение журнала обучения
- Задавать вопросы
- Писать код, а не только читать

---

## 🎓 Критерии готовности к собеседованию

### Junior Go Developer:
- ✅ Понимание базового синтаксиса
- ✅ Работа с структурами и интерфейсами
- ✅ Базовая обработка ошибок
- ✅ Понимание горутин и каналов
- ✅ Работа с HTTP
- ✅ Базовое тестирование

### Middle Go Developer:
- ✅ Всё из Junior +
- ✅ Продвинутая конкурентность
- ✅ Оптимизация и профилирование
- ✅ Архитектурные паттерны
- ✅ Best practices
- ✅ Code review

---

## 📞 Следующие шаги

1. **Сегодня:** Начать с Модуля 1.1 (Типы данных)
2. **Эта неделя:** Пройти весь Уровень 1
3. **Через месяц:** Завершить Уровень 2
4. **Через 2 месяца:** Готовность к собеседованию

**Готов начать? Давай стартуем с Модуля 1.1!** 🚀
