// ── Scoring Panel Module ─────────────────────────────────────────────

import { state } from './state.js';
import { apiGet, apiPost } from './api.js';
import { showToast } from './utils.js';

let selectedEpic = null;
let rolesList = [];

export function initScoringPanel() {
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

    const userProfile = state.get('userProfile');
    const isLeaderOrAdmin = userProfile && (userProfile.role === 'admin' || userProfile.role === 'superadmin' || userProfile.role === 'leader');

    let actionButtonHtml = '';
    if (epic.status === 'NEW' && isLeaderOrAdmin) {
        actionButtonHtml = `<button id="btn-start-scoring" class="btn btn-primary">⚡ Запустить скоринг</button>`;
    }

    const finalScoreText = epic.final_score !== null && epic.final_score !== undefined
        ? `<div class="badge" style="background: rgba(16, 185, 129, 0.2); color: var(--color-role-be); font-size: 14px; padding: 6px 14px;">Итоговая оценка: ${epic.final_score} SP</div>`
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
                    <div class="form-group" style="margin-top: 10px;">
                        <label>Выберите оценку (Fibonacci SP)</label>
                        <div class="vote-options-grid">
                            ${[1, 2, 3, 5, 8, 13, 21, 34, 55, 89].map(num => `
                                <button class="btn-vote" data-value="${num}">${num}</button>
                            `).join('')}
                        </div>
                    </div>
                    <button id="btn-submit-vote" class="btn btn-primary" style="width: 100%; margin-top: 10px;" disabled>Отправить оценку</button>
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
                            <th>Оценка (SP)</th>
                        </tr>
                    </thead>
                    <tbody id="role-scores-tbody">
                        ${roleScores.length === 0 ? '<tr><td colspan="2" style="color: var(--text-muted); text-align: center; padding: 20px 0;">Нет оценок</td></tr>' : 
                            roleScores.map(rs => `
                                <tr>
                                    <td><strong>${rs.role_name || rs.role_id}</strong></td>
                                    <td>${rs.weighted_avg || rs.score} SP</td>
                                </tr>
                            `).join('')
                        }
                    </tbody>
                </table>
            </div>
        </div>

        <!-- Risks Section -->
        <div class="scoring-section-card" style="margin-top: 12px;">
            <h3>Оценка рисков</h3>
            <div id="scoring-risks-container">
                ${risks.length === 0 ? '<div style="color: var(--text-muted); text-align: center; padding: 20px 0;">Риски не добавлены для этого эпика.</div>' : 
                    risks.map(risk => {
                        const hasScore = risk.weighted_score !== null;
                        const scoreDisplay = hasScore ? `<span class="risk-score-value">Итоговый вес: ${risk.weighted_score}</span>` : '<span style="color: var(--text-muted)">Не оценен</span>';
                        
                        return `
                            <div class="risk-vote-item" data-risk-id="${risk.id}">
                                <div style="display: flex; justify-content: space-between; align-items: center;">
                                    <div class="risk-vote-desc">${risk.description}</div>
                                    <div>${scoreDisplay}</div>
                                </div>
                                ${epic.status === 'SCORING' && !hasScore ? `
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
                                ` : ''}
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
        let selectedVoteVal = null;
        const voteBtns = container.querySelectorAll('.btn-vote');
        const submitBtn = document.getElementById('btn-submit-vote');

        voteBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                voteBtns.forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                selectedVoteVal = parseInt(btn.dataset.value, 10);
                if (submitBtn) submitBtn.disabled = false;
            });
        });

        submitBtn?.addEventListener('click', async () => {
            try {
                await apiPost('/scores/epic', {
                    epic_id: epic.id,
                    score: selectedVoteVal
                });
                showToast('Ваша оценка принята!', 'success');
                // Reload details
                selectEpic(epic);
            } catch (err) {
                showToast('Не удалось отправить оценку: ' + err.message, 'error');
            }
        });

        // Risks vote buttons
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
