# UX-постановка: thinner-gantt-labels (задача 1.1)

## Находки

**П1 (подтверждение).** design.md Решение 3 верно: копия-контур в `gantt-renderer.js:854-867` берёт `font-weight` из `getComputedStyle(label).fontWeight` (строка 866), `stroke-width` инлайном не задаёт — он приходит только из класса `.bar-label-outline`. Правка JS не нужна.

**П2. Вес вынесенной подписи (`.bar-label.big`) эпика/истории держится на порядке правил.** `.gantt .bar-label.big` и `.gantt .gantt-epic .bar-label` / `.gantt .gantt-story .bar-label` имеют одинаковую специфичность (0,3,0); побеждает более позднее правило. Сейчас правила уровней стоят после `.big` — поэтому вынесенная подпись эпика получает вес эпика. Порядок блоков обязательно сохранить и в `gantt.css`, и в `buildStyleText()`.

**П3 (информационная).** Базовое правило Frappe `.gantt .bar-label { … font-weight: 400 }` в `buildStyleText()` (`gantt-image-export.js:416`) переопределяется ниже, не трогать.

## Постановка для frontend

### Токены — `web/gantt/css/gantt.css`, блок `:root` с `--g-*`

```css
--gantt-label-weight: 400;
--gantt-label-weight-epic: 500;
--gantt-label-outline-width: 2px;
```

### `gantt.css`

| Селектор | Было | Стало |
|---|---|---|
| `.gantt .bar-label` | `font-weight: 600` | `var(--gantt-label-weight)` |
| `.gantt .bar-label.big` | `font-weight: 600` | `var(--gantt-label-weight)` |
| `.gantt .bar-label-outline` | `stroke-width: 3px` | `var(--gantt-label-outline-width)` |
| `.gantt .gantt-epic .bar-label` | `font-weight: 700` | `var(--gantt-label-weight-epic)`, `font-size: 13px` без изменений |
| `.gantt .gantt-story .bar-label` | `font-weight: 600` | `var(--gantt-label-weight)`, `font-size: 12px` без изменений |

### `gantt-image-export.js`

- `TOKEN_NAMES`: добавить `'--gantt-label-weight'`, `'--gantt-label-weight-epic'`, `'--gantt-label-outline-width'`.
- `buildStyleText()`: те же пять замен в строках приложения (`.bar-label`, `.bar-label.big`, `.bar-label-outline`, `.gantt-epic .bar-label`, `.gantt-story .bar-label`); базовое правило Frappe не трогать; порядок строк сохранить.

### Не трогать

`gantt-renderer.js`; приём «две копии текста»; инлайн `fill`/`stroke`-цвета; `font-size`; базовое правило Frappe в экспорте; комментарии-истории (можно дописать фразу про токены); подключение шрифтов (Inter 400/500 уже загружены).

### Приёмка — статика

1. `grep -nE 'font-weight: (600|700)' web/gantt/css/gantt.css` не находит правил подписей баров.
2. `grep -nE 'bar-label.*font-weight: (600|700)|bar-label-outline.*3px' web/gantt/js/gantt-image-export.js` пуст.
3. DevTools: computed `font-weight` подписи задачи и истории — 400, эпика — 500; `stroke-width` контура — 2px, в том числе у вынесенной подписи.
4. Вынесенная подпись эпика (`.bar-label.big` в `.gantt-epic`) — computed `font-weight` 500.
5. В SVG экспорта нет неразвёрнутых `var(--gantt-label-*)`.

### Приёмка — ручная (раздел 3 tasks.md)

**Где:** диаграмма с задачами всех шести ролей, историями и эпиками, включая хотя бы один эпик/историю с подписью, вынесенной за бар.

**Окружения:** десктопный браузер; мобильный Telegram WebApp (iOS и Android, если доступны; недоступная платформа помечается как непроверенная).

**Провал читаемости:** подпись сливается с заливкой бара (особенно на светлых барах эпика/истории); подпись эпика неотличима по толщине от истории/задачи; в Telegram сглаживание делает текст рваным.

**Откат, по шагу с перепроверкой:** (1) `--gantt-label-outline-width: 3px`; (2) `--gantt-label-weight: 500`; (3) итог и причину записать в design.md, при отсутствии проблем — пометка «не потребовалось».

**Экспорт (PNG и SVG):** подпись эпика заметно жирнее истории. У Helvetica/Arial нет начертания 500 — если эпик сольётся с историей, поднять `--gantt-label-weight-epic` до 600, перепроверить экран (не стал ли снова «тяжёлым») и картинку, записать в design.md.
