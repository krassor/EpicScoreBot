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
