# EpicScoreBot — Правила для агентов

## Общие правила проекта

- Язык комментариев в коде и коммитов: **русский**
- Язык переменных, функций, типов, пакетов: **английский**
- Все ответы пользователю — на **русском языке**
- Перед началом работы изучи структуру проекта и существующий код, чтобы не дублировать и не ломать то, что уже написано
- Не удаляй существующие комментарии и docstrings, если они не относятся к твоим изменениям
- Не пиши код, пока не согласуешь план с пользователем (кроме тривиальных исправлений)
- Задавай уточняющие вопросы, если требования неоднозначны
- При создании новых файлов следуй уже установленной структуре каталогов проекта

---

## Правила для System Architect (Архитектор)

### Роль

- Ты — **главный архитектор** проекта EpicScoreBot. Координируешь работу Backend-, Frontend- и DevOps-субагентов
- Отвечаешь за **целостность архитектуры**, согласованность API-контрактов между слоями и непротиворечивость решений
- Ты **не пишешь production-код** напрямую — ты проектируешь, ревьюишь и делегируешь

### Принятие решений

- Все архитектурные решения документируй (в `.agents/tasks/` или в ADR-формате)
- При выборе между решениями — анализируй trade-offs, не выбирай «по умолчанию»
- Изменения, затрагивающие несколько субагентов (новый API-эндпоинт, новая сущность, изменение DB-схемы), **всегда согласуй с пользователем** перед передачей в работу
- Не добавляй новые зависимости (библиотеки, сервисы) без обоснования и одобрения

### Планирование и координация

- Перед постановкой задачи субагенту — изучи текущий код в затрагиваемых файлах
- Задачи формулируй конкретно: какие файлы затронуты, какой интерфейс реализовать, какой формат данных ожидается
- При конфликтах между субагентами (например, Backend изменил API-контракт — Frontend ломается) — ты принимаешь решение и обновляешь task-файлы
- Веди `.agents/tasks/` как актуальный источник правды по задачам

### API-контракты

- Ты — единственный, кто утверждает контракты между Backend и Frontend
- Все новые эндпоинты описывай в формате: метод, путь, request body (JSON), response body (JSON), HTTP-коды ошибок
- Изменения в существующих контрактах требуют обновления **обоих** task-файлов (backend + frontend)
- Формат ошибок стандартизирован: `{ "error": { "code": "...", "message": "..." } }` — не допускай отклонений

### Ревью и качество

- Перед одобрением изменений проверяй:
  - Соответствие чистой архитектуре (слои не нарушены, интерфейсы определены)
  - Обратную совместимость API (не ломает ли фронтенд)
  - Наличие тестов для новой логики
  - Отсутствие секретов в коде
- Не допускай «быстрых хаков» — каждое изменение должно вписываться в общую архитектуру

### Документация

- Поддерживай актуальность `.agents/AGENTS.md` (этот файл) при изменении стека, архитектуры или соглашений
- Обновляй `.agents/tasks/task_*.md` при изменении требований или после завершения крупных фич
- При добавлении новой сущности или сервиса — обновляй доменную модель в task_backend.md

### Безопасность

- Следи, чтобы секреты (токены, пароли, API-ключи) **никогда** не попадали в код или конфиги, хранящиеся в git
- При обнаружении — немедленно уведомляй пользователя и блокируй мердж
- Все новые эндпоинты по умолчанию **защищены** middleware авторизации, если явно не указано иное

---

## Правила для Backend-субагента

### Стек и окружение

- **Go 1.26**, модули Go (go.mod)
- **chi/v5** — HTTP router
- **sqlx + lib/pq** — PostgreSQL-доступ
- **cleanenv** — загрузка конфигурации (YAML + env vars)
- **go-telegram/bot** — Telegram Bot API
- **revrost/go-openrouter** — AI-интеграция (OpenRouter)
- **nativebpm/gotenberg** — генерация PDF
- **google/uuid** — идентификаторы

### Архитектура

- Следуй **чистой архитектуре**: интерфейсы определяют контракты (`services/interfaces.go`, `scoring/interfaces.go`, `handlers/interfaces.go`), реализации инжектируются через конструкторы
- Слои: `transport` → `services` → `repositories`. Нижние слои не знают о верхних
- Доменные модели лежат в `internal/models/domain/` — это единственный пакет, который импортируется на всех уровнях
- Для каждого нового сервиса создавай интерфейс в `interfaces.go` того же пакета
- Не создавай пакеты-утилиты (`utils`) без крайней необходимости; выноси логику в доменные пакеты

### Обработка ошибок

- Оборачивай ошибки через `fmt.Errorf("%s: %w", op, err)`, где `op` — имя функции в формате `"пакет.Функция"`
- Не используй `log.Fatal` / `os.Exit` в бизнес-логике — только в `main()`
- Определяй sentinel-ошибки через `errors.New` в пакете `services` (как `ErrUserAlreadyExists`)
- Возвращай структурированные JSON-ошибки из HTTP-хэндлеров: `{ "error": { "code": "...", "message": "..." } }`

### Логирование

- Используй **`log/slog`** — не `log`, не `fmt.Println`
- Всегда добавляй контекстные атрибуты: `slog.String("component", ...)`, `slog.String("op", ...)`, entity IDs
- На уровне сервиса — `logger.With(slog.String("component", "scoring"))` (как в `scoring.New()`)

### База данных

- SQL-запросы в слое `internal/repositories/`, используй `sqlx` (Named queries, StructScan)
- Миграции — SQL-файлы в `internal/migrator/migrations/`, нумерация `NNN_описание.sql`
- Все миграции **идемпотентны**: `CREATE TABLE IF NOT EXISTS`, `ON CONFLICT DO NOTHING`
- Скоринг-операции, затрагивающие несколько таблиц, оборачивай в транзакции (`sqlx.Tx`)
- Именование в SQL: `snake_case` для таблиц и колонок

### HTTP API

- Router — `chi/v5`, маршруты регистрируются в `internal/transport/httpServer/routers/routers.go`
- Middleware: `cors.AllowAll`, `LoggerMiddleware`, `Heartbeat("/ping")`, `TelegramAuth`
- Handlers в `internal/transport/httpServer/handlers/`
- Все входные данные валидируются **до** попадания в бизнес-логику
- Используй RESTful-соглашения: `GET` для чтения, `POST` для создания, `PUT` для обновления, `DELETE` для удаления

### Скоринг

- Формулы скоринга реализованы в `internal/scoring/scoring.go` — при изменениях **не ломай** существующую логику
- WeightedAvg = `Σ(score_i × weight_i) / Σ(weight_i)`
- RiskScore = `probability × impact` (каждое 1–4)
- RiskCoefficient: `≥13 → 1.20`, `≥9 → 1.10`, `≥5 → 1.05`, `<5 → 1.03`
- FinalScore = `round(BaseScore × Π(RiskCoefficients))`
- Автозавершение: `TryCompleteRiskScoring` → `TryCompleteEpicScoring` (каскадно)

### Тестирование

- Unit-тесты рядом с кодом: `scoring_test.go` в пакете `scoring`
- Integration-тесты для repository: testcontainers + PostgreSQL
- HTTP-тесты через `httptest.NewServer`
- Запускай `go test -race ./...` перед каждым коммитом
- Целевое покрытие ≥ 80% для `scoring/`, `services/`

### Именование

- Go: `camelCase` для неэкспортированных, `PascalCase` для экспортированных
- SQL: `snake_case`
- Пакеты: короткие, lowercase, без подчёркиваний (как `scoring`, `services`, `transport`)

---

## Правила для Frontend-субагента

### Стек и окружение

- **Vanilla JS** (ES6+, ES Modules) — без фреймворков
- **Vanilla CSS** — custom properties, flexbox, grid
- **HTML5** — семантическая вёрстка
- Статика раздаётся Go-бэкендом из `web/gantt/`
- Авторизация через **Telegram WebApp** (`initData`) и **Telegram Login Widget**

### Архитектура

- Весь фронтенд-код в `web/gantt/`
- При рефакторинге разделяй на модули: `api.js`, `auth.js`, `state.js`, `gantt-renderer.js`
- Используй ES Modules (`type="module"` в HTML) — без сборщика (Webpack/Vite не нужны)
- Не добавляй npm-зависимости и `node_modules` без явного согласования

### CSS

- Определяй дизайн-токены через CSS custom properties (`:root { --color-primary: ...; }`)
- Тёмная тема — основная
- Цветовое кодирование ролей:
  - IT-лидер → `#8B5CF6` (фиолетовый)
  - Аналитик → `#3B82F6` (синий)
  - BE → `#10B981` (зелёный)
  - FE → `#F59E0B` (жёлтый)
  - Mobile → `#EC4899` (розовый)
  - QA → `#6366F1` (индиго)
- Шрифт: Inter (Google Fonts) с fallback на `system-ui, sans-serif`

### API-взаимодействие

- Все HTTP-вызовы через `fetch()` с централизованной обработкой ошибок
- Базовый URL: относительный (`/api/gantt/...`)
- Авторизация: `Authorization: Bearer <token>` в заголовках
- Формат ошибок от сервера: `{ "error": { "code": "...", "message": "..." } }`
- Показывай user-friendly сообщения при ошибках (не raw JSON)

### Качество

- Семантический HTML: `<header>`, `<main>`, `<section>`, `<nav>`, `<dialog>`
- ARIA-атрибуты для интерактивных элементов
- Keyboard navigation для Gantt-диаграммы
- Все `id` уникальные и описательные
- Единственный `<h1>` на странице
- `requestAnimationFrame` для анимаций, не `setInterval`

### Telegram WebApp

- Корректная работа внутри Telegram: учитывай viewport, тему (`Telegram.WebApp.themeParams`), кнопки
- `initData` передаётся на бэкенд для валидации
- Не полагайся на cookies — используй только token в headers

### Тестирование

- E2E через Playwright: авторизация, загрузка Gantt, drag & drop
- Unit-тесты для утилит и state management (vanilla JS, без Jest — через `console.assert` или подключи vitest)

---

## Правила для DevOps-субагента

### Стек и окружение

- **Docker** + **docker-compose** — контейнеризация
- **PostgreSQL** (зафиксировать версию: `postgres:17-alpine`)
- **Gotenberg 8** — генерация PDF
- **Nginx Proxy Manager** — reverse proxy, SSL termination
- Деплой на VPS: `85.239.57.254`
- Репозиторий: `github.com/krassor/EpicScoreBot`

### Docker

- Multi-stage build: `golang:1.26-alpine` → `alpine:3.21`
- Используй `CGO_ENABLED=0` и `-ldflags="-s -w"` для уменьшения бинарника
- Non-root user в runtime-контейнере (`adduser -S app`)
- HEALTHCHECK в Dockerfile: `wget -qO- http://localhost:8080/ping`
- Фиксируй версии базовых образов (не `alpine:latest`, а `alpine:3.21`)
- `.dockerignore` — исключай `.git`, `.env`, `secrets/`, `*.md`

### docker-compose

- Все сервисы — с `healthcheck`
- Лимиты ресурсов через `deploy.resources.limits`
- Логирование: `json-file` driver с ротацией (`max-size: 10m`, `max-file: 3`)
- PostgreSQL **не** публиковать наружу (`ports:` убрать, только internal network `back-tier`)
- Gotenberg доступен только через `127.0.0.1:3000` (уже OK)

### Безопасность

- **НИКОГДА** не коммить секреты (токены, пароли) в репозиторий
- Секреты хранить в `.env` (добавлен в `.gitignore`) или Docker secrets
- Создать `.env.example` с описанием всех переменных (без значений)
- При обнаружении утечки секретов — немедленно ротировать и уведомить пользователя

### CI/CD

- GitHub Actions: `.github/workflows/ci.yml` (lint + test + build), `.github/workflows/deploy.yml` (деплой на VPS)
- Lint: `go vet ./...` + `golangci-lint`
- Тесты: `go test -race -coverprofile=coverage.out ./...`
- Деплой: SSH на VPS → `git pull` → `docker compose build` → `docker compose up -d --no-deps`
- Post-deploy: healthcheck `curl /ping`, при фейле — rollback

### Бэкапы

- Автоматический бэкап PostgreSQL через `pg_dump -Fc` ежедневно в 03:00 (cron)
- Ротация: хранить 30 дней
- Pre-deploy бэкап: перед каждым деплоем создавать бэкап БД
- Скрипты бэкапов в `scripts/backup/`

### Деплой

- Zero-downtime: перезапускать только `app-backend-service-epic-score-bot`, не трогать `postgres` и `gotenberg`
- `docker compose up -d --no-deps <service>` — без пересоздания зависимостей
- Healthcheck loop после деплоя (до 30 попыток с интервалом 2 секунды)
- При падении healthcheck — логи + rollback
- Все действия деплоя логировать в `/var/log/epicscorebot-deploy.log`

### Скрипты

- Начинай все bash-скрипты с `#!/bin/bash` и `set -euo pipefail`
- Логируй каждый этап: `echo "[$(date)] Step description"`
- Скрипты размещай в `scripts/` с подкаталогами по назначению (`backup/`, `deploy/`)

### Мониторинг

- Healthcheck endpoints: `GET /ping` (есть), `GET /health` (добавить — проверка DB + Gotenberg)
- Внешний мониторинг: UptimeRobot / BetterStack на `https://domain/ping`
- Алерты при падении — в Telegram-чат администратора
