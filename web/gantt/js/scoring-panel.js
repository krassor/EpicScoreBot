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


export function initScoringPanel() {
    // Inject custom styles for admin controls
    injectStyles();

    // Listen for team changes to reload epics
    state.subscribe('selectedTeamId', (teamId) => {
        if (teamId) {
            loadScoringEpics(teamId);
        } else {
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
        renderEpicsList(epics);
    } catch (err) {
        showToast('Не удалось загрузить список эпиков: ' + err.message, 'error');
    }
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
        currentStories = await apiGet(`/epics/${selectedEpic.id}/stories`).catch(() => ([]));
        
        if (currentStories.length > 0) {
            // If there are stories and no story is selected, select the first one
            if (!selectedStory || !currentStories.some(s => s.id === selectedStory.id)) {
                selectedStory = currentStories[0];
            }
            
            // Fetch selected story details
            selectedStoryScores = await apiGet(`/epics/${selectedStory.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }));
            selectedStoryRoleScores = await apiGet(`/epics/${selectedStory.id}/role-scores`).catch(() => ([]));
            const risksData = await apiGet(`/epics/${selectedStory.id}/risks`).catch(() => ({ risks: [] }));
            selectedStoryRisks = risksData.risks || [];
        } else {
            selectedStory = null;
            // Fetch epic details (fallback when no stories exist)
            selectedEpicScores = await apiGet(`/epics/${selectedEpic.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }));
            selectedEpicRoleScores = await apiGet(`/epics/${selectedEpic.id}/role-scores`).catch(() => ([]));
            const risksData = await apiGet(`/epics/${selectedEpic.id}/risks`).catch(() => ({ risks: [] }));
            selectedEpicRisks = risksData.risks || [];
        }
        
        renderDetails();
    } catch (err) {
        showToast('Не удалось загрузить данные: ' + err.message, 'error');
    }
}

function renderDetails() {
    const container = document.getElementById('scoring-details');
    if (!container || !selectedEpic) return;

    const userProfile = state.get('userProfile');
    const isLeaderOrAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin' || userProfile.role === 'leader');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');

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
                ${selectedStory ? renderStoryDetailsHtml(selectedStory, selectedStoryScores, selectedStoryRoleScores, selectedStoryRisks) : `
                    <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; min-height: 200px; color: var(--text-muted); text-align: center;">
                        <span>👈 Выберите историю (сторю) слева для голосования и оценки рисков</span>
                    </div>
                `}
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

function renderRoleScoresTableRows(roleScores) {
    if (!roleScores || roleScores.length === 0) {
        return '<tr><td colspan="2" style="color: var(--text-muted); text-align: center; padding: 12px 0;">Нет оценок</td></tr>';
    }
    return roleScores.map(rs => `
        <tr>
            <td><strong>${rs.role_name || rs.role_id}</strong></td>
            <td>${rs.weighted_avg !== undefined ? rs.weighted_avg : rs.score} чд</td>
        </tr>
    `).join('');
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

        return `
            <tr>
                <td><strong>${m.first_name} ${m.last_name || ''}</strong>${m.telegram_id ? ` <span style="color:var(--text-muted); font-size:10px;">@${m.telegram_id}</span>` : ''}</td>
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
        if (isAdmin && epicOrStory.status === 'SCORING') {
            const members = scoresData?.members || [];
            adminRiskSelectorsHtml = `
                <div class="risk-vote-selectors admin-risk-override" style="margin-top: 8px; display: flex; gap: 8px; flex-wrap: wrap;">
                    <div class="risk-sel-group" style="flex: 1.5; min-width: 120px;">
                        <label style="font-size: 11px;">Участник</label>
                        <select class="select risk-admin-user" style="width: 100%; padding: 4px; font-size: 12px;">
                            ${members.map(m => `<option value="${m.id}">${m.first_name} ${m.last_name || ''}</option>`).join('')}
                        </select>
                    </div>
                    <div class="risk-sel-group" style="flex: 1; min-width: 80px;">
                        <label style="font-size: 11px;">Вероятность</label>
                        <select class="select risk-prob" style="width: 100%; padding: 4px; font-size: 12px;">
                            <option value="1">1</option>
                            <option value="2">2</option>
                            <option value="3">3</option>
                            <option value="4">4</option>
                        </select>
                    </div>
                    <div class="risk-sel-group" style="flex: 1; min-width: 80px;">
                        <label style="font-size: 11px;">Влияние</label>
                        <select class="select risk-imp" style="width: 100%; padding: 4px; font-size: 12px;">
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
    if (isAdmin && story.status === 'SCORING') {
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

    return `
        <div style="border-bottom: 1px solid var(--color-border); padding-bottom: 10px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-start; gap: 10px;">
            <div style="min-width: 0;">
                <div style="display: flex; align-items: center; gap: 8px;">
                    <h3 style="margin: 0; font-size: 15px; font-weight: 700; color: var(--color-text);">${story.number}: ${story.name}</h3>
                    ${isAdmin ? `<button id="btn-edit-story" class="btn btn-secondary btn-sm" data-story-id="${story.id}" title="Редактировать историю" style="padding: 2px 6px; font-size: 11px;">✏️ Редактировать</button>` : ''}
                </div>
                <div style="font-size: 12px; color: var(--text-muted); margin-top: 4px; overflow-wrap: break-word;">${story.description || 'Нет описания.'}</div>
            </div>
            <div>
                ${finalScoreText}
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
                <table class="scores-table" style="font-size: 13px;">
                    <thead>
                        <tr>
                            <th>Роль</th>
                            <th>Оценка (чд)</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${renderRoleScoresTableRows(roleScores)}
                    </tbody>
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

            try {
                await apiPost(`/epics/${selectedEpic.id}/stories`, {
                    name,
                    description
                });
                showToast('История успешно добавлена!', 'success');
                await loadEpicData();
            } catch (err) {
                showToast('Не удалось создать историю: ' + err.message, 'error');
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
                await loadEpicData();
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
                await loadEpicData();
            } catch (err) {
                showToast('Не удалось проставить оценку: ' + err.message, 'error');
            }
        });
    });
    // 5. Admin: Edit Epic & Story
    if (isAdmin) {
        const btnEditEpic = document.getElementById('btn-edit-epic');
        if (btnEditEpic) {
            btnEditEpic.onclick = () => {
                if (selectedEpic) openEditEpicModal(selectedEpic);
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
        <div class="modal-content" style="max-width: 550px; width: 90%;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <h3 style="margin: 0; font-size: 16px;">Редактирование Эпика</h3>
                <button class="btn-close-modal" style="background: none; border: none; font-size: 18px; cursor: pointer; color: var(--text-muted);">&times;</button>
            </div>
            <form id="form-edit-epic">
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
                <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 10px;">
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
                                <label class="checkbox-label" style="font-size: 12px;">
                                    <input type="checkbox" value="${r.id}" ${isChecked ? 'checked' : ''} ${!isNew ? 'disabled' : ''}> ${r.name}
                                </label>
                            `;
                        }).join('')}
                    </div>
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
        <div class="modal-content" style="max-width: 500px; width: 90%;">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <h3 style="margin: 0; font-size: 16px;">Редактирование Истории</h3>
                <button class="btn-close-modal" style="background: none; border: none; font-size: 18px; cursor: pointer; color: var(--text-muted);">&times;</button>
            </div>
            <form id="form-edit-story">
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


