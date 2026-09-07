// ── Scoring Panel Module ─────────────────────────────────────────────

import { state } from './state.js';
import { apiGet, apiPost, apiPut, apiDelete } from './api.js';
import { showToast, showErrorModal } from './utils.js';

let selectedEpic = null;
let rolesList = [];
let adminEpicVotes = {}; // Store temporary admin votes before save: { userId: score }

let currentStories = [];
let selectedStory = null;
let adminStoryVotes = {};

let selectedEpicScores = null;
let selectedEpicRoleScores = null;
let selectedEpicRisks = null;

let selectedStoryScores = null;
let selectedStoryRoleScores = null;
let selectedStoryRisks = null;

// ── Фильтры списка «Эпики команды» ───────────────────────────────────
// allTeamEpics — полный (нефильтрованный) список эпиков команды, как он пришёл с бэкенда;
// именно к нему применяются клиентские фильтры перед каждым рендером списка.
let allTeamEpics = [];
// epicFilters — текущие значения трёх фильтров списка эпиков. Пустая строка означает «Все»
// (без ограничения по этому параметру).
let epicFilters = { year: '', quarter: '', type: '' };
// loadedEpicsTeamId — team_id, для которого сейчас загружен allTeamEpics. Используется,
// чтобы отличить смену команды (фильтры нужно сбросить на «Все») от повторной загрузки
// списка для той же команды, например после старта скоринга или удаления эпика (фильтры
// пользователя в этом случае сохраняются).
let loadedEpicsTeamId = null;


export function initScoringPanel() {
    // Inject custom styles for admin controls
    injectStyles();

    // Listen for team changes to reload epics
    state.subscribe('selectedTeamId', (teamId) => {
        if (teamId) {
            loadScoringEpics(teamId);
        } else {
            allTeamEpics = [];
            loadedEpicsTeamId = null;
            resetEpicsFilters();
            renderEpicsList([]);
            clearDetails();
        }
    });

    // Load list of roles from backend
    loadRoles();

    // Listen for story creation from admin panel to hot-reload current epic details
    window.addEventListener('story-created', async (e) => {
        if (selectedEpic && selectedEpic.id === e.detail.epicId) {
            await loadEpicData();
        }
    });
}

function injectStyles() {
    if (document.getElementById('admin-scoring-styles')) return;
    const style = document.createElement('style');
    style.id = 'admin-scoring-styles';
    style.textContent = `
        .btn-vote-mini {
            background: var(--bg-tertiary);
            border: 1px solid var(--color-border);
            color: var(--color-text);
            padding: 4px 8px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 12px;
            font-weight: 600;
            transition: all 0.2s ease;
        }
        .btn-vote-mini:hover {
            border-color: var(--color-primary);
            background: rgba(79, 70, 229, 0.1);
        }
        .btn-vote-mini.active {
            background: var(--color-primary);
            border-color: var(--color-primary);
            color: #fff;
            box-shadow: 0 0 8px rgba(79, 70, 229, 0.4);
        }
        .admin-scores-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 12px;
        }
        .admin-scores-table th, .admin-scores-table td {
            padding: 10px 12px;
            text-align: left;
            border-bottom: 1px solid var(--color-border);
            vertical-align: middle;
        }
        .admin-scores-table th {
            color: var(--text-muted);
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .admin-scores-table tr:hover {
            background: rgba(255, 255, 255, 0.02);
        }
        .vote-mini-grid {
            display: flex;
            flex-wrap: wrap;
            gap: 4px;
        }
        .risk-votes-list {
            margin-top: 8px;
            padding-left: 12px;
            border-left: 2px solid var(--color-border);
            display: flex;
            flex-direction: column;
            gap: 6px;
            font-size: 13px;
        }
        .risk-vote-member-item {
            color: var(--color-text);
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .risk-vote-member-badge {
            background: var(--bg-tertiary);
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
        }
    `;
    document.head.appendChild(style);
}

async function loadRoles() {
    try {
        const data = await apiGet('/roles');
        rolesList = data.roles || [];
        state.set('roles', rolesList);
    } catch (e) {
        console.error('Failed to load roles:', e);
        // Fallback roles
        rolesList = [
            { id: '1', name: 'Аналитик' },
            { id: '2', name: 'BE разработчик' },
            { id: '3', name: 'FE разработчик' },
            { id: '4', name: 'Mobile разработчик' },
            { id: '5', name: 'Тестировщик' },
            { id: '6', name: 'IT-лидер' }
        ];
    }
}

async function loadScoringEpics(teamId) {
    const listContainer = document.getElementById('scoring-epics-list');
    if (listContainer) {
        listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-muted);">Загрузка...</div>';
    }

    try {
        const data = await apiGet(`/epics?team_id=${teamId}&all=true`);
        const epics = data.epics || [];
        allTeamEpics = epics;

        // Фильтры сбрасываются на «Все» только при смене команды, а не при каждой
        // перезагрузке списка для той же команды (например, после старта скоринга).
        const teamChanged = teamId !== loadedEpicsTeamId;
        loadedEpicsTeamId = teamId;

        ensureEpicsFilterBar();
        if (teamChanged) {
            resetEpicsFilters();
        }
        updateYearFilterOptions(epics);

        renderFilteredEpicsList();
    } catch (err) {
        showToast('Не удалось загрузить список эпиков: ' + err.message, 'error');
    }
}

// ensureEpicsFilterBar — создаёт (один раз) панель из трёх фильтров над списком
// «Эпики команды»: Год, Квартал, Тип эпика. Использует уже существующие в проекте
// CSS-классы .form-grid-3col/.form-group/.select (та же сетка, что и в форме
// редактирования эпика для полей Год/Квартал/Тип), новых стилей не вводит.
function ensureEpicsFilterBar() {
    let bar = document.getElementById('scoring-epics-filters');
    if (bar) return bar;

    const listContainer = document.getElementById('scoring-epics-list');
    if (!listContainer || !listContainer.parentElement) return null;

    bar = document.createElement('div');
    bar.id = 'scoring-epics-filters';
    bar.className = 'form-grid-3col';
    bar.setAttribute('style', 'padding: 12px 20px; border-bottom: 1px solid var(--border-color);');
    bar.innerHTML = `
        <div class="form-group">
            <label for="scoring-filter-year">Год</label>
            <select id="scoring-filter-year" class="select">
                <option value="">Все</option>
            </select>
        </div>
        <div class="form-group">
            <label for="scoring-filter-quarter">Квартал</label>
            <select id="scoring-filter-quarter" class="select">
                <option value="">Все</option>
                <option value="1">Q1</option>
                <option value="2">Q2</option>
                <option value="3">Q3</option>
                <option value="4">Q4</option>
            </select>
        </div>
        <div class="form-group">
            <label for="scoring-filter-type">Тип эпика</label>
            <select id="scoring-filter-type" class="select">
                <option value="">Все</option>
                <option value="feature">Feature</option>
                <option value="architecture">Architecture</option>
                <option value="techdebt">Techdebt</option>
            </select>
        </div>
    `;
    listContainer.parentElement.insertBefore(bar, listContainer);

    bar.querySelectorAll('select').forEach(select => {
        select.addEventListener('change', onEpicsFilterChange);
    });

    return bar;
}

// onEpicsFilterChange — обработчик изменения любого из трёх фильтров: пересчитывает
// epicFilters из текущих значений <select> и перерисовывает список без обращения к серверу.
function onEpicsFilterChange() {
    const yearSelect = document.getElementById('scoring-filter-year');
    const quarterSelect = document.getElementById('scoring-filter-quarter');
    const typeSelect = document.getElementById('scoring-filter-type');
    epicFilters = {
        year: yearSelect ? yearSelect.value : '',
        quarter: quarterSelect ? quarterSelect.value : '',
        type: typeSelect ? typeSelect.value : ''
    };
    renderFilteredEpicsList();
}

// updateYearFilterOptions — перестраивает список опций фильтра «Год» как отсортированное
// множество различных значений epic.year из фактически загруженного списка эпиков команды.
function updateYearFilterOptions(epics) {
    const yearSelect = document.getElementById('scoring-filter-year');
    if (!yearSelect) return;

    const years = Array.from(new Set(
        epics.map(epic => epic.year).filter(year => year !== undefined && year !== null)
    )).sort((a, b) => a - b);

    const previousValue = yearSelect.value;
    yearSelect.innerHTML = '<option value="">Все</option>' +
        years.map(year => `<option value="${year}">${year}</option>`).join('');

    // Сохраняем выбранное значение, если оно всё ещё присутствует среди актуальных лет
    // (например, при повторной загрузке списка той же команды без смены фильтров).
    if (years.some(year => String(year) === previousValue)) {
        yearSelect.value = previousValue;
    }
}

// resetEpicsFilters — сбрасывает все три фильтра списка эпиков на значение «Все».
// Вызывается при смене команды, т.к. набор годов/кварталов/типов новой команды
// может не иметь ничего общего со старыми значениями фильтров.
function resetEpicsFilters() {
    epicFilters = { year: '', quarter: '', type: '' };
    const yearSelect = document.getElementById('scoring-filter-year');
    const quarterSelect = document.getElementById('scoring-filter-quarter');
    const typeSelect = document.getElementById('scoring-filter-type');
    if (yearSelect) yearSelect.value = '';
    if (quarterSelect) quarterSelect.value = '';
    if (typeSelect) typeSelect.value = '';
}

// applyEpicsFilters — клиентская фильтрация: эпик проходит, если для каждого из трёх
// параметров выбрано «Все» (пустая строка) либо значение совпадает с полем эпика.
function applyEpicsFilters(epics) {
    return epics.filter(epic => {
        if (epicFilters.year && String(epic.year) !== epicFilters.year) return false;
        if (epicFilters.quarter && String(epic.quarter) !== epicFilters.quarter) return false;
        if (epicFilters.type && epic.type !== epicFilters.type) return false;
        return true;
    });
}

// renderFilteredEpicsList — применяет текущие фильтры к allTeamEpics и рендерит результат.
// Используется вместо прямого вызова renderEpicsList(allTeamEpics) везде, где список
// эпиков команды (пере)загружается или изменяются фильтры.
function renderFilteredEpicsList() {
    renderEpicsList(applyEpicsFilters(allTeamEpics));
}

function renderEpicsList(epics) {
    const container = document.getElementById('scoring-epics-list');
    if (!container) return;

    if (epics.length === 0) {
        container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-muted);">Нет эпиков в команде</div>';
        return;
    }

    container.innerHTML = '';
    epics.forEach(epic => {
        const item = document.createElement('div');
        item.className = 'scoring-epic-item';
        if (selectedEpic && selectedEpic.id === epic.id) {
            item.classList.add('active');
        }

        const statusClass = epic.status.toLowerCase();
        let statusText = 'Новый';
        if (epic.status === 'SCORING') statusText = 'Оценка';
        if (epic.status === 'SCORED') statusText = 'Оценен';

        item.innerHTML = `
            <div class="scoring-epic-header">
                <span class="epic-num">${epic.number}</span>
                <span class="status-badge ${statusClass}">${statusText}</span>
            </div>
            <div class="scoring-epic-name">${epic.name}</div>
        `;

        item.addEventListener('click', () => {
            document.querySelectorAll('.scoring-epic-item').forEach(el => el.classList.remove('active'));
            item.classList.add('active');
            selectEpic(epic);
        });

        container.appendChild(item);
    });
}

function clearDetails() {
    const container = document.getElementById('scoring-details');
    if (container) {
        container.innerHTML = `
            <div class="empty-state">
                <div class="empty-icon">⚡</div>
                <h2>Выберите эпик для оценки</h2>
            </div>
        `;
    }
    selectedEpic = null;
}

async function selectEpic(epic) {
    selectedEpic = epic;
    adminEpicVotes = {}; 
    adminStoryVotes = {};
    selectedStory = null;
    currentStories = [];
    
    const container = document.getElementById('scoring-details');
    if (!container) return;

    container.innerHTML = '<div style="padding: 40px; text-align: center; color: var(--text-muted);"><div class="loader"></div><p style="margin-top:16px;">Загрузка деталей эпика...</p></div>';

    await loadEpicData();
}

async function loadEpicData() {
    if (!selectedEpic) return;
    
    try {
        // 1. Fetch stories
        // Защита от null: apiGet может успешно вернуть 200 OK с телом null
        // (например, если бэкенд не инициализировал пустой срез) — в этом
        // случае .catch() не сработает, поэтому дополнительно подстраховываемся `|| []`.
        currentStories = (await apiGet(`/epics/${selectedEpic.id}/stories`).catch(() => ([]))) || [];

        if (currentStories.length > 0) {
            // Переприсваиваем на свежий объект из currentStories по id: `some()` только
            // проверяет наличие id, но не подменяет ссылку, из-за чего selectedStory мог
            // оставаться устаревшим (старый status/final_score) даже когда сторя с тем же
            // id уже перезагружена с сервера. Если сторя пропала из списка — как и раньше,
            // берём первую.
            const freshSelectedStory = selectedStory
                ? currentStories.find(s => s.id === selectedStory.id)
                : null;
            selectedStory = freshSelectedStory || currentStories[0];

            // Fetch selected story details
            selectedStoryScores = (await apiGet(`/epics/${selectedStory.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }))) || { scores: [], expected: 0, received: 0 };
            selectedStoryRoleScores = (await apiGet(`/epics/${selectedStory.id}/role-scores`).catch(() => ([]))) || [];
            const risksData = (await apiGet(`/epics/${selectedStory.id}/risks`).catch(() => ({ risks: [] }))) || { risks: [] };
            selectedStoryRisks = risksData.risks || [];
        } else {
            selectedStory = null;
            // Fetch epic details (fallback when no stories exist)
            selectedEpicScores = (await apiGet(`/epics/${selectedEpic.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }))) || { scores: [], expected: 0, received: 0 };
            selectedEpicRoleScores = (await apiGet(`/epics/${selectedEpic.id}/role-scores`).catch(() => ([]))) || [];
            const risksData = (await apiGet(`/epics/${selectedEpic.id}/risks`).catch(() => ({ risks: [] }))) || { risks: [] };
            selectedEpicRisks = risksData.risks || [];
        }

        renderDetails();
    } catch (err) {
        showToast('Не удалось загрузить данные: ' + err.message, 'error');
    }
}

// Каскадный рефреш после админского редактирования оценки: пересчёт final_score
// стори может каскадно изменить final_score родительского эпика на бэкенде,
// поэтому дополнительно перезагружаем список эпиков и актуализируем selectedEpic,
// который иначе останется "протухшим" и покажет старый бейдж в шапке.
async function refreshAfterAdminEdit() {
    const teamId = state.get('selectedTeamId');
    if (teamId && selectedEpic) {
        try {
            const data = await apiGet(`/epics?team_id=${teamId}&all=true`);
            const epics = data.epics || [];
            const fresh = epics.find(e => e.id === selectedEpic.id);
            if (fresh) selectedEpic = fresh;
            allTeamEpics = epics;
            // Фильтры намеренно не сбрасываются и не пересобираются — это точечный
            // рефреш той же команды после админского редактирования оценки, а не смена команды.
            renderFilteredEpicsList();
        } catch (e) {
            // не блокируем остальной рефреш, если список эпиков не удалось перезагрузить
        }
    }
    await loadEpicData(); // вызывает renderDetails() в конце
}

function renderDetails() {
    const container = document.getElementById('scoring-details');
    if (!container || !selectedEpic) return;

    const userProfile = state.get('userProfile');
    const isLeaderOrAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin' || userProfile.role === 'leader');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');
    // Удаление эпика доступно только superadmin (в отличие от редактирования, доступного также admin) —
    // соответствует ограничению доступа на бэкенде (DELETE /epics/{epic_id} защищён RoleAuth "superadmin")
    const isSuperAdmin = userProfile && userProfile.role === 'superadmin';

    let actionButtonHtml = '';
    if (selectedEpic.status === 'NEW' && isLeaderOrAdmin) {
        actionButtonHtml = `<button id="btn-start-scoring" class="btn btn-primary">⚡ Запустить скоринг</button>`;
    }

    const finalScoreText = selectedEpic.final_score !== null && selectedEpic.final_score !== undefined
        ? `<div class="badge" style="background: rgba(16, 185, 129, 0.2); color: var(--color-role-be); font-size: 14px; padding: 6px 14px;">Итоговая оценка: ${selectedEpic.final_score} чд</div>`
        : `<div class="badge" style="background: var(--bg-tertiary); font-size: 14px; padding: 6px 14px;">Статус оценки: ${selectedEpic.status === 'NEW' ? 'Не начата' : 'В процессе'}</div>`;

    let html = `
        <div class="scoring-details-header">
            <div class="scoring-epic-title">
                <div style="display: flex; align-items: center; gap: 8px;">
                    <h2>${selectedEpic.number}: ${selectedEpic.name}</h2>
                    ${isAdmin ? `<button id="btn-edit-epic" class="btn btn-secondary btn-sm" title="Редактировать эпик" style="padding: 3px 8px; font-size: 12px;">✏️ Редактировать</button>` : ''}
                    ${isSuperAdmin ? `<button id="btn-delete-epic" class="btn btn-danger btn-sm" title="Удалить эпик" style="padding: 3px 8px; font-size: 12px;">🗑️ Удалить</button>` : ''}
                    ${isAdmin && selectedEpic.status === 'SCORING' ? `<button id="btn-notify-epic" class="btn btn-secondary btn-sm" title="Напомнить непроголосовавшим участникам" style="padding: 3px 8px; font-size: 12px;">🔔 Напомнить непроголосовавшим</button>` : ''}
                </div>
                <div class="scoring-epic-desc">${selectedEpic.description || 'Нет описания.'}</div>
            </div>
            <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 8px;">
                ${finalScoreText}
                ${actionButtonHtml}
            </div>
        </div>
    `;

    // Render 3-level split screen with stories
    html += `
        <div class="scoring-panel-grid" style="margin-top: 20px;">
            <!-- Left Column: Stories list & Admin Create Form -->
            <div class="scoring-section-card">
                <h3>Истории (стори) эпика (${currentStories.length})</h3>
                <div class="stories-list" style="display: flex; flex-direction: column; gap: 8px; max-height: 400px; overflow-y: auto; padding-right: 4px;">
                    ${currentStories.length === 0 ? `
                        <div style="padding: 20px; text-align: center; color: var(--text-muted); font-size: 13px;">
                            Нет историй (сторей) в этом эпике.
                        </div>
                    ` : currentStories.map(story => {
                        const isSelected = selectedStory && selectedStory.id === story.id;
                        const statusClass = story.status.toLowerCase();
                        let statusText = 'Новый';
                        if (story.status === 'SCORING') statusText = 'Оценка';
                        if (story.status === 'SCORED') statusText = 'Оценен';

                        const scoreBadge = story.final_score !== null && story.final_score !== undefined
                            ? `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: var(--color-role-be); font-size: 11px;">${story.final_score} чд</span>`
                            : `<span class="badge ${statusClass}" style="font-size: 11px;">${statusText}</span>`;

                        const deleteBtn = isAdmin ? `
                            <button class="btn-delete-story" data-story-id="${story.id}" title="Удалить историю" style="background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 4px; font-size: 14px; display: flex; align-items: center; justify-content: center; transition: color 0.2s;">
                                ❌
                            </button>
                        ` : '';

                        return `
                            <div class="story-item ${isSelected ? 'active' : ''}" data-story-id="${story.id}" style="display: flex; align-items: center; justify-content: space-between; padding: 10px 12px; background: ${isSelected ? 'rgba(79, 70, 229, 0.15)' : 'var(--bg-tertiary)'}; border: 1px solid ${isSelected ? 'var(--color-primary)' : 'var(--color-border)'}; border-radius: 6px; cursor: pointer; transition: all 0.2s;">
                                <div style="display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; margin-right: 8px;">
                                    <div style="font-weight: 600; font-size: 13px; color: var(--color-text); text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
                                        ${story.number}: ${story.name}
                                    </div>
                                    <div style="font-size: 11px; color: var(--text-muted); text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
                                        ${story.description || 'Нет описания'}
                                    </div>
                                </div>
                                <div style="display: flex; align-items: center; gap: 8px;">
                                    ${scoreBadge}
                                    ${deleteBtn}
                                </div>
                            </div>
                        `;
                    }).join('')}
                </div>

                ${isAdmin && selectedEpic.status === 'NEW' ? `
                    <div class="create-story-section" style="margin-top: 16px; border-top: 1px solid var(--color-border); padding-top: 16px;">
                        <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 10px; color: var(--color-text);">Создать новую историю (сторю)</h4>
                        <div class="form-group" style="margin-bottom: 8px;">
                            <input type="text" id="new-story-name" class="input" placeholder="Название истории" style="width: 100%;">
                        </div>
                        <div class="form-group" style="margin-bottom: 8px;">
                            <textarea id="new-story-desc" class="input" placeholder="Описание истории" style="width: 100%; min-height: 60px; resize: vertical; font-family: inherit;"></textarea>
                        </div>
                        <button id="btn-create-story" class="btn btn-primary" style="width: 100%; padding: 8px 12px; font-size: 13px;">Добавить историю</button>
                    </div>
                ` : ''}
            </div>

            <!-- Right Column: Selected Story Details -->
            <div class="scoring-section-card" id="story-details-card">
                ${selectedStory ? renderStoryDetailsHtml(selectedStory, selectedStoryScores, selectedStoryRoleScores, selectedStoryRisks) : (
                    currentStories.length === 0 ? `
                    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; min-height: 200px; color: var(--text-muted); text-align: center;">
                        <span>📭 У этого эпика пока нет ни одной истории (сторя).${isAdmin && selectedEpic.status === 'NEW' ? ' Добавьте её через форму слева.' : ''}</span>
                    </div>
                    ` : `
                    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; min-height: 200px; color: var(--text-muted); text-align: center;">
                        <span>👈 Выберите историю (сторю) слева для голосования и оценки рисков</span>
                    </div>
                    `
                )}
            </div>
        </div>
    `;

    container.innerHTML = html;
    bindEvents(isAdmin, isLeaderOrAdmin);
}

function renderProgressHtml(scoresData) {
    if (!scoresData) return '';
    const received = scoresData.scores_received || scoresData.scores?.length || 0;
    const expected = scoresData.scores_expected || 1;
    const percent = Math.min(100, Math.round((received / expected) * 100));
    return `
        <div class="form-group" style="margin-top: 10px;">
            <label style="font-size: 12px;">Прогресс оценок: ${received} из ${expected} (${percent}%)</label>
            <div style="background: var(--bg-tertiary); border-radius: 6px; height: 8px; width: 100%; overflow: hidden; margin-top: 4px;">
                <div style="background: var(--accent-primary); width: ${percent}%; height: 100%; transition: width 0.3s ease;"></div>
            </div>
        </div>
    `;
}

// Оверрайд оценки роли (третья колонка) доступен только администратору и только
// после завершения скоринга стори (SCORED) — тот же признак admin-прав, что и у
// остальных элементов переопределения на этой панели (кнопка «Изменить» у оценок
// участников, кнопка «Переопределить» у финальной оценки).
function isRoleScoreOverrideVisible(story, isAdmin) {
    return !!(isAdmin && story && story.status === 'SCORED');
}

// Массовое проставление экспертной оценки роли (btn-expert-role-score) доступно только
// администратору и только пока скоринг ещё идёт (SCORING) — отдельный, новый механизм
// для активной фазы скоринга, не путать с переопределением выше (доступно только после SCORED).
function isExpertRoleVoteVisible(story, isAdmin) {
    return !!(isAdmin && story && story.status === 'SCORING');
}

// calculateRoleScoreAggregate — средневзвешенная оценка и покрытие голосами одной роли,
// посчитанные на клиенте из уже загруженного ответа GET /epics/{id}/scores (scoresData).
//
// Формула зеркалит scoring.CalculateEpicRoleAvg (internal/scoring/scoring.go:46) —
// Σ(score × weight) / Σ(weight) по голосам с этим role_id, при нулевом суммарном весе
// возвращается 0, как и на бэкенде. Источник истины для формулы — именно эта функция
// бэкенда, менять формулу здесь без сверки с ней нельзя.
//
// Расчёт живёт на клиенте, а не берётся из GET /epics/{id}/role-scores, по двум причинам:
// 1) пока стори в статусе SCORING, этот эндпоинт всегда возвращает пустой список — бэкенд
//    заполняет epic_role_scores только внутри TryCompleteEpicScoring, когда проголосовала
//    уже вся команда (см. комментарий у buildEvaluatingRolesForScoring);
// 2) ответ GET /epics/{id}/scores уже содержит всё необходимое — user.weight по каждому
//    голосу и members[] с weight/role_id по каждому участнику команды, — то есть считать
//    агрегат не из чего дополнительно запрашивать, всё уже загружено в selectedStoryScores.
//
// Покрытие роли (voted_count/expected_count) считается по текущему составу команды
// (scoresData.members), а не по числу голосов: expected_count — сколько участников
// команды сейчас закреплено за этой ролью, voted_count — сколько из них уже проголосовало
// ИМЕННО за эту роль. Участник без role_id (пустая строка — например, если бэкенд не
// смог определить его роль) не совпадёт ни с одним roleId и поэтому не учитывается ни в
// числителе, ни в знаменателе ни одной роли.
//
// Голос засчитывается в voted_count только при совпадении и user_id, и role_id: голос
// хранит ту роль, которая была у участника на момент голосования, и если участник сменил
// роль уже после этого, его старый голос не попадёт в weightedSum новой роли (там фильтр
// по role_id). Считать его проголосовавшим за новую роль было бы неверно — роль показала
// бы окончательное значение, посчитанное без него. При таком расхождении роль остаётся
// непокрытой и показывает прочерк — сознательно консервативный исход.
function calculateRoleScoreAggregate(roleId, scoresData) {
    const scores = (scoresData && scoresData.scores) || [];
    const members = (scoresData && scoresData.members) || [];

    const roleMembers = members.filter(m => m.role_id === roleId);
    const expected_count = roleMembers.length;
    const voted_count = roleMembers.filter(
        m => scores.some(s => s.user_id === m.id && s.role_id === roleId)
    ).length;

    const roleScores = scores.filter(s => s.role_id === roleId);
    let weightedSum = 0;
    let totalWeight = 0;
    roleScores.forEach(s => {
        const weight = (s.user && typeof s.user.weight === 'number') ? s.user.weight : 0;
        weightedSum += s.score * weight;
        totalWeight += weight;
    });
    const weighted_avg = totalWeight === 0 ? 0 : weightedSum / totalWeight;

    return { weighted_avg, voted_count, expected_count };
}

// buildEvaluatingRolesForScoring — источник данных для таблицы «Оценки по ролям» пока
// скоринг ещё идёт (SCORING). GET /epics/{id}/role-scores в этом статусе всегда возвращает
// пустой список (заполняется только внутри TryCompleteEpicScoring при переходе в SCORED),
// поэтому список ролей строится из evaluating_role_ids уже загруженного объекта стори/эпика,
// а имена ролей сопоставляются через общий rolesList (тот же справочник, что и в форме
// редактирования эпика для чекбоксов ролей) — нового запроса за списком ролей не требуется.
// Дополнительно по каждой роли считается weighted_avg/voted_count/expected_count из
// scoresData (ответ GET /epics/{id}/scores) — см. calculateRoleScoreAggregate — чтобы
// рендер оставался чистым отображением, а расчёт был сосредоточен здесь.
function buildEvaluatingRolesForScoring(story, scoresData) {
    const roleIds = (story && story.evaluating_role_ids) || [];
    return roleIds.map(roleId => {
        const role = rolesList.find(r => r.id === roleId);
        const aggregate = calculateRoleScoreAggregate(roleId, scoresData);
        return {
            role_id: roleId,
            role_name: role ? role.name : roleId,
            weighted_avg: aggregate.weighted_avg,
            voted_count: aggregate.voted_count,
            expected_count: aggregate.expected_count,
        };
    });
}

// Строки таблицы «Оценки по ролям» пока скоринг ещё идёт (SCORING). Колонка «Оценка (чд)»:
// если по роли проголосовали все ожидаемые участники команды (voted_count === expected_count
// и expected_count > 0) — показывается посчитанная средневзвешенная, в ТОМ ЖЕ формате, что
// и в renderRoleScoresTableRows() для статуса SCORED (`<значение> чд`), чтобы ячейка визуально
// не менялась при переходе стори в SCORED. Иначе — прочерк и индикатор покрытия вида
// «(voted_count/expected_count)» приглушённым цветом. Колонка «Действие» (поле ввода +
// кнопка «Оценить всей ролью») показывается независимо от покрытия — админ может исправить
// ошибочную экспертную оценку и у уже покрытой роли.
function renderRoleScoresTableRowsScoring(evaluatingRoles, story, isAdmin) {
    const showExpertVote = isExpertRoleVoteVisible(story, isAdmin);
    const colspan = showExpertVote ? 3 : 2;
    if (!evaluatingRoles || evaluatingRoles.length === 0) {
        return `<tr><td colspan="${colspan}" style="color: var(--text-muted); text-align: center; padding: 12px 0;">Нет оцениваемых ролей</td></tr>`;
    }
    return evaluatingRoles.map(r => {
        const isFullyCovered = r.expected_count > 0 && r.voted_count === r.expected_count;
        // Формат ячейки при полном покрытии — точно как в renderRoleScoresTableRows()
        // для статуса SCORED, чтобы значение не «дёргалось» при переходе стори в SCORED.
        const scoreCellHtml = isFullyCovered
            ? `<td>${r.weighted_avg} чд</td>`
            : `<td style="color: var(--text-muted);">— (${r.voted_count}/${r.expected_count})</td>`;
        return `
        <tr>
            <td><strong>${r.role_name}</strong></td>
            ${scoreCellHtml}
            ${showExpertVote ? `
            <td>
                <div style="display: flex; align-items: center; gap: 6px;">
                    <input type="number" class="input expert-role-score-input" data-role-id="${r.role_id}" min="0" step="1" placeholder="чд" style="width: 70px; padding: 4px 6px; font-size: 12px;">
                    <button class="btn btn-secondary btn-expert-role-score" data-role-id="${r.role_id}" data-role-name="${r.role_name}" style="padding: 4px 8px; font-size: 11px;">Оценить всей ролью</button>
                </div>
            </td>` : ''}
        </tr>
    `;
    }).join('');
}

// Полное содержимое таблицы «Оценки по ролям» пока скоринг ещё идёт (SCORING) —
// см. buildEvaluatingRolesForScoring для объяснения источника данных. scoresData
// (ответ GET /epics/{id}/scores) нужен для расчёта весов/покрытия по ролям.
function renderRoleScoresTableHtmlScoring(story, isAdmin, scoresData) {
    const showExpertVote = isExpertRoleVoteVisible(story, isAdmin);
    const evaluatingRoles = buildEvaluatingRolesForScoring(story, scoresData);
    return `
        <thead>
            <tr>
                <th>Роль</th>
                <th>Оценка (чд)</th>
                ${showExpertVote ? '<th>Действие</th>' : ''}
            </tr>
        </thead>
        <tbody>
            ${renderRoleScoresTableRowsScoring(evaluatingRoles, story, isAdmin)}
        </tbody>
    `;
}

function renderRoleScoresTableRows(roleScores, story, isAdmin) {
    const showOverride = isRoleScoreOverrideVisible(story, isAdmin);
    const colspan = showOverride ? 3 : 2;
    if (!roleScores || roleScores.length === 0) {
        return `<tr><td colspan="${colspan}" style="color: var(--text-muted); text-align: center; padding: 12px 0;">Нет оценок</td></tr>`;
    }
    return roleScores.map(rs => `
        <tr>
            <td><strong>${rs.role_name || rs.role_id}</strong></td>
            <td>${rs.weighted_avg !== undefined ? rs.weighted_avg : rs.score} чд</td>
            ${showOverride ? `
            <td>
                <div style="display: flex; align-items: center; gap: 6px;">
                    <input type="number" class="input role-score-override-input" data-role-id="${rs.role_id}" min="0" step="1" placeholder="чд" style="width: 70px; padding: 4px 6px; font-size: 12px;">
                    <button class="btn btn-primary btn-override-role-score" data-role-id="${rs.role_id}" style="padding: 4px 8px; font-size: 11px;">Переопределить</button>
                </div>
            </td>` : ''}
        </tr>
    `).join('');
}

// Полное содержимое таблицы «Оценки по ролям» (thead + tbody), чтобы можно было
// целиком заменить innerHTML таблицы при точечном рефреше после переопределения
// оценки роли, без перерисовки всей панели скоринга.
//
// Пока стори/эпик в статусе SCORING, источник данных — не roleScores (ответ
// GET /epics/{id}/role-scores, в этом статусе всегда пустой), а evaluating_role_ids
// самой стори/эпика плюс scoresData (ответ GET /epics/{id}/scores, нужен для расчёта
// средневзвешенной и покрытия по ролям) — см. renderRoleScoresTableHtmlScoring. При
// SCORED и других статусах поведение не меняется: scoresData туда не пробрасывается,
// используется только roleScores.
function renderRoleScoresTableHtml(roleScores, story, isAdmin, scoresData) {
    if (story && story.status === 'SCORING') {
        return renderRoleScoresTableHtmlScoring(story, isAdmin, scoresData);
    }
    const showOverride = isRoleScoreOverrideVisible(story, isAdmin);
    return `
        <thead>
            <tr>
                <th>Роль</th>
                <th>Оценка (чд)</th>
                ${showOverride ? '<th>Действие</th>' : ''}
            </tr>
        </thead>
        <tbody>
            ${renderRoleScoresTableRows(roleScores, story, isAdmin)}
        </tbody>
    `;
}

// Обработчики кнопок «Переопределить» в строках таблицы «Оценки по ролям».
// Вынесены отдельно, т.к. навешиваются и при первичном рендере панели, и при
// точечном рефреше только этой таблицы после успешного переопределения.
function bindRoleScoreOverrideEvents(scopeEl) {
    if (!scopeEl) return;
    scopeEl.querySelectorAll('.btn-override-role-score').forEach(btn => {
        btn.addEventListener('click', async () => {
            if (!selectedStory) return;
            const roleId = btn.dataset.roleId;
            const row = btn.closest('tr');
            const input = row?.querySelector(`.role-score-override-input[data-role-id="${roleId}"]`);
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите оценку роли', 'error');
                return;
            }
            const score = Number(valStr);
            if (isNaN(score) || score < 0) {
                showToast('Оценка роли должна быть числом не меньше 0', 'error');
                return;
            }

            try {
                await apiPost('/admin/scores/role', {
                    epic_id: selectedStory.id,
                    role_id: roleId,
                    score: score
                });
                showToast('Оценка роли переопределена!', 'success');
                await refreshRoleScoresTable();
            } catch (err) {
                showErrorModal(err.message);
            }
        });
    });
}

// Обработчики кнопок «Оценить всей ролью» в строках таблицы «Оценки по ролям» (видны
// только пока стори/эпик в статусе SCORING, см. isExpertRoleVoteVisible). Перед отправкой
// запроса открывается модалка подтверждения (openExpertRoleScoreModal) — сам запрос
// уходит на сервер только после явного подтверждения администратором.
function bindExpertRoleScoreEvents(scopeEl) {
    if (!scopeEl) return;
    scopeEl.querySelectorAll('.btn-expert-role-score').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!selectedStory) return;
            const roleId = btn.dataset.roleId;
            const roleName = btn.dataset.roleName;
            const row = btn.closest('tr');
            const input = row?.querySelector(`.expert-role-score-input[data-role-id="${roleId}"]`);
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите оценку роли', 'error');
                return;
            }
            const score = Number(valStr);
            if (isNaN(score) || score < 0) {
                showToast('Оценка роли должна быть числом не меньше 0', 'error');
                return;
            }

            openExpertRoleScoreModal(roleId, roleName, score);
        });
    });
}

// Модальное окно подтверждения массового экспертного проставления оценки роли —
// предупреждает, что действие перезапишет личные голоса участников этой роли, если
// они уже есть (design.md, Decision 3). Паттерн подтверждения скопирован с уже
// существующей модалки удаления эпика (openDeleteEpicModal).
function openExpertRoleScoreModal(roleId, roleName, score) {
    let modal = document.getElementById('modal-expert-role-score');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-expert-role-score';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content">
            <div class="modal-header">
                <h2>Экспертная оценка роли</h2>
                <button class="btn-icon btn-close-modal">✕</button>
            </div>
            <div class="modal-body">
                <p>
                    Проставить оценку <strong>${score} чд</strong> за роль <strong>${roleName}</strong> всем участникам команды с этой ролью?
                </p>
                <p style="color: var(--color-danger);">
                    Это перезапишет личные голоса участников этой роли, если они уже есть. Отменить действие будет невозможно.
                </p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                <button type="button" id="btn-confirm-expert-role-score" class="btn btn-primary">Проставить оценку</button>
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

    const btnConfirm = modal.querySelector('#btn-confirm-expert-role-score');
    btnConfirm.onclick = async () => {
        if (!selectedStory) {
            closeModal();
            return;
        }

        try {
            await apiPost('/admin/scores/role/expert', {
                epic_id: selectedStory.id,
                role_id: roleId,
                score: score
            });
            showToast('Экспертная оценка роли проставлена!', 'success');
            closeModal();
            await loadEpicData();
        } catch (err) {
            showErrorModal(err.message);
        }
    };
}

// Точечный рефреш только таблицы «Оценки по ролям» после переопределения: заново
// запрашивает GET /epics/{id}/role-scores и заменяет innerHTML таблицы, не трогая
// остальную панель скоринга стори. Пока стори в статусе SCORING, содержимое строк
// зависит ещё и от selectedStoryScores (голоса участников, из которых считается
// средневзвешенная и покрытие по ролям — см. calculateRoleScoreAggregate), поэтому
// он тоже перезапрашивается здесь тем же способом с .catch()-фолбэком, что в loadEpicData().
async function refreshRoleScoresTable() {
    if (!selectedStory) return;
    selectedStoryRoleScores = (await apiGet(`/epics/${selectedStory.id}/role-scores`).catch(() => ([]))) || [];
    selectedStoryScores = (await apiGet(`/epics/${selectedStory.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }))) || { scores: [], expected: 0, received: 0 };

    const table = document.getElementById('role-scores-table');
    if (!table) return;

    const userProfile = state.get('userProfile');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');

    table.innerHTML = renderRoleScoresTableHtml(selectedStoryRoleScores, selectedStory, isAdmin, selectedStoryScores);
    bindRoleScoreOverrideEvents(table);
    // Кнопки «Оценить всей ролью» видны только в статусе SCORING (renderRoleScoresTableHtmlScoring);
    // без повторной привязки они остались бы без обработчиков после перерисовки таблицы.
    bindExpertRoleScoreEvents(table);
}

function renderAdminScoresTableRows(epicOrStory, scoresData) {
    if (!scoresData) return '';
    const isStory = !!epicOrStory.parent_epic_id || !!epicOrStory.parent_id;
    const activeVotesObj = isStory ? adminStoryVotes : adminEpicVotes;
    const members = scoresData.members || [];
    
    if (members.length === 0) {
        return '<tr><td colspan="4" style="color: var(--text-muted); text-align: center; padding: 12px 0;">Нет участников в команде</td></tr>';
    }
    
    return members.map(m => {
        const userScore = scoresData.scores?.find(s => s.user_id === m.id);
        const hasVoted = !!userScore;
        const isEditing = activeVotesObj[m.id] !== undefined || !hasVoted;
        const selectedVal = activeVotesObj[m.id] !== undefined ? activeVotesObj[m.id] : (userScore ? userScore.score : null);

        let voteControlHtml = '';
        if (isEditing) {
            voteControlHtml = `
                <div style="display: flex; align-items: center; justify-content: center;">
                    <input type="number" class="input admin-score-input" data-user-id="${m.id}" data-is-story="${isStory}" min="0" max="500" placeholder="чд" value="${selectedVal !== null ? selectedVal : ''}" style="width: 70px; text-align: center; margin-right: 6px; padding: 4px 6px; font-size:12px;">
                    <button class="btn btn-primary btn-save-admin-vote" data-user-id="${m.id}" data-is-story="${isStory}" style="padding:4px 8px; font-size:11px;">Ок</button>
                </div>
            `;
        } else {
            voteControlHtml = `<span style="font-weight:600; color:var(--color-role-be);">${userScore.score} чд</span>`;
        }

        let actionsHtml = '';
        if (isEditing) {
            actionsHtml = `
                <div style="display:flex; gap:4px;">
                    ${hasVoted ? `<button class="btn btn-secondary btn-cancel-admin-vote" data-user-id="${m.id}" data-is-story="${isStory}" style="padding:4px 8px; font-size:11px;">Отмена</button>` : ''}
                </div>
            `;
        } else {
            actionsHtml = `<button class="btn btn-secondary btn-edit-admin-vote" data-user-id="${m.id}" data-is-story="${isStory}" style="padding:4px 8px; font-size:11px;">Изменить</button>`;
        }

        // telegram_id может храниться как с ведущим "@" (например, при CSV-импорте),
        // так и без него (как сохраняет Telegram-бот) — нормализуем перед отображением,
        // чтобы не получить "@@username".
        const telegramUsername = m.telegram_id ? m.telegram_id.replace(/^@+/, '') : '';
        const telegramLinkHtml = telegramUsername
            ? ` <a href="https://t.me/${telegramUsername}" target="_blank" rel="noopener noreferrer" class="telegram-username-link">@${telegramUsername}</a>`
            : '';

        return `
            <tr>
                <td><strong>${m.first_name} ${m.last_name || ''}</strong>${telegramLinkHtml}</td>
                <td><span class="badge" style="background:var(--bg-tertiary); font-size:10px; padding: 2px 6px;">${m.role_name || 'Без роли'}</span></td>
                <td>${voteControlHtml}</td>
                <td>${actionsHtml}</td>
            </tr>
        `;
    }).join('');
}

function renderRisksHtml(epicOrStory, scoresData, risks) {
    if (!risks || risks.length === 0) {
        return '<div style="color: var(--text-muted); text-align: center; padding: 12px 0;">Риски не добавлены.</div>';
    }
    
    const isStory = !!epicOrStory.parent_epic_id || !!epicOrStory.parent_id;
    const userProfile = state.get('userProfile');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');
    
    return risks.map(risk => {
        const hasScore = risk.weighted_score !== null && risk.weighted_score !== undefined;
        const scoreDisplay = hasScore ? `<span class="risk-score-value">Итоговый вес: ${risk.weighted_score}</span>` : '<span style="color: var(--text-muted)">Не оценен</span>';
        
        const riskScoresHtml = risk.scores && risk.scores.length > 0
            ? `<div class="risk-votes-list">
                ${risk.scores.map(rs => {
                    const userName = `${rs.user?.first_name || ''} ${rs.user?.last_name || ''}`.trim() || rs.user?.telegram_id || 'Участник';
                    return `
                        <div class="risk-vote-member-item">
                            <span style="color: var(--text-muted);">${userName}:</span>
                            <span class="risk-vote-member-badge" style="color: var(--color-role-be)">P: ${rs.probability}</span>
                            <span class="risk-vote-member-badge" style="color: var(--color-role-fe)">I: ${rs.impact}</span>
                        </div>
                    `;
                }).join('')}
               </div>`
            : '<div style="color: var(--text-muted); font-size: 11px; margin-top: 4px; padding-left: 12px;">Оценок участников нет</div>';

        const userRiskScore = risk.scores?.find(rs => rs.user_id === userProfile?.id);
        const hasUserScoredRisk = !!userRiskScore;

        let userRiskSelectorsHtml = '';
        if (!isAdmin && epicOrStory.status === 'SCORING' && !hasUserScoredRisk) {
            userRiskSelectorsHtml = `
                <div class="risk-vote-selectors" style="margin-top: 8px;">
                    <div class="risk-sel-group">
                        <label style="font-size: 11px;">Вероятность (1-4)</label>
                        <select class="select risk-prob" style="min-width: 0; padding: 4px; font-size: 12px;">
                            <option value="1">1 - Низкая</option>
                            <option value="2">2 - Умеренная</option>
                            <option value="3">3 - Высокая</option>
                            <option value="4">4 - Критическая</option>
                        </select>
                    </div>
                    <div class="risk-sel-group">
                        <label style="font-size: 11px;">Влияние (1-4)</label>
                        <select class="select risk-imp" style="min-width: 0; padding: 4px; font-size: 12px;">
                            <option value="1">1 - Незначительное</option>
                            <option value="2">2 - Умеренное</option>
                            <option value="3">3 - Серьезное</option>
                            <option value="4">4 - Катастрофическое</option>
                        </select>
                    </div>
                    <button class="btn btn-secondary btn-vote-risk" data-is-story="${isStory}" style="align-self: flex-end; padding: 4px 8px; font-size: 12px;">Оценить</button>
                </div>
            `;
        }

        let adminRiskSelectorsHtml = '';
        if (isAdmin && (epicOrStory.status === 'SCORING' || epicOrStory.status === 'SCORED')) {
            const members = scoresData?.members || [];
            adminRiskSelectorsHtml = `
                <div class="risk-vote-selectors admin-risk-override" style="margin-top: 8px; display: flex; gap: 8px; flex-wrap: wrap;">
                    <div class="risk-sel-group" style="flex: 1.5; min-width: 120px;">
                        <label style="font-size: 11px;">Участник</label>
                        <select class="select risk-admin-user" style="width: 100%; min-width: 0; padding: 4px; font-size: 12px;">
                            ${members.map(m => `<option value="${m.id}">${m.first_name} ${m.last_name || ''}</option>`).join('')}
                        </select>
                    </div>
                    <div class="risk-sel-group" style="flex: 1; min-width: 80px;">
                        <label style="font-size: 11px;">Вероятность</label>
                        <select class="select risk-prob" style="width: 100%; min-width: 0; padding: 4px; font-size: 12px;">
                            <option value="1">1</option>
                            <option value="2">2</option>
                            <option value="3">3</option>
                            <option value="4">4</option>
                        </select>
                    </div>
                    <div class="risk-sel-group" style="flex: 1; min-width: 80px;">
                        <label style="font-size: 11px;">Влияние</label>
                        <select class="select risk-imp" style="width: 100%; min-width: 0; padding: 4px; font-size: 12px;">
                            <option value="1">1</option>
                            <option value="2">2</option>
                            <option value="3">3</option>
                            <option value="4">4</option>
                        </select>
                    </div>
                    <button class="btn btn-primary btn-vote-risk-admin" data-is-story="${isStory}" style="align-self: flex-end; padding: 4px 8px; font-size: 11px;">Оценить за участника</button>
                </div>
            `;
        }

        const editRiskBtnHtml = isAdmin
            ? `<button class="btn-edit-risk" data-risk-id="${risk.id}" title="Редактировать риск" style="background: none; border: none; cursor: pointer; color: var(--text-muted); font-size: 13px; padding: 0 4px;">✏️</button>`
            : '';

        return `
            <div class="risk-vote-item" data-risk-id="${risk.id}" style="margin-bottom: 12px; padding-bottom: 12px; border-bottom: 1px solid var(--color-border);">
                <div style="display: flex; justify-content: space-between; align-items: center;">
                    <div style="display: flex; align-items: center; gap: 4px;">
                        <div class="risk-vote-desc" style="font-weight:600; font-size: 13px;">${risk.description}</div>
                        ${editRiskBtnHtml}
                    </div>
                    <div style="font-size: 12px;">${scoreDisplay}</div>
                </div>
                <div style="margin-top: 6px;">
                    <span style="font-size: 11px; color: var(--text-muted);">Оценки команды:</span>
                    ${riskScoresHtml}
                </div>
                ${userRiskSelectorsHtml}
                ${adminRiskSelectorsHtml}
            </div>
        `;
    }).join('');
}

function renderStoryDetailsHtml(story, scoresData, roleScores, risks) {
    const userProfile = state.get('userProfile');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');

    let adminPanelHtml = '';
    if (isAdmin && (story.status === 'SCORING' || story.status === 'SCORED')) {
        adminPanelHtml = `
        <div style="margin-top: 16px; border-top: 1px solid var(--color-border); padding-top: 16px;">
            <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">Оценки участников (Админ)</h4>
            <table class="admin-scores-table" style="font-size: 12px;">
                <thead>
                    <tr>
                        <th>Участник</th>
                        <th>Роль</th>
                        <th>Оценка</th>
                        <th>Действие</th>
                    </tr>
                </thead>
                <tbody>
                    ${renderAdminScoresTableRows(story, scoresData)}
                </tbody>
            </table>
        </div>
        `;
    }

    const finalScoreText = story.final_score !== null && story.final_score !== undefined
        ? `<div class="badge" style="background: rgba(16, 185, 129, 0.15); color: var(--color-role-be); font-size: 12px; padding: 4px 10px;">Финальная оценка: ${story.final_score} чд</div>`
        : `<div class="badge" style="background: var(--bg-tertiary); font-size: 12px; padding: 4px 10px;">Статус: ${story.status === 'NEW' ? 'Новый' : 'Оценка'}</div>`;

    // Прямой override итоговой оценки доступен админу только после завершения скоринга (SCORED)
    const overrideFinalScoreHtml = isAdmin && story.status === 'SCORED' ? `
        <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 2px;">
            <div style="display: flex; align-items: center; gap: 6px;">
                <input type="number" id="input-override-final-score" class="input" min="0" step="1" value="${story.final_score}" style="width: 80px; padding: 4px 6px; font-size: 12px;">
                <button id="btn-recalc-final-score" class="btn btn-secondary" title="Подставить в поле значение, рассчитанное по формуле (без сохранения)" style="padding: 4px 8px; font-size: 11px;">Пересчитать по формуле</button>
                <button id="btn-override-final-score" class="btn btn-primary" style="padding: 4px 8px; font-size: 11px;">Переопределить</button>
            </div>
            <span style="font-size: 11px; color: var(--text-muted);">Переопределяет расчёт по формуле</span>
        </div>
    ` : '';

    return `
        <div style="border-bottom: 1px solid var(--color-border); padding-bottom: 10px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-start; gap: 10px;">
            <div style="min-width: 0;">
                <div style="display: flex; align-items: center; gap: 8px;">
                    <h3 style="margin: 0; font-size: 15px; font-weight: 700; color: var(--color-text);">${story.number}: ${story.name}</h3>
                    ${isAdmin ? `<button id="btn-edit-story" class="btn btn-secondary btn-sm" data-story-id="${story.id}" title="Редактировать историю" style="padding: 2px 6px; font-size: 11px;">✏️ Редактировать</button>` : ''}
                </div>
                <div style="font-size: 12px; color: var(--text-muted); margin-top: 4px; overflow-wrap: break-word;">${story.description || 'Нет описания.'}</div>
            </div>
            <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 8px;">
                ${finalScoreText}
                ${overrideFinalScoreHtml}
            </div>
        </div>

        <div style="display: grid; grid-template-columns: 1fr; gap: 16px;">
            <div>
                <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">Оценить Сторю</h4>
                ${story.status === 'NEW' ? '<div style="color: var(--text-muted); text-align: center; padding: 15px 0; font-size: 13px;">Оценка еще не запущена.</div>' : ''}
                ${story.status === 'SCORED' ? '<div style="color: var(--color-role-be); text-align: center; padding: 15px 0; font-weight: 600; font-size: 13px;">Оценка завершена!</div>' : ''}
                
                ${story.status === 'SCORING' ? `
                    <div class="form-group" style="margin-bottom: 8px;">
                        <label for="vote-role-select-story" style="font-size: 12px;">Ваша роль</label>
                        <select id="vote-role-select-story" class="select" style="width: 100%; padding: 6px 10px; font-size: 13px;">
                            ${rolesList.map(r => `<option value="${r.id}">${r.name}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group" style="display: flex; align-items: center;">
                        <input type="number" id="input-story-score" class="input" min="0" max="500" placeholder="0-500 чд" style="width: 100px; margin-right: 10px; padding: 6px 10px; font-size: 13px;">
                        <button id="btn-submit-story-vote" class="btn btn-primary" style="padding: 6px 12px; font-size: 13px;">Сохранить</button>
                    </div>
                    ${renderProgressHtml(scoresData)}
                ` : ''}
            </div>

            <div style="margin-top: 8px;">
                <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">Оценки по ролям</h4>
                <table class="scores-table" id="role-scores-table" style="font-size: 13px;">
                    ${renderRoleScoresTableHtml(roleScores, story, isAdmin, scoresData)}
                </table>
            </div>
        </div>

        ${adminPanelHtml}

        <div style="margin-top: 16px; border-top: 1px solid var(--color-border); padding-top: 16px;">
            <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">Оценка рисков истории (стори)</h4>
            <div id="scoring-risks-container-story">
                ${renderRisksHtml(story, scoresData, risks)}
            </div>
        </div>
    `;
}

function bindEvents(isAdmin, isLeaderOrAdmin) {
    const container = document.getElementById('scoring-details');
    if (!container) return;

    // 1. Epic start scoring button
    if (selectedEpic && selectedEpic.status === 'NEW' && isLeaderOrAdmin) {
        document.getElementById('btn-start-scoring')?.addEventListener('click', () => startEpicScoring(selectedEpic.id));
    }

    // 2. Select story inside list
    container.querySelectorAll('.story-item').forEach(el => {
        el.addEventListener('click', (e) => {
            if (e.target.closest('.btn-delete-story')) return;
            const storyId = el.dataset.storyId;
            const story = currentStories.find(s => s.id === storyId);
            if (story) {
                selectedStory = story;
                adminStoryVotes = {};
                loadEpicData();
            }
        });
    });

    // 3. Admin: Create new story
    if (isAdmin && selectedEpic && selectedEpic.status === 'NEW') {
        const btnCreateStory = document.getElementById('btn-create-story');
        btnCreateStory?.addEventListener('click', async () => {
            const nameInput = document.getElementById('new-story-name');
            const descInput = document.getElementById('new-story-desc');
            if (!nameInput || !descInput) return;

            const name = nameInput.value.trim();
            const description = descInput.value.trim();

            if (!name) {
                showToast('Пожалуйста, введите название истории', 'error');
                return;
            }

            // Блокируем кнопку сразу, чтобы повторный/двойной клик не запустил второй запрос
            btnCreateStory.disabled = true;
            try {
                await apiPost(`/epics/${selectedEpic.id}/stories`, {
                    name,
                    description
                });
                showToast('История успешно добавлена!', 'success');
                await loadEpicData();
            } catch (err) {
                showToast('Не удалось создать историю: ' + err.message, 'error');
            } finally {
                btnCreateStory.disabled = false;
            }
        });
    }

    // 4. Admin: Delete story
    if (isAdmin) {
        container.querySelectorAll('.btn-delete-story').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                e.stopPropagation();
                const storyId = btn.dataset.storyId;
                if (!confirm('Вы уверены, что хотите удалить эту историю?')) return;

                try {
                    await apiDelete(`/stories/${storyId}`);
                    showToast('История успешно удалена!', 'success');
                    if (selectedStory && selectedStory.id === storyId) {
                        selectedStory = null;
                    }
                    await loadEpicData();
                } catch (err) {
                    showToast('Не удалось удалить историю: ' + err.message, 'error');
                }
            });
        });
    }

    // 5. Submit epic vote (when epic has no stories)
    if (selectedEpic && !selectedStory && selectedEpic.status === 'SCORING') {
        const btnSubmitVote = document.getElementById('btn-submit-vote');
        btnSubmitVote?.addEventListener('click', async () => {
            const input = document.getElementById('input-epic-score');
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите оценку', 'error');
                return;
            }
            const score = parseInt(valStr, 10);
            if (isNaN(score) || score < 0 || score > 500) {
                showToast('Оценка должна быть числом от 0 до 500', 'error');
                return;
            }

            try {
                await apiPost('/scores/epic', {
                    epic_id: selectedEpic.id,
                    score: score
                });
                showToast('Ваша оценка принята!', 'success');
                await loadEpicData();
            } catch (err) {
                showErrorModal(err.message);
            }
        });
    }

    // 6. Submit story vote
    if (selectedStory && selectedStory.status === 'SCORING') {
        const btnSubmitStoryVote = document.getElementById('btn-submit-story-vote');
        btnSubmitStoryVote?.addEventListener('click', async () => {
            const input = document.getElementById('input-story-score');
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите оценку', 'error');
                return;
            }
            const score = parseInt(valStr, 10);
            if (isNaN(score) || score < 0 || score > 500) {
                showToast('Оценка должна быть числом от 0 до 500', 'error');
                return;
            }

            try {
                await apiPost('/scores/epic', {
                    epic_id: selectedStory.id,
                    score: score
                });
                showToast('Ваша оценка истории принята!', 'success');
                await loadEpicData();
            } catch (err) {
                showErrorModal(err.message);
            }
        });
    }

    // 7. Normal User: Vote on Risk
    container.querySelectorAll('.btn-vote-risk').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            const item = e.target.closest('.risk-vote-item');
            const riskId = item.dataset.riskId;
            const probability = parseInt(item.querySelector('.risk-prob').value, 10);
            const impact = parseInt(item.querySelector('.risk-imp').value, 10);

            try {
                await apiPost('/scores/risk', {
                    risk_id: riskId,
                    probability,
                    impact
                });
                showToast('Оценка риска принята!', 'success');
                await loadEpicData();
            } catch (err) {
                showErrorModal(err.message);
            }
        });
    });

    // 8. Admin: Override User Risk Vote
    container.querySelectorAll('.btn-vote-risk-admin').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            const item = e.target.closest('.risk-vote-item');
            const riskId = item.dataset.riskId;
            const userId = item.querySelector('.risk-admin-user').value;
            const probability = parseInt(item.querySelector('.risk-prob').value, 10);
            const impact = parseInt(item.querySelector('.risk-imp').value, 10);

            try {
                await apiPost('/admin/scores/risk', {
                    risk_id: riskId,
                    user_id: userId,
                    probability,
                    impact
                });
                showToast('Оценка риска участника успешно сохранена!', 'success');
                await refreshAfterAdminEdit();
            } catch (err) {
                showToast('Не удалось оценить риск: ' + err.message, 'error');
            }
        });
    });

    // 9. Admin: Edit/Cancel/Save Vote overrides
    container.querySelectorAll('.btn-edit-admin-vote').forEach(btn => {
        btn.addEventListener('click', () => {
            const userId = btn.dataset.userId;
            const isStory = btn.dataset.isStory === 'true';
            const activeVotesObj = isStory ? adminStoryVotes : adminEpicVotes;
            const scoresData = isStory ? selectedStoryScores : selectedEpicScores;
            const userScore = scoresData?.scores?.find(s => s.user_id === userId);
            
            activeVotesObj[userId] = userScore ? userScore.score : null;
            renderDetails();
        });
    });

    container.querySelectorAll('.btn-cancel-admin-vote').forEach(btn => {
        btn.addEventListener('click', () => {
            const userId = btn.dataset.userId;
            const isStory = btn.dataset.isStory === 'true';
            const activeVotesObj = isStory ? adminStoryVotes : adminEpicVotes;
            
            delete activeVotesObj[userId];
            renderDetails();
        });
    });

    container.querySelectorAll('.btn-save-admin-vote').forEach(btn => {
        btn.addEventListener('click', async () => {
            const userId = btn.dataset.userId;
            const isStory = btn.dataset.isStory === 'true';
            const activeVotesObj = isStory ? adminStoryVotes : adminEpicVotes;
            
            const row = btn.closest('tr');
            const input = row.querySelector(`.admin-score-input[data-user-id="${userId}"]`);
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите оценку', 'error');
                return;
            }
            const score = parseInt(valStr, 10);
            if (isNaN(score) || score < 0 || score > 500) {
                showToast('Оценка должна быть числом от 0 до 500', 'error');
                return;
            }

            try {
                const targetId = isStory ? selectedStory.id : selectedEpic.id;
                await apiPost('/admin/scores/epic', {
                    epic_id: targetId,
                    user_id: userId,
                    score: score
                });
                showToast('Оценка успешно проставлена!', 'success');
                delete activeVotesObj[userId];
                await refreshAfterAdminEdit();
            } catch (err) {
                showToast('Не удалось проставить оценку: ' + err.message, 'error');
            }
        });
    });
    // 9b. Admin: Override role score directly in the role scores table row
    bindRoleScoreOverrideEvents(container);

    // 9c. Admin: Mass expert vote for a whole role while scoring is still active (SCORING)
    bindExpertRoleScoreEvents(container);

    // 10. Admin: Override story final_score directly (only when scoring already finished)
    if (isAdmin && selectedStory && selectedStory.status === 'SCORED') {
        // 10a. Recalculate final score preview by formula: fills the input, does NOT save it —
        // сохранение по-прежнему требует явного нажатия «Переопределить».
        const btnRecalcFinalScore = document.getElementById('btn-recalc-final-score');
        btnRecalcFinalScore?.addEventListener('click', async () => {
            try {
                const preview = await apiGet(`/admin/scores/${selectedStory.id}/recalc-preview`);
                const input = document.getElementById('input-override-final-score');
                if (input && preview && preview.final_score !== undefined) {
                    input.value = preview.final_score;
                }
            } catch (err) {
                showErrorModal(err.message);
            }
        });

        const btnOverrideFinalScore = document.getElementById('btn-override-final-score');
        btnOverrideFinalScore?.addEventListener('click', async () => {
            const input = document.getElementById('input-override-final-score');
            if (!input) return;
            const valStr = input.value.trim();
            if (valStr === '') {
                showToast('Пожалуйста, введите итоговую оценку', 'error');
                return;
            }
            const finalScore = Number(valStr);
            if (isNaN(finalScore) || finalScore < 0) {
                showToast('Итоговая оценка должна быть числом не меньше 0', 'error');
                return;
            }

            try {
                await apiPost('/admin/scores/final', {
                    epic_id: selectedStory.id,
                    final_score: finalScore
                });
                showToast('Итоговая оценка обновлена!', 'success');
                await refreshAfterAdminEdit();
            } catch (err) {
                showErrorModal(err.message);
            }
        });
    }

    // 5. Admin: Edit Epic & Story
    if (isAdmin) {
        const btnEditEpic = document.getElementById('btn-edit-epic');
        if (btnEditEpic) {
            btnEditEpic.onclick = () => {
                if (selectedEpic) openEditEpicModal(selectedEpic);
            };
        }
        // Кнопка рендерится только для superadmin, но проверка isAdmin выше её тоже покрывает
        // (superadmin проходит isAdmin), явная проверка существования узла — на случай admin без кнопки
        const btnDeleteEpic = document.getElementById('btn-delete-epic');
        if (btnDeleteEpic) {
            btnDeleteEpic.onclick = () => {
                if (selectedEpic) openDeleteEpicModal(selectedEpic);
            };
        }
        const btnNotifyEpic = document.getElementById('btn-notify-epic');
        if (btnNotifyEpic) {
            btnNotifyEpic.onclick = () => {
                if (selectedEpic) notifyEpicReminders(selectedEpic.id);
            };
        }
        const btnEditStory = document.getElementById('btn-edit-story');
        if (btnEditStory) {
            btnEditStory.onclick = () => {
                if (selectedStory) openEditStoryModal(selectedStory);
            };
        }

        container.querySelectorAll('.btn-edit-risk').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const riskId = btn.dataset.riskId;
                const risk = selectedStoryRisks.find(r => r.id === riskId);
                if (risk) openEditRiskModal(risk);
            });
        });
    }
}

async function startEpicScoring(epicId) {
    if (currentStories.length === 0) {
        alert('Невозможно запустить оценку: у эпика нет ни одной истории (стори).');
        return;
    }

    try {
        await apiPost('/epics/start', { epic_id: epicId });
        showToast('Процесс оценки успешно запущен!', 'success');
        
        const teamId = state.get('selectedTeamId');
        await loadScoringEpics(teamId);
        
        if (selectedEpic && selectedEpic.id === epicId) {
            selectedEpic.status = 'SCORING';
            await loadEpicData();
        }
    } catch (err) {
        showToast('Не удалось запустить оценку: ' + err.message, 'error');
    }
}

// Отправка напоминаний непроголосовавшим участникам команды эпика (доступно только admin/superadmin,
// только для эпика в статусе SCORING) через POST /epics/notify
async function notifyEpicReminders(epicId) {
    try {
        const result = await apiPost('/epics/notify', { epic_id: epicId });
        const sentCount = result.sent_count || 0;
        const failedIds = result.failed_telegram_ids || [];

        let message = `Напоминания отправлены: ${sentCount}`;
        // Тип тоста ограничен существующей CSS-палитрой (success/error/info),
        // поэтому при наличии неудачных отправок используем 'error', даже если часть напоминаний ушла успешно
        let type = 'success';
        if (failedIds.length > 0) {
            message += `. Не удалось отправить: ${failedIds.join(', ')}`;
            type = 'error';
        }
        showToast(message, type);
    } catch (err) {
        showToast('Не удалось отправить напоминания: ' + err.message, 'error');
    }
}

async function openEditEpicModal(epic) {
    let modal = document.getElementById('modal-edit-epic');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-edit-epic';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    const isNew = epic.status === 'NEW';
    const teams = state.get('teams') || [];
    const roles = rolesList || [];

    let teamsList = teams;
    if (!teamsList.length) {
        try {
            const data = await apiGet('/teams');
            teamsList = data.teams || [];
        } catch (e) {}
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content">
            <div class="modal-header">
                <h2>Редактирование Эпика</h2>
                <button class="btn-icon btn-close-modal">✕</button>
            </div>
            <form id="form-edit-epic">
                <div class="modal-body">
                    <div class="form-group">
                        <label>Номер эпика</label>
                        <input type="text" id="edit-epic-number" class="input" value="${epic.number || ''}" required>
                    </div>
                    <div class="form-group">
                        <label>Название эпика</label>
                        <input type="text" id="edit-epic-name" class="input" value="${epic.name || ''}" required>
                    </div>
                    <div class="form-group">
                        <label>Описание</label>
                        <textarea id="edit-epic-desc" class="input" style="min-height: 70px;">${epic.description || ''}</textarea>
                    </div>
                    <div class="form-group">
                        <label>Команда ${!isNew ? '<span style="font-size: 11px; color: var(--color-danger);">(Заблокировано: скоринг запущен)</span>' : ''}</label>
                        <select id="edit-epic-team" class="select" ${!isNew ? 'disabled' : ''}>
                            ${teamsList.map(t => `<option value="${t.id}" ${t.id === epic.team_id ? 'selected' : ''}>${t.name}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-grid-3col">
                        <div class="form-group">
                            <label>Год</label>
                            <input type="number" id="edit-epic-year" class="input" value="${epic.year || 2026}" min="2000" max="2100" required>
                        </div>
                        <div class="form-group">
                            <label>Квартал</label>
                            <select id="edit-epic-quarter" class="select">
                                <option value="1" ${epic.quarter === 1 ? 'selected' : ''}>Q1</option>
                                <option value="2" ${epic.quarter === 2 ? 'selected' : ''}>Q2</option>
                                <option value="3" ${epic.quarter === 3 ? 'selected' : ''}>Q3</option>
                                <option value="4" ${epic.quarter === 4 ? 'selected' : ''}>Q4</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label>Тип</label>
                            <select id="edit-epic-type" class="select">
                                <option value="feature" ${epic.type === 'feature' ? 'selected' : ''}>Feature</option>
                                <option value="architecture" ${epic.type === 'architecture' ? 'selected' : ''}>Architecture</option>
                                <option value="techdebt" ${epic.type === 'techdebt' ? 'selected' : ''}>Techdebt</option>
                            </select>
                        </div>
                    </div>
                    <div class="form-group">
                        <label>Роли-оценщики ${!isNew ? '<span style="font-size: 11px; color: var(--color-danger);">(Заблокировано)</span>' : ''}</label>
                        <div class="checkbox-container-list" id="edit-epic-roles-container" style="max-height: 120px; overflow-y: auto;">
                            ${roles.map(r => {
                                const isChecked = (epic.evaluating_role_ids || []).includes(r.id);
                                return `
                                    <div class="checkbox-item">
                                        <input type="checkbox" id="edit-epic-role-${r.id}" value="${r.id}" ${isChecked ? 'checked' : ''} ${!isNew ? 'disabled' : ''}>
                                        <label for="edit-epic-role-${r.id}">${r.name}</label>
                                    </div>
                                `;
                            }).join('')}
                        </div>
                    </div>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                    <button type="submit" class="btn btn-primary">Сохранить</button>
                </div>
            </form>
        </div>
    `;

    modal.classList.remove('hidden');
    modal.style.display = 'flex';

    modal.querySelectorAll('.btn-close-modal, .modal-overlay').forEach(btn => {
        btn.onclick = () => {
            modal.classList.add('hidden');
            modal.style.display = 'none';
        };
    });

    const form = modal.querySelector('#form-edit-epic');
    form.onsubmit = async (e) => {
        e.preventDefault();
        const number = document.getElementById('edit-epic-number').value.trim();
        const name = document.getElementById('edit-epic-name').value.trim();
        const description = document.getElementById('edit-epic-desc').value.trim();
        const teamId = document.getElementById('edit-epic-team').value;
        const year = parseInt(document.getElementById('edit-epic-year').value, 10);
        const quarter = parseInt(document.getElementById('edit-epic-quarter').value, 10);
        const type = document.getElementById('edit-epic-type').value;

        const roleCbs = document.querySelectorAll('#edit-epic-roles-container input[type="checkbox"]:checked');
        const evaluatingRoleIds = Array.from(roleCbs).map(cb => cb.value);

        try {
            const updatedEpic = await apiPut(`/epics/${epic.id}`, {
                number,
                name,
                description,
                team_id: teamId,
                year,
                quarter,
                type,
                evaluating_role_ids: evaluatingRoleIds
            });
            showToast('Эпик успешно обновлен!', 'success');
            modal.classList.add('hidden');
            modal.style.display = 'none';
            selectedEpic = updatedEpic;
            const currentTeamId = state.get('selectedTeamId');
            await loadScoringEpics(currentTeamId);
            await loadEpicData();
        } catch (err) {
            showToast('Ошибка при обновлении эпика: ' + err.message, 'error');
        }
    };
}

async function openEditStoryModal(story) {
    let modal = document.getElementById('modal-edit-story');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-edit-story';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    const isNew = story.status === 'NEW';
    const teamId = (selectedEpic && selectedEpic.team_id) || (story && story.team_id) || state.get('selectedTeamId');
    let epicsList = [];
    if (teamId) {
        try {
            const data = await apiGet(`/epics?team_id=${teamId}&all=true`);
            epicsList = (data.epics || []).filter(e => !e.parent_epic_id);
        } catch (e) {}
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content">
            <div class="modal-header">
                <h2>Редактирование Истории</h2>
                <button class="btn-icon btn-close-modal">✕</button>
            </div>
            <form id="form-edit-story">
                <div class="modal-body">
                    <div class="form-group">
                        <label>Номер истории</label>
                        <input type="text" id="edit-story-number" class="input" value="${story.number || ''}" required>
                    </div>
                    <div class="form-group">
                        <label>Название истории</label>
                        <input type="text" id="edit-story-name" class="input" value="${story.name || ''}" required>
                    </div>
                    <div class="form-group">
                        <label>Описание истории</label>
                        <textarea id="edit-story-desc" class="input" style="min-height: 70px;">${story.description || ''}</textarea>
                    </div>
                    <div class="form-group">
                        <label>Родительский Эпик ${!isNew ? '<span style="font-size: 11px; color: var(--color-danger);">(Заблокировано: скоринг запущен)</span>' : ''}</label>
                        <select id="edit-story-parent-epic" class="select" ${!isNew ? 'disabled' : ''}>
                            ${epicsList.map(e => `<option value="${e.id}" ${story.parent_epic_id === e.id ? 'selected' : ''}>${e.number}: ${e.name}</option>`).join('')}
                        </select>
                    </div>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                    <button type="submit" class="btn btn-primary">Сохранить</button>
                </div>
            </form>
        </div>
    `;

    modal.classList.remove('hidden');
    modal.style.display = 'flex';

    modal.querySelectorAll('.btn-close-modal, .modal-overlay').forEach(btn => {
        btn.onclick = () => {
            modal.classList.add('hidden');
            modal.style.display = 'none';
        };
    });

    const form = modal.querySelector('#form-edit-story');
    form.onsubmit = async (e) => {
        e.preventDefault();
        const number = document.getElementById('edit-story-number').value.trim();
        const name = document.getElementById('edit-story-name').value.trim();
        const description = document.getElementById('edit-story-desc').value.trim();
        const parentEpicId = document.getElementById('edit-story-parent-epic').value;

        try {
            const updatedStory = await apiPut(`/stories/${story.id}`, {
                number,
                name,
                description,
                parent_epic_id: parentEpicId
            });
            showToast('История успешно обновлена!', 'success');
            modal.classList.add('hidden');
            modal.style.display = 'none';
            selectedStory = updatedStory;
            await loadEpicData();
        } catch (err) {
            showToast('Ошибка при обновлении истории: ' + err.message, 'error');
        }
    };
}

function openEditRiskModal(risk) {
    let modal = document.getElementById('modal-edit-risk');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-edit-risk';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content" style="max-width: 500px; width: 90%;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <h3 style="margin: 0; font-size: 16px;">Редактирование риска</h3>
                <button class="btn-close-modal" style="background: none; border: none; font-size: 18px; cursor: pointer; color: var(--text-muted);">&times;</button>
            </div>
            <form id="form-edit-risk">
                <div class="form-group">
                    <label>Описание риска</label>
                    <textarea id="edit-risk-desc" class="input" style="min-height: 70px;" required>${risk.description || ''}</textarea>
                </div>
                <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px;">
                    <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                    <button type="submit" class="btn btn-primary">Сохранить</button>
                </div>
            </form>
        </div>
    `;

    modal.classList.remove('hidden');
    modal.style.display = 'flex';

    modal.querySelectorAll('.btn-close-modal, .modal-overlay').forEach(btn => {
        btn.onclick = () => {
            modal.classList.add('hidden');
            modal.style.display = 'none';
        };
    });

    const form = modal.querySelector('#form-edit-risk');
    form.onsubmit = async (e) => {
        e.preventDefault();
        const description = document.getElementById('edit-risk-desc').value.trim();
        if (!description) {
            showToast('Пожалуйста, введите описание риска', 'error');
            return;
        }

        try {
            await apiPut(`/risks/${risk.id}`, { description });
            showToast('Риск успешно обновлён!', 'success');
            modal.classList.add('hidden');
            modal.style.display = 'none';
            await loadEpicData();
        } catch (err) {
            showToast('Ошибка при обновлении риска: ' + err.message, 'error');
        }
    };
}

// Модальное окно подтверждения удаления эпика (только superadmin, см. openDeleteEpicModal caller).
// Кастомная модалка вместо window.confirm — явное требование дизайна (design.md Decision 3),
// т.к. удаление каскадно и безвозвратно затрагивает истории/риски/оценки.
function openDeleteEpicModal(epic) {
    let modal = document.getElementById('modal-delete-epic');
    if (!modal) {
        modal = document.createElement('div');
        modal.id = 'modal-delete-epic';
        modal.className = 'modal hidden';
        document.body.appendChild(modal);
    }

    modal.innerHTML = `
        <div class="modal-overlay"></div>
        <div class="modal-content">
            <div class="modal-header">
                <h2>Удаление эпика</h2>
                <button class="btn-icon btn-close-modal">✕</button>
            </div>
            <div class="modal-body">
                <p>
                    Вы уверены, что хотите безвозвратно удалить эпик <strong>${epic.number}: ${epic.name}</strong>?
                </p>
                <p style="color: var(--color-danger);">
                    Это действие также удалит все истории, риски и оценки этого эпика. Отменить удаление будет невозможно.
                </p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                <button type="button" id="btn-confirm-delete-epic" class="btn btn-danger">Удалить</button>
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

    const btnConfirm = modal.querySelector('#btn-confirm-delete-epic');
    btnConfirm.onclick = async () => {
        try {
            await apiDelete(`/epics/${epic.id}`);
            showToast('Эпик успешно удалён!', 'success');
            closeModal();
            clearDetails();
            await loadScoringEpics(state.get('selectedTeamId'));
        } catch (err) {
            // Список эпиков и панель деталей намеренно не трогаем — эпик остаётся как есть
            showToast('Не удалось удалить эпик: ' + err.message, 'error');
        }
    };
}


