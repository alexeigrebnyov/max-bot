# Деплой MAX Bot

## Подготовка сервера

### 1. Установите Docker (если еще не установлен)

```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
# Перелогиньтесь для применения изменений
```

### 2. Клонируйте репозиторий

```bash
cd /opt  # или любая другая директория (можно использовать /home/user, ~/projects и т.д.)
git clone <URL_вашего_репозитория> max-bot
cd max-bot
```

**Важно:** Репозиторий можно клонировать в любую директорию, где у вас есть права на запись.

### 3. Создайте файл .env

```bash
nano .env
```

Содержимое:
```
BOT_TOKEN=ваш_токен_от_MAX
PORT=9003
LOG_LEVEL=info
LOG_FORMAT=text
```

Сохраните (Ctrl+O, Enter, Ctrl+X)

### 4. Сделайте скрипт исполняемым

```bash
chmod +x deploy.sh
```

**Важно:** Файл `deploy.sh` уже находится в репозитории, просто нужно дать ему права на выполнение.

---

## Важная информация о данных

### База данных сохраняется автоматически

- База данных `storage.db` хранится в директории `./data/` **на сервере**, а не внутри Docker контейнера
- При каждом деплое (пересборке контейнера) **все контакты сохраняются**
- Вам **не нужно** вручную копировать БД — скрипт монтирует `./data/` в контейнер
- Даже при удалении контейнера (`docker rm max-bot`) база остается на месте

### Файлы, которые нужно хранить на сервере

1. `.env` — токен бота и настройки (не хранится в git)
2. `./data/storage.db` — база контактов (создается автоматически при первом запуске)
3. `./data/storage.db.backup-*` — бэкапы БД (создаются при деплое с параметром `yes`)

---

## Деплой

### Вариант 1: Деплой из ветки dev (тестирование)

```bash
./deploy.sh dev yes
```

- `dev` - ветка для деплоя
- `yes` - создать бэкап БД перед деплоем

### Вариант 2: Деплой из main (продакшн)

```bash
./deploy.sh main yes
```

### Вариант 3: Быстрый деплой без бэкапа

```bash
./deploy.sh main
```

---

## Управление контейнером

### Просмотр логов

```bash
# Последние 100 строк
docker logs --tail 100 max-bot

# В реальном времени
docker logs -f max-bot
```

### Остановка

```bash
docker stop max-bot
```

### Запуск

```bash
docker start max-bot
```

### Перезапуск

```bash
docker restart max-bot
```

### Удаление контейнера

```bash
docker stop max-bot
docker rm max-bot
```

---

## Проверка работы

### 1. Проверьте статус контейнера

```bash
docker ps | grep max-bot
```

Должно быть `Up X minutes/hours`

### 2. Проверьте логи

```bash
docker logs --tail 50 max-bot
```

Должно быть:
```
level=INFO msg="Bot info: ID=..., Nick=..."
level=INFO msg="http server starting" port=9003
```

### 3. Проверьте API

```bash
curl http://localhost:9003/
```

Должен вернуть JSON с информацией о боте.

### 4. Проверьте веб-интерфейс

Откройте в браузере: `http://your-server-ip:9003/admin`

---

## Бэкапы

### Автоматический бэкап при деплое

```bash
./deploy.sh main yes
```

Бэкапы сохраняются в `./data/storage.db.backup-YYYYMMDD-HHMMSS`

### Ручной бэкап

```bash
cp ./data/storage.db ./data/storage.db.backup-$(date +%Y%m%d-%H%M%S)
```

### Восстановление из бэкапа

```bash
# Остановите бот
docker stop max-bot

# Восстановите БД
cp ./data/storage.db.backup-20260405-120000 ./data/storage.db

# Запустите бот
docker start max-bot
```

---

## Обновление после изменений

### Если вы внесли изменения локально

```bash
# 1. Локально: закоммитьте и запушьте
git add -A
git commit -m "Ваши изменения"
git push origin dev  # или main

# 2. На сервере: задеплойте
./deploy.sh dev yes
```

### Если изменения внесены на сервере

```bash
# 1. Закоммитьте изменения
git add -A
git commit -m "Изменения на сервере"

# 2. Запушьте в репозиторий (опционально)
git push origin dev

# 3. Задеплойте
./deploy.sh dev yes
```

---

## Мониторинг

### Проверка использования ресурсов

```bash
docker stats max-bot
```

### Размер БД

```bash
ls -lh ./data/storage.db
```

### Количество контактов

```bash
sqlite3 ./data/storage.db "SELECT COUNT(*) FROM contacts;"
```

---

## Troubleshooting

### Контейнер не запускается

```bash
# Проверьте логи
docker logs max-bot

# Проверьте .env
cat .env

# Проверьте порт
netstat -tulpn | grep 9003
```

### Ошибка "port already in use"

```bash
# Найдите процесс на порту 9003
sudo lsof -i :9003

# Остановите старый контейнер
docker stop max-bot
docker rm max-bot
```

### БД заблокирована

```bash
# Остановите контейнер
docker stop max-bot

# Проверьте процессы
fuser ./data/storage.db

# Запустите снова
docker start max-bot
```

---

## Безопасность

### 1. Файрвол

Если нужен доступ к API извне:

```bash
# Ubuntu/Debian
sudo ufw allow 9003/tcp
sudo ufw reload
```

### 2. Защита .env

```bash
chmod 600 .env
```

### 3. HTTPS (опционально)

Используйте nginx как reverse proxy с SSL:

```nginx
server {
    listen 443 ssl;
    server_name bot.yourdomain.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:9003;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Автоматизация

### Cron для автоматического деплоя

```bash
# Редактировать crontab
crontab -e

# Деплой каждый день в 3:00
0 3 * * * cd /opt/max-bot && ./deploy.sh main yes >> /var/log/max-bot-deploy.log 2>&1
```

### Systemd service (альтернатива Docker restart policy)

Создайте `/etc/systemd/system/max-bot.service`:

```ini
[Unit]
Description=MAX Bot Service
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/max-bot
ExecStart=/usr/bin/docker start max-bot
ExecStop=/usr/bin/docker stop max-bot
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Активация:
```bash
sudo systemctl enable max-bot
sudo systemctl start max-bot
```
