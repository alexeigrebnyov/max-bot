#!/bin/bash
# Скрипт деплоя MAX Bot на удаленный сервер
# Использование: ./deploy.sh [branch] [backup]
# Примеры:
#   ./deploy.sh dev        - деплой из ветки dev без бэкапа
#   ./deploy.sh main yes   - деплой из main с бэкапом БД
#   ./deploy.sh            - деплой из main без бэкапа (по умолчанию)

set -e  # Остановить при ошибке

BRANCH=${1:-main}
BACKUP=${2:-no}
CONTAINER_NAME="max-bot"
IMAGE_NAME="max-bot-service"
DATA_DIR="./data"
DB_FILE="$DATA_DIR/storage.db"

echo "=========================================="
echo "MAX Bot Deployment Script"
echo "=========================================="
echo "Branch: $BRANCH"
echo "Backup: $BACKUP"
echo "Time: $(date '+%Y-%m-%d %H:%M:%S')"
echo "=========================================="

# Проверка наличия .env
if [ ! -f .env ]; then
    echo "❌ Ошибка: файл .env не найден!"
    echo "Создайте файл .env с переменной BOT_TOKEN"
    exit 1
fi

# Бэкап БД если запрошен
if [ "$BACKUP" = "yes" ] && [ -f "$DB_FILE" ]; then
    BACKUP_FILE="$DATA_DIR/storage.db.backup-$(date +%Y%m%d-%H%M%S)"
    echo "📦 Создание бэкапа БД: $BACKUP_FILE"
    cp "$DB_FILE" "$BACKUP_FILE"
    echo "✅ Бэкап создан"
fi

# Остановка и удаление старого контейнера
echo "🛑 Остановка контейнера..."
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    docker stop $CONTAINER_NAME 2>/dev/null || true
    docker rm $CONTAINER_NAME 2>/dev/null || true
    echo "✅ Контейнер остановлен"
else
    echo "ℹ️  Контейнер не запущен"
fi

# Обновление кода
echo "📥 Обновление кода из ветки $BRANCH..."
# Определяем доступный remote (для совместимости с разными настройками)
if git remote | grep -q "^origin$"; then
    REMOTE="origin"
else
    REMOTE="my"
fi
echo "Using remote: $REMOTE"
git fetch $REMOTE
git checkout $BRANCH
git pull $REMOTE $BRANCH
echo "✅ Код обновлен"

# Сборка образа
echo "🔨 Сборка Docker образа..."
docker build -t $IMAGE_NAME:$BRANCH .
echo "✅ Образ собран: $IMAGE_NAME:$BRANCH"

# Создание директории для данных
mkdir -p $DATA_DIR

# Запуск контейнера
echo "🚀 Запуск контейнера..."
docker run -d \
    --restart unless-stopped \
    -p 9003:9003 \
    --env-file .env \
    -v "$(pwd)/$DATA_DIR:/app/data" \
    --name $CONTAINER_NAME \
    $IMAGE_NAME:$BRANCH

echo "✅ Контейнер запущен"

# Ожидание запуска
echo "⏳ Ожидание запуска сервиса..."
sleep 3

# Проверка статуса
if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "✅ Контейнер работает"
    echo ""
    echo "=========================================="
    echo "Деплой завершен успешно!"
    echo "=========================================="
    echo "Логи: docker logs -f $CONTAINER_NAME"
    echo "Остановка: docker stop $CONTAINER_NAME"
    echo "Веб-интерфейс: http://localhost:9003/admin"
    echo "=========================================="
    echo ""
    echo "📋 Последние логи:"
    docker logs --tail 20 $CONTAINER_NAME
else
    echo "❌ Ошибка: контейнер не запустился"
    echo "Проверьте логи: docker logs $CONTAINER_NAME"
    exit 1
fi
