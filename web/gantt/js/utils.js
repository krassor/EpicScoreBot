// ── Utilities Module ─────────────────────────────────────────────────

export function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;

    // Add appropriate icon prefix
    let icon = 'ℹ️ ';
    if (type === 'success') icon = '✅ ';
    if (type === 'error') icon = '❌ ';

    toast.textContent = icon + message;

    // Контейнер объявляется программами экранного доступа как role="status"/
    // aria-live="polite" (index.html). Для ошибок это слишком слабое объявление —
    // усиливаем непосредственно на тосте (web-async-feedback, «Уведомления доступны
    // программам экранного доступа»).
    if (type === 'error') {
        toast.setAttribute('role', 'alert');
        toast.setAttribute('aria-live', 'assertive');
    }

    container.appendChild(toast);
    
    // Animate and remove
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(20px)';
        setTimeout(() => toast.remove(), 250);
    }, 4000);
}

// Карта известных серверных сообщений об ошибках на понятный пользователю русский текст.
// Ключи — точные строки из internal/transport/httpServer/handlers/*.go (writeError/
// writeErrorCode), сверенные вручную grep'ом по errors.New(/fmt.Errorf( — придуманных
// строк здесь нет. Сообщения с параметрами в тексте (например, "epic already in
// status %s") сюда не попадают: точное совпадение по ключу для них невозможно,
// такие сообщения показываются пользователю как есть (см. design.md, Decision 6).
export const KNOWN_ERROR_MESSAGES = {
    'user has no role assigned': 'Вам не назначена роль в системе, поэтому вы не можете голосовать. Обратитесь к администратору.',
    'user not registered in system': 'Вы не зарегистрированы в системе как участник. Обратитесь к администратору.',
    'user not registered': 'Вы не зарегистрированы в системе как участник. Обратитесь к администратору.',
    'forbidden': 'Недостаточно прав для этого действия.',
    'access denied': 'Недостаточно прав для этого действия.',
    // internal/transport/httpServer/handlers/admin.go — ограничение прав team-admin:
    // команда, к которой обращается запрос, не входит в зону ответственности
    // текущего team-admin (см. h.teamsInAdminScope).
    'team_id outside of admin scope': 'Эта команда находится за пределами ваших полномочий администратора.',
    // internal/transport/httpServer/handlers/{scoring,admin_scores}.go — диапазон оценки
    'score must be between 0 and 500': 'Оценка должна быть в диапазоне от 0 до 500 человеко-дней.',
    'score must be >= 0': 'Оценка должна быть не меньше 0.',
    'final_score must be >= 0': 'Итоговая оценка должна быть не меньше 0.',
    'probability and impact must be between 1 and 4': 'Вероятность и влияние риска должны быть в диапазоне от 1 до 4.',
    // internal/transport/httpServer/handlers/admin.go — вес оценивающей роли эпика
    'weight must be between 0 and 100': 'Вес должен быть в диапазоне от 0 до 100.',
    // internal/services/epic.go — недопустимый статус сущности для запрошенного действия
    'epic must have at least one story': 'У эпика нет ни одной истории — оценку запустить нельзя.',
    'cannot start scoring of a story directly, start parent epic scoring': 'Запустить оценку истории напрямую нельзя — оценка запускается для родительского эпика.',
    'cannot delete story of an epic that is already in progress or scored': 'Нельзя удалить историю эпика, который уже находится на оценке или оценён.',
    // internal/transport/httpServer/handlers/schedule_settings.go — настройка
    // расписания команды (backfill-idle-gaps-in-schedule, ux-brief.md раздел 6)
    'team not found': 'Команда не найдена. Возможно, её удалили. Обновите страницу.',
    'failed to update schedule settings': 'Не удалось сохранить настройку. Попробуйте ещё раз.',
    // internal/transport/httpServer/handlers/gantt.go — SetTaskStartConstraint
    // (add-task-start-constraints, ux-brief.md раздел 7). Остальные пять кодов
    // ошибок этого эндпоинта сервер уже отдаёт по-русски, сюда добавлять не надо.
    'invalid not_before_date, expected YYYY-MM-DD': 'Дата указана в неверном формате.',
    'failed to set task start constraint': 'Не удалось сохранить ограничение. Попробуйте ещё раз.',
};

// Причины ошибок, которые требуют осознанного действия пользователя (не просто
// «повторить запрос»): отсутствие прав, отсутствие роли, нарушенное бизнес-
// ограничение. Для них handleApiError показывает модалку независимо от флага
// blocking — тост слишком легко пропустить, а без объяснения пользователь не
// поймёт, что делать дальше (design.md, Decision 6).
const ACTION_REQUIRED_ERRORS = new Set([
    'forbidden',
    'access denied',
    'user has no role assigned',
    'user not registered in system',
    'user not registered',
    'team_id outside of admin scope',
    'epic must have at least one story',
    'cannot start scoring of a story directly, start parent epic scoring',
    'cannot delete story of an epic that is already in progress or scored',
    // internal/transport/httpServer/handlers/schedule_settings.go — отказ по team-scoped
    // проверке администратора настройки расписания (ux-brief.md раздел 6): сервер уже
    // отдаёт эти строки по-русски, KNOWN_ERROR_MESSAGES для них не нужен, но без включения
    // сюда handleApiError показал бы их тостом, а не блокирующей модалкой.
    'только администратор команды может изменять настройки расписания',
    'вы не администратор этой команды',
]);

// showErrorModal открывается программно из catch-блоков асинхронных операций
// (design.md, Decision 3) — единственная модалка из списка 1.2, у которой нет
// прямого клика-инициатора. Элементом-инициатором по умолчанию берётся
// document.activeElement на момент вызова: в большинстве случаев это тот же
// элемент, с которого пользователь запустил упавшую операцию (клик не успел
// сместить фокус за время await). Вызывающий код может передать инициатор
// явно вторым аргументом, если это не так.
export function showErrorModal(message, initiator = document.activeElement) {
    const text = KNOWN_ERROR_MESSAGES[message] || message;

    let modal = document.getElementById('modal-error');
    if (!modal) {
        modal = document.createElement('dialog');
        modal.id = 'modal-error';
        modal.className = 'modal';
        modal.setAttribute('aria-labelledby', 'modal-error-title');
        document.body.appendChild(modal);
        // Клик по фону (области dialog за пределами .modal-content) закрывает
        // модалку — замена клика по .modal-overlay при переходе на <dialog>.
        modal.addEventListener('click', (e) => {
            if (!e.target.closest('.modal-content')) closeModal(modal);
        });
    }

    modal.innerHTML = `
        <div class="modal-content" style="max-width: 420px;">
            <div class="modal-header">
                <h2 id="modal-error-title">Не удалось выполнить действие</h2>
            </div>
            <div class="modal-body">
                <p>${escapeHtml(text)}</p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-primary" id="modal-error-close">Понятно</button>
            </div>
        </div>
    `;

    openModal(modal, initiator);

    modal.querySelector('#modal-error-close').onclick = () => closeModal(modal);
}

export function formatDate(date) {
    if (!date) return '';
    const d = new Date(date);
    if (isNaN(d.getTime())) return '';
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

// escapeHtml — экранирует пользовательский текст перед подстановкой в шаблонные
// строки, которые затем присваиваются innerHTML (design.md, Decision 5). Порядок
// замен важен: & заменяется первым, иначе амперсанды, появившиеся при замене
// других символов (&lt; и т.п.), были бы заэкранированы повторно.
export function escapeHtml(text) {
    if (text === null || text === undefined) return '';
    return String(text)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

// withSubmitLock — блокирует submit-кнопку формы на время выполнения asyncFn,
// снимает блокировку в finally независимо от результата (design.md, Decision 7).
// Заменяет собой отдельные ручные реализации блокировки, разбросанные по app.js /
// scoring-panel.js / gantt-renderer.js, единым паттерном.
export async function withSubmitLock(target, asyncFn) {
    const btns = resolveLockTargets(target);
    btns.forEach(b => { b.disabled = true; });
    try {
        return await asyncFn();
    } finally {
        btns.forEach(b => { b.disabled = false; });
    }
}

// Приводит аргумент withSubmitLock к списку блокируемых кнопок.
// Принимает <form> (ищет её submit-кнопку), одну кнопку или список кнопок —
// в интерфейсе есть и формы, и модалки с парой кнопок вне <form>.
function resolveLockTargets(target) {
    if (!target) return [];
    if (Array.isArray(target)) return target.filter(Boolean);
    if (target instanceof HTMLFormElement) {
        const btn = target.querySelector('button[type="submit"], input[type="submit"]');
        return btn ? [btn] : [];
    }
    return [target];
}

// handleApiError — единая точка показа ошибки API (design.md, Decision 6).
// Сообщение прогоняется через KNOWN_ERROR_MESSAGES; канал выбирается так:
//   - showErrorModal — если вызывающий код явно попросил (options.blocking)
//     либо причина ошибки требует осознанного действия пользователя
//     (нет прав, не назначена роль, нарушено бизнес-ограничение);
//   - showToast(..., 'error') — во всех остальных случаях.
// options.title, если передан, добавляется как префикс тоста (сохраняет
// привычный вид существующих сообщений вида "Не удалось создать команду: ...").
// showErrorModal/showToast не меняются — это низкоуровневые примитивы.
export function handleApiError(err, options = {}) {
    const { title, blocking } = options;
    const rawMessage = err?.message || String(err);

    if (blocking || ACTION_REQUIRED_ERRORS.has(rawMessage)) {
        showErrorModal(rawMessage);
        return;
    }

    const knownMessage = KNOWN_ERROR_MESSAGES[rawMessage];
    const text = knownMessage || rawMessage;
    showToast(title ? `${title}: ${text}` : text, 'error');
}

// renderTableState — общий рендер состояний loading/empty/error для контейнеров
// таблиц (design.md, Decision 8). Рендерит НАПРЯМУЮ в переданный контейнер, без
// участия state.subscribe — это принципиально: при ошибке запроса state.set не
// вызывается, поэтому подписка никогда не сработает и таблица осталась бы в
// состоянии "Загрузка..." навсегда, если бы рендер шёл через неё.
//
// options:
//   emptyText — текст для состояния 'empty' (общего текста нет: пустое
//               состояние всегда должно быть конкретным для места использования);
//   errorText — текст для состояния 'error' (по умолчанию — общая формулировка
//               "Не удалось загрузить данные."; переопределяется, когда для места
//               использования есть более конкретный текст, см. секцию 1.3);
//   onRetry   — обработчик клика по кнопке «Повторить» в состоянии 'error';
//   colspan   — если контейнер является <tbody>/<tr>-родителем, оборачивает
//               содержимое в <tr><td colspan="..."> для валидной разметки таблицы.
export function renderTableState(container, state, options = {}) {
    if (!container) return;
    const { emptyText = '', errorText = 'Не удалось загрузить данные.', onRetry, colspan } = options;

    let inner;
    if (state === 'loading') {
        inner = `
            <div class="table-state table-state-loading">
                <div class="loader"></div>
                <p>Загрузка…</p>
            </div>
        `;
    } else if (state === 'empty') {
        inner = `
            <div class="table-state table-state-empty">
                <p>${emptyText}</p>
            </div>
        `;
    } else if (state === 'error') {
        inner = `
            <div class="table-state table-state-error">
                <p>${errorText}</p>
                <button type="button" class="btn btn-secondary btn-table-state-retry">Повторить</button>
            </div>
        `;
    } else {
        return;
    }

    container.innerHTML = colspan ? `<tr><td colspan="${colspan}">${inner}</td></tr>` : inner;

    if (state === 'error' && typeof onRetry === 'function') {
        const btn = container.querySelector('.btn-table-state-retry');
        if (btn) btn.addEventListener('click', onRetry);
    }
}

// Инициаторы программных открытий <dialog> — запоминаются явно только когда
// openModal вызван с параметром initiator (design.md, Decision 3). Для открытий
// по клику по кнопке нативный showModal() сам восстанавливает фокус на элементе,
// который был активен на момент вызова, — WeakMap не нужен.
const modalInitiators = new WeakMap();

// openModal — обёртка над <dialog>.showModal(). initiator передаётся явно там,
// где открытие модалки не является прямым следствием пользовательского клика
// (например, showErrorModal, вызываемый из catch-блока асинхронной операции) —
// иначе браузер не сможет определить, куда вернуть фокус после закрытия.
export function openModal(el, initiator) {
    if (!el || typeof el.showModal !== 'function') return;
    if (initiator) {
        modalInitiators.set(el, initiator);
        el.addEventListener('close', () => {
            const stored = modalInitiators.get(el);
            modalInitiators.delete(el);
            if (stored && typeof stored.focus === 'function') {
                stored.focus();
            }
        }, { once: true });
    }
    el.showModal();
}

// closeModal — обёртка над <dialog>.close(). Событие 'close' срабатывает как при
// вызове .close(), так и при закрытии по Escape — обработчик восстановления
// фокуса, подписанный в openModal, отработает в обоих случаях одинаково.
export function closeModal(el) {
    if (!el || typeof el.close !== 'function') return;
    if (el.open) el.close();
}
