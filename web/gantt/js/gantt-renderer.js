// ── Gantt Chart Renderer Module ──────────────────────────────────────

import { state } from './state.js';
import { apiPut, apiGet } from './api.js';
import { showToast } from './utils.js';

const SVG_NS = 'http://www.w3.org/2000/svg';

let ganttChart = null;

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

export function initGanttRenderer() {
    // Subscribe to state changes
    state.subscribe('tasks', (tasks) => {
        renderGantt(tasks);
    });

    state.subscribe('viewMode', (mode) => {
        if (ganttChart) {
            ganttChart.change_view_mode(mode);
        }
    });

    setupGanttEvents();
    setupParentDragReorder();
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

    const userProfile = state.get('userProfile');
    const isReadOnly = !userProfile || userProfile.role === 'member';

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
            });
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
                showToast('Не удалось обновить прогресс: ' + err.message, 'error');
                renderGantt(state.get('tasks'));
            }
        },
    });

    applyPostRenderEnhancements(tasks);
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

    document.getElementById('task-details-modal').classList.remove('hidden');
}

function closeTaskDetailsModal() {
    document.getElementById('task-details-modal').classList.add('hidden');
    currentDetailsTaskId = null;
    currentDetailsOriginal = null;
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
    if (Object.keys(payload).length === 0) {
        closeTaskDetailsModal();
        return;
    }

    try {
        await apiPut(`/tasks/${currentDetailsTaskId}`, payload);
        showToast('Задача обновлена', 'success');
        closeTaskDetailsModal();
        // Прогресс/смещение листовой задачи может сдвинуть расписание всей
        // команды (конвейер) и зафиксировать факт закрытия — тянем полный
        // список заново.
        await reloadCurrentTeamTasks();
    } catch (err) {
        showToast('Не удалось сохранить: ' + err.message, 'error');
    }
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
                    await apiPut(`${endpointBase}/${id}/reorder`, { new_sort_order: newSortOrder });
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
            showToast('Не удалось изменить порядок: ' + err.message, 'error');
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
    document.getElementById('reorder-modal').classList.remove('hidden');
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
        showToast('Не удалось загрузить истории эпика: ' + err.message, 'error');
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
function openEpicReorderModal() {
    const epics = (state.get('epics') || []).filter(e => !e.parent_epic_id);

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
    document.getElementById('reorder-modal').classList.add('hidden');
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
        showToast('Не удалось сохранить изменения: ' + err.message, 'error');
    }
}

function setupGanttEvents() {
    document.getElementById('modal-close')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-cancel')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-save')?.addEventListener('click', saveOrder);
    document.querySelector('.modal-overlay')?.addEventListener('click', closeReorderModal);

    // Новые точки входа для переупорядочивания эпиков/сторей (тулбар Ганта).
    document.getElementById('btn-reorder-epics')?.addEventListener('click', openEpicReorderModal);
    document.getElementById('btn-reorder-stories')?.addEventListener('click', openStoryReorderModal);

    // Модалка деталей задачи (простановка % выполнения по клику на бар).
    document.getElementById('task-details-close')?.addEventListener('click', closeTaskDetailsModal);
    document.getElementById('task-details-cancel')?.addEventListener('click', closeTaskDetailsModal);
    document.getElementById('task-details-save')?.addEventListener('click', saveTaskDetails);
    document.querySelector('#task-details-modal .modal-overlay')?.addEventListener('click', closeTaskDetailsModal);

    // View modes
    document.querySelectorAll('.btn-view').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.btn-view').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.set('viewMode', btn.dataset.mode);
        });
    });
}
