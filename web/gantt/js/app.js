// ── Main Application Entry Point ─────────────────────────────────────

import { state } from './state.js';
import { checkAuth, logout } from './auth.js';
import { apiGet, apiPost } from './api.js';
import { showToast } from './utils.js';

// Import panel initializers
import { initGanttRenderer } from './gantt-renderer.js';
import { initAdminPanel } from './admin-panel.js';
import { initScoringPanel } from './scoring-panel.js';
import { initAIChat } from './ai-chat.js';
import { initReportsPanel } from './reports-panel.js';

document.addEventListener('DOMContentLoaded', () => {
    console.log('DOMContentLoaded: Инициализация модулей...');
    
    // 1. Initialise modules with error isolation
    const modules = [
        { name: 'GanttRenderer', init: initGanttRenderer },
        { name: 'AdminPanel', init: initAdminPanel },
        { name: 'ScoringPanel', init: initScoringPanel },
        { name: 'ReportsPanel', init: initReportsPanel },
        { name: 'AIChat', init: initAIChat }
    ];

    modules.forEach(m => {
        try {
            console.log(`Инициализация модуля ${m.name}...`);
            m.init();
        } catch (e) {
            console.error(`Ошибка при инициализации модуля ${m.name}:`, e);
        }
    });

    // 2. Setup state listeners
    state.subscribe('userProfile', (profile) => {
        if (profile) {
            handleProfileLoaded(profile);
        }
    });

    state.subscribe('selectedTeamId', async (teamId) => {
        if (teamId) {
            await loadEpics(teamId);
            await loadTasks(teamId);
        } else {
            state.set('epics', []);
            state.set('tasks', []);
        }
    });

    // 3. Bind global event listeners
    setupGlobalEventListeners();

    // 4. Kick off authentication flow
    checkAuth();
});

function handleProfileLoaded(profile) {
    // Fill header user info
    const nameEl = document.getElementById('user-name');
    const badgeEl = document.getElementById('user-role-badge');
    const profileContainer = document.getElementById('user-profile');
    
    if (nameEl) nameEl.textContent = profile.first_name || profile.username;
    if (badgeEl) {
        badgeEl.textContent = getRoleDisplayName(profile.role);
        badgeEl.className = `badge role-${profile.role}`;
    }
    if (profileContainer) profileContainer.style.display = 'inline-flex';

    // Apply RBAC to Tabs
    const tabAdminBtn = document.querySelector('.nav-tab[data-tab="admin"]');
    if (tabAdminBtn) {
        if (profile.role === 'admin' || profile.role === 'superadmin') {
            tabAdminBtn.style.display = 'inline-flex';
        } else {
            tabAdminBtn.style.display = 'none';
            // If active tab was admin but user role doesn't permit it, switch to gantt
            if (state.get('activeTab') === 'admin') {
                switchTab('gantt');
            }
        }
    }

    const btnGenerate = document.getElementById('btn-generate');
    if (btnGenerate) {
        if (profile.role === 'member') {
            btnGenerate.style.display = 'none';
        } else {
            btnGenerate.style.display = 'inline-flex';
        }
    }

    // Expand Telegram WebApp if available
    if (window.Telegram && window.Telegram.WebApp) {
        window.Telegram.WebApp.expand();
    }

    // Set default start date to today
    const today = new Date().toISOString().split('T')[0];
    const startDateInput = document.getElementById('start-date');
    if (startDateInput && !startDateInput.value) {
        startDateInput.value = today;
    }

    // Load teams and roles
    loadTeams();
    loadRoles();
}

function getRoleDisplayName(role) {
    if (role === 'superadmin') return 'Суперадмин';
    if (role === 'admin') return 'Администратор';
    if (role === 'leader') return 'Лидер команды';
    return 'Участник';
}

async function loadTeams() {
    try {
        const data = await apiGet('/teams');
        const teams = data.teams || [];
        state.set('teams', teams);

        const select = document.getElementById('team-select');
        if (!select) return;

        select.innerHTML = '<option value="">Выберите команду...</option>';
        teams.forEach(team => {
            const opt = document.createElement('option');
            opt.value = team.id;
            opt.textContent = team.name;
            select.appendChild(opt);
        });

        // Auto-select first team if only one exists
        if (teams.length === 1 && !select.value) {
            select.value = teams[0].id;
            select.dispatchEvent(new Event('change'));
        }
    } catch (err) {
        if (err.message !== 'FORBIDDEN' && err.message !== 'UNAUTHORIZED') {
            showToast('Ошибка загрузки команд: ' + err.message, 'error');
        }
    }
}

async function loadEpics(teamId) {
    const epicSelect = document.getElementById('epic-select');
    const btnGenerate = document.getElementById('btn-generate');
    
    if (epicSelect) {
        epicSelect.innerHTML = '<option value="">Выберите эпик...</option>';
        epicSelect.disabled = true;
    }
    if (btnGenerate) btnGenerate.disabled = true;

    try {
        const data = await apiGet(`/epics?team_id=${teamId}`);
        const epics = data.epics || [];
        state.set('epics', epics);

        if (epicSelect && epics.length > 0) {
            epics.forEach(epic => {
                const opt = document.createElement('option');
                opt.value = epic.id;
                opt.textContent = `${epic.number}: ${epic.name}`;
                if (epic.final_score) {
                    opt.textContent += ` (${epic.final_score} SP)`;
                }
                epicSelect.appendChild(opt);
            });
            epicSelect.disabled = false;
        }
    } catch (err) {
        showToast('Ошибка загрузки эпиков: ' + err.message, 'error');
    }
}

async function loadTasks(teamId) {
    try {
        const data = await apiGet(`/tasks?team_id=${teamId}`);
        state.set('tasks', data.tasks || []);
    } catch (err) {
        showToast('Ошибка загрузки задач: ' + err.message, 'error');
    }
}

async function generateTasks() {
    const epicId = document.getElementById('epic-select').value;
    const startDate = document.getElementById('start-date').value;

    if (!epicId) {
        showToast('Выберите эпик для генерации задач', 'error');
        return;
    }
    if (!startDate) {
        showToast('Укажите дату начала генерации', 'error');
        return;
    }

    try {
        await apiPost('/tasks/generate', {
            epic_id: epicId,
            start_date: startDate,
        });
        showToast('Задачи успешно сгенерированы!', 'success');

        // Reload tasks
        const teamId = state.get('selectedTeamId');
        await loadTasks(teamId);
    } catch (err) {
        showToast('Ошибка генерации задач: ' + err.message, 'error');
    }
}

function switchTab(tabName) {
    state.set('activeTab', tabName);

    // Update tab button classes
    document.querySelectorAll('.nav-tab').forEach(btn => {
        if (btn.dataset.tab === tabName) {
            btn.classList.add('active');
            btn.setAttribute('aria-current', 'page');
        } else {
            btn.classList.remove('active');
            btn.removeAttribute('aria-current');
        }
    });

    // Update tab content displays
    document.querySelectorAll('.tab-content').forEach(section => {
        if (section.id === `tab-content-${tabName}`) {
            section.classList.remove('hidden');
        } else {
            section.classList.add('hidden');
        }
    });
}

function setupGlobalEventListeners() {
    // Logout button
    document.getElementById('btn-logout')?.addEventListener('click', logout);

    // Navigation Tabs
    document.querySelectorAll('.nav-tab').forEach(btn => {
        btn.addEventListener('click', () => {
            switchTab(btn.dataset.tab);
        });
    });

    // Team selection change
    document.getElementById('team-select')?.addEventListener('change', (e) => {
        state.set('selectedTeamId', e.target.value);
    });

    // Epic selection change (Gantt tab)
    document.getElementById('epic-select')?.addEventListener('change', (e) => {
        const btnGenerate = document.getElementById('btn-generate');
        if (btnGenerate) {
            btnGenerate.disabled = !e.target.value;
        }
        state.set('selectedEpicId', e.target.value);
    });

    // Generate tasks button
    document.getElementById('btn-generate')?.addEventListener('click', generateTasks);
}

async function loadRoles() {
    try {
        const data = await apiGet('/roles');
        const roles = data.roles || [];
        state.set('roles', roles);
    } catch (err) {
        if (err.message !== 'FORBIDDEN' && err.message !== 'UNAUTHORIZED') {
            showToast('Ошибка загрузки ролей: ' + err.message, 'error');
        }
    }
}

