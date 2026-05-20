#!/bin/bash
# redeploy.sh — обновление и пересборка max-bot через docker compose.
# Использует docker-compose.yml в корне репо (имя контейнера max-bot-service).
#
# Что делает:
#   1. (опционально) бэкап ./data/storage.db в ./data/storage.db.backup-<timestamp>
#   2. git fetch + checkout + pull нужной ветки из автоопределяемого remote
#   3. docker compose down  (останавливает и удаляет контейнер)
#   4. docker compose up -d --build  (пересобирает образ и запускает)
#   5. короткое ожидание + статус и последние логи
#
# Использование:
#   ./redeploy.sh                  — текущая ветка, без бэкапа
#   ./redeploy.sh dev              — переключиться на dev, без бэкапа
#   ./redeploy.sh main yes         — переключиться на main, с бэкапом БД
#   ./redeploy.sh "" yes           — текущая ветка, с бэкапом
#
# Данные в ./data сохраняются за счёт bind-mount в docker-compose.yml
# (./data:/app/data). Контейнер при `down` пересоздаётся, но volume на хосте
# не трогается.

set -e

BRANCH=${1:-}
BACKUP=${2:-no}
DATA_DIR="./data"
DB_FILE="$DATA_DIR/storage.db"

# Если ветка не указана — используем текущую
if [ -z "$BRANCH" ]; then
    BRANCH=$(git rev-parse --abbrev-ref HEAD)
fi

# Автоопределение remote (как в deploy.sh: сначала origin, затем my, иначе первый)
if git remote | grep -q "^origin$"; then
    REMOTE="origin"
elif git remote | grep -q "^my$"; then
    REMOTE="my"
else
    REMOTE=$(git remote | head -1)
    if [ -z "$REMOTE" ]; then
        echo "❌ Ошибка: ни одного git remote не настроено"
        exit 1
    fi
fi

echo "=========================================="
echo "MAX Bot redeploy via docker compose"
echo "=========================================="
echo "Branch:  $BRANCH"
echo "Backup:  $BACKUP"
echo "Remote:  $REMOTE"
echo "Time:    $(date '+%Y-%m-%d %H:%M:%S')"
echo "=========================================="

# Проверка .env (compose требует BOT_TOKEN через ${BOT_TOKEN:?})
if [ ! -f .env ]; then
    echo "❌ Ошибка: файл .env не найден!"
    echo "Создайте .env c BOT_TOKEN, либо используйте deploy.sh с другим конфигом."
    exit 1
fi

# Бэкап БД (если запрошен и файл есть)
if [ "$BACKUP" = "yes" ] && [ -f "$DB_FILE" ]; then
    BACKUP_FILE="$DATA_DIR/storage.db.backup-$(date +%Y%m%d-%H%M%S)"
    echo "📦 Создание бэкапа БД: $BACKUP_FILE"
    cp "$DB_FILE" "$BACKUP_FILE"
    echo "✅ Бэкап создан"
elif [ "$BACKUP" = "yes" ]; then
    echo "ℹ️  BACKUP=yes, но $DB_FILE отсутствует — бэкап пропущен"
fi

# Обновление кода
echo "📥 Обновление кода: $REMOTE/$BRANCH"
git fetch "$REMOTE"
git checkout "$BRANCH"
git pull "$REMOTE" "$BRANCH"
echo "✅ Код обновлён"

# Определяем docker compose CLI (Compose v2: docker compose; v1: docker-compose)
if docker compose version >/dev/null 2>&1; then
    DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
    DC="docker-compose"
else
    echo "❌ Ошибка: ни 'docker compose', ни 'docker-compose' не найден"
    exit 1
fi

# Остановка
echo "🛑 $DC down..."
$DC down
echo "✅ Контейнер остановлен"

# Пересборка и запуск
echo "🔨 $DC up -d --build..."
$DC up -d --build
echo "✅ Контейнер запущен"

# Короткое ожидание
echo "⏳ Ожидание запуска сервиса..."
sleep 3

# Статус и логи
echo ""
echo "=========================================="
echo "Redeploy завершён"
echo "=========================================="
$DC ps
echo "---"
echo "📋 Последние логи:"
$DC logs --tail 20
