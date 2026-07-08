# Task: Frontend-субагент — Веб-интерфейс EpicScoreBot

## Роль

Ты — **Frontend-разработчик** веб-интерфейса EpicScoreBot. Текущий фронтенд — Gantt-диаграмма на **Vanilla JS/CSS/HTML**, авторизация через **Telegram WebApp/Login Widget**. Статика раздаётся Go-бэкендом с `web/gantt/`.

---

## Необходимые Skills

Перед началом работы **обязательно прочитай** SKILL.md для каждого из указанных skills:

| Skill | Путь | Назначение |
|-------|------|------------|
| `frontend-design` | `~/.gemini/antigravity-cli/skills/frontend-design/SKILL.md` | Проектирование UI, дизайн-системы, визуальная архитектура |
| `frontend-developer` | `~/.gemini/antigravity-cli/skills/frontend-developer/SKILL.md` | Лучшие практики фронтенд-разработки, компоненты, state management |
| `frontend-architecture` | `~/.gemini/antigravity-cli/skills/frontend-architecture/SKILL.md` | Архитектура фронтенда — модульность, разделение ответственности |
| `api-design-principles` | `~/.gemini/antigravity-cli/skills/api-design-principles/SKILL.md` | Понимание API-контрактов для интеграции с бэкендом |
| `e2e-testing` | `~/.gemini/antigravity-cli/skills/e2e-testing/SKILL.md` | E2E тестирование (Playwright) для проверки UI |
| `code-review-and-quality` | `~/.gemini/antigravity-cli/skills/code-review-and-quality/SKILL.md` | Качество кода |
| `clean-code` | `~/.gemini/antigravity-cli/skills/clean-code/SKILL.md` | Чистый код |

> [!NOTE]
> Skills `frontend-developer` и `frontend-architecture` ориентированы на React/Next.js. Адаптируй принципы и паттерны к Vanilla JS контексту проекта. Если принято решение мигрировать на фреймворк — согласуй с Архитектором.

---

## Текущая структура фронтенда

```
web/
└── gantt/
    ├── index.html    # Главная страница Gantt UI (5.3 KB)
    ├── style.css     # Стили (16.1 KB)
    ├── app.js        # Основная логика (16.8 KB)
    └── test.js       # Заглушка тестов (20 B)
```

### Текущий стек

- **HTML5** — семантическая вёрстка
- **Vanilla CSS** — custom properties, flexbox/grid
- **Vanilla JS** — модульный ES6+, Fetch API
- **Авторизация** — Telegram Login Widget + WebApp `initData`

---

## API-контракты для фронтенда

### Авторизация

```
POST /api/gantt/auth/webapp
  Body: { "initData": "<Telegram WebApp initData string>" }
  Response: { "token": "...", "user": { "id": "uuid", "first_name": "...", "telegram_id": "..." } }

GET /api/gantt/auth?id=...&first_name=...&hash=...
  → Telegram Login Widget callback (redirect-based)
```

### Данные Gantt

```
GET /api/gantt/teams
  Headers: Authorization: Bearer <token>
  Response: [
    { "id": "uuid", "name": "Team Alpha", "description": "..." }
  ]

GET /api/gantt/epics?team_id=<uuid>
  Headers: Authorization: Bearer <token>
  Response: [
    {
      "id": "uuid", "number": "EPIC-42", "name": "...",
      "final_score": 34.0, "status": "SCORED"
    }
  ]

GET /api/gantt/tasks?team_id=<uuid>
  Headers: Authorization: Bearer <token>
  Response: [
    {
      "id": "uuid", "epic_id": "uuid", "role_id": "uuid|null",
      "name": "EPIC-42: Интеграция",
      "start_date": "2026-07-10T00:00:00Z",
      "end_date": "2026-07-25T00:00:00Z",
      "progress": 0.35, "sort_order": 1,
      "is_parent": true, "parent_task_id": null
    }
  ]

POST /api/gantt/tasks/generate
  Headers: Authorization: Bearer <token>
  Body: { "epic_id": "uuid", "start_date": "2026-07-10" }
  Response: [ ...массив GanttTask... ]

PUT /api/gantt/tasks/{id}/
  Headers: Authorization: Bearer <token>
  Body: { "start_date": "2026-07-12T00:00:00Z", "end_date": "2026-07-20T00:00:00Z" }
  Response: { "status": "ok" }

PUT /api/gantt/tasks/{id}/reorder
  Headers: Authorization: Bearer <token>
  Body: { "new_sort_order": 3 }
  Response: [ ...обновлённый массив GanttTask с новым порядком... ]

DELETE /api/gantt/tasks/{id}/
  Headers: Authorization: Bearer <token>
  Response: { "status": "ok" }
```

### Ошибки API

```json
// Все ошибки в едином формате:
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid or expired token"
  }
}

// HTTP Status Codes:
// 400 — Bad Request (невалидные данные)
// 401 — Unauthorized (невалидный/просроченный токен)
// 403 — Forbidden (нет доступа к ресурсу)
// 404 — Not Found (сущность не найдена)
// 409 — Conflict (дубликат)
// 500 — Internal Server Error
```

---

## Шаги реализации

### Шаг 1. Аудит текущего кода

1. Изучить `web/gantt/app.js` — понять текущую архитектуру:
   - Как организован state management
   - Как работает Telegram авторизация
   - Как рендерится Gantt-диаграмма
   - Какие API-вызовы используются
2. Изучить `web/gantt/style.css` — определить дизайн-систему:
   - CSS custom properties (цвета, spacing, typography)
   - Breakpoints для responsive
3. Изучить `web/gantt/index.html` — семантика и структура

### Шаг 2. Рефакторинг архитектуры

1. **Разделить `app.js`** на модули:
   ```
   web/gantt/
   ├── js/
   │   ├── app.js           # Entry point, инициализация
   │   ├── api.js            # HTTP-клиент, все вызовы API
   │   ├── auth.js           # Telegram авторизация
   │   ├── state.js          # Централизованный state management
   │   ├── gantt-renderer.js # Рендеринг Gantt-диаграммы
   │   ├── task-editor.js    # Модальные окна, формы задач
   │   └── utils.js          # Утилиты (даты, форматирование)
   ├── css/
   │   ├── variables.css     # CSS custom properties (дизайн-токены)
   │   ├── base.css          # Reset, typography, base styles
   │   ├── gantt.css         # Стили Gantt-диаграммы
   │   ├── components.css    # Кнопки, формы, модалки
   │   └── responsive.css    # Media queries
   ├── index.html
   └── test.js
   ```

2. **Паттерн модулей**: ES Modules (`type="module"` в HTML) без сборщика

### Шаг 3. Улучшение UX Gantt-диаграммы

1. **Drag & drop** для изменения дат задач (перетаскивание баров)
2. **Drag & drop** для reorder (перетаскивание строк)
3. **Progress bar** с визуальным заполнением
4. **Zoom** — переключение между днями/неделями/месяцами
5. **Tooltip** при наведении на задачу (детали: score, роль, даты)
6. **Color coding** по ролям (BE, FE, Mobile, QA, Analyst, IT-leader)
7. **Responsive** — адаптация под мобильные устройства Telegram WebApp

### Шаг 4. Новые страницы (планируемые)

#### Дашборд оценки (`/scoring`)

```
Отображает:
- Список эпиков с их статусами (NEW → SCORING → SCORED)
- Progress bar: сколько участников оценили / всего
- Итоговые оценки по ролям (таблица)
- Риски эпика со статусами и коэффициентами
```

API для дашборда (предоставляется Backend-субагентом):
```
GET /api/epics?team_id=<uuid>&status=SCORING
GET /api/epics/{id}/scores
GET /api/epics/{id}/role-scores
GET /api/epics/{id}/risks
GET /api/risks/{id}/scores
```

#### Отчёт по команде (`/report`)

```
Отображает:
- Сводная таблица: все SCORED эпики команды
- Графики: распределение оценок, risk heatmap
- Кнопка «Скачать PDF» → GET /api/teams/{id}/report
```

### Шаг 5. Дизайн-система

Определить и задокументировать CSS-токены:

```css
:root {
  /* Цвета */
  --color-primary: #4F46E5;
  --color-primary-hover: #4338CA;
  --color-success: #22C55E;
  --color-warning: #F59E0B;
  --color-danger: #EF4444;
  --color-bg: #0F172A;
  --color-surface: #1E293B;
  --color-text: #F1F5F9;
  --color-text-muted: #94A3B8;
  --color-border: #334155;

  /* Роли (цвет-код для Gantt баров) */
  --color-role-it-leader: #8B5CF6;
  --color-role-analyst: #3B82F6;
  --color-role-be: #10B981;
  --color-role-fe: #F59E0B;
  --color-role-mobile: #EC4899;
  --color-role-qa: #6366F1;

  /* Typography */
  --font-family: 'Inter', system-ui, sans-serif;
  --font-size-sm: 0.875rem;
  --font-size-base: 1rem;
  --font-size-lg: 1.25rem;
  --font-size-xl: 1.5rem;

  /* Spacing */
  --space-xs: 0.25rem;
  --space-sm: 0.5rem;
  --space-md: 1rem;
  --space-lg: 1.5rem;
  --space-xl: 2rem;

  /* Borders */
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 12px;
}
```

### Шаг 6. Тестирование

1. **Unit-тесты** для модулей `state.js`, `utils.js` (vanilla JS)
2. **E2E-тесты** через Playwright:
   - Авторизация через мок Telegram
   - Загрузка и отображение Gantt-диаграммы
   - Drag & drop задач
   - Генерация задач для эпика
3. **Визуальная регрессия** — скриншоты ключевых состояний

---

## Требования к качеству

1. **Accessibility**: семантический HTML, ARIA-атрибуты, keyboard navigation
2. **Performance**: ленивая загрузка, виртуализация для больших списков, requestAnimationFrame для анимаций
3. **Progressive Enhancement**: базовая функциональность без JS (static HTML fallback)
4. **Error Handling**: user-friendly сообщения об ошибках, retry для сетевых ошибок
5. **Telegram WebApp**: корректная работа внутри Telegram (viewport, тема, кнопки)

---

## Зависимости от других субагентов

| Зависимость | Описание |
|-------------|----------|
| **Backend** | API-контракты, формат ответов, авторизация. При добавлении новых эндпоинтов — согласовать JSON-схему |
| **DevOps** | Статика раздаётся из `web/gantt/` внутри Docker-контейнера бэкенда. Nginx Proxy Manager проксирует на порт 8080 |

---

## Доработки и Багфиксы (Выполненные Архитектором и переданные на сопровождение)

1. **Багфикс: Заполнение чекбоксов ролей**
   - Добавлен асинхронный метод `loadRoles()` и вызов в `handleProfileLoaded` для своевременной загрузки ролей с сервера и рендеринга чекбокс-списков ролей.
2. **Багфикс: Безопасная инициализация модулей**
   - Блок инициализации модулей фронтенда в `app.js` обернут в безопасный цикл `try-catch` с логированием, чтобы сбои одного модуля не блокировали инициализацию остальных.
3. **Отладка панели администратора**
   - Добавлено логирование жизненного цикла в консоль браузера в `admin-panel.js`.
4. **Улучшение контрастности чекбоксов**
   - Стандартные чекбоксы браузера в `components.css` стилизованы под кастомные элементы с неоновым свечением при наведении и фокусе, а при выборе — ярко-фиолетовой заливкой с белой галочкой.

