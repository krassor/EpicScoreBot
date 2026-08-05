#!/bin/bash
set -euo pipefail

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

log_msg "Starting deployment process on VPS..."

# Определяем директорию проекта
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

log_msg "Project directory identified as: $PROJECT_DIR"
cd "$PROJECT_DIR"

# Сохраняем текущий коммит для возможного отката
OLD_COMMIT=$(git rev-parse HEAD)
log_msg "Current commit hash: $OLD_COMMIT"

# 1. Pull changes
log_msg "Step 1: Pulling latest changes from main branch..."
if ! git pull origin main; then
    log_msg "ERROR: Failed to pull changes from origin main."
    exit 1
fi

NEW_COMMIT=$(git rev-parse HEAD)
log_msg "New commit hash: $NEW_COMMIT"

if [ "$OLD_COMMIT" = "$NEW_COMMIT" ]; then
    log_msg "No new changes pulled, but proceeding with rebuild/restart as requested."
fi

# 2. Build backend container
log_msg "Step 2: Building backend container with --no-cache..."
if ! docker compose build --no-cache app-backend-service-epic-score-bot; then
    log_msg "ERROR: Docker build failed."
    exit 1
fi

# 3. Recreate backend container
log_msg "Step 3: Recreating backend container..."
if ! docker compose up -d --no-deps app-backend-service-epic-score-bot; then
    log_msg "ERROR: Failed to restart container."
    log_msg "Initiating rollback to commit $OLD_COMMIT..."
    git reset --hard "$OLD_COMMIT"
    docker compose build --no-cache app-backend-service-epic-score-bot
    docker compose up -d --no-deps app-backend-service-epic-score-bot
    exit 1
fi

# 4. Healthcheck loop
log_msg "Step 4: Running healthcheck on http://localhost:8080/ping..."
HEALTH_URL="http://localhost:8080/ping"
SUCCESS=false

for i in {1..30}; do
    log_msg "Healthcheck attempt $i/30..."
    if HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 "$HEALTH_URL"); then
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
    docker compose logs --tail=50 app-backend-service-epic-score-bot >> "$LOG_FILE" || true
    docker compose logs --tail=50 app-backend-service-epic-score-bot || true
    
    log_msg "Initiating rollback due to failed healthcheck..."
    git reset --hard "$OLD_COMMIT"
    docker compose build --no-cache app-backend-service-epic-score-bot
    docker compose up -d --no-deps app-backend-service-epic-score-bot
    log_msg "Rollback completed. Checking rolled-back health..."
    
    if curl -s --max-time 2 "$HEALTH_URL" > /dev/null; then
        log_msg "Rollback healthcheck passed."
    else
        log_msg "CRITICAL: Rollback healthcheck failed as well!"
    fi
    exit 1
fi

log_msg "Deployment successfully completed!"
