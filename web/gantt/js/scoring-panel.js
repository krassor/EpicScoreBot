// ── Scoring Panel Module ─────────────────────────────────────────────

import { state } from './state.js';
import { apiGet, apiPost } from './api.js';
import { showToast } from './utils.js';

let selectedEpic = null;
let rolesList = [];
let adminEpicVotes = {}; // Store temporary admin votes before save: { userId: score }

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
    adminEpicVotes = {}; // Clear admin edits cache on epic selection
    const container = document.getElementById('scoring-details');
    if (!container) return;

    container.innerHTML = '<div style="padding: 40px; text-align: center; color: var(--text-muted);"><div class="loader"></div><p style="margin-top:16px;">Загрузка деталей эпика...</p></div>';

    try {
        // Fetch scoring details & risks
        const scoresData = await apiGet(`/epics/${epic.id}/scores`).catch(() => ({ scores: [], expected: 0, received: 0 }));
        const roleScores = await apiGet(`/epics/${epic.id}/role-scores`).catch(() => ([]));
        const risksData = await apiGet(`/epics/${epic.id}/risks`).catch(() => ({ risks: [] }));

        renderEpicDetails(epic, scoresData, roleScores, risksData.risks || []);
    } catch (err) {
        showToast('Не удалось загрузить данные эпика: ' + err.message, 'error');
    }
}

function renderEpicDetails(epic, scoresData, roleScores, risks) {
    const container = document.getElementById('scoring-details');
    if (!container) return;
    scoresData = scoresData || {};
    roleScores = roleScores || [];
    risks = risks || [];

    const userProfile = state.get('userProfile');
    const isLeaderOrAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin' || userProfile.role === 'leader');
    const isAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin');

    let actionButtonHtml = '';
    if (epic.status === 'NEW' && isLeaderOrAdmin) {
        actionButtonHtml = `<button id="btn-start-scoring" class="btn btn-primary">⚡ Запустить скоринг</button>`;
    }

    const finalScoreText = epic.final_score !== null && epic.final_score !== undefined
        ? `<div class="badge" style="background: rgba(16, 185, 129, 0.2); color: var(--color-role-be); font-size: 14px; padding: 6px 14px;">Итоговая оценка: ${epic.final_score} чд</div>`
        : `<div class="badge" style="background: var(--bg-tertiary); font-size: 14px; padding: 6px 14px;">Статус оценки: ${epic.status === 'NEW' ? 'Не начата' : 'В процессе'}</div>`;

    let progressHtml = '';
    if (epic.status === 'SCORING') {
        const received = scoresData.scores_received || scoresData.scores?.length || 0;
        const expected = scoresData.scores_expected || 1;
        const percent = Math.min(100, Math.round((received / expected) * 100));
        progressHtml = `
            <div class="form-group" style="margin-top: 10px;">
                <label>Прогресс оценок: ${received} из ${expected} (${percent}%)</label>
                <div style="background: var(--bg-tertiary); border-radius: 6px; height: 8px; width: 100%; overflow: hidden; margin-top: 4px;">
                    <div style="background: var(--accent-primary); width: ${percent}%; height: 100%; transition: width 0.3s ease;"></div>
                </div>
            </div>
        `;
    }

    container.innerHTML = `
        <div class="scoring-details-header">
            <div class="scoring-epic-title">
                <h2>${epic.number}: ${epic.name}</h2>
                <div class="scoring-epic-desc">${epic.description || 'Нет описания.'}</div>
            </div>
            <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 8px;">
                ${finalScoreText}
                ${actionButtonHtml}
            </div>
        </div>

        <div class="scoring-panel-grid">
            <!-- Left block: Voting and progress -->
            <div class="scoring-section-card">
                <h3>Оценить Эпик</h3>
                ${epic.status === 'NEW' ? '<div style="color: var(--text-muted); text-align: center; padding: 20px 0;">Оценка еще не запущена. Ожидайте запуска лидером команды.</div>' : ''}
                ${epic.status === 'SCORED' ? '<div style="color: var(--color-role-be); text-align: center; padding: 20px 0; font-weight: 600;">Оценка завершена! Все участники проголосовали.</div>' : ''}
                
                ${epic.status === 'SCORING' ? `
                    <div class="form-group">
                        <label for="vote-role-select">Ваша роль в этой оценке</label>
                        <select id="vote-role-select" class="select" style="width: 100%;">
                            ${rolesList.map(r => `<option value="${r.id}">${r.name}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group" style="margin-top: 10px; display: flex; align-items: center;">
                        <input type="number" id="input-epic-score" class="input" min="0" max="500" placeholder="0-500 чд" style="width: 120px; margin-right: 10px;">
                        <button id="btn-submit-vote" class="btn btn-primary">Сохранить оценку</button>
                    </div>
                    ${progressHtml}
                ` : ''}
            </div>

            <!-- Right block: Role averages table -->
            <div class="scoring-section-card">
                <h3>Оценки по ролям</h3>
                <table class="scores-table">
                    <thead>
                        <tr>
                            <th>Роль</th>
                            <th>Оценка (чд)</th>
                        </tr>
                    </thead>
                    <tbody id="role-scores-tbody">
                        ${roleScores.length === 0 ? '<tr><td colspan="2" style="color: var(--text-muted); text-align: center; padding: 20px 0;">Нет оценок</td></tr>' : 
                            roleScores.map(rs => `
                                <tr>
                                    <td><strong>${rs.role_name || rs.role_id}</strong></td>
                                    <td>${rs.weighted_avg || rs.score} чд</td>
                                </tr>
                            `).join('')
                        }
                    </tbody>
                </table>
            </div>
        </div>

        ${isAdmin && epic.status === 'SCORING' ? `
        <!-- Admin Overrides Table -->
        <div class="scoring-section-card" style="margin-top: 12px;">
            <h3>Панель администратора: Оценки участников</h3>
            <table class="admin-scores-table">
                <thead>
                    <tr>
                        <th>ФИО</th>
                        <th>Роль</th>
                        <th>Текущая оценка</th>
                        <th>Управление</th>
                    </tr>
                </thead>
                <tbody>
                    ${(scoresData.members || []).map(m => {
                        const userScore = scoresData.scores?.find(s => s.user_id === m.id);
                        const hasVoted = !!userScore;
                        const isEditing = adminEpicVotes[m.id] !== undefined || !hasVoted;
                        const selectedVal = adminEpicVotes[m.id] !== undefined ? adminEpicVotes[m.id] : (userScore ? userScore.score : null);

                        let voteControlHtml = '';
                        if (isEditing) {
                            voteControlHtml = `
                                <div style="display: flex; align-items: center; justify-content: center;">
                                    <input type="number" class="input admin-score-input" data-user-id="${m.id}" min="0" max="500" placeholder="чд" value="${selectedVal !== null ? selectedVal : ''}" style="width: 80px; text-align: center; margin-right: 8px;">
                                    <button class="btn btn-primary btn-save-admin-epic" data-user-id="${m.id}" style="padding:4px 8px; font-size:12px;">Сохранить</button>
                                </div>
                            `;
                        } else {
                            voteControlHtml = `<span style="font-weight:600; color:var(--color-role-be);">${userScore.score} чд</span>`;
                        }

                        let actionsHtml = '';
                        if (isEditing) {
                            actionsHtml = `
                                <div style="display:flex; gap:6px;">
                                    ${hasVoted ? `<button class="btn btn-secondary btn-cancel-admin-epic" data-user-id="${m.id}" style="padding:4px 8px; font-size:12px;">Отмена</button>` : ''}
                                </div>
                            `;
                        } else {
                            actionsHtml = `<button class="btn btn-secondary btn-edit-admin-epic" data-user-id="${m.id}" style="padding:4px 8px; font-size:12px;">Изменить</button>`;
                        }

                        return `
                            <tr>
                                <td><strong>${m.first_name} ${m.last_name || ''}</strong>${m.telegram_id ? ` <span style="color:var(--text-muted); font-size:11px;">@${m.telegram_id}</span>` : ''}</td>
                                <td><span class="badge" style="background:var(--bg-tertiary); font-size:11px;">${m.role_name || 'Без роли'}</span></td>
                                <td>${voteControlHtml}</td>
                                <td>${actionsHtml}</td>
                            </tr>
                        `;
                    }).join('') || '<tr><td colspan="4" style="color: var(--text-muted); text-align: center; padding: 20px 0;">Нет участников в команде</td></tr>'}
                </tbody>
            </table>
        </div>
        ` : ''}

        <!-- Risks Section -->
        <div class="scoring-section-card" style="margin-top: 12px;">
            <h3>Оценка рисков</h3>
            <div id="scoring-risks-container">
                ${risks.length === 0 ? '<div style="color: var(--text-muted); text-align: center; padding: 20px 0;">Риски не добавлены для этого эпика.</div>' : 
                    risks.map(risk => {
                        const hasScore = risk.weighted_score !== null;
                        const scoreDisplay = hasScore ? `<span class="risk-score-value">Итоговый вес: ${risk.weighted_score}</span>` : '<span style="color: var(--text-muted)">Не оценен</span>';
                        
                        // Render user scores for this risk
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
                            : '<div style="color: var(--text-muted); font-size: 12px; margin-top: 4px; padding-left: 12px;">Оценок участников нет</div>';

                        // Voter selectors for ordinary users
                        const userRiskScore = risk.scores?.find(rs => rs.user_id === userProfile?.id);
                        const hasUserScoredRisk = !!userRiskScore;

                        let userRiskSelectorsHtml = '';
                        if (!isAdmin && epic.status === 'SCORING' && !hasUserScoredRisk) {
                            userRiskSelectorsHtml = `
                                <div class="risk-vote-selectors">
                                    <div class="risk-sel-group">
                                        <label>Вероятность (1-4)</label>
                                        <select class="select risk-prob" style="min-width: 0;">
                                            <option value="1">1 - Низкая</option>
                                            <option value="2">2 - Умеренная</option>
                                            <option value="3">3 - Высокая</option>
                                            <option value="4">4 - Критическая</option>
                                        </select>
                                    </div>
                                    <div class="risk-sel-group">
                                        <label>Влияние (1-4)</label>
                                        <select class="select risk-imp" style="min-width: 0;">
                                            <option value="1">1 - Незначительное</option>
                                            <option value="2">2 - Умеренное</option>
                                            <option value="3">3 - Серьезное</option>
                                            <option value="4">4 - Катастрофическое</option>
                                        </select>
                                    </div>
                                    <button class="btn btn-secondary btn-vote-risk" style="align-self: flex-end;">Оценить риск</button>
                                </div>
                            `;
                        }

                        // Voter selectors for admin overrides
                        let adminRiskSelectorsHtml = '';
                        if (isAdmin && epic.status === 'SCORING') {
                            adminRiskSelectorsHtml = `
                                <div class="risk-vote-selectors admin-risk-override" style="margin-top: 8px; display: flex; gap: 8px; flex-wrap: wrap;">
                                    <div class="risk-sel-group" style="flex: 1.5; min-width: 150px;">
                                        <label>Участник</label>
                                        <select class="select risk-admin-user" style="width: 100%;">
                                            ${(scoresData.members || []).map(m => `<option value="${m.id}">${m.first_name} ${m.last_name || ''} (${m.role_name || '?'})</option>`).join('')}
                                        </select>
                                    </div>
                                    <div class="risk-sel-group" style="flex: 1; min-width: 100px;">
                                        <label>Вероятность (1-4)</label>
                                        <select class="select risk-prob" style="width: 100%;">
                                            <option value="1">1 - Низкая</option>
                                            <option value="2">2 - Умеренная</option>
                                            <option value="3">3 - Высокая</option>
                                            <option value="4">4 - Критическая</option>
                                        </select>
                                    </div>
                                    <div class="risk-sel-group" style="flex: 1; min-width: 100px;">
                                        <label>Влияние (1-4)</label>
                                        <select class="select risk-imp" style="width: 100%;">
                                            <option value="1">1 - Незначительное</option>
                                            <option value="2">2 - Умеренное</option>
                                            <option value="3">3 - Серьезное</option>
                                            <option value="4">4 - Катастрофическое</option>
                                        </select>
                                    </div>
                                    <button class="btn btn-primary btn-vote-risk-admin" style="align-self: flex-end; padding: 6px 12px; font-size:13px;">Оценить за участника</button>
                                </div>
                            `;
                        }

                        return `
                            <div class="risk-vote-item" data-risk-id="${risk.id}" style="margin-bottom: 16px; padding-bottom: 16px; border-bottom: 1px solid var(--color-border);">
                                <div style="display: flex; justify-content: space-between; align-items: center;">
                                    <div class="risk-vote-desc" style="font-weight:600;">${risk.description}</div>
                                    <div>${scoreDisplay}</div>
                                </div>
                                <div style="margin-top: 6px;">
                                    <span style="font-size: 12px; color: var(--text-muted);">Оценки команды:</span>
                                    ${riskScoresHtml}
                                </div>
                                ${userRiskSelectorsHtml}
                                ${adminRiskSelectorsHtml}
                            </div>
                        `;
                    }).join('')
                }
            </div>
        </div>
    `;

    // Bind event listeners
    if (epic.status === 'NEW' && isLeaderOrAdmin) {
        document.getElementById('btn-start-scoring')?.addEventListener('click', () => startEpicScoring(epic.id));
    }

    if (epic.status === 'SCORING') {
        const submitBtn = document.getElementById('btn-submit-vote');

        submitBtn?.addEventListener('click', async () => {
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
                    epic_id: epic.id,
                    score: score
                });
                showToast('Ваша оценка принята!', 'success');
                // Reload details
                selectEpic(epic);
            } catch (err) {
                showToast('Не удалось отправить оценку: ' + err.message, 'error');
            }
        });

        // Risks vote buttons for normal users
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
                    selectEpic(epic);
                } catch (err) {
                    showToast('Не удалось оценить риск: ' + err.message, 'error');
                }
            });
        });
    }

    // Admin listeners
    if (isAdmin) {
        // Edit button
        container.querySelectorAll('.btn-edit-admin-epic').forEach(btn => {
            btn.addEventListener('click', () => {
                const userId = btn.dataset.userId;
                const userScore = scoresData.scores?.find(s => s.user_id === userId);
                adminEpicVotes[userId] = userScore ? userScore.score : null;
                renderEpicDetails(epic, scoresData, roleScores, risks);
            });
        });

        // Cancel button
        container.querySelectorAll('.btn-cancel-admin-epic').forEach(btn => {
            btn.addEventListener('click', () => {
                const userId = btn.dataset.userId;
                delete adminEpicVotes[userId];
                renderEpicDetails(epic, scoresData, roleScores, risks);
            });
        });

        // Save override button
        container.querySelectorAll('.btn-save-admin-epic').forEach(btn => {
            btn.addEventListener('click', async () => {
                const userId = btn.dataset.userId;
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
                    await apiPost('/admin/scores/epic', {
                        epic_id: epic.id,
                        user_id: userId,
                        score: score
                    });
                    showToast('Оценка успешно проставлена!', 'success');
                    delete adminEpicVotes[userId];
                    selectEpic(epic);
                } catch (err) {
                    showToast('Не удалось проставить оценку: ' + err.message, 'error');
                }
            });
        });

        // Admin risk override button
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
                    selectEpic(epic);
                } catch (err) {
                    showToast('Не удалось оценить риск: ' + err.message, 'error');
                }
            });
        });
    }
}

async function startEpicScoring(epicId) {
    try {
        await apiPost('/epics/start', { epic_id: epicId });
        showToast('Процесс оценки успешно запущен!', 'success');
        
        // Reload epics list and current epic details
        const teamId = state.get('selectedTeamId');
        await loadScoringEpics(teamId);
        
        if (selectedEpic && selectedEpic.id === epicId) {
            selectedEpic.status = 'SCORING';
            selectEpic(selectedEpic);
        }
    } catch (err) {
        showToast('Не удалось запустить оценку: ' + err.message, 'error');
    }
}
