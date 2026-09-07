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
            populateYearSelect([]);
            renderEpicSelectForPeriod();
        }
        updateRegenerateQuarterButtonState();
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

    // Одиночная и массовая (квартальная) генерация задач — один и тот же уровень
    // доступа, скрываются для роли member одинаково (спека — «Доступ к массовой
    // перегенерации»).
    const btnGenerate = document.getElementById('btn-generate');
    const btnRegenerateQuarter = document.getElementById('btn-regenerate-quarter');
    [btnGenerate, btnRegenerateQuarter].forEach(btn => {
        if (!btn) return;
        btn.style.display = profile.role === 'member' ? 'none' : 'inline-flex';
    });

    // Переупорядочивание эпиков/историй — тот же уровень доступа, что и генерация Ганта.
    const btnReorderEpics = document.getElementById('btn-reorder-epics');
    const btnReorderStories = document.getElementById('btn-reorder-stories');
    [btnReorderEpics, btnReorderStories].forEach(btn => {
        if (!btn) return;
        btn.style.display = profile.role === 'member' ? 'none' : 'inline-flex';
    });

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

    // Год/квартал панели Ганта — по умолчанию текущие календарные значения
    // (спека «Выбор года и квартала на панели инструментов Ганта»).
    initGanttPeriodSelectors();

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

        // Список годов на панели периода строится из фактических годов эпиков
        // команды (design.md Decision 4), затем список для генерации фильтруется
        // по уже выбранным году/кварталу.
        populateYearSelect(epics);
        renderEpicSelectForPeriod();
    } catch (err) {
        showToast('Ошибка загрузки эпиков: ' + err.message, 'error');
    }
}

// ── Панель периода вкладки «Гант» (год/квартал, add-gantt-quarter-regenerate) ────────
// Список кварталов фиксирован разметкой (Q1–Q4). Список годов строится динамически
// из годов эпиков команды и всегда содержит текущий календарный год (спека
// «Выбор года и квартала на панели инструментов Ганта»).

function getCurrentQuarter() {
    return Math.floor(new Date().getMonth() / 3) + 1;
}

// initGanttPeriodSelectors — выставляет год/квартал по умолчанию (текущие календарные
// значения) при загрузке приложения, до выбора команды и загрузки эпиков.
function initGanttPeriodSelectors() {
    populateYearSelect([]);
    const quarterSelect = document.getElementById('gantt-quarter');
    if (quarterSelect) quarterSelect.value = String(getCurrentQuarter());
}

// populateYearSelect — перестраивает список опций селекта года как отсортированное
// множество различных epic.year из переданного списка эпиков, всегда дополняя его
// текущим календарным годом (даже если у команды нет эпиков этого года), и по
// возможности сохраняет ранее выбранное значение.
function populateYearSelect(epics) {
    const yearSelect = document.getElementById('gantt-year');
    if (!yearSelect) return;

    const currentYear = new Date().getFullYear();
    const years = Array.from(new Set([
        currentYear,
        ...(epics || []).map(epic => epic.year).filter(year => year !== undefined && year !== null)
    ])).sort((a, b) => a - b);

    const previousValue = yearSelect.value || String(currentYear);
    yearSelect.innerHTML = years.map(year => `<option value="${year}">${year}</option>`).join('');
    yearSelect.value = years.some(year => String(year) === previousValue)
        ? previousValue
        : String(currentYear);
}

// renderEpicSelectForPeriod — перестраивает #epic-select только из эпиков команды,
// чьи year/quarter совпадают с выбранными на панели периода (клиентская фильтрация
// уже загруженного state.get('epics'), design.md Decision 4). Диаграмма Ганта при
// этом не трогается — фильтр влияет только на список для одиночной генерации.
function renderEpicSelectForPeriod() {
    const epicSelect = document.getElementById('epic-select');
    if (!epicSelect) return;

    const btnGenerate = document.getElementById('btn-generate');
    const yearSelect = document.getElementById('gantt-year');
    const quarterSelect = document.getElementById('gantt-quarter');
    const year = yearSelect ? yearSelect.value : '';
    const quarter = quarterSelect ? quarterSelect.value : '';

    const allEpics = state.get('epics') || [];
    const previousEpicId = epicSelect.value;

    const filteredEpics = allEpics.filter(epic =>
        String(epic.year) === year && String(epic.quarter) === quarter
    );

    epicSelect.innerHTML = '<option value="">Выберите эпик...</option>';
    filteredEpics.forEach(epic => {
        const opt = document.createElement('option');
        opt.value = epic.id;
        opt.textContent = `${epic.number}: ${epic.name}`;
        if (epic.final_score) {
            opt.textContent += ` (${epic.final_score} SP)`;
        }
        epicSelect.appendChild(opt);
    });

    epicSelect.disabled = !state.get('selectedTeamId') || filteredEpics.length === 0;

    // Если ранее выбранный эпик выпал из нового периода — сбрасываем выбор
    // и блокируем одиночную генерацию (спека — «Выбранный эпик выпал из периода»).
    const stillPresent = filteredEpics.some(epic => epic.id === previousEpicId);
    epicSelect.value = stillPresent ? previousEpicId : '';
    state.set('selectedEpicId', epicSelect.value);

    if (btnGenerate) {
        btnGenerate.disabled = !epicSelect.value;
    }
}

// updateRegenerateQuarterButtonState — кнопка массовой перегенерации доступна только
// при выбранной команде; более тонкая валидация (дата, наличие эпиков периода)
// выполняется при клике, см. onRegenerateQuarterClick.
function updateRegenerateQuarterButtonState() {
    const btn = document.getElementById('btn-regenerate-quarter');
    if (btn) btn.disabled = !state.get('selectedTeamId');
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

// ── Массовая перегенерация задач квартала (add-gantt-quarter-regenerate) ─────────────

// onRegenerateQuarterClick — клиентская валидация перед показом модалки подтверждения
// (спека «Валидация параметров массовой перегенерации», «Не заполнена дата в
// интерфейсе»): без команды и без даты начала запрос вообще не готовится.
function onRegenerateQuarterClick() {
    const teamId = state.get('selectedTeamId');
    if (!teamId) {
        showToast('Выберите команду', 'error');
        return;
    }

    const startDate = document.getElementById('start-date').value;
    if (!startDate) {
        showToast('Укажите дату начала для перегенерации квартала', 'error');
        return;
    }

    const yearSelect = document.getElementById('gantt-year');
    const quarterSelect = document.getElementById('gantt-quarter');
    const year = yearSelect ? parseInt(yearSelect.value, 10) : NaN;
    const quarter = quarterSelect ? parseInt(quarterSelect.value, 10) : NaN;

    openRegenerateQuarterModal({ teamId, year, quarter, startDate });
}

// openRegenerateQuarterModal — кастомная модалка подтверждения по образцу
// openDeleteEpicModal (web/gantt/js/scoring-panel.js) вместо window.confirm
// (design.md Decision 5): операция необратима, поэтому в тексте явно указаны
// период, дата начала и предупреждение о том, что затронуты только заскоренные
// эпики периода, а прогресс/фактические даты их задач будут потеряны.
function openRegenerateQuarterModal({ teamId, year, quarter, startDate }) {
    let modal = document.getElementById('modal-regenerate-quarter');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-regenerate-quarter';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content">
            <div class="modal-header">
                <h2>Перегенерация задач квартала</h2>
                <button type="button" class="btn-icon btn-close-modal">✕</button>
            </div>
            <div class="modal-body">
                <p>
                    Перегенерировать задачи всех заскоренных эпиков за
                    <strong>${year} год, Q${quarter}</strong> начиная с даты
                    <strong>${startDate}</strong>?
                </p>
                <p style="color: var(--color-danger);">
                    Будут перегенерированы только эпики этого периода в статусе
                    «Заскорен» — эпики в статусах «Новый» и «На скоринге» не
                    затрагиваются. Существующие задачи выбранных эпиков будут удалены
                    и созданы заново вместе с прогрессом и фактическими датами их
                    выполнения. Отменить это действие будет невозможно.
                </p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                <button type="button" id="btn-confirm-regenerate-quarter" class="btn btn-danger">Перегенерировать</button>
            </div>
        </div>
    `;

    modal.classList.remove('hidden');
    modal.style.display = 'flex';

    const closeModal = () => {
        modal.classList.add('hidden');
        modal.style.display = 'none';
    };

    modal.querySelectorAll('.btn-close-modal, .modal-overlay').forEach(btn => {
        btn.onclick = closeModal;
    });

    const btnConfirm = modal.querySelector('#btn-confirm-regenerate-quarter');
    btnConfirm.onclick = async () => {
        closeModal();
        await runRegenerateQuarter({ teamId, year, quarter, startDate });
    };
}

// runRegenerateQuarter — вызов POST /tasks/generate-quarter (контракт бэкенда не
// меняется): кнопка блокируется на время запроса, после успеха задачи команды
// перезагружаются, а пользователь получает сводку по количеству перегенерированных
// и (если есть) неуспешных эпиков (спека «Подтверждение перед массовой
// перегенерацией», «Валидация параметров массовой перегенерации»).
async function runRegenerateQuarter({ teamId, year, quarter, startDate }) {
    const btn = document.getElementById('btn-regenerate-quarter');
    const originalLabel = btn ? btn.innerHTML : '';
    if (btn) {
        btn.disabled = true;
        btn.innerHTML = '⏳ Перегенерация...';
    }

    try {
        const result = await apiPost('/tasks/generate-quarter', {
            team_id: teamId,
            year,
            quarter,
            start_date: startDate,
        });

        if (!result.epics_total) {
            showToast('В выбранном квартале нет заскоренных эпиков — перегенерировать нечего', 'info');
        } else if (result.epics_failed > 0) {
            showToast(
                `Перегенерировано эпиков: ${result.epics_regenerated} из ${result.epics_total}. ` +
                `Не удалось перегенерировать: ${result.epics_failed}`,
                'error'
            );
        } else {
            showToast(`Задачи квартала перегенерированы: ${result.epics_regenerated} эпик(ов)`, 'success');
        }

        await loadTasks(teamId);
    } catch (err) {
        showToast('Ошибка перегенерации квартала: ' + err.message, 'error');
    } finally {
        if (btn) {
            btn.disabled = !state.get('selectedTeamId');
            btn.innerHTML = originalLabel;
        }
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

    // Смена года/квартала на панели периода (Гант) — перестраивает список эпиков
    // для генерации немедленно, без перезагрузки страницы (спека — «Смена квартала
    // перестраивает список»). Сама диаграмма Ганта не затрагивается.
    document.getElementById('gantt-year')?.addEventListener('change', renderEpicSelectForPeriod);
    document.getElementById('gantt-quarter')?.addEventListener('change', renderEpicSelectForPeriod);

    // Массовая перегенерация задач квартала
    document.getElementById('btn-regenerate-quarter')?.addEventListener('click', onRegenerateQuarterClick);
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

