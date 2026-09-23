# UX-постановка: экран «Доступ ограничен» различает причину отказа

Задача 4.1 заявки `fix-webapp-user-identity`. Реализует субагент `frontend` (секция 5).

Метод: чтение разметки, стилей и кода. Браузерная проверка не выполнялась (MCP `claude-in-chrome` не подключён), поэтому утверждения о визуальном результате и порядке фокуса выведены из разметки и токенов, а не из скриншота.

## Находки

**P1 — `web/gantt/js/api.js:47`: ветка 403 теряет `errData`.** `handleHttpError` бросает `new Error('FORBIDDEN')` без `.code`/`.message`, хотя `errData` уже получен и ниже по файлу (строки 55–56) разбирается — но до той ветки исполнение не доходит. Пока это не починено, подставлять в разметку нечего: коды `USERNAME_REQUIRED`/`USER_NOT_REGISTERED` и их тексты не долетают до DOM.

**P2 — `web/gantt/index.html:51–61`: одна причина отказа на весь экран, хотя их теперь две с разной природой.** `USERNAME_REQUIRED` (человек чинит сам за минуту в настройках Telegram) и `USER_NOT_REGISTERED` (чинит только администратор) сейчас неотличимы: тот же значок 🚫, тот же заголовок, тот же текст. Пользователь без username воспринимает это как окончательный запрет и либо бросает попытки, либо идёт в поддержку с вопросом, который закрыл бы сам за полминуты.

**P2 — `web/gantt/index.html:52–53`: цвет мимо токенов.** `style="border-color: #ef4444"` и `style="color: #ef4444"` — хардкод при живом `--color-danger: #ef4444` в `variables.css:25`. Раз экран всё равно переделывается, хардкод не тащить в новое состояние.

**P2 — `#denied-overlay` не размечен для скринридера, фокус не переводится.** Ни одного `role`/`aria-*`, заголовок не связан с текстом, при появлении оверлея фокус остаётся на `body`. Полноэкранное состояние, заменяющее всё приложение, происходит для экранного диктора беззвучно. Это тот же системный пробел, что в бэклоге по `.modal`, но на `div.auth-overlay`, не входящем в те 18 модалок.

**P3 — `index.html:56`: вторая строка подразумевает только сценарий «ждать администратора».** «Если доступ вам уже выдали, обновите проверку» для `USERNAME_REQUIRED` просто ложна — ждать нечего, действие за пользователем.

## Постановка

### Файлы

- `web/gantt/index.html` — разметка `#denied-overlay` (строки 51–61)
- `web/gantt/css/base.css` — блок `.auth-card` (~строки 96–107)
- `web/gantt/js/api.js` — `handleHttpError`, ветка `status === 403` (строки 35–53)

**Не трогать:** `web/gantt/js/auth.js` (обработчики `denied-btn-refresh`/`denied-btn-logout`, `checkAuth`, `showAuth`/`showApp`/`showConnectionError` — логика вызовов не меняется, меняется только видимое содержимое); `id` кнопок `denied-btn-refresh`/`denied-btn-logout`; `#connection-error-overlay` и `#auth-overlay`.

### 1. `index.html` — разметка `#denied-overlay`

Структура с id-якорями и aria; дефолтное содержимое — **дословно** текущий текст (он же состояние-фолбэк, когда сервер не прислал код):

```html
<div id="denied-overlay" class="auth-overlay hidden" role="alertdialog" aria-modal="true"
     aria-labelledby="denied-title" aria-describedby="denied-message denied-hint">
    <div class="auth-card auth-card--danger" id="denied-card">
        <div class="auth-logo" id="denied-icon">🚫</div>
        <h2 id="denied-title" tabindex="-1">Доступ ограничен</h2>
        <p id="denied-message">Вы не являетесь участником ни одной из зарегистрированных команд и не зарегистрированы как администратор.</p>
        <p id="denied-hint">Если доступ вам уже выдали, обновите проверку.</p>
        <div class="auth-overlay-actions">
            <button type="button" id="denied-btn-refresh" class="btn btn-primary">Обновить</button>
            <button type="button" id="denied-btn-logout" class="btn btn-secondary">Выйти</button>
        </div>
    </div>
</div>
```

Инлайновые `style="border-color:#ef4444"` / `style="color:#ef4444"` убрать — заменены модификатором класса. `tabindex="-1"` на `<h2>` нужен, чтобы переводить в него фокус программно, не делая заголовок частью Tab-порядка.

### 2. `css/base.css` — модификаторы `.auth-card` вместо инлайн-стилей

Рядом с существующим блоком `.auth-card`:

```css
.auth-card--danger { border-color: var(--color-danger); }
.auth-card--danger .auth-logo { color: var(--color-danger); }

.auth-card--warning { border-color: var(--color-warning); }
.auth-card--warning .auth-logo { color: var(--color-warning); }
```

Используются существующие токены `--color-danger` (`#ef4444`) и `--color-warning` (`#f59e0b`) из `variables.css`. Новых значений не вводить.

### 3. `js/api.js` — заполнение содержимого по коду ошибки

В ветке `status === 403`, где `appEl` скрыт (строки 41–50), перед `throw`:

1. Прочитать `const code = errData?.error?.code || ''` и `const message = errData?.error?.message || ''`.
2. Заполнить оверлей по `code` — **ровно два варианта**, всё прочее (пустой код, незнакомый код, пустой `message`) трактуется как вариант по умолчанию.

**`USERNAME_REQUIRED`:**

| Элемент | Значение |
|---|---|
| `#denied-card` | класс `auth-card--warning` вместо `auth-card--danger` |
| `#denied-icon` | `🆔` |
| `#denied-title` | `Укажите @username в Telegram` |
| `#denied-message` | `message`, если непустой; иначе `У вас не задан @username в Telegram. Без него мы не можем найти вас в списке участников.` |
| `#denied-hint` | `Откройте Telegram → Настройки → укажите @username, затем вернитесь сюда и нажмите «Проверить снова».` |
| `#denied-btn-refresh` | `Проверить снова` |
| `#denied-btn-logout` | `Выйти` (без изменений) |

**По умолчанию** (`USER_NOT_REGISTERED`, пустой код, незнакомый код, пустой `message`):

| Элемент | Значение |
|---|---|
| `#denied-card` | класс `auth-card--danger` (текущий) |
| `#denied-icon` | `🚫` |
| `#denied-title` | `Доступ ограничен` |
| `#denied-message` | `message`, если непустой; иначе дословно прежний текст: `Вы не являетесь участником ни одной из зарегистрированных команд и не зарегистрированы как администратор.` |
| `#denied-hint` | `Если доступ вам уже выдали, обновите проверку.` |
| `#denied-btn-refresh` | `Обновить` (без изменений) |
| `#denied-btn-logout` | `Выйти` (без изменений) |

Текст пишется через `.textContent`, **не** через `innerHTML` и не шаблонной строкой: серверное `message` не экранируется и не проверяется на разметку (см. спеку `web-content-escaping`), а `textContent` рендерит его буквально в любом случае, поэтому отдельный `escapeHtml` не нужен. Тот же приём уже применён в `utils.js` для `showToast`.

3. После заполнения — `document.getElementById('denied-title').focus()`, чтобы скринридер объявил новый заголовок как активный диалог и фокус клавиатуры не оставался на `body`. **`aria-live` не добавлять** — вместе с переводом фокуса даст двойное объявление.
4. Бросаемая ошибка получает `.code`/`.message`, как в общей ветке ниже по файлу (строки 58–60), вместо голого `new Error('FORBIDDEN')`:

```js
const error = new Error(message || 'FORBIDDEN');
error.code = code || 'UNKNOWN_ERROR';
error.status = 403;
throw error;
```

`auth.js` этот `error.code`/`.message` не читает и трогать его не нужно: `checkAuth()` при `e.status === 403` просто прячет loading-оверлей, а сам оверлей уже отрисован в `api.js`.

### 4. Esc и фокус-ловушка — сознательно не добавляются

`#denied-overlay` — не слой поверх видимого контента (`#app` в этот момент `display:none`), а состояние «вместо приложения»: закрывать его нечем, кроме действия «Обновить»/«Выйти». Закрытие по `Escape` оставило бы пустую нефункциональную страницу — это был бы новый баг, а не доступность. Требование `web-modal-accessibility` про Esc и фокус-ловушку писано под `.modal`, наслаивающиеся на видимый контент, и сюда неприменимо. Зафиксировано явно, чтобы не выглядело упущением.

## Критерии приёмки

1. При 403 на `/profile` с телом `{"error":{"code":"USERNAME_REQUIRED","message":"…"}}` оверлей показывает иконку 🆔, рамку в цвете `--color-warning`, заголовок «Укажите @username в Telegram», основным текстом — ровно `message` из ответа, подсказку про Telegram → Настройки → «Проверить снова», кнопку `denied-btn-refresh` с текстом «Проверить снова».
2. При 403 с `USER_NOT_REGISTERED` (либо любым другим или пустым кодом, либо пустым `message`) содержимое побайтово совпадает с нынешним: 🚫, рамка `--color-danger`, «Доступ ограничен», текст по умолчанию (при пустом `message`) или из ответа, кнопка «Обновить».
3. Ни в одном состоянии `message` от сервера не попадает в DOM как разметка: `<`, `>`, `&` видны как текст.
4. Брошенная в `handleHttpError` ошибка при 403 имеет непустые `.code` и `.message`.
5. При появлении оверлея фокус находится на `#denied-title`; `Tab`/`Shift+Tab` достигает обеих кнопок и не уходит на элементы `#app`.
6. `Escape` не закрывает `#denied-overlay`.
7. Повторный клик «Обновить»/«Проверить снова» при том же коде перерисовывает содержимое (включая повторный перевод фокуса), а не оставляет разметку от предыдущего вызова.
8. `id` и обработчики `denied-btn-refresh`/`denied-btn-logout`, а также `#connection-error-overlay`/`#auth-overlay` не изменены.

## Вне объёма

- **Автоматическая повторная проверка при возврате в приложение** (`visibilitychange`/`focus` → `checkAuth()`), чтобы не требовать ручного клика после смены username в настройках Telegram. Реальное улучшение сценария, но это поведенческое расширение, а не подача текста — отдельной задачей.
- **Перевод `#denied-overlay`/`#auth-overlay`/`#connection-error-overlay` на `<dialog>`** с переиспользованием `openModal`/`closeModal` (`utils.js:276–297`, там уже есть нативный фокус-трап и Esc). Унифицировало бы доступность всех auth-экранов одним ходом, но это редизайн экранов авторизации, который заявка прямо просила не трогать.
- Точные тексты сообщений бэкенда для `USERNAME_REQUIRED`/`USER_NOT_REGISTERED` — ответственность задачи 3.1; здесь заданы только дефолты на случай пустого `message`.
