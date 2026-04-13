// ── Gantt Chart Application ────────────────────────────────────────────

const API_BASE = '/api/gantt';

let ganttChart = null;
let currentTasks = [];
let currentViewMode = 'Day';
let globalRole = 'member';

// ── Auth ──────────────────────────────────────────────────────────────

function checkAuth() {
    // If we have the tg_sys_auth cookie, we're authenticated.
    const hasCookie = document.cookie.split(';')
        .some(c => c.trim().startsWith('tg_sys_auth='));
    if (hasCookie) {
        showApp();
        return;
    }

    // Check if we arrived via Telegram Login redirect (hash in URL).
    const params = new URLSearchParams(window.location.search);
    if (params.get('hash')) {
        // The server should have set the cookie. Reload without params.
        window.location.href = '/gantt/';
        return;
    }

    // Show auth overlay.
    showAuth();
}

function showAuth() {
    document.getElementById('auth-overlay').classList.remove('hidden');
    document.getElementById('app').classList.add('hidden');
}

function showApp() {
    document.getElementById('auth-overlay').classList.add('hidden');
    document.getElementById('app').classList.remove('hidden');
    init();
}

// ── Toast Notifications ───────────────────────────────────────────────

function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(20px)';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

// ── API Client ────────────────────────────────────────────────────────

async function apiGet(path) {
    const resp = await fetch(`${API_BASE}${path}`, {
        credentials: 'include',
    });
    if (resp.status === 401) {
        showAuth();
        throw new Error('unauthorized');
    }
    if (resp.status === 403) {
        showAccessDenied();
        throw new Error('forbidden');
    }
    if (!resp.ok) {
        const err = await resp.json().catch(() => ({}));
        throw new Error(err.error || resp.statusText);
    }
    return resp.json();
}

async function apiPost(path, body) {
    const resp = await fetch(`${API_BASE}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        credentials: 'include',
    });
    if (resp.status === 401) {
        showAuth();
        throw new Error('unauthorized');
    }
    if (!resp.ok) {
        const err = await resp.json().catch(() => ({}));
        throw new Error(err.error || resp.statusText);
    }
    return resp.json();
}

async function apiPut(path, body) {
    const resp = await fetch(`${API_BASE}${path}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        credentials: 'include',
    });
    if (resp.status === 401) {
        showAuth();
        throw new Error('unauthorized');
    }
    if (!resp.ok) {
        const err = await resp.json().catch(() => ({}));
        throw new Error(err.error || resp.statusText);
    }
    return resp.json();
}

// ── Init ──────────────────────────────────────────────────────────────

async function init() {
    // Set default start date to today.
    const today = new Date().toISOString().split('T')[0];
    document.getElementById('start-date').value = today;

    await loadTeams();
    setupEventListeners();
}

// ── Teams ─────────────────────────────────────────────────────────────

async function loadTeams() {
    try {
        const data = await apiGet('/teams');
        globalRole = data.role || 'member';

        // Apply RBAC to UI
        const btnGenerate = document.getElementById('btn-generate');
        if (globalRole === 'member') {
            btnGenerate.style.display = 'none';
        } else {
            btnGenerate.style.display = 'inline-flex';
        }

        const select = document.getElementById('team-select');
        select.innerHTML = '<option value="">Выберите команду...</option>';
        if (data.teams) {
            for (const team of data.teams) {
                const opt = document.createElement('option');
                opt.value = team.id;
                opt.textContent = team.name;
                select.appendChild(opt);
            }
        }
    } catch (err) {
        if (err.message !== 'forbidden' && err.message !== 'unauthorized') {
            showToast('Ошибка загрузки команд: ' + err.message, 'error');
        }
    }
}

function showAccessDenied() {
    document.getElementById('auth-overlay').classList.add('hidden');
    document.getElementById('app').classList.add('hidden');
    document.getElementById('denied-overlay').classList.remove('hidden');
}

// ── Epics ─────────────────────────────────────────────────────────────

async function loadEpics(teamID) {
    const epicSelect = document.getElementById('epic-select');
    const btnGenerate = document.getElementById('btn-generate');
    epicSelect.innerHTML = '<option value="">Выберите эпик...</option>';
    epicSelect.disabled = true;
    btnGenerate.disabled = true;

    if (!teamID) return;

    try {
        const data = await apiGet(`/epics?team_id=${teamID}`);
        if (data.epics && data.epics.length > 0) {
            for (const epic of data.epics) {
                const opt = document.createElement('option');
                opt.value = epic.id;
                opt.textContent = `${epic.number}: ${epic.name}`;
                if (epic.final_score) {
                    opt.textContent += ` (${epic.final_score} SP)`;
                }
                epicSelect.appendChild(opt);
            }
            epicSelect.disabled = false;
        }
    } catch (err) {
        showToast('Ошибка загрузки эпиков: ' + err.message, 'error');
    }
}

// ── Tasks ─────────────────────────────────────────────────────────────

async function loadTasks(teamID) {
    if (!teamID) {
        currentTasks = [];
        renderGantt([]);
        return;
    }

    try {
        const data = await apiGet(`/tasks?team_id=${teamID}`);
        currentTasks = data.tasks || [];
        renderGantt(currentTasks);
    } catch (err) {
        showToast('Ошибка загрузки задач: ' + err.message, 'error');
    }
}

async function generateTasks() {
    const epicID = document.getElementById('epic-select').value;
    const startDate = document.getElementById('start-date').value;

    if (!epicID) {
        showToast('Выберите эпик', 'error');
        return;
    }
    if (!startDate) {
        showToast('Укажите дату начала', 'error');
        return;
    }

    try {
        await apiPost('/tasks/generate', {
            epic_id: epicID,
            start_date: startDate,
        });
        showToast('Задачи сгенерированы', 'success');

        // Reload tasks.
        const teamID = document.getElementById('team-select').value;
        await loadTasks(teamID);
    } catch (err) {
        showToast('Ошибка генерации: ' + err.message, 'error');
    }
}

// ── Gantt Rendering ───────────────────────────────────────────────────

function renderGantt(tasks) {
    const container = document.getElementById('gantt-chart');
    const emptyState = document.getElementById('gantt-empty');
    const taskCount = document.getElementById('task-count');

    taskCount.textContent = `${tasks.length} задач`;

    if (tasks.length === 0) {
        container.innerHTML = '';
        emptyState.classList.remove('hidden');
        ganttChart = null;
        return;
    }

    emptyState.classList.add('hidden');

    // Convert to Frappe Gantt format.
    const ganttTasks = tasks.map(t => ({
        id: t.id,
        name: t.name,
        start: t.start,
        end: t.end,
        progress: t.progress,
        dependencies: t.dependencies || '',
        custom_class: t.custom_class || '',
        _is_parent: t.is_parent,
        _parent_id: t.parent_id,
        _sort_order: t.sort_order,
        _role_id: t.role_id,
    }));

    // Clear and re-render.
    container.innerHTML = '';

    ganttChart = new Gantt(container, ganttTasks, {
        view_mode: currentViewMode,
        date_format: 'YYYY-MM-DD',
        language: 'ru',
        infinite_padding: false,
        readonly: globalRole === 'member',
        on_click: task => {
            if (globalRole === 'member') return;
            if (task._is_parent) {
                openReorderModal(task);
            }
        },
        on_date_change: async (task, start, end) => {
            const startStr = formatDate(start);
            const endStr = formatDate(end);
            try {
                await apiPut(`/tasks/${task.id}`, {
                    start: startStr,
                    end: endStr,
                });
                showToast('Даты обновлены', 'success');
            } catch (err) {
                showToast('Ошибка: ' + err.message, 'error');
            }
        },
        on_progress_change: async (task, progress) => {
            try {
                await apiPut(`/tasks/${task.id}`, {
                    progress: progress,
                });
            } catch (err) {
                showToast('Ошибка: ' + err.message, 'error');
            }
        },
    });
}

function formatDate(date) {
    const d = new Date(date);
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

// ── Reorder Modal ─────────────────────────────────────────────────────

const roleColorMap = {
    'Аналитик': 'analyst',
    'BE разработчик': 'developer',
    'FE разработчик': 'developer',
    'Mobile разработчик': 'developer',
    'Тестировщик': 'qa',
    'IT-лидер': 'leader',
};

let reorderEpicTasks = [];

function openReorderModal(parentTask) {
    // Find children of this parent.
    const children = currentTasks.filter(
        t => t.parent_id === parentTask.id && !t.is_parent
    );
    if (children.length === 0) {
        showToast('У этого эпика нет подзадач', 'info');
        return;
    }

    reorderEpicTasks = children.sort(
        (a, b) => a.sort_order - b.sort_order
    );

    document.getElementById('modal-title').textContent =
        `Порядок ролей: ${parentTask.name.trim()}`;

    const body = document.getElementById('modal-body');
    body.innerHTML = '';

    for (const task of reorderEpicTasks) {
        const roleName = task.name.trim();
        const colorClass = roleColorMap[roleName] || 'default';

        const item = document.createElement('div');
        item.className = 'role-item';
        item.dataset.taskId = task.id;
        item.innerHTML = `
            <span class="drag-handle">⠿</span>
            <span class="role-color ${colorClass}"></span>
            <span class="role-name">${roleName}</span>
            <div class="role-order">
                <label>Порядок:</label>
                <input type="number" min="1" max="99"
                       value="${task.sort_order}"
                       data-task-id="${task.id}">
            </div>
        `;
        body.appendChild(item);
    }

    document.getElementById('reorder-modal').classList.remove('hidden');
}

function closeReorderModal() {
    document.getElementById('reorder-modal').classList.add('hidden');
    reorderEpicTasks = [];
}

async function saveReorder() {
    const inputs = document.querySelectorAll(
        '#modal-body .role-order input'
    );
    try {
        for (const input of inputs) {
            const taskId = input.dataset.taskId;
            const sortOrder = parseInt(input.value, 10);
            if (isNaN(sortOrder) || sortOrder < 1) continue;

            const task = reorderEpicTasks.find(t => t.id === taskId);
            if (task && task.sort_order !== sortOrder) {
                await apiPut(`/tasks/${taskId}/reorder`, {
                    sort_order: sortOrder,
                });
            }
        }
        showToast('Порядок обновлён', 'success');
        closeReorderModal();

        // Reload tasks.
        const teamID = document.getElementById('team-select').value;
        await loadTasks(teamID);
    } catch (err) {
        showToast('Ошибка: ' + err.message, 'error');
    }
}

// ── Event Listeners ───────────────────────────────────────────────────

function setupEventListeners() {
    // Team select.
    document.getElementById('team-select').addEventListener(
        'change', async (e) => {
            const teamID = e.target.value;
            await loadEpics(teamID);
            await loadTasks(teamID);
        }
    );

    // Epic select.
    document.getElementById('epic-select').addEventListener(
        'change', (e) => {
            document.getElementById('btn-generate').disabled =
                !e.target.value;
        }
    );

    // Generate button.
    document.getElementById('btn-generate').addEventListener(
        'click', generateTasks
    );

    // View mode buttons.
    document.querySelectorAll('.btn-view').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.btn-view').forEach(
                b => b.classList.remove('active')
            );
            btn.classList.add('active');
            currentViewMode = btn.dataset.mode;
            if (ganttChart) {
                ganttChart.change_view_mode(currentViewMode);
            }
        });
    });

    // Modal.
    document.getElementById('modal-close').addEventListener(
        'click', closeReorderModal
    );
    document.getElementById('modal-cancel').addEventListener(
        'click', closeReorderModal
    );
    document.getElementById('modal-save').addEventListener(
        'click', saveReorder
    );
    document.querySelector('.modal-overlay')?.addEventListener(
        'click', closeReorderModal
    );
}

// ── Bootstrap ─────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', checkAuth);
