// ── Main Application Entry Point ─────────────────────────────────────

import { state } from './state.js';
import { checkAuth, logout } from './auth.js';
import { apiGet, apiPost, apiPut } from './api.js';
import { showToast, handleApiError, withSubmitLock, openModal, closeModal } from './utils.js';

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
        await loadScheduleSettings(teamId);
        updateRegenerateQuarterButtonState();
    });

    // 3. Bind global event listeners
    setupGlobalEventListeners();

    // 4. Kick off authentication flow
    checkAuth();
});

// syncSelectTitle — выставляет нативный title равным тексту выбранной опции.
// Используется списками, усекающими значение многоточием (.select--team,
// .select--epic), чтобы полный текст оставался доступен по наведению
// (stabilize-control-widths, ux-brief.md раздел 2). #gantt-year/#gantt-quarter
// сюда не входят — у них title занят статичной подписью поля.
function syncSelectTitle(select) {
    if (!select) return;
    const selectedOption = select.options[select.selectedIndex];
    select.title = selectedOption ? selectedOption.textContent : '';
}

function handleProfileLoaded(profile) {
    // Fill header user info
    const nameEl = document.getElementById('user-name');
    const badgeEl = document.getElementById('user-role-badge');
    const profileContainer = document.getElementById('user-profile');
    
    if (nameEl) {
        const displayName = profile.first_name || profile.username;
        nameEl.textContent = displayName;
        // title — доступ к полному имени при усечении #user-name на узкой
        // ширине (stabilize-control-widths, ux-brief.md раздел 4).
        nameEl.title = displayName;
    }
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
        syncSelectTitle(select);

        // Auto-select first team if only one exists
        if (teams.length === 1 && !select.value) {
            select.value = teams[0].id;
            syncSelectTitle(select);
            select.dispatchEvent(new Event('change'));
        }
    } catch (err) {
        if (err.message !== 'FORBIDDEN' && err.message !== 'UNAUTHORIZED') {
            handleApiError(err, { title: 'Ошибка загрузки команд' });
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
        handleApiError(err, { title: 'Ошибка загрузки эпиков' });
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
            opt.textContent += ` (${epic.final_score} чд)`;
        }
        epicSelect.appendChild(opt);
    });

    epicSelect.disabled = !state.get('selectedTeamId') || filteredEpics.length === 0;

    // Если ранее выбранный эпик выпал из нового периода — сбрасываем выбор
    // и блокируем одиночную генерацию (спека — «Выбранный эпик выпал из периода»).
    const stillPresent = filteredEpics.some(epic => epic.id === previousEpicId);
    epicSelect.value = stillPresent ? previousEpicId : '';
    state.set('selectedEpicId', epicSelect.value);
    syncSelectTitle(epicSelect);

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
        handleApiError(err, { title: 'Ошибка загрузки задач' });
    }
}

// ── Настройка команды «не начинать задачи раньше сегодня» (backfill-idle-gaps-in-schedule,
// ux-brief.md) ─────────────────────────────────────────────────────────────────────────
// Настройка команды, а не состояние тулбара: читается для любой выбранной команды и любой
// роли (GET доступен всем аутентифицированным), меняет её только админ этой команды.
// DOM-узлы статичны (index.html) — innerHTML не перестраивается, состояния переключаются
// через classList/атрибуты, обработчик change навешивается один раз в setupGlobalEventListeners.

// loadScheduleSettings — читает текущее значение настройки для teamId и переключает блок
// между состояниями «команда не выбрана» / «загрузка» / «ошибка» / «значение загружено».
// При ошибке GET handleApiError не вызывается (ux-brief.md раздел 5): это фоновая подгрузка
// состояния тулбара, а не пользовательское действие — вместо тоста показывается кнопка «Повторить».
async function loadScheduleSettings(teamId) {
    const row = document.getElementById('toolbar-row-settings');
    const checkbox = document.getElementById('backfill-block-past-toggle');
    const label = document.getElementById('backfill-checkbox-label');
    const hint = document.getElementById('backfill-setting-hint');
    const memberHint = document.getElementById('backfill-setting-member-hint');
    const errorHint = document.getElementById('backfill-setting-error-hint');
    const retryBtn = document.getElementById('btn-backfill-setting-retry');
    if (!row || !checkbox || !label || !hint || !memberHint || !errorHint || !retryBtn) return;

    if (!teamId) {
        row.classList.add('hidden');
        return;
    }
    row.classList.remove('hidden');

    // Состояние «загрузка»: чекбокс disabled и подпись «Загрузка настройки…» — значение
    // ещё неизвестно. indeterminate — DOM-свойство, а не атрибут (setAttribute его не
    // выставляет): вместе со scoped-правилом .team-setting input:indeterminate
    // (components.css) это не даёт чекбоксу выглядеть ни включённым, ни выключенным,
    // пока ответ сервера не получен (ux-brief.md раздел 5, критерий приёмки 3).
    label.classList.remove('hidden');
    checkbox.disabled = true;
    checkbox.checked = false;
    checkbox.indeterminate = true;
    hint.textContent = 'Загрузка настройки…';
    hint.classList.remove('hidden');
    memberHint.classList.add('hidden');
    errorHint.classList.add('hidden');
    retryBtn.classList.add('hidden');

    try {
        const data = await apiGet(`/teams/${teamId}/schedule-settings`);
        // Пока запрос летел, могла быть выбрана другая команда — тогда этот ответ устарел.
        if (state.get('selectedTeamId') !== teamId) return;

        const role = state.get('userProfile')?.role;
        const isMember = role === 'member';

        checkbox.indeterminate = false;
        checkbox.checked = !!data.backfill_block_past;
        checkbox.disabled = isMember;
        hint.textContent = 'Настройка команды: действует на всё расписание и пересчитывает даты всех задач.';
        memberHint.classList.toggle('hidden', !isMember);
    } catch (err) {
        if (state.get('selectedTeamId') !== teamId) return;
        // Ошибка загрузки — чекбокс скрывается целиком, вместо него текст с кнопкой
        // повтора: показывать чекбокс в невыясненном состоянии нельзя (ux-brief.md раздел 5).
        checkbox.indeterminate = false;
        label.classList.add('hidden');
        hint.classList.add('hidden');
        errorHint.classList.remove('hidden');
        retryBtn.classList.remove('hidden');
    }
}

// onBackfillSettingToggle — обработчик клика по чекбоксу настройки. Читает teamId из state
// в момент клика (а не из замыкания на момент загрузки), т.к. слушатель навешивается один раз.
async function onBackfillSettingToggle(e) {
    const checkbox = e.target;
    const teamId = state.get('selectedTeamId');
    const hintEl = document.getElementById('backfill-setting-hint');
    const next = checkbox.checked;
    const prevChecked = !next;

    if (!teamId) {
        checkbox.checked = prevChecked;
        return;
    }

    await withSubmitLock(checkbox, async () => {
        if (hintEl) hintEl.textContent = 'Пересчитываем расписание…';
        try {
            const result = await apiPut(`/teams/${teamId}/schedule-settings`, { backfill_block_past: next });
            if (hintEl) hintEl.textContent = 'Настройка команды: действует на всё расписание и пересчитывает даты всех задач.';
            showToast(`Расписание пересчитано: обновлено задач — ${result.count}`, result.count > 0 ? 'success' : 'info');
            await loadTasks(teamId);
        } catch (err) {
            // При ошибке чекбокс возвращается в состояние до клика, а не остаётся
            // в промежуточном положении (ux-brief.md раздел 6, критерий приёмки 8).
            checkbox.checked = prevChecked;
            if (hintEl) hintEl.textContent = 'Настройка команды: действует на всё расписание и пересчитывает даты всех задач.';
            handleApiError(err, { title: 'Не удалось изменить настройку' });
        }
    });
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
        handleApiError(err, { title: 'Ошибка генерации задач' });
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
        modal = document.createElement('dialog');
        modal.id = 'modal-regenerate-quarter';
        modal.className = 'modal';
        modal.setAttribute('aria-labelledby', 'modal-regenerate-quarter-title');
        document.body.appendChild(modal);
        // Клик по фону (области dialog за пределами .modal-content) закрывает
        // модалку — замена клика по .modal-overlay при переходе на <dialog>
        // (design.md, Decision 3). Слушатель вешается один раз при создании
        // элемента, а не при каждой перерисовке innerHTML.
        modal.addEventListener('click', (e) => {
            if (!e.target.closest('.modal-content')) closeModal(modal);
        });
    }

    modal.innerHTML = `
        <div class="modal-content">
            <div class="modal-header">
                <h2 id="modal-regenerate-quarter-title">Перегенерация задач квартала</h2>
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
            <div class="modal-footer modal-footer--destructive">
                <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                <button type="button" id="btn-confirm-regenerate-quarter" class="btn btn-danger">Перегенерировать</button>
            </div>
        </div>
    `;

    openModal(modal);

    modal.querySelectorAll('.btn-close-modal').forEach(btn => {
        btn.onclick = () => closeModal(modal);
    });

    const btnConfirm = modal.querySelector('#btn-confirm-regenerate-quarter');
    btnConfirm.onclick = async () => {
        closeModal(modal);
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

    // btn-regenerate-quarter не является submit-кнопкой формы (кнопка вызывается
    // из модалки подтверждения) — передаётся в withSubmitLock напрямую, чтобы
    // блокировка отправки оставалась единым паттерном (design.md, Decision 7).
    await withSubmitLock(btn, async () => {
        if (btn) btn.innerHTML = '⏳ Перегенерация...';
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
            handleApiError(err, { title: 'Ошибка перегенерации квартала' });
        } finally {
            if (btn) btn.innerHTML = originalLabel;
        }
    });

    // withSubmitLock всегда снимает блокировку (disabled = false) в своём
    // finally; здесь восстанавливается фактически корректное состояние — кнопка
    // недоступна, если команда не выбрана (условие не связано с самой
    // блокировкой запроса, поэтому применяется отдельным шагом после неё).
    if (btn) btn.disabled = !state.get('selectedTeamId');
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
        syncSelectTitle(e.target);
        state.set('selectedTeamId', e.target.value);
    });

    // Epic selection change (Gantt tab)
    document.getElementById('epic-select')?.addEventListener('change', (e) => {
        syncSelectTitle(e.target);
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

    // Настройка команды «не начинать задачи раньше сегодня» (backfill-idle-gaps-in-schedule)
    document.getElementById('backfill-block-past-toggle')?.addEventListener('change', onBackfillSettingToggle);
    document.getElementById('btn-backfill-setting-retry')?.addEventListener('click', () => {
        loadScheduleSettings(state.get('selectedTeamId'));
    });
}

async function loadRoles() {
    try {
        const data = await apiGet('/roles');
        const roles = data.roles || [];
        state.set('roles', roles);
    } catch (err) {
        if (err.message !== 'FORBIDDEN' && err.message !== 'UNAUTHORIZED') {
            handleApiError(err, { title: 'Ошибка загрузки ролей' });
        }
    }
}

