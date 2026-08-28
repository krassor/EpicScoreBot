#!/bin/bash
set -euo pipefail

# Скрипт деплоя приложения из опубликованного в Docker Hub образа
# Использование: bash scripts/deploy/deploy_image.sh <version>
# Переменные окружения:
#   DOCKERHUB_USERNAME — обязательна, имя пользователя Docker Hub
#   (другие переменные опциональны и имеют дефолты)

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:-}"
if [ -z "$DOCKERHUB_USERNAME" ]; then
    echo "Error: DOCKERHUB_USERNAME environment variable is required"
    exit 1
fi

DOCKER_REPO="${DOCKERHUB_USERNAME}/epicscorebot"
LOG_FILE="/var/log/epicscorebot-deploy.log"

log_msg() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo "$msg"
    echo "$msg" >> "$LOG_FILE"
}

# Проверим, что файл логов доступен для записи
touch "$LOG_FILE" 2>/dev/null || {
    echo "Warning: cannot write to $LOG_FILE. Using stdout only."
    LOG_FILE="/dev/null"
}

log_msg "Starting image-based deployment process (version: $VERSION)..."

# Определяем директорию проекта
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

log_msg "Project directory identified as: $PROJECT_DIR"
cd "$PROJECT_DIR"

# Определяем текущий тег образа запущенного контейнера для отката
log_msg "Step 1: Determining current image tag for rollback..."
CONTAINER_NAME="app-backend-service-epic-score-bot"

OLD_VERSION=""
if docker inspect "$CONTAINER_NAME" &>/dev/null; then
    OLD_IMAGE=$(docker inspect --format='{{.Config.Image}}' "$CONTAINER_NAME")
    log_msg "Current container image: $OLD_IMAGE"

    # Попытаемся извлечь тег версии из образа (например, "krassor/epicscorebot:1.2.3")
    if [[ "$OLD_IMAGE" =~ :([^:]+)$ ]]; then
        OLD_VERSION="${BASH_REMATCH[1]}"
        log_msg "Extracted old version tag: $OLD_VERSION"
    fi
else
    log_msg "Container $CONTAINER_NAME is not running; no rollback version available (first deploy?)"
fi

# 2. Pull новой версии образа
log_msg "Step 2: Pulling image $DOCKER_REPO:$VERSION from Docker Hub..."
if ! docker pull "$DOCKER_REPO:$VERSION"; then
    log_msg "ERROR: Failed to pull image $DOCKER_REPO:$VERSION"
    exit 1
fi

# 3. Обновить контейнер до новой версии
log_msg "Step 3: Updating container with VERSION=$VERSION..."
if ! VERSION="$VERSION" docker compose up -d --no-deps "$CONTAINER_NAME"; then
    log_msg "ERROR: Failed to update container with new version."
    exit 1
fi

# 4. Healthcheck loop
log_msg "Step 4: Running healthcheck on http://localhost:8080/ping..."
HEALTH_URL="http://localhost:8080/ping"
SUCCESS=false

for i in {1..30}; do
    log_msg "Healthcheck attempt $i/30..."
    if HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 "$HEALTH_URL" 2>/dev/null || true); then
        if [ "$HTTP_STATUS" = "200" ]; then
            log_msg "Healthcheck passed! Server responded with HTTP 200"
            SUCCESS=true
            break
        else
            log_msg "Warning: Server responded with HTTP $HTTP_STATUS"
        fi
    else
        log_msg "Service is not responding yet..."
    fi
    sleep 2
done

if [ "$SUCCESS" = false ]; then
    log_msg "ERROR: Healthcheck failed after 30 attempts!"
    log_msg "Showing logs of backend container:"
    docker compose logs --tail=50 "$CONTAINER_NAME" >> "$LOG_FILE" || true
    docker compose logs --tail=50 "$CONTAINER_NAME" || true

    # Откат на предыдущую версию, если она известна
    if [ -n "$OLD_VERSION" ]; then
        log_msg "Initiating rollback due to failed healthcheck (rolling back to $OLD_VERSION)..."
        log_msg "Step 5: Pulling previous image $DOCKER_REPO:$OLD_VERSION..."
        if ! docker pull "$DOCKER_REPO:$OLD_VERSION"; then
            log_msg "ERROR: Failed to pull old image for rollback. Manual intervention required."
            exit 1
        fi

        log_msg "Step 6: Updating container back to VERSION=$OLD_VERSION..."
        if ! VERSION="$OLD_VERSION" docker compose up -d --no-deps "$CONTAINER_NAME"; then
            log_msg "ERROR: Failed to rollback container!"
            exit 1
        fi

        log_msg "Step 7: Verifying rollback healthcheck..."
        ROLLBACK_SUCCESS=false
        for i in {1..10}; do
            log_msg "Rollback healthcheck attempt $i/10..."
            if curl -s --max-time 2 "$HEALTH_URL" > /dev/null 2>&1; then
                log_msg "Rollback healthcheck passed."
                ROLLBACK_SUCCESS=true
                break
            fi
            sleep 2
        done

        if [ "$ROLLBACK_SUCCESS" = false ]; then
            log_msg "CRITICAL: Rollback healthcheck also failed! Manual intervention required."
        fi

        exit 1
    else
        log_msg "ERROR: No previous version available for rollback (first deploy?). Exiting with error."
        exit 1
    fi
fi

log_msg "Deployment successfully completed! Running version: $VERSION"
exit 0
