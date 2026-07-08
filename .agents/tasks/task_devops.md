# Task: DevOps-субагент — Инфраструктура EpicScoreBot

## Роль

Ты — **DevOps-инженер** проекта EpicScoreBot. Отвечаешь за контейнеризацию, CI/CD, мониторинг, бэкапы и деплой. Текущий стек: **Docker + docker-compose**, **PostgreSQL**, **Gotenberg** (PDF), **Nginx Proxy Manager** (reverse proxy + SSL). Деплой на VPS `85.239.57.254`.

---

## Необходимые Skills

Перед началом работы **обязательно прочитай** SKILL.md для каждого из указанных skills:

| Skill | Путь | Назначение |
|-------|------|------------|
| `docker-expert` | `~/.gemini/antigravity-cli/skills/docker-expert/SKILL.md` | Оптимизация Docker, multi-stage builds, security hardening |
| `ci-cd-and-automation` | `~/.gemini/antigravity-cli/skills/ci-cd-and-automation/SKILL.md` | CI/CD пайплайны, автоматизация |
| `deployment-procedures` | `~/.gemini/antigravity-cli/skills/deployment-procedures/SKILL.md` | Процедуры деплоя, rollback, верификация |
| `cloud-devops` | `~/.gemini/antigravity-cli/skills/cloud-devops/SKILL.md` | Облачная инфраструктура, мониторинг |
| `architecture` | `~/.gemini/antigravity-cli/skills/architecture/SKILL.md` | ADR, архитектурные решения инфраструктуры |
| `database-migration` | `~/.gemini/antigravity-cli/skills/database-migration/SKILL.md` | Миграции БД в контексте деплоя |
| `database-design` | `~/.gemini/antigravity-cli/skills/database-design/SKILL.md` | Конфигурация PostgreSQL, бэкапы |
| `code-review-and-quality` | `~/.gemini/antigravity-cli/skills/code-review-and-quality/SKILL.md` | Ревью конфигурационных файлов |
| `kubernetes-deployment` | `~/.gemini/antigravity-cli/skills/kubernetes-deployment/SKILL.md` | K8s (если планируется миграция с docker-compose) |
| `bash-scripting` | `~/.gemini/antigravity-cli/skills/bash-scripting/SKILL.md` | Автоматизация через shell-скрипты |

---

## Текущая инфраструктура

### docker-compose.yml (4 сервиса)

```yaml
services:
  app-backend-service-epic-score-bot:   # Go-приложение на порту 8080
  postgres:                              # PostgreSQL (epic-score-db)
  gotenberg:                             # Gotenberg 8 (PDF generation, порт 3000)
  NPM:                                   # Nginx Proxy Manager (80/443/81)
```

### Dockerfile (multi-stage)

```dockerfile
# Stage 1: Build (golang:1.26-alpine)
FROM golang:1.26-alpine AS builder
RUN go build -o bin/epicScoreBot app/main.go

# Stage 2: Runtime (alpine:latest)
FROM alpine:latest
COPY --from=builder /build/bin/epicScoreBot /bin/epicScoreBot
COPY --from=builder /build/web /web
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

### Скрипты

| Файл | Назначение |
|------|------------|
| `entrypoint.sh` | Создание директорий для config/templates/pdf, запуск Go-приложения |
| `run_docker_build.sh` | Полный цикл: `git pull` → `docker compose down` → prune → build → up |
| `scripts/migration/backup.sh` | Бэкап PostgreSQL |
| `scripts/migration/restore.sh` | Восстановление PostgreSQL |

### Переменные окружения (.env)

```env
VOLUME_PATH=...
CONFIG_FILEPATH=...
CONFIG_FILENAME=...
HTML_TEMPLATE_FILEPATH=...
HTML_TEMPLATE_FILENAME=...
PDF_FILEPATH=...
```

### Сети

- `back-tier` — внутренняя сеть для коммуникации между сервисами
- Gotenberg доступен только по `127.0.0.1:3000` (не виден извне)

---

## Шаги реализации

### Шаг 1. Оптимизация Docker

#### 1.1 Улучшить Dockerfile

Текущие проблемы и улучшения:

```dockerfile
# Проблема: git устанавливается в runtime — не нужен
# Проблема: alpine:latest — непредсказуемая версия
# Проблема: нет non-root user

# Целевая версия:
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/epicScoreBot app/main.go

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S app -G app
COPY --from=builder /build/bin/epicScoreBot /bin/epicScoreBot
COPY --from=builder /build/web /web
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER app
ENV HTTP_SERVER_PORT=8080
EXPOSE $HTTP_SERVER_PORT
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/ping || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
```

#### 1.2 Улучшить docker-compose.yml

```yaml
services:
  app-backend-service-epic-score-bot:
    # Добавить:
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/ping"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 256M
          cpus: "0.5"
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  postgres:
    # Добавить:
    image: postgres:17-alpine     # Зафиксировать версию!
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U epicbot -d epic-score-db"]
      interval: 10s
      timeout: 5s
      retries: 5
    # Убрать пароль из docker-compose, использовать secrets
    environment:
      - POSTGRES_PASSWORD_FILE=/run/secrets/db_password
    secrets:
      - db_password

  gotenberg:
    image: gotenberg/gotenberg:8  # OK — уже зафиксировано
    # Добавить healthcheck
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 5s

secrets:
  db_password:
    file: ./secrets/db_password.txt
```

### Шаг 2. CI/CD Pipeline (GitHub Actions)

Создать `.github/workflows/`:

#### 2.1 `ci.yml` — Continuous Integration

```yaml
# Триггер: push/PR на main, develop
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.26' }
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v6

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_DB: epic-score-db-test
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
        ports: ['5432:5432']
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.26' }
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go tool cover -func=coverage.out

  build:
    needs: [lint, test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker build -t epicscorebot:${{ github.sha }} .
```

#### 2.2 `deploy.yml` — Continuous Deployment

```yaml
# Триггер: push на main (после успешного CI)
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    needs: ci
    runs-on: ubuntu-latest
    steps:
      - name: Deploy to VPS
        uses: appleboy/ssh-action@v1
        with:
          host: 85.239.57.254
          username: ${{ secrets.VPS_USER }}
          key: ${{ secrets.VPS_SSH_KEY }}
          script: |
            cd /home/EpicScoreBot
            git pull origin main
            docker compose build --no-cache app-backend-service-epic-score-bot
            docker compose up -d --no-deps app-backend-service-epic-score-bot
            # Подождать healthcheck
            sleep 10
            curl -sf http://localhost:8080/ping || (docker compose logs app-backend-service-epic-score-bot && exit 1)
```

### Шаг 3. Бэкап и восстановление БД

#### 3.1 Автоматизация бэкапов

Создать `scripts/backup/automated_backup.sh`:

```bash
#!/bin/bash
# Автоматический бэкап PostgreSQL
# Вызывается через cron: 0 3 * * * /home/EpicScoreBot/scripts/backup/automated_backup.sh

BACKUP_DIR="/home/backups/epicscorebot"
DATE=$(date +%Y%m%d_%H%M%S)
RETAIN_DAYS=30

mkdir -p "$BACKUP_DIR"

docker exec postgres pg_dump -U epicbot -d epic-score-db -Fc \
  > "$BACKUP_DIR/epicscorebot_${DATE}.dump"

# Ротация: удалить бэкапы старше N дней
find "$BACKUP_DIR" -name "*.dump" -mtime +$RETAIN_DAYS -delete

echo "[$(date)] Backup completed: epicscorebot_${DATE}.dump"
```

#### 3.2 Cron на VPS

```cron
# /etc/cron.d/epicscorebot-backup
0 3 * * * root /home/EpicScoreBot/scripts/backup/automated_backup.sh >> /var/log/epicscorebot-backup.log 2>&1
```

### Шаг 4. Мониторинг и алерты

#### 4.1 Healthcheck endpoints

Backend уже предоставляет `GET /ping`. Расширить:

```
GET /ping              → 200 "." (уже есть, chi middleware.Heartbeat)
GET /health            → 200 { "status": "ok", "db": "ok", "gotenberg": "ok" }
GET /health/readiness  → 200 (только когда приложение полностью готово)
```

#### 4.2 Docker compose healthchecks

Все сервисы должны иметь healthcheck (см. Шаг 1.2).

#### 4.3 Внешний мониторинг

Рекомендации:
- **UptimeRobot / BetterStack** — мониторинг `https://domain/ping` каждые 5 минут
- **Алерты в Telegram** — при падении сервиса уведомление в чат администратора

### Шаг 5. Безопасность

#### 5.1 Secrets management

```
# Текущая проблема: пароли и API-токены в config.yml и docker-compose.yml
# Решение:

1. Перенести чувствительные данные в .env (не коммитить в git):
   DB_PASSWORD=epicbot@
   TG_BOT_TOKEN=8475029258:AAF86aOU8cljOWt03jifcqM8lf367bg7hvI
   AI_API_TOKEN=sk-or-v1-d2e066b1...

2. Обновить config.yml — читать из env vars:
   db:
     password: ${DB_PASSWORD}
   bot:
     tgbot_apitoken: ${TG_BOT_TOKEN}
     AI:
       aiapitoken: ${AI_API_TOKEN}

3. Добавить в .gitignore:
   .env
   secrets/
```

> [!CAUTION]
> В текущем `config/config.yml` хранятся Telegram Bot Token и AI API Token **в открытом виде**! Это **критическая уязвимость**. Необходимо немедленно:
> 1. Ротировать все скомпрометированные токены
> 2. Перенести секреты в .env или Docker secrets
> 3. Убедиться, что `.env` в `.gitignore`

#### 5.2 Сетевая безопасность

```yaml
# Gotenberg НЕ должен быть доступен извне (уже OK: 127.0.0.1:3000)
# PostgreSQL НЕ должен быть доступен извне
# → Убрать ports: для postgres из docker-compose (доступ только через back-tier)

services:
  postgres:
    # ports:          # УБРАТЬ! Доступ только через internal network
    #   - 5432:5432
    networks:
      - back-tier
```

### Шаг 6. Улучшение скрипта деплоя

Переписать `run_docker_build.sh`:

```bash
#!/bin/bash
set -euo pipefail

DEPLOY_DIR="/home/EpicScoreBot"
LOG_FILE="/var/log/epicscorebot-deploy.log"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"; }

cd "$DEPLOY_DIR"

log "=== Deploy started ==="

# 1. Pull latest code
log "Pulling latest code..."
git pull origin main

# 2. Backup DB before deploy
log "Creating pre-deploy backup..."
./scripts/backup/automated_backup.sh

# 3. Build new image
log "Building Docker image..."
docker compose build --no-cache app-backend-service-epic-score-bot

# 4. Deploy with zero-downtime (restart only app, not DB)
log "Deploying..."
docker compose up -d --no-deps app-backend-service-epic-score-bot

# 5. Wait for health check
log "Waiting for health check..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:8080/ping > /dev/null 2>&1; then
    log "✅ Health check passed"
    break
  fi
  if [ "$i" -eq 30 ]; then
    log "❌ Health check failed after 30 attempts!"
    log "Rolling back..."
    docker compose logs --tail=50 app-backend-service-epic-score-bot
    exit 1
  fi
  sleep 2
done

# 6. Cleanup
log "Cleaning up old images..."
docker image prune -f

log "=== Deploy completed ==="
```

---

## Целевая архитектура (диаграмма)

```
                    ┌─────────────────────────┐
                    │   Nginx Proxy Manager    │
                    │   (SSL termination)      │
                    │   :80 / :443 / :81       │
                    └──────────┬──────────────┘
                               │ back-tier
                    ┌──────────▼──────────────┐
                    │   EpicScoreBot (Go)      │
                    │   :8080                  │
                    │   ├── HTTP API (chi)     │
                    │   ├── Telegram Bot       │
                    │   ├── Scoring Engine     │
                    │   └── AI Client          │
                    └──┬──────────────┬───────┘
                       │ back-tier    │ back-tier
              ┌────────▼──────┐  ┌───▼─────────┐
              │  PostgreSQL   │  │  Gotenberg   │
              │  :5432        │  │  :3000       │
              │  (internal)   │  │  (internal)  │
              └───────────────┘  └──────────────┘
```

---

## Требования к качеству

1. **Воспроизводимость**: Все зависимости с зафиксированными версиями
2. **Безопасность**: Non-root containers, secrets management, minimal attack surface
3. **Наблюдаемость**: Healthchecks, structured logging, alerting
4. **Отказоустойчивость**: Автоматические бэкапы, rollback-стратегия, graceful shutdown
5. **Документация**: README с инструкциями по деплою, .env.example с описанием переменных

---

## Зависимости от других субагентов

| Зависимость | Описание |
|-------------|----------|
| **Backend** | Предоставляет Dockerfile, entrypoint.sh, healthcheck эндпоинты. Изменения в зависимостях Go → пересборка образа |
| **Frontend** | Статика в `web/gantt/` копируется в Docker-образ бэкенда на этапе сборки |
