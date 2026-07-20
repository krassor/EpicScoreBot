// ── Gantt Chart Renderer Module ──────────────────────────────────────

import { state } from './state.js';
import { apiPut, apiGet } from './api.js';
import { showToast, formatDate } from './utils.js';

let ganttChart = null;
let reorderEpicTasks = [];

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
            displayName = `\u00a0\u00a0└─\u00a0${t.name}`;
        } else if (level === 3) {
            displayName = `\u00a0\u00a0\u00a0\u00a0└─\u00a0${t.name}`;
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

    container.innerHTML = '';
    
    const userProfile = state.get('userProfile');
    const isReadOnly = !userProfile || userProfile.role === 'member';

    ganttChart = new Gantt(container, ganttTasks, {
        view_mode: state.get('viewMode'),
        date_format: 'YYYY-MM-DD',
        language: 'ru',
        infinite_padding: false,
        readonly: isReadOnly,
        on_click: task => {
            if (isReadOnly) return;
            if (task._is_parent) {
                openReorderModal(task);
            }
        },
        on_date_change: async (task, start, end) => {
            const startStr = formatDate(start);
            const endStr = formatDate(end);
            try {
                await apiPut(`/tasks/${task.id}`, {
                    start_date: startStr,
                    end_date: endStr,
                });
                showToast('Даты задачи обновлены', 'success');
                // Reload tasks for current team
                reloadCurrentTeamTasks();
            } catch (err) {
                showToast('Не удалось обновить даты: ' + err.message, 'error');
            }
        },
        on_progress_change: async (task, progress) => {
            try {
                await apiPut(`/tasks/${task.id}`, {
                    progress: progress / 100, // API expects 0.0-1.0
                });
                showToast('Прогресс задачи обновлен', 'success');
            } catch (err) {
                showToast('Не удалось обновить прогресс: ' + err.message, 'error');
            }
        },
    });
}

function getTaskClass(task) {
    if (task.is_parent) {
        if (task.parent_task_id || task.parent_id) {
            return 'gantt-story';
        }
        return 'gantt-epic';
    }
    
    // Extrapolate role name from task name if role_id is not directly resolved
    // Usually name matches role: "Аналитик", "BE разработчик", etc.
    const name = task.name.trim();
    const colorClass = roleColorMap[name] || 'default';
    return `gantt-${colorClass}`;
}

async function reloadCurrentTeamTasks() {
    const teamId = state.get('selectedTeamId');
    if (!teamId) return;
    try {
        const data = await apiGet(`/tasks?team_id=${teamId}`);
        state.set('tasks', data.tasks || []);
    } catch (err) {
        console.error('Failed to reload tasks:', err);
    }
}

// ── Reorder Modal logic ──────────────────────────────────────────────

function openReorderModal(parentTask) {
    const currentTasks = state.get('tasks');
    const children = currentTasks.filter(
        t => (t.parent_task_id === parentTask.id || t.parent_id === parentTask.id) && !t.is_parent
    );
    
    if (children.length === 0) {
        showToast('У этого эпика нет подзадач для сортировки', 'info');
        return;
    }

    reorderEpicTasks = children.sort((a, b) => a.sort_order - b.sort_order);

    document.getElementById('modal-title').textContent = `Порядок ролей: ${parentTask.name.trim()}`;

    const body = document.getElementById('modal-body');
    body.innerHTML = '';

    for (const task of reorderEpicTasks) {
        const roleName = task.name.trim();
        const colorClass = roleColorMap[roleName] || 'default';

        const item = document.createElement('div');
        item.className = 'role-item';
        item.dataset.taskId = task.id;
        item.draggable = true;
        item.innerHTML = `
            <span class="drag-handle">⠿</span>
            <span class="role-color ${colorClass}"></span>
            <span class="role-name">${roleName}</span>
            <div class="role-order">
                <label>Порядок:</label>
                <input type="number" min="1" max="99" class="select"
                       value="${task.sort_order}"
                       data-task-id="${task.id}" style="width: 60px; padding: 4px 8px;">
            </div>
        `;
        body.appendChild(item);
    }

    setupDragAndDropForModal();
    document.getElementById('reorder-modal').classList.remove('hidden');
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
    reorderEpicTasks = [];
}

async function saveReorder() {
    const inputs = document.querySelectorAll('#modal-body .role-order input');
    try {
        let updated = false;
        for (const input of inputs) {
            const taskId = input.dataset.taskId;
            const sortOrder = parseInt(input.value, 10);
            if (isNaN(sortOrder) || sortOrder < 1) continue;

            const task = reorderEpicTasks.find(t => t.id === taskId);
            if (task && task.sort_order !== sortOrder) {
                await apiPut(`/tasks/${taskId}/reorder`, {
                    new_sort_order: sortOrder,
                });
                updated = true;
            }
        }
        
        if (updated) {
            showToast('Порядок ролей успешно обновлен', 'success');
        }
        closeReorderModal();
        reloadCurrentTeamTasks();
    } catch (err) {
        showToast('Не удалось сохранить изменения: ' + err.message, 'error');
    }
}

function setupGanttEvents() {
    document.getElementById('modal-close')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-cancel')?.addEventListener('click', closeReorderModal);
    document.getElementById('modal-save')?.addEventListener('click', saveReorder);
    document.querySelector('.modal-overlay')?.addEventListener('click', closeReorderModal);

    // View modes
    document.querySelectorAll('.btn-view').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.btn-view').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.set('viewMode', btn.dataset.mode);
        });
    });
}
