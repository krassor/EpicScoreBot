// ── Gantt Chart Renderer Module ──────────────────────────────────────

import { state } from './state.js';
import { apiPut, apiGet } from './api.js';
import { showToast, handleApiError, withSubmitLock, openModal, closeModal } from './utils.js';

const SVG_NS = 'http://www.w3.org/2000/svg';

// ── Заполнение доступной ширины (fit-gantt-chart-width) ──────────────
//
// Штатные ширины колонки по масштабу — сверено по факту в UMD-бандле
// frappe-gantt@1.2.2 с CDN (design.md, Context): Day не задаёт column_width
// в дескрипторе режима и падает на дефолт библиотеки 45, Week — 140,
// Month — 120. Year в интерфейсе не используется (design.md Non-Goals).
const DEFAULT_COLUMN_WIDTHS = { Day: 45, Week: 140, Month: 120 };

// Задержка гашения дребезга resize (design.md Решение 5) — события идут
// десятками в секунду при перетаскивании края окна, каждое применение
// ширины перерисовывает SVG целиком.
const RESIZE_DEBOUNCE_MS = 150;

let ganttChart = null;

// Хэндл текущего отложенного пересчёта ширины после resize — используется
// для гашения дребезга (см. scheduleFitColumnWidthRecalc).
let resizeDebounceHandle = null;

// Отпечаток набора задач текущей отрисованной диаграммы (отсортированные id
// через запятую) — используется, чтобы отличить «структурное» изменение
// (добавили/удалили задачу, сменили команду) от «того же набора задач,
// просто обновились даты/прогресс/sort_order» (типичный случай после
// reorder). Во втором случае можно обойтись инкрементальным refresh().
let lastTaskIds = null;

// Конфигурация текущей открытой модалки переупорядочивания (роли / стори / эпики).
// Заполняется при открытии модалки, используется единым обработчиком сохранения.
let currentOrderConfig = null;

// Последняя известная позиция драга родительского бара (эпик/стори),
// накопленная через on_date_change (см. renderGantt). В Frappe Gantt 1.2.2
// нет отдельного колбэка «на отпускание» — момент отпускания определяем
// собственным document-level mouseup-слушателем (см. initGanttRenderer),
// который читает это значение и запускает reorder-логику. null, когда
// драг родительского бара не в процессе — по этому флагу mouseup-слушатель
// отличает «наш» drag-reorder от посторонних кликов/mouseup в остальном
// приложении.
let pendingParentDrag = null;

// In-flight guard от гонки при быстром повторном драге: пока выполняется
// один reorder (apiPut/reloadCurrentTeamTasks ещё не завершились), второй
// mouseup с уже накопленным pendingParentDrag не должен запускать
// параллельный handleParentDragRelease (см. design.md Risks, «Гонка при
// быстром повторном драге»). Выставляется в начале handleParentDragRelease,
// сбрасывается в finally — независимо от исхода (успех/откат/ошибка).
let isParentReorderInFlight = false;

const roleColorMap = {
    'Аналитик': 'analyst',
    'BE разработчик': 'be',
    'FE разработчик': 'fe',
    'Mobile разработчик': 'mobile',
    'Тестировщик': 'qa',
    'IT-лидер': 'leader',
};

// isGanttReadOnly — единый признак «редактирование диаграммы недоступно»,
// используется и для readonly/readonly_dates/гейта on_click самой диаграммы
// (renderGantt), и для скрытия управления исполнителем в модалке деталей
// задачи (openTaskDetailsModal, задача 5.4 add-gantt-task-assignees) — чтобы
// эти два места гарантированно не разошлись со временем.
function isGanttReadOnly() {
    const userProfile = state.get('userProfile');
    return !userProfile || userProfile.role === 'member';
}

export function initGanttRenderer() {
    // Subscribe to state changes
    state.subscribe('tasks', (tasks) => {
        renderGantt(tasks);
    });

    state.subscribe('viewMode', (mode) => {
        if (ganttChart) {
            ganttChart.change_view_mode(mode);
            // Ширина колонки, подобранная под предыдущий масштаб, иначе
            // «утечёт» в новый — column_width в опциях побеждает штатное
            // значение режима безусловно (design.md Решение 3, спека
            // «Ширина пересчитывается при смене масштаба»). Вызов синхронный,
            // без await/setTimeout — второй (уже верный) рендер перекрывает
            // первый в том же такте JS, до следующей отрисовки браузера.
            applyFitColumnWidth(ganttChart);
        }
    });

    // Возврат на вкладку «Гант» после resize, случившегося на другой
    // вкладке (tasks.md 2.7): switchTab (app.js) вызывает
    // state.set('activeTab', ...) ДО снятия класса `.hidden` с
    // `#tab-content-gantt` — на момент срабатывания этого колбэка контейнер
    // ещё может быть скрыт (нулевая ширина). requestAnimationFrame переносит
    // фактический пересчёт на следующий кадр, когда switchTab уже полностью
    // отработал (он весь синхронный) и `.hidden` снят — это не тот же
    // приём, что двухшаговый первый рендер (там пересчёт обязан быть
    // синхронным, здесь, наоборот, обязан быть отложен на кадр, т.к. сам
    // источник изменения — переключение вкладки, а не построение чарта).
    state.subscribe('activeTab', (tab) => {
        if (tab !== 'gantt') return;
        requestAnimationFrame(() => {
            if (!ganttChart) return;
            applyFitColumnWidth(ganttChart);
        });
    });

    setupGanttEvents();
    setupParentDragReorder();
    setupGanttResizeHandler();
}

// ── Вычисление и применение ширины колонки под доступную ширину ─────
// (fit-gantt-chart-width, design.md Решения 1–5)

// Доступная ширина рабочей области диаграммы — ширина `.app-gantt-wrapper`
// за вычетом его собственных горизонтальных отступов (`padding: clamp(12px,
// 3vw, 24px)`, gantt.css:61-67). Без вычета отступов полотно оказалось бы
// шире контейнера и появилась бы лишняя прокрутка (design.md Решение 5).
function getGanttWrapperAvailableWidth() {
    const wrapper = document.getElementById('app-gantt-wrapper');
    if (!wrapper) return 0;

    const rect = wrapper.getBoundingClientRect();
    const styles = window.getComputedStyle(wrapper);
    const paddingLeft = parseFloat(styles.paddingLeft) || 0;
    const paddingRight = parseFloat(styles.paddingRight) || 0;

    return Math.max(0, rect.width - paddingLeft - paddingRight);
}

// computeFitColumnWidth — считает ширину колонки как
// `max(штатная ширина режима, доступная ширина / число колонок)`,
// без верхнего клампа (продуктовое решение зафиксировано в ux-brief.md п.1
// и подтверждено пользователем — потолок растягивания не вводится).
//
// Число колонок берётся из состояния уже построенного экземпляра
// (`chart.dates`, сверено по факту в UMD-бандле — setup_date_values()
// заполняет именно это поле), а не пересчитывается заново из дат задач:
// дублирование логики `padding` дало бы расхождение на несколько пикселей
// (design.md Решение 2).
//
// Возвращает null, если доступная ширина сейчас неизвестна (контейнер
// скрыт — например, активна другая вкладка): в этом случае пересчёт
// пропускается целиком, а не откатывается к штатной ширине — иначе
// resize-событие, случайно произошедшее при скрытой вкладке «Гант»,
// схлопнуло бы уже подобранную ширину до штатной.
// getCurrentViewModeName — единая точка чтения текущего масштаба,
// используется и для подбора штатной ширины (computeFitColumnWidth), и для
// того, чтобы передать этот же масштаб обратно в update_options
// (applyFitColumnWidth, задача 2.8) — оба места обязаны читать один и тот
// же масштаб в одной синхронной точке, а не с разницей в такте, иначе
// штатная ширина будет посчитана под один режим, а применена к другому.
function getCurrentViewModeName(chart) {
    return (chart && chart.config && chart.config.view_mode && chart.config.view_mode.name)
        || state.get('viewMode')
        || 'Day';
}

function computeFitColumnWidth(chart) {
    if (!chart || !chart.config) return null;

    const viewModeName = getCurrentViewModeName(chart);
    const defaultWidth = DEFAULT_COLUMN_WIDTHS[viewModeName] || DEFAULT_COLUMN_WIDTHS.Day;

    const columnCount = Array.isArray(chart.dates) ? chart.dates.length : 0;
    if (!columnCount) return defaultWidth;

    const availableWidth = getGanttWrapperAvailableWidth();
    if (availableWidth <= 0) return null;

    return Math.max(defaultWidth, availableWidth / columnCount);
}

// applyFitColumnWidth — применяет вычисленную ширину через
// `update_options({ column_width })` (design.md Решение 3): это
// единственный путь библиотеки, который меняет `config.column_width` не
// затираемым способом (прямое присваивание в `config` перетёрлось бы при
// первой же смене масштаба) и в конце вызывает `trigger_event("view_change")`,
// на который приложение уже подписано (`on_view_change` → applyPostRenderEnhancements).
//
// Позиция прокрутки сохраняется не абсолютным `scrollLeft` (это сделала бы
// сама библиотека через ветку `change_view_mode(undefined, true)` внутри
// `update_options`), а относительной долей `scrollLeft / scrollWidth`
// (ux-brief.md п.5): при изменении ширины колонки меняется масштаб
// «пикселей на день», и абсолютный scrollLeft начинает указывать на другую
// дату — пересчитываем долю сами и применяем её поверх того, что уже
// сделала библиотека.
function applyFitColumnWidth(chart) {
    if (!chart) return;

    const newWidth = computeFitColumnWidth(chart);
    if (newWidth === null) return;

    if (chart.config.column_width === newWidth) return;

    // Масштаб читается той же функцией и в той же синхронной точке, что и
    // штатная ширина внутри computeFitColumnWidth (вызов чуть выше) —
    // гарантирует, что ниже передаётся ровно тот режим, под который была
    // посчитана ширина, а не промежуточное/устаревшее состояние.
    const viewModeName = getCurrentViewModeName(chart);

    const scrollContainer = chart.$container;
    const oldScrollWidth = scrollContainer ? scrollContainer.scrollWidth : 0;
    const scrollRatio = (scrollContainer && oldScrollWidth > 0)
        ? scrollContainer.scrollLeft / oldScrollWidth
        : 0;

    // Задача 2.8: update_options({column_width}) без view_mode откатывает
    // this.options.view_mode на значение из original_options (опции
    // конструктора/предыдущего update_options) — setup_options пересобирает
    // this.options из original_options при каждом вызове, а
    // change_view_mode(void 0, true) внутри update_options берёт откаченное
    // значение как масштаб по умолчанию. Явная передача view_mode вместе с
    // column_width перебивает эту откатку тем же способом, каким сама
    // библиотека резолвит строковое имя режима в change_view_mode
    // (this.options.view_modes.find(...) — доступен, т.к. приложение не
    // передаёт свой view_modes в конструктор, и библиотека молча
    // подставляет свой дефолтный полный список режимов).
    chart.update_options({ column_width: newWidth, view_mode: viewModeName });

    if (scrollContainer) {
        scrollContainer.scrollLeft = scrollRatio * scrollContainer.scrollWidth;
    }
}

// scheduleFitColumnWidthRecalc — гасит дребезг resize простым debounce по
// времени (design.md Решение 5: альтернатива ResizeObserver оставлена
// реализации, здесь выбран `window.resize`, т.к. в приложении не требуется
// ловить изменения раскладки без изменения размера окна, а отдельный
// observer добавил бы обязанность по отписке при уходе с вкладки «Гант»).
// Ширина контейнера читается уже внутри applyFitColumnWidth в момент
// фактического срабатывания — не из значения, вычисленного заранее
// (ux-brief.md п.3).
function scheduleFitColumnWidthRecalc() {
    if (resizeDebounceHandle) {
        clearTimeout(resizeDebounceHandle);
    }
    resizeDebounceHandle = setTimeout(() => {
        resizeDebounceHandle = null;
        if (!ganttChart) return;
        applyFitColumnWidth(ganttChart);
    }, RESIZE_DEBOUNCE_MS);
}

// setupGanttResizeHandler — регистрируется один раз при инициализации
// модуля (симметрично setupParentDragReorder), не внутри renderGantt.
function setupGanttResizeHandler() {
    window.addEventListener('resize', scheduleFitColumnWidthRecalc);
}

// Собственный document-level mouseup-слушатель для завершения drag-reorder
// родительских баров (эпик/стори) — регистрируется один раз при
// инициализации модуля, а НЕ внутри renderGantt (вызывается на каждый
// рендер диаграммы), иначе на каждое обновление плодился бы новый слушатель
// (та же утечка, которую убирает переход на ganttChart.refresh() в разделе 3).
//
// Frappe Gantt 1.2.2 сам вешает свой mouseup-обработчик на внутренний $svg
// (см. bind_bar_events в бандле библиотеки) — mouseup там всплывает до
// document, поэтому наш слушатель гарантированно сработает после того, как
// библиотека зафиксировала финальную визуальную позицию бара. Гейт по
// pendingParentDrag !== null защищает от срабатывания на посторонних
// mouseup в остальном приложении (клик по кнопке, закрытие модалки и т.д.).
function setupParentDragReorder() {
    document.addEventListener('mouseup', () => {
        if (!pendingParentDrag) return;
        const drag = pendingParentDrag;
        pendingParentDrag = null;

        if (isParentReorderInFlight) {
            // Предыдущий reorder (apiPut/reloadCurrentTeamTasks) ещё не
            // завершился — не запускаем параллельный handleParentDragRelease
            // (см. design.md Risks, «Гонка при быстром повторном драге»).
            // Второй драг просто визуально откатывается, как будто reorder
            // не состоялся, без UI-индикации «занято» — приемлемо для
            // первой итерации.
            renderGantt(state.get('tasks'));
            return;
        }

        handleParentDragRelease(drag);
    });
}

function renderGantt(tasks) {
    const container = document.getElementById('gantt-chart');
    const emptyState = document.getElementById('gantt-empty');
    const taskCount = document.getElementById('task-count');

    if (!container || !emptyState || !taskCount) return;

    taskCount.textContent = `${tasks.length} задач`;

    if (tasks.length === 0) {
        container.innerHTML = '';
        emptyState.classList.remove('hidden');
        ganttChart = null;
        lastTaskIds = null;
        return;
    }

    emptyState.classList.add('hidden');

    // Build a map for easy parent lookups to determine hierarchy depth
    const taskMap = {};
    tasks.forEach(t => {
        taskMap[t.id] = t;
    });

    const getTaskLevel = (t) => {
        const parentId = t.parent_task_id || t.parent_id;
        if (!parentId) return 1; // Epic level

        const parent = taskMap[parentId];
        if (!parent) return 2; // Sibling story level

        const grandParentId = parent.parent_task_id || parent.parent_id;
        if (!grandParentId) return 2; // Story level

        return 3; // Role task level
    };

    // Convert to Frappe Gantt format
    const ganttTasks = tasks.map(t => {
        const level = getTaskLevel(t);
        let displayName = t.name;
        if (level === 2) {
            displayName = `  └─ ${t.name}`;
        } else if (level === 3) {
            displayName = `    └─ ${t.name}`;
            // Имя исполнителя в подписи (design.md Решение 10, ux-brief.md п.1):
            // только для ролевых задач С исполнителем — задача без исполнителя
            // остаётся чистым именем роли, отличие бара (см. applyPostRenderEnhancements)
            // уже сигнализирует «нет кандидатов» без текстового шума.
            // Фамилия, а не полное имя — assignee_name приходит как «Имя Фамилия»,
            // берём последнее слово.
            if (t.role_id && t.assignee_id && t.assignee_name) {
                const lastName = t.assignee_name.trim().split(/\s+/).pop();
                displayName += ` — ${lastName}`;
            }
        }

        return {
            id: t.id,
            name: displayName,
            start: t.start_date || t.start,
            end: t.end_date || t.end,
            progress: t.progress * 100, // Frappe Gantt expects 0-100
            dependencies: t.dependencies || '',
            custom_class: t.custom_class || getTaskClass(t),
            _is_parent: t.is_parent,
            _parent_id: t.parent_task_id || t.parent_id,
            _sort_order: t.sort_order,
            _role_id: t.role_id,
        };
    });

    const isReadOnly = isGanttReadOnly();

    // Отпечаток нового набора задач — если совпадает с уже отрисованным,
    // структура диаграммы (набор строк) не менялась, можно обойтись
    // инкрементальным refresh() вместо полной пересборки DOM/SVG.
    const newTaskIds = ganttTasks.map(t => t.id).slice().sort().join(',');

    if (ganttChart && lastTaskIds === newTaskIds) {
        // Инкрементальное обновление: Frappe Gantt пересобирает бары из
        // ganttTasks на месте, не трогая обёртку .gantt-container и не
        // переинициализируя обработчики событий на $svg — в отличие от
        // container.innerHTML='' + new Gantt(...), который раньше плодил
        // новый набор document-level слушателей на каждое обновление.
        //
        // Побочный эффект (проверено на бандле Frappe Gantt 1.2.2):
        // change_view_mode(), вызываемый изнутри refresh(), сбрасывает
        // прокрутку к началу диапазона дат (options.scroll_to не задан),
        // если явно её не восстановить — поэтому сохраняем и возвращаем
        // scrollLeft вручную.
        const scrollContainer = ganttChart.$container;
        const savedScrollLeft = scrollContainer ? scrollContainer.scrollLeft : undefined;

        ganttChart.refresh(ganttTasks);

        if (scrollContainer && savedScrollLeft !== undefined) {
            requestAnimationFrame(() => {
                scrollContainer.scrollLeft = savedScrollLeft;
                // Пересчёт ширины колонки под доступную ширину
                // (fit-gantt-chart-width, design.md Решение 4) — после того,
                // как scrollLeft уже восстановлен, чтобы относительная доля
                // прокрутки внутри applyFitColumnWidth считалась от верной
                // базовой позиции, а не от временной, оставленной refresh().
                applyFitColumnWidth(ganttChart);
            });
        } else {
            applyFitColumnWidth(ganttChart);
        }

        applyPostRenderEnhancements(tasks);
        return;
    }

    // Структурное изменение (первый рендер, смена команды/набора задач) —
    // полная пересборка, как и раньше.
    container.innerHTML = '';
    lastTaskIds = newTaskIds;

    ganttChart = new Gantt(container, ganttTasks, {
        view_mode: state.get('viewMode'),
        date_format: 'YYYY-MM-DD',
        language: 'ru',
        infinite_padding: false,
        readonly: isReadOnly,
        // Расписание по-прежнему полностью считает бэкенд (конвейерный
        // планировщик) — но теперь драг родительских баров (эпик/стори)
        // разрешён на уровне библиотеки и переинтерпретируется нашим кодом
        // как reorder, а не как прямое изменение дат (см. on_date_change
        // ниже). Для листовых (ролевых) баров и для пользователей без права
        // редактирования драг фактически заблокирован — библиотека Frappe
        // Gantt 1.2.2 не даёт настроить readonly для отдельной задачи, только
        // глобально, поэтому блокировка для листовых баров реализована через
        // немедленный откат в колбэке on_date_change, а не через эту опцию.
        readonly_dates: isReadOnly,
        // Единственный канал информации об исполнителе для роли member,
        // которой модалка деталей задачи недоступна (задача 5.5). Опция
        // называется `popup`, а не `custom_popup_html` — в постановке
        // фигурировало имя опции из более старой версии библиотеки; в
        // установленной 1.2.2 (сверено по факту в исходниках UMD-бандла с
        // CDN, а не по памяти) единственная точка расширения попапа — эта.
        popup: buildGanttPopup,
        // Пересобираем факт-маркеры/подсветку завершённых задач после
        // каждого рендера (в т.ч. при переключении Day/Week/Month —
        // Frappe Gantt полностью перерисовывает бары и теряет наш DOM).
        on_view_change: () => applyPostRenderEnhancements(tasks),
        on_click: task => {
            if (isReadOnly) return;
            if (task._is_parent) {
                openReorderModal(task);
            } else {
                openTaskDetailsModal(task);
            }
        },
        on_date_change: (task, newStart, newEnd) => {
            if (isReadOnly) {
                // Защита в глубину: обычно сюда не попадём, т.к. readonly_dates
                // уже true для read-only пользователей и библиотека вообще не
                // инициирует драг, но откатываем явно на случай расхождения.
                renderGantt(state.get('tasks'));
                return;
            }
            if (!task._is_parent) {
                // Листовые (ролевые) задачи не драгаются в этой итерации —
                // немедленно отменяем визуальное перемещение (тот же паттерн
                // отката, что раньше применялся сегодня для ВСЕХ баров).
                showToast('Даты ролевых задач рассчитываются автоматически конвейерным планировщиком', 'info');
                renderGantt(state.get('tasks'));
                return;
            }
            // Для родительских баров (эпик/стори) не мешаем нативному
            // визуальному перемещению во время драга — но и не чистый no-op:
            // в Frappe Gantt 1.2.2 нет отдельного колбэка «на отпускание»,
            // этот колбэк стреляет многократно во время драга (на каждое
            // пересечение границы даты), поэтому просто дёшево запоминаем
            // последнюю позицию — вся тяжёлая логика reorder запускается
            // отдельно, из mouseup-слушателя (см. setupParentDragReorder).
            pendingParentDrag = { taskId: task.id, newStart, newEnd };
        },
        on_progress_change: async (task, progress) => {
            if (task._is_parent) {
                // Прогресс стори/эпика — агрегат по детям, руками не редактируется.
                showToast('Прогресс стори/эпика считается автоматически из ролей', 'info');
                renderGantt(state.get('tasks'));
                return;
            }
            try {
                await apiPut(`/tasks/${task.id}`, {
                    progress: progress / 100, // API expects 0.0-1.0
                });
                showToast('Прогресс задачи обновлен', 'success');
                // Прогресс листовой задачи может сдвинуть расписание всей
                // команды (конвейер) и зафиксировать факт закрытия — тянем
                // полный список задач заново.
                await reloadCurrentTeamTasks();
            } catch (err) {
                handleApiError(err, { title: 'Не удалось обновить прогресс' });
                renderGantt(state.get('tasks'));
            }
        },
    });

    // Двухшаговый первый рендер (design.md Решение 2, ux-brief.md п.5):
    // экземпляр только что построен со штатной шириной колонки текущего
    // масштаба (chart.dates уже посчитан библиотекой в setup_dates), теперь
    // можно узнать фактическое число колонок и применить вычисленную
    // ширину. Вызывается синхронно, в том же такте JS, без await/setTimeout
    // между шагами — иначе пользователь на долю секунды увидел бы
    // нерастянутую сетку.
    applyFitColumnWidth(ganttChart);

    applyPostRenderEnhancements(tasks);
}

// ── Попап Frappe Gantt (задача 5.5) ──────────────────────────────────
//
// Единственный канал информации об исполнителе для роли member, которой
// модалка деталей задачи недоступна (on_click гейтится isGanttReadOnly()
// целиком, см. renderGantt). Опция чарта называется `popup`, а не
// `custom_popup_html` (в постановке фигурировало имя из более старой версии
// библиотеки) — в установленной 1.2.2 это единственная точка расширения.
//
// Для родительских (эпик/стори) баров разметка попапа воспроизводит ровно
// то, что делает библиотека по умолчанию при отсутствии опции `popup`
// (сверено по исходникам UMD-бандла frappe-gantt@1.2.2 с CDN, а не по
// памяти — см. минифицированный дефолт `popup: n => {...}` в бандле):
// заголовок = имя задачи, подзаголовок = task.description (у наших задач не
// заполняется, поэтому пусто — как и сегодня для ВСЕХ баров), детали = диапазон
// дат в формате "MMM DD" на языке чарта + длительность в днях + прогресс.
// Для ролевых листовых баров к тем же деталям добавляется строка состояния
// исполнителя — единственное отличие от дефолта.
function formatPopupMonthDay(date, lang) {
    const month = new Intl.DateTimeFormat(lang, { month: 'short' }).format(date);
    const day = String(date.getDate()).padStart(2, '0');
    return `${month} ${day}`;
}

// assigneeStatusText — строка состояния исполнителя ролевой задачи по
// сырым (не Frappe-обёрнутым) полям задачи, тем же 4-состояниям, что и
// в модалке деталей (populateAssigneeSelect) и в ux-brief.md, задача 5.5.
function assigneeStatusText(t) {
    if (t.assignee_pin_invalid) return '⚠ Закрепление недействительно — работает автоматически';
    if (t.assignee_is_manual) return 'Закреплён вручную';
    if (t.assignee_id) return 'Назначен автоматически';
    return 'Нет исполнителя — нет участников с этой ролью в команде';
}

function buildGanttPopup({ task, chart, set_title, set_subtitle, set_details }) {
    set_title(task.name);
    set_subtitle(task.description ? task.description : '');

    const lang = chart.options.language;
    const startStr = formatPopupMonthDay(task._start, lang);
    // Конец диапазона в Frappe Gantt хранится как полночь СЛЕДУЮЩЕГО дня
    // (исключающая граница) — минус секунда, чтобы отобразить последний
    // включённый день, тем же приёмом, что и дефолтный попап библиотеки.
    const endStr = formatPopupMonthDay(new Date(task._end.getTime() - 1000), lang);
    const excluded = task.ignored_duration ? ` + ${task.ignored_duration} искл.` : '';
    let details = `${startStr} - ${endStr} (${task.actual_duration} дн.${excluded})<br/>Прогресс: ${Math.floor(task.progress * 100) / 100}%`;

    if (!task._is_parent && task._role_id) {
        const rawTasks = state.get('tasks');
        const rawTask = rawTasks.find(x => x.id === task.id);
        if (rawTask) {
            details += `<br/>${assigneeStatusText(rawTask)}`;
        }
    }

    set_details(details);
}

function cleanTaskName(name) {
    if (!name) return '';
    return name.replace(/^[└─\s ]+/g, '').trim();
}

function getTaskClass(task) {
    if (!task) return 'gantt-default';

    if (task.is_parent) {
        if (task.parent_task_id || task.parent_id) {
            return 'gantt-story';
        }
        return 'gantt-epic';
    }

    // Extrapolate role name from task name if role_id is not directly resolved
    // Usually name matches role: "Аналитик", "BE разработчик", etc.
    const cleanName = cleanTaskName(task.name);
    let colorClass = roleColorMap[cleanName];

    if (!colorClass) {
        const lower = cleanName.toLowerCase();
        if (lower.includes('be') || lower.includes('backend') || lower.includes('бэкенд')) {
            colorClass = 'be';
        } else if (lower.includes('fe') || lower.includes('frontend') || lower.includes('фронтенд')) {
            colorClass = 'fe';
        } else if (lower.includes('mobile') || lower.includes('мобильн')) {
            colorClass = 'mobile';
        } else if (lower.includes('аналит') || lower.includes('analyst')) {
            colorClass = 'analyst';
        } else if (lower.includes('тестиров') || lower.includes('qa')) {
            colorClass = 'qa';
        } else if (lower.includes('лидер') || lower.includes('leader')) {
            colorClass = 'leader';
        } else {
            colorClass = 'default';
        }
    }

    return `gantt-${colorClass}`;
}

// Задача считается завершённой, если прогресс достиг 100% (1.0 в исходных
// данных 0.0–1.0) или бэкенд уже зафиксировал факт закрытия.
function isTaskCompleted(t) {
    return (typeof t.progress === 'number' && t.progress >= 1) || !!t.actual_end_date;
}

// ── Пост-рендер доработки бара: подсветка завершённых задач + факт-маркер ──
//
// Frappe Gantt 1.2.2 не поддерживает несколько custom-классов на одной
// задаче (внутренний Bar.refresh() делает classList.add(task.custom_class)
// одним токеном — если передать строку с пробелом, браузер выбросит
// DOMException и сломает рендер всей диаграммы). Поэтому не комбинируем
// классы через custom_class, а навешиваем gantt-completed и рисуем
// факт-маркер уже поверх готового SVG после рендера, точечно по data-id.
function applyPostRenderEnhancements(tasks) {
    const container = document.getElementById('gantt-chart');
    if (!container) return;

    tasks.forEach(t => {
        const wrapper = container.querySelector(`.bar-wrapper[data-id="${t.id}"]`);
        if (!wrapper) return;
        const barRect = wrapper.querySelector('.bar');
        if (!barRect) return;

        const x = parseFloat(barRect.getAttribute('x'));
        const y = parseFloat(barRect.getAttribute('y'));
        const width = parseFloat(barRect.getAttribute('width'));
        const height = parseFloat(barRect.getAttribute('height'));

        if (isTaskCompleted(t)) {
            wrapper.classList.add('gantt-completed');

            const icon = document.createElementNS(SVG_NS, 'text');
            icon.setAttribute('class', 'gantt-completed-icon');
            icon.setAttribute('x', String(x + width - 6));
            icon.setAttribute('y', String(y + height / 2 + 4));
            icon.setAttribute('text-anchor', 'end');
            icon.textContent = '✓';
            wrapper.appendChild(icon);
        }

        // Нет исполнителя / недействующее закрепление (задача 5.3,
        // add-gantt-task-assignees) — два разных состояния по спеке и по
        // риску (design.md Решение 7), кодируются разными классами.
        // Классы навешиваются точечно (см. комментарий выше про
        // custom_class), а не через custom_class сервера.
        if (!t.is_parent && t.role_id && !t.assignee_id) {
            wrapper.classList.add('gantt-no-assignee');
        }
        if (!t.is_parent && t.assignee_pin_invalid) {
            wrapper.classList.add('gantt-pin-invalid');

            const pinIcon = document.createElementNS(SVG_NS, 'text');
            pinIcon.setAttribute('class', 'gantt-pin-invalid-icon');
            pinIcon.setAttribute('x', String(x + 6));
            pinIcon.setAttribute('y', String(y + height / 2 + 4));
            pinIcon.setAttribute('text-anchor', 'start');
            pinIcon.textContent = '⚠';
            wrapper.appendChild(pinIcon);
        }

        // Факт-маркер — только для листовых задач с зафиксированным фактом,
        // отличающимся от планового окончания.
        const endStr = t.end_date || t.end;
        if (!t.is_parent && t.actual_end_date && t.actual_end_date !== endStr) {
            const startStr = t.start_date || t.start;
            const startMs = new Date(startStr).getTime();
            const endMs = new Date(endStr).getTime();
            const factMs = new Date(t.actual_end_date).getTime();
            const totalMs = endMs - startMs;
            // Позиция факта вычисляется пропорционально внутри уже
            // отрисованного бара (его x/width в текущем масштабе), поэтому
            // не зависит от внутренней шкалы дат Frappe Gantt (Day/Week/Month).
            const ratio = totalMs > 0 ? (factMs - startMs) / totalMs : 1;
            const factX = x + ratio * width;
            // Строки дат ISO YYYY-MM-DD корректно сравниваются лексикографически.
            const isEarly = t.actual_end_date < endStr;
            const cls = isEarly ? 'fact-marker-early' : 'fact-marker-late';

            const line = document.createElementNS(SVG_NS, 'line');
            line.setAttribute('class', `fact-marker ${cls}`);
            line.setAttribute('x1', String(factX));
            line.setAttribute('x2', String(factX));
            line.setAttribute('y1', String(y - 4));
            line.setAttribute('y2', String(y + height + 4));
            wrapper.appendChild(line);

            const r = 4;
            const cy = y - 4;
            const diamond = document.createElementNS(SVG_NS, 'polygon');
            diamond.setAttribute('class', `fact-marker-diamond ${cls}`);
            diamond.setAttribute(
                'points',
                `${factX},${cy - r} ${factX + r},${cy} ${factX},${cy + r} ${factX - r},${cy}`
            );
            wrapper.appendChild(diamond);
        }
    });
}

// ── Task details modal: единственная явная точка входа для простановки % ──
// выполнения листовой (ролевой) задачи — открывается кликом по её бару.
// Хендл прогресса на самом баре Frappe Gantt остаётся рабочим как быстрый
// способ для тех, кто уже знает про него, но не единственный.

let currentDetailsTaskId = null;
// Исходные значения открытой задачи — чтобы отправлять в PUT только реально
// изменившиеся поля (progress / start_offset_days независимы друг от друга).
let currentDetailsOriginal = null;
// Исходное значение селекта исполнителя на момент открытия модалки —
// '' (Автоматически) для состояний 1/3/4, assignee_id для состояния 2
// (см. ux-brief.md, «Критичное правило сохранения намерения»). null —
// «сохранение исполнителя недоступно/ещё не определено» (read-only
// пользователь ИЛИ кандидаты роли ещё не загружены) — в этом состоянии
// saveTaskDetails() обязан трактовать выбор как «не изменился», иначе
// быстрый клик «Сохранить» до завершения загрузки мог бы уйти с неверным
// намерением.
let currentDetailsAssigneeOriginal = null;

function formatDisplayDate(isoDate) {
    if (!isoDate) return '';
    const d = new Date(isoDate);
    if (isNaN(d.getTime())) return isoDate;
    return d.toLocaleDateString('ru-RU');
}

function openTaskDetailsModal(feTask) {
    const rawTasks = state.get('tasks');
    const t = rawTasks.find(x => x.id === feTask.id);
    if (!t) return;

    currentDetailsTaskId = t.id;

    document.getElementById('task-details-title').textContent = cleanTaskName(t.name);
    document.getElementById('task-details-dates').textContent =
        `${formatDisplayDate(t.start_date || t.start)} — ${formatDisplayDate(t.end_date || t.end)}`;

    const factGroup = document.getElementById('task-details-fact-group');
    if (t.actual_end_date) {
        factGroup.classList.remove('hidden');
        const effort = (t.actual_effort_days !== undefined && t.actual_effort_days !== null)
            ? `, факт. трудоёмкость: ${t.actual_effort_days} р.д.`
            : '';
        document.getElementById('task-details-fact').textContent =
            `Завершено ${formatDisplayDate(t.actual_end_date)}${effort}`;
    } else {
        factGroup.classList.add('hidden');
    }

    const percent = Math.round((t.progress || 0) * 100);
    const offset = t.start_offset_days || 0;

    document.getElementById('task-details-progress').value = percent;
    document.getElementById('task-details-offset').value = offset;
    currentDetailsOriginal = { percent, offset };

    // Исполнитель (задачи 5.1/5.4) — сбрасываем «исходное значение» синхронно
    // ДО запуска асинхронной загрузки кандидатов: пока populateAssigneeSelect
    // не завершится, currentDetailsAssigneeOriginal остаётся null, и
    // saveTaskDetails() не станет отправлять PUT .../assignee на основании
    // устаревшего (от предыдущей открытой задачи) значения, если пользователь
    // успеет нажать «Сохранить» раньше, чем прогрузится список кандидатов.
    currentDetailsAssigneeOriginal = null;
    populateAssigneeSelect(t);

    openModal(document.getElementById('task-details-modal'));
}

function closeTaskDetailsModal() {
    closeModal(document.getElementById('task-details-modal'));
    currentDetailsTaskId = null;
    currentDetailsOriginal = null;
    currentDetailsAssigneeOriginal = null;
}

// resolveRoleName — отображаемое имя роли по её id из уже загруженного
// state.get('roles') (app.js грузит его один раз при старте приложения).
function resolveRoleName(roleId) {
    const roles = state.get('roles') || [];
    const role = roles.find(r => r.id === roleId);
    return role ? role.name : '';
}

// populateAssigneeSelect — заполняет селект исполнителя и текст под ним по
// одному из 5 состояний (загрузка / авто / закреплён / закрепление
// недействительно / нет кандидатов), ux-brief.md п.2. Кандидатов роли
// загружает заново при каждом открытии модалки (без кеша — состав команды
// мог измениться), напрямую через apiGet, по аналогии с
// openStoryReorderModal. Скрывает саму группу целиком для read-only
// пользователя (задача 5.4) — независимо от того, что on_click и так не
// даёт такому пользователю открыть модалку (защита в глубину).
async function populateAssigneeSelect(t) {
    const group = document.getElementById('task-details-assignee-group');
    const select = document.getElementById('task-details-assignee');
    const help = document.getElementById('task-details-assignee-help');
    if (!group || !select || !help) return;

    if (isGanttReadOnly()) {
        group.classList.add('hidden');
        return;
    }
    group.classList.remove('hidden');
    help.style.color = '';

    if (!t.role_id) {
        // Защита в глубину: openTaskDetailsModal открывается только для
        // листовых (ролевых) баров, у которых role_id есть всегда — сюда
        // попасть не должны, но на случай расхождения не оставляем селект
        // в состоянии "Загрузка...".
        select.innerHTML = '<option value="">Автоматически</option>';
        select.value = '';
        help.textContent = '';
        currentDetailsAssigneeOriginal = '';
        return;
    }

    // Состояние «Загрузка».
    select.innerHTML = '<option value="" disabled selected>Загрузка исполнителей…</option>';
    select.disabled = true;
    help.textContent = '';

    const teamId = state.get('selectedTeamId');
    let members;
    try {
        const data = await apiGet(`/teams/${teamId}/members`);
        members = data.members || [];
    } catch (err) {
        // Модалку могли успеть закрыть/переоткрыть на другую задачу, пока
        // шёл запрос — не затираем её текущее состояние чужим ответом.
        if (currentDetailsTaskId !== t.id) return;
        select.innerHTML = '<option value="" disabled selected>Ошибка загрузки исполнителей</option>';
        select.disabled = true;
        help.textContent = '';
        handleApiError(err, { title: 'Не удалось загрузить список исполнителей' });
        return;
    }

    if (currentDetailsTaskId !== t.id) return;

    const candidates = members.filter(m => (m.role_ids || []).includes(t.role_id));

    select.innerHTML = '';
    const autoOpt = document.createElement('option');
    autoOpt.value = '';
    autoOpt.textContent = 'Автоматически';
    select.appendChild(autoOpt);
    candidates.forEach(m => {
        const opt = document.createElement('option');
        opt.value = m.id;
        opt.textContent = `${m.first_name} ${m.last_name || ''}`;
        select.appendChild(opt);
    });
    select.disabled = false;

    // Состояние 4: нет кандидатов роли в команде.
    if (candidates.length === 0) {
        select.value = '';
        const roleName = resolveRoleName(t.role_id);
        help.textContent = `В команде нет участников с ролью «${roleName}». Задача останется без исполнителя, пока в команде не появится участник с этой ролью.`;
        currentDetailsAssigneeOriginal = '';
        return;
    }

    // Состояние 3: закрепление есть, но не действует — контракт API не
    // сообщает, КТО был закреплён, только фактического исполнителя и флаг
    // is_manual=false. Единственный честный пункт селекта — «Автоматически»
    // (ux-brief.md объясняет это подробно в предупреждающем тексте).
    if (t.assignee_pin_invalid) {
        select.value = '';
        help.style.color = 'var(--color-warning)';
        help.textContent = `Ручное закрепление на эту задачу есть, но закреплённый ранее человек больше не кандидат этой роли в команде (потерял роль или покинул её). Сейчас задачу автоматически ведёт ${t.assignee_name || 'неизвестный участник'}. Закрепление вернётся в силу, если этот человек снова станет кандидатом, либо будет заменено, если вы выберете здесь другого.`;
        currentDetailsAssigneeOriginal = '';
        return;
    }

    // Состояние 2: закреплён и работает.
    if (t.assignee_is_manual) {
        select.value = t.assignee_id;
        help.textContent = `Закреплено за ${t.assignee_name}. Закрепление гарантирует исполнителя, но не дату: если он занят другими задачами, старт задачи сдвинется на его освобождение.`;
        currentDetailsAssigneeOriginal = t.assignee_id;
        return;
    }

    // Состояние 1: авто.
    if (t.assignee_id) {
        select.value = '';
        help.textContent = `Сейчас задачу ведёт ${t.assignee_name} — назначен автоматически. Закрепление гарантирует исполнителя, но не дату: если он занят другими задачами, старт задачи сдвинется на его освобождение.`;
        currentDetailsAssigneeOriginal = '';
        return;
    }

    // Кандидаты роли есть, но assignee_id отсутствует — по контракту не
    // должно происходить одновременно (assignee_id отсутствует только при
    // пустом пуле роли), но на случай расхождения не оставляем
    // currentDetailsAssigneeOriginal неопределённым.
    select.value = '';
    currentDetailsAssigneeOriginal = '';
}

async function saveTaskDetails() {
    if (!currentDetailsTaskId) {
        closeTaskDetailsModal();
        return;
    }

    const percent = parseInt(document.getElementById('task-details-progress').value, 10);
    if (isNaN(percent) || percent < 0 || percent > 100) {
        showToast('Прогресс должен быть числом от 0 до 100', 'error');
        return;
    }

    const offset = parseInt(document.getElementById('task-details-offset').value, 10);
    if (isNaN(offset)) {
        showToast('Смещение должно быть целым числом дней', 'error');
        return;
    }

    // Отправляем только реально изменившиеся поля — progress и
    // start_offset_days независимы друг от друга на бэкенде.
    const payload = {};
    if (!currentDetailsOriginal || percent !== currentDetailsOriginal.percent) {
        payload.progress = percent / 100;
    }
    if (!currentDetailsOriginal || offset !== currentDetailsOriginal.offset) {
        payload.start_offset_days = offset;
    }

    // Исполнитель — отдельный PUT, отправляется, ТОЛЬКО если пользователь
    // реально изменил выбор относительно того, что было при открытии
    // модалки (ux-brief.md, «Критичное правило сохранения намерения»).
    // Без этой проверки в состоянии 3 (селект по умолчанию показывает
    // "Автоматически" = "") нажатие «Сохранить» без изменения выбора
    // безусловно отправило бы {"user_id": null} и сняло бы дремлющее
    // закрепление, которое обязано сохраняться в ожидании возврата
    // человека в пул. currentDetailsAssigneeOriginal === null означает
    // «сохранение недоступно/ещё не определено» (read-only или кандидаты
    // ещё не загрузились) — в этом случае изменение не отправляем.
    const assigneeGroup = document.getElementById('task-details-assignee-group');
    const assigneeSelect = document.getElementById('task-details-assignee');
    const assigneeVisible = assigneeGroup && !assigneeGroup.classList.contains('hidden');
    const assigneeChanged = assigneeVisible
        && currentDetailsAssigneeOriginal !== null
        && assigneeSelect
        && assigneeSelect.value !== currentDetailsAssigneeOriginal;

    if (Object.keys(payload).length === 0 && !assigneeChanged) {
        closeTaskDetailsModal();
        return;
    }

    // Блокировка кнопок на время запроса (закрывает дефект: без неё двойной
    // клик уходит двумя параллельными запросами) — через общий withSubmitLock
    // (design.md, Decision 7). Save/Cancel — пара кнопок модалки, а не submit
    // формы, поэтому передаются списком: блокируются обе.
    const saveBtn = document.getElementById('task-details-save');
    const cancelBtn = document.getElementById('task-details-cancel');
    const taskId = currentDetailsTaskId;
    await withSubmitLock([saveBtn, cancelBtn], async () => {
        try {
            // Порядок: сначала исполнитель, затем прогресс/смещение — оба в
            // одном try/catch.
            if (assigneeChanged) {
                await apiPut(`/tasks/${taskId}/assignee`, { user_id: assigneeSelect.value || null });
            }
            if (Object.keys(payload).length > 0) {
                await apiPut(`/tasks/${taskId}`, payload);
            }
            showToast('Задача обновлена', 'success');
            closeTaskDetailsModal();
            // Любое из изменений (прогресс/смещение/исполнитель) может сдвинуть
            // расписание всей команды (конвейер) — тянем полный список заново.
            await reloadCurrentTeamTasks();
        } catch (err) {
            handleApiError(err, { title: 'Не удалось сохранить' });
            // Один из двух PUT выше мог уже примениться на бэкенде, пока второй
            // упал — перезагружаем задачи, чтобы диаграмма не разошлась с
            // фактическим состоянием (модалку при этом не закрываем, как и
            // раньше).
            await reloadCurrentTeamTasks();
        }
    });
}

async function reloadCurrentTeamTasks() {
    const teamId = state.get('selectedTeamId');
    if (!teamId) return;
    try {
        // Реордер/прогресс любой задачи может пересчитать расписание всей
        // команды (глобальный конвейер), поэтому всегда тянем полный список
        // задач команды, а не какой-то частичный/локальный набор.
        const data = await apiGet(`/tasks?team_id=${teamId}`);
        state.set('tasks', data.tasks || []);
    } catch (err) {
        console.error('Failed to reload tasks:', err);
    }
}

// ── Drag-reorder на графике: эпики/стори ─────────────────────────────
//
// Запускается из mouseup-слушателя (setupParentDragReorder) с последней
// накопленной в on_date_change позицией драга родительского бара.
// Алгоритм — X-координатный аналог Y-координатного reorder в модалке
// («Порядок…», см. setupDragAndDropForModal/saveOrder ниже): определяем,
// куда «легла» перетащенная задача среди соседей того же уровня, и если
// относительный порядок действительно изменился — рассылаем PUT-запросы на
// уже существующие /epics/{id}/reorder и /stories/{id}/reorder ровно так
// же, как это делает saveOrder() для модалки.

// Возвращает список «соседей» того же уровня, что и task (включая сам
// task): для эпика (нет parent) — все top-level эпики команды, для стори —
// все стори того же родительского эпика. rawTasks уже отфильтрован по
// выбранной команде (см. reloadCurrentTeamTasks), поэтому дополнительная
// фильтрация по команде не нужна.
function getParentLevelNeighbors(rawTasks, task) {
    const parentId = task.parent_task_id || task.parent_id;
    if (!parentId) {
        return rawTasks.filter(t => t.is_parent && !(t.parent_task_id || t.parent_id));
    }
    return rawTasks.filter(t => t.is_parent && String(t.parent_task_id || t.parent_id) === String(parentId));
}

function taskIntervalMidMs(t) {
    const startMs = new Date(t.start_date || t.start).getTime();
    const endMs = new Date(t.end_date || t.end).getTime();
    return (startMs + endMs) / 2;
}

function sameIdOrder(a, b) {
    if (a.length !== b.length) return false;
    return a.every((id, i) => String(id) === String(b[i]));
}

async function handleParentDragRelease(drag) {
    // In-flight guard (см. модульную переменную isParentReorderInFlight) —
    // выставляется на всё время выполнения функции независимо от того, по
    // какой ветке (ранний return / успех / ошибка) она завершится.
    isParentReorderInFlight = true;
    try {
        const rawTasks = state.get('tasks');
        const task = rawTasks.find(t => String(t.id) === String(drag.taskId));
        if (!task) {
            // Задача исчезла из списка, пока шёл драг (например, список уже
            // успел обновиться по другой причине) — просто синхронизируем
            // диаграмму с актуальным состоянием.
            renderGantt(rawTasks);
            return;
        }

        const neighbors = getParentLevelNeighbors(rawTasks, task);
        const originalOrderIds = neighbors
            .slice()
            .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
            .map(t => t.id);

        // Позиция, куда пользователь фактически перетащил бар (середина
        // нового интервала дат) — сравнивается с серединами интервалов
        // соседей, тем же принципом, что и поиск nextSibling по Y-координате
        // в модалке.
        const draggedMidMs = (new Date(drag.newStart).getTime() + new Date(drag.newEnd).getTime()) / 2;

        const others = neighbors
            .filter(t => String(t.id) !== String(task.id))
            .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));

        const insertBeforeIndex = others.findIndex(o => draggedMidMs <= taskIntervalMidMs(o));
        const newOrderIds = others.map(o => o.id);
        if (insertBeforeIndex === -1) {
            newOrderIds.push(task.id);
        } else {
            newOrderIds.splice(insertBeforeIndex, 0, task.id);
        }

        if (sameIdOrder(newOrderIds, originalOrderIds)) {
            // Позиция не пересекла ни одного соседа (осталась в своём
            // исходном промежутке) — reorder не состоялся: порядок не
            // меняем, запрос не шлём, бар визуально возвращается на
            // исходную (авторитетную, посчитанную бэкендом) позицию.
            renderGantt(state.get('tasks'));
            return;
        }

        // Порядок соседей изменился — переносим относительный порядок в
        // новые sort_order (renumber по позиции, аналогично
        // updateModalInputsOrder) и отправляем PUT только для тех
        // элементов, чей номер реально изменился (тот же принцип, что и
        // saveOrder() для модалки).
        const endpointBase = (task.parent_task_id || task.parent_id) ? '/stories' : '/epics';

        try {
            let updatedAny = false;
            for (let i = 0; i < newOrderIds.length; i++) {
                const id = newOrderIds[i];
                const newSortOrder = i + 1;
                const item = neighbors.find(t => String(t.id) === String(id));
                if (item && item.sort_order !== newSortOrder) {
                    // В URL уходит epics.id/stories.id (item.epic_id), а не
                    // gantt_tasks.id (id/newOrderIds[i]) — см. design.md,
                    // Decision 5: reorder-эндпоинты интерпретируют {id} как
                    // Epic.ID, а не как id строки gantt_tasks.
                    await apiPut(`${endpointBase}/${item.epic_id}/reorder`, { new_sort_order: newSortOrder });
                    updatedAny = true;
                }
            }

            if (updatedAny) {
                showToast('Порядок успешно обновлён', 'success');
                // Реордер пересчитывает расписание всей команды (конвейер) —
                // тянем полный список задач заново, как и после reorder
                // через модалку.
                await reloadCurrentTeamTasks();
            } else {
                renderGantt(state.get('tasks'));
            }
        } catch (err) {
            // По аналогии с обработкой ошибок on_progress_change — сообщаем
            // и откатываем визуально к последнему известному состоянию.
            // Часть соседей могла уже обновиться на бэкенде (промежуточный
            // запрос упал) — это то же ограничение, что и у существующего
            // saveOrder() для модалки, отдельно не чиним в рамках этого
            // изменения.
            handleApiError(err, { title: 'Не удалось изменить порядок' });
            renderGantt(state.get('tasks'));
        }
    } finally {
        isParentReorderInFlight = false;
    }
}

// ── Reorder Modal logic ──────────────────────────────────────────────
//
// Общий переиспользуемый рендер списка с drag & drop и полем "Порядок",
// используется для трёх сценариев: роли внутри стори (существующий),
// стори внутри эпика и эпики внутри команды (новые).

// Открывает модалку переупорядочивания для произвольного набора элементов.
// items — массив в исходном порядке отображения.
// config: { getId(item), getLabel(item), getColorClass(item) }
// onSave(itemId, newSortOrder) — вызывается для каждого элемента, чей порядок
// действительно изменился; должен сам сделать нужный PUT-запрос.
function openOrderModal(title, items, config, onSave) {
    if (!items || items.length === 0) {
        showToast('Нет элементов для сортировки', 'info');
        return;
    }

    currentOrderConfig = { items, getId: config.getId, onSave };

    document.getElementById('modal-title').textContent = title;

    const body = document.getElementById('modal-body');
    body.innerHTML = '';

    items.forEach((item, index) => {
        const id = config.getId(item);
        const label = config.getLabel(item);
        const colorClass = config.getColorClass ? config.getColorClass(item) : 'default';
        // Если у элемента уже есть явный sort_order — показываем его,
        // иначе (например, список эпиков команды пока без sort_order)
        // используем текущую позицию в списке.
        const order = (item.sort_order !== undefined && item.sort_order !== null)
            ? item.sort_order
            : index + 1;

        const el = document.createElement('div');
        el.className = 'role-item';
        el.dataset.itemId = id;
        el.draggable = true;
        el.innerHTML = `
            <span class="drag-handle">⠿</span>
            <span class="role-color ${colorClass}"></span>
            <span class="role-name">${label}</span>
            <div class="role-order">
                <label>Порядок:</label>
                <input type="number" min="1" max="99" class="input"
                       value="${order}"
                       data-item-id="${id}">
            </div>
        `;
        body.appendChild(el);
    });

    setupDragAndDropForModal();
    openModal(document.getElementById('reorder-modal'));
}

// Клик по бару стори — переупорядочивание ролей внутри неё (как и раньше).
function openReorderModal(parentTask) {
    const currentTasks = state.get('tasks');
    const children = currentTasks.filter(
        t => (t.parent_task_id === parentTask.id || t.parent_id === parentTask.id) && !t.is_parent
    );

    if (children.length === 0) {
        showToast('У этого эпика нет подзадач для сортировки', 'info');
        return;
    }

    const sorted = [...children].sort((a, b) => a.sort_order - b.sort_order);

    openOrderModal(
        `Порядок ролей: ${cleanTaskName(parentTask.name)}`,
        sorted,
        {
            getId: t => t.id,
            getLabel: t => cleanTaskName(t.name),
            getColorClass: t => roleColorMap[cleanTaskName(t.name)] || 'default',
        },
        (itemId, newSortOrder) => apiPut(`/tasks/${itemId}/reorder`, { new_sort_order: newSortOrder })
    );
}

// Точка входа из тулбара — переупорядочивание историй внутри выбранного
// в тулбаре эпика (#epic-select).
async function openStoryReorderModal() {
    const epicId = state.get('selectedEpicId');
    if (!epicId) {
        showToast('Сначала выберите эпик в выпадающем списке тулбара', 'info');
        return;
    }

    let stories;
    try {
        stories = await apiGet(`/epics/${epicId}/stories`);
    } catch (err) {
        handleApiError(err, { title: 'Не удалось загрузить истории эпика' });
        return;
    }

    if (!stories || stories.length === 0) {
        showToast('У выбранного эпика нет историй для сортировки', 'info');
        return;
    }

    const epic = (state.get('epics') || []).find(e => e.id === epicId);
    const epicLabel = epic ? `${epic.number}: ${epic.name}` : '';

    openOrderModal(
        `Порядок историй: ${epicLabel}`,
        stories,
        {
            getId: s => s.id,
            getLabel: s => `${s.number}: ${s.name}`,
            getColorClass: () => 'default',
        },
        (itemId, newSortOrder) => apiPut(`/stories/${itemId}/reorder`, { new_sort_order: newSortOrder })
    );
}

// Точка входа из тулбара — переупорядочивание топ-эпиков текущей команды.
async function openEpicReorderModal() {
    const teamId = state.get('selectedTeamId');

    let epics;
    try {
        const data = await apiGet(`/epics?team_id=${teamId}`);
        epics = (data.epics || []).filter(e => !e.parent_epic_id);
    } catch (err) {
        handleApiError(err, { title: 'Не удалось загрузить эпиков' });
        return;
    }

    if (epics.length === 0) {
        showToast('В этой команде нет эпиков для сортировки', 'info');
        return;
    }

    openOrderModal(
        'Порядок эпиков команды',
        epics,
        {
            getId: e => e.id,
            getLabel: e => `${e.number}: ${e.name}`,
            getColorClass: () => 'default',
        },
        (itemId, newSortOrder) => apiPut(`/epics/${itemId}/reorder`, { new_sort_order: newSortOrder })
    );
}

function setupDragAndDropForModal() {
    const body = document.getElementById('modal-body');
    const items = body.querySelectorAll('.role-item');

    items.forEach(item => {
        item.addEventListener('dragstart', () => {
            item.classList.add('dragging');
        });

        item.addEventListener('dragend', () => {
            item.classList.remove('dragging');
            // Recalculate manual orders based on new positions
            updateModalInputsOrder();
        });
    });

    body.addEventListener('dragover', (e) => {
        e.preventDefault();
        const draggingItem = body.querySelector('.role-item.dragging');
        const siblings = [...body.querySelectorAll('.role-item:not(.dragging)')];

        const nextSibling = siblings.find(sibling => {
            const box = sibling.getBoundingClientRect();
            return e.clientY <= box.top + box.height / 2;
        });

        body.insertBefore(draggingItem, nextSibling);
    });
}

function updateModalInputsOrder() {
    const items = document.querySelectorAll('#modal-body .role-item');
    items.forEach((item, index) => {
        const input = item.querySelector('input');
        if (input) {
            input.value = index + 1;
        }
    });
}

function closeReorderModal() {
    closeModal(document.getElementById('reorder-modal'));
    currentOrderConfig = null;
}

async function saveOrder() {
    if (!currentOrderConfig) {
        closeReorderModal();
        return;
    }

    const { items, getId, onSave } = currentOrderConfig;
    const inputs = document.querySelectorAll('#modal-body .role-order input');
    try {
        let updated = false;
        for (const input of inputs) {
            const itemId = input.dataset.itemId;
            const newSortOrder = parseInt(input.value, 10);
            if (isNaN(newSortOrder) || newSortOrder < 1) continue;

            const item = items.find(it => String(getId(it)) === String(itemId));
            const currentSortOrder = item && item.sort_order !== undefined ? item.sort_order : null;
            if (item && currentSortOrder !== newSortOrder) {
                await onSave(itemId, newSortOrder);
                updated = true;
            }
        }

        if (updated) {
            showToast('Порядок успешно обновлен', 'success');
        }
        closeReorderModal();
        // Порядок ролей/сторей/эпиков влияет на расписание всей команды —
        // перезагружаем полный список задач.
        reloadCurrentTeamTasks();
    } catch (err) {
        handleApiError(err, { title: 'Не удалось сохранить изменения' });
    }
}

function setupGanttEvents() {
    document.getElementById('modal-close')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-cancel')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-save')?.addEventListener('click', saveOrder);
    // Клик по фону (области dialog за пределами .modal-content) закрывает
    // модалку — замена клика по .modal-overlay при переходе на <dialog>
    // (design.md, Decision 3). Селектор явно скоупнут на #reorder-modal, что
    // снимает задачу 9.1 (там был неспецифичный querySelector('.modal-overlay')).
    document.getElementById('reorder-modal')?.addEventListener('click', (e) => {
        if (!e.target.closest('.modal-content')) closeReorderModal();
    });

    // Новые точки входа для переупорядочивания эпиков/сторей (тулбар Ганта).
    document.getElementById('btn-reorder-epics')?.addEventListener('click', openEpicReorderModal);
    document.getElementById('btn-reorder-stories')?.addEventListener('click', openStoryReorderModal);

    // Модалка деталей задачи (простановка % выполнения по клику на бар).
    document.getElementById('task-details-close')?.addEventListener('click', closeTaskDetailsModal);
    document.getElementById('task-details-cancel')?.addEventListener('click', closeTaskDetailsModal);
    document.getElementById('task-details-save')?.addEventListener('click', saveTaskDetails);
    document.getElementById('task-details-modal')?.addEventListener('click', (e) => {
        if (!e.target.closest('.modal-content')) closeTaskDetailsModal();
    });

    // View modes
    document.querySelectorAll('.btn-view').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.btn-view').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.set('viewMode', btn.dataset.mode);
        });
    });
}
