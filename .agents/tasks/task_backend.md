# Task: Backend-субагент — Сервис оценки задач (EpicScoreBot)

## Роль

Ты — **Backend-разработчик** сервиса EpicScoreBot. Сервис написан на **Go 1.26**, использует **chi router**, **PostgreSQL** (через sqlx), **Telegram Bot API**, **OpenRouter AI** и **Gotenberg** для генерации PDF-отчётов.

---

## Необходимые Skills

Перед началом работы **обязательно прочитай** SKILL.md для каждого из указанных skills:

| Skill | Путь | Назначение |
|-------|------|------------|
| `golang-pro` | `~/.gemini/antigravity-cli/skills/golang-pro/SKILL.md` | Паттерны и best practices Go |
| `api-design-principles` | `~/.gemini/antigravity-cli/skills/api-design-principles/SKILL.md` | Проектирование REST API |
| `api-designer` | `~/.gemini/antigravity-cli/skills/api-designer/SKILL.md` | Контракты и спецификации API |
| `backend-architect` | `~/.gemini/antigravity-cli/skills/backend-architect/SKILL.md` | Архитектурные решения backend |
| `database-design` | `~/.gemini/antigravity-cli/skills/database-design/SKILL.md` | Проектирование БД |
| `database-migration` | `~/.gemini/antigravity-cli/skills/database-migration/SKILL.md` | Миграции PostgreSQL |
| `go-concurrency-patterns` | `~/.gemini/antigravity-cli/skills/go-concurrency-patterns/SKILL.md` | Паттерны конкурентности Go |
| `code-review-and-quality` | `~/.gemini/antigravity-cli/skills/code-review-and-quality/SKILL.md` | Качество и ревью кода |
| `error-handling-patterns` | `~/.gemini/antigravity-cli/skills/error-handling-patterns/SKILL.md` | Обработка ошибок |
| `clean-code` | `~/.gemini/antigravity-cli/skills/clean-code/SKILL.md` | Чистый код |

---

## Архитектура проекта

```
internal/
├── ai/              # Интеграция с OpenRouter (Claude Haiku 4.5)
├── config/          # Загрузка конфигурации (cleanenv, YAML)
├── gantt/           # Бизнес-логика Gantt-диаграмм
├── graceful/        # Graceful shutdown
├── migrator/        # SQL-миграции (встроенные через embed)
│   └── migrations/  # SQL-файлы миграций (001–004)
├── models/
│   ├── domain/      # Доменные модели (Team, User, Epic, Risk, Score, GanttTask)
│   └── repositories/# DTO для хранилища
├── report/          # Генерация PDF-отчётов (Gotenberg + HTML template)
├── repositories/    # Слой доступа к данным (sqlx + PostgreSQL)
├── scoring/         # Движок скоринга (взвешенные средние, коэффициенты рисков)
├── services/        # Бизнес-логика (UserService, TeamService, EpicService, RiskService, RoleService)
├── telegram/        # Telegram Bot (команды, callbacks, сессии, admin)
├── transport/
│   └── httpServer/  # HTTP API (chi router, middleware, handlers)
└── utils/           # Утилиты
```

---

## Доменная модель

### Сущности

| Сущность | Таблица | Ключевые поля |
|----------|---------|---------------|
| **Team** | `teams` | `id UUID`, `name TEXT UNIQUE`, `description TEXT` |
| **Role** | `roles` | `id UUID`, `name TEXT UNIQUE`, `description TEXT` |
| **User** | `users` | `id UUID`, `first_name`, `last_name`, `telegram_id TEXT UNIQUE`, `chat_id BIGINT`, `weight INT (0–100)` |
| **Epic** | `epics` | `id UUID`, `number TEXT`, `name TEXT`, `description TEXT`, `team_id UUID FK`, `status TEXT (NEW/SCORING/SCORED)`, `final_score NUMERIC` |
| **Risk** | `risks` | `id UUID`, `description TEXT`, `epic_id UUID FK`, `status TEXT`, `weighted_score NUMERIC` |
| **EpicScore** | `epic_scores` | `id UUID`, `epic_id FK`, `user_id FK`, `role_id FK`, `score INT`, `UNIQUE(epic_id, user_id)` |
| **EpicRoleScore** | `epic_role_scores` | `id UUID`, `epic_id FK`, `role_id FK`, `weighted_avg NUMERIC`, `UNIQUE(epic_id, role_id)` |
| **RiskScore** | `risk_scores` | `id UUID`, `risk_id FK`, `user_id FK`, `probability INT (1–4)`, `impact INT (1–4)`, `UNIQUE(risk_id, user_id)` |
| **GanttTask** | `gantt_tasks` | `id UUID`, `epic_id FK`, `role_id UUID?`, `name TEXT`, `start_date`, `end_date`, `progress FLOAT`, `sort_order INT`, `is_parent BOOL`, `parent_task_id UUID?` |

### Связи

- `user_teams` — M:N (User ↔ Team)
- `user_roles` — M:N (User ↔ Role)
- `Epic` → `Team` (N:1)
- `Risk` → `Epic` (N:1)
- `EpicScore` → `Epic`, `User`, `Role`
- `RiskScore` → `Risk`, `User`
- `GanttTask` → `Epic`, `Role?`, `GanttTask?` (self-ref для parent)

---

## Алгоритм скоринга

### Формула оценки эпика

```
1. Каждый участник команды ставит score (int) за эпик под своей ролью
2. WeightedAvgPerRole = Σ(score_i × user_weight_i) / Σ(user_weight_i)
3. EpicBaseScore = Σ(WeightedAvgPerRole) по всем ролям
4. Для каждого риска: RiskWeightedScore = Σ(probability_i × impact_i × weight_i) / Σ(weight_i)
5. RiskCoefficient: ≥13 → 1.20, ≥9 → 1.10, ≥5 → 1.05, <5 → 1.03
6. FinalScore = EpicBaseScore × Π(RiskCoefficient_j) для всех рисков
7. FinalScore = round(FinalScore)
```

### Автоматическое завершение

- `TryCompleteRiskScoring`: когда все участники команды проголосовали за риск → вычислить и сохранить `weighted_score`
- `TryCompleteEpicScoring`: когда все участники проголосовали за эпик **и** все риски оценены → вычислить `final_score`

---

## Существующие API-контракты (HTTP)

### Маршруты (chi router)

```
GET  /ping                         → Heartbeat (middleware)
GET  /gantt/*                      → Static files (web/gantt/)

POST /api/gantt/auth/webapp        → Telegram WebApp авторизация
GET  /api/gantt/auth               → Telegram Login Widget callback

# Защищённые (TelegramAuth middleware):
GET  /api/gantt/teams              → Список команд пользователя
GET  /api/gantt/epics              → Эпики команды со статусом SCORED
GET  /api/gantt/tasks              → Gantt-задачи команды
POST /api/gantt/tasks/generate     → Генерация задач на основе scored эпиков
PUT  /api/gantt/tasks/{id}/        → Обновление дат задачи
PUT  /api/gantt/tasks/{id}/reorder → Изменение порядка задачи
DELETE /api/gantt/tasks/{id}/      → Удаление задачи
```

---

## Шаги для реализации новых функций

### Шаг 1. Расширение API для полного CRUD эпиков и рисков

Добавить REST-эндпоинты для управления сущностями напрямую (помимо Telegram):

```
# Эпики
POST   /api/epics                  → Создать эпик
GET    /api/epics                  → Список всех эпиков (фильтр по status, team_id)
GET    /api/epics/{id}             → Получить эпик по ID
PUT    /api/epics/{id}/status      → Обновить статус эпика
DELETE /api/epics/{id}             → Удалить эпик

# Оценки эпиков
POST   /api/epics/{id}/scores      → Отправить оценку
GET    /api/epics/{id}/scores      → Получить все оценки эпика
GET    /api/epics/{id}/role-scores  → Получить агрегированные оценки по ролям

# Риски
POST   /api/epics/{id}/risks       → Создать риск для эпика
GET    /api/epics/{id}/risks       → Список рисков эпика
DELETE /api/risks/{id}             → Удалить риск

# Оценки рисков
POST   /api/risks/{id}/scores      → Отправить оценку риска
GET    /api/risks/{id}/scores      → Получить оценки риска

# Команды
GET    /api/teams                  → Список команд
GET    /api/teams/{id}/users       → Участники команды
GET    /api/teams/{id}/epics       → Эпики команды

# Отчёты
GET    /api/teams/{id}/report      → PDF-отчёт по команде
GET    /api/teams/{id}/report/data → JSON-данные для отчёта
```

### Шаг 2. Формат запросов/ответов (JSON)

#### Создание эпика

```json
// POST /api/epics
// Request:
{
  "number": "EPIC-42",
  "name": "Интеграция платёжной системы",
  "description": "Подключить Stripe для обработки платежей",
  "team_id": "550e8400-e29b-41d4-a716-446655440000"
}

// Response (201 Created):
{
  "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "number": "EPIC-42",
  "name": "Интеграция платёжной системы",
  "description": "Подключить Stripe для обработки платежей",
  "team_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "NEW",
  "final_score": null,
  "created_at": "2026-07-07T20:00:00Z",
  "updated_at": "2026-07-07T20:00:00Z"
}
```

#### Отправка оценки

```json
// POST /api/epics/{id}/scores
// Request:
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "role_id": "660e8400-e29b-41d4-a716-446655440000",
  "score": 13
}

// Response (201 Created):
{
  "status": "ok",
  "scoring_complete": false,
  "scores_received": 3,
  "scores_expected": 5
}
```

#### Отправка оценки риска

```json
// POST /api/risks/{id}/scores
// Request:
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "probability": 3,
  "impact": 4
}

// Response (201 Created):
{
  "status": "ok",
  "scoring_complete": true,
  "weighted_score": 9.6,
  "risk_coefficient": 1.10
}
```

### Шаг 3. Миграция БД

Создать файл `internal/migrator/migrations/005_api_improvements.sql`:

- Добавить индексы для оптимизации API-запросов
- Добавить `updated_at` триггеры для автообновления
- Создать `api_tokens` таблицу для JWT/API-key аутентификации (если нужно REST API без Telegram)

### Шаг 4. Middleware и безопасность

- Реализовать `APIKeyAuth` middleware для REST API (альтернатива TelegramAuth)
- Rate limiting middleware
- Request validation middleware (входные данные)
- Structured error responses:

```json
{
  "error": {
    "code": "EPIC_NOT_FOUND",
    "message": "Epic with ID '...' not found",
    "details": {}
  }
}
```

### Шаг 5. Тестирование

- Unit-тесты для `scoring` пакета (формулы)
- Integration-тесты для repository-слоя (testcontainers + PostgreSQL)
- HTTP handler тесты (httptest)
- Покрытие ≥ 80% для `scoring/`, `services/`

---

## Требования к качеству кода

1. **Архитектура**: Чистая архитектура — интерфейсы определяют контракты, реализации инжектируются
2. **Ошибки**: Обёртка ошибок через `fmt.Errorf("%s: %w", op, err)` с именем операции
3. **Логирование**: `slog` с контекстом (`component`, `op`, entity IDs)
4. **Именование**: snake_case для SQL, camelCase для Go, PascalCase для экспортированных типов
5. **Валидация**: Все входные данные валидируются до попадания в бизнес-логику
6. **Транзакции**: Скоринг-операции оборачиваются в DB-транзакции
7. **Идемпотентность**: `UNIQUE` constraints в БД + проверки `HasUserScored*`

---

## Зависимости от других субагентов

| Зависимость | Описание |
|-------------|----------|
| **Frontend** | Предоставляет API-контракты для Gantt UI и будущего веб-интерфейса |
| **DevOps** | Контейнер собирается через Dockerfile, конфиг монтируется через volume |

---

## Шаг 6. Возможность проставления оценок администратором за участников

Реализовать API-механизм для проставления оценок эпиков и рисков администратором/суперадминистратором вместо любого из участников команды.

### 1. Новые REST API эндпоинты
* **`POST /api/gantt/admin/scores/epic`** — запись оценки эпика от лица участника:
  * Доступ: только роли `admin` или `superadmin` (через `RoleAuth` middleware).
  * Request Body: `{ "epic_id": "uuid", "user_id": "uuid", "score": int }`.
  * Поведение: определяет роль целевого пользователя `user_id` и сохраняет оценку в таблицу `epic_scores` через `repo.CreateEpicScore`, затем запускает каскадный пересчет `TryCompleteEpicScoring`.
* **`POST /api/gantt/admin/scores/risk`** — запись оценки риска от лица участника:
  * Доступ: только роли `admin` или `superadmin`.
  * Request Body: `{ "risk_id": "uuid", "user_id": "uuid", "probability": int, "impact": int }`.
  * Поведение: сохраняет оценку в таблицу `risk_scores` через `repo.CreateRiskScore` и запускает каскадный пересчет `TryCompleteRiskScoring`.

### 2. Расширение существующих GET-эндпоинтов
* **`GET /api/gantt/epics/{epic_id}/scores`** (хэндлер `GetEpicScores`):
  * Добавить в JSON-ответ массив `members` — список всех участников команды, привязанной к эпику.
  * Для каждого участника возвращать: `user_id`, `first_name`, `last_name`, `role_id`, `role_name`, а также `score` (текущая оценка за данный эпик, если она проставлена, иначе `null`).
* **`GET /api/gantt/epics/{epic_id}/risks`** (хэндлер `GetEpicRisks`):
  * Для каждого возвращаемого риска добавить поле `scores` — массив сырых оценок участников за данный риск (содержит `user_id`, `probability`, `impact`).

### 3. Тестирование
* Написать unit-тесты в `internal/transport/httpServer/handlers/admin_scores_test.go` для покрытия новых эндпоинтов и проверки разграничения прав (для не-админов должен возвращаться статус `403 Forbidden`).

---

## Шаг 7. Изменение диапазона оценок эпика (0 - 500)

Изменить логику валидации оценок эпиков на бэкенде:
1. В хэндлере `SubmitEpicScore` в `internal/transport/httpServer/handlers/scoring.go` заменить проверку `req.Score <= 0` на `req.Score < 0 || req.Score > 500`. При некорректном значении возвращать `400 Bad Request` с текстом `"score must be between 0 and 500"`.
2. В хэндлере `AdminSubmitEpicScore` в `internal/transport/httpServer/handlers/admin_scores.go` заменить аналогичную проверку `req.Score <= 0` на `req.Score < 0 || req.Score > 500` и возвращать ошибку с тем же текстом.
3. Обновить unit-тесты в `internal/transport/httpServer/handlers/admin_scores_test.go` и других местах, чтобы проверить граничные значения `0` и `500` как валидные, а отрицательные и превышающие `500` — как некорректные.


