// ── Admin Panel Module ────────────────────────────────────────────────

import { state } from './state.js';
import { apiPost, apiGet } from './api.js';
import { showToast } from './utils.js';

export function initAdminPanel() {
    // Subscribe to teams to populate select inputs
    state.subscribe('teams', (teams) => {
        populateTeamSelects(teams);
    });

    setupFormListeners();
}

function populateTeamSelects(teams) {
    const selects = [
        document.getElementById('epic-team-select'),
        document.getElementById('risk-team-select'),
        document.getElementById('import-team-select')
    ];

    selects.forEach(select => {
        if (!select) return;
        
        // Save current value
        const val = select.value;
        
        select.innerHTML = '<option value="">Выберите команду...</option>';
        teams.forEach(team => {
            const opt = document.createElement('option');
            opt.value = team.id;
            opt.textContent = team.name;
            select.appendChild(opt);
        });
        
        // Restore value if still valid
        if (teams.some(t => t.id === val)) {
            select.value = val;
        }
    });
}

function setupFormListeners() {
    // Create Team Form
    const teamForm = document.getElementById('form-create-team');
    teamForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const name = document.getElementById('team-name').value.trim();
        const description = document.getElementById('team-desc').value.trim();

        try {
            const newTeam = await apiPost('/teams', { name, description });
            showToast(`Команда "${newTeam.name || name}" успешно создана!`, 'success');
            teamForm.reset();
            
            // Reload all teams in the application
            reloadTeams();
        } catch (err) {
            showToast('Не удалось создать команду: ' + err.message, 'error');
        }
    });

    // Create Epic Form
    const epicForm = document.getElementById('form-create-epic');
    epicForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const teamId = document.getElementById('epic-team-select').value;
        const number = document.getElementById('epic-number').value.trim();
        const name = document.getElementById('epic-name').value.trim();
        const description = document.getElementById('epic-desc').value.trim();

        try {
            await apiPost('/epics', { team_id: teamId, number, name, description });
            showToast(`Эпик "${number}: ${name}" успешно создан!`, 'success');
            epicForm.reset();
            
            // Reload epics if this was the active team
            if (teamId === state.get('selectedTeamId')) {
                reloadEpics(teamId);
            }
        } catch (err) {
            showToast('Не удалось создать эпик: ' + err.message, 'error');
        }
    });

    // Create Risk Form - Dynamic Epics dropdown based on selected team
    const riskTeamSelect = document.getElementById('risk-team-select');
    const riskEpicSelect = document.getElementById('risk-epic-select');
    
    riskTeamSelect?.addEventListener('change', async (e) => {
        const teamId = e.target.value;
        if (!teamId) {
            riskEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
            riskEpicSelect.disabled = true;
            return;
        }

        try {
            riskEpicSelect.innerHTML = '<option value="">Загрузка эпиков...</option>';
            riskEpicSelect.disabled = true;
            
            const data = await apiGet(`/epics?team_id=${teamId}&all=true`);
            const epics = data.epics || [];
            
            riskEpicSelect.innerHTML = '<option value="">Выберите эпик...</option>';
            if (epics.length === 0) {
                riskEpicSelect.innerHTML = '<option value="">Нет эпиков в этой команде</option>';
            } else {
                epics.forEach(epic => {
                    const opt = document.createElement('option');
                    opt.value = epic.id;
                    opt.textContent = `${epic.number}: ${epic.name}`;
                    riskEpicSelect.appendChild(opt);
                });
                riskEpicSelect.disabled = false;
            }
        } catch (err) {
            showToast('Не удалось загрузить эпики: ' + err.message, 'error');
        }
    });

    const riskForm = document.getElementById('form-create-risk');
    riskForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const epicId = riskEpicSelect.value;
        const description = document.getElementById('risk-desc').value.trim();

        try {
            await apiPost('/risks', { description, epic_id: epicId });
            showToast('Риск успешно добавлен к эпику!', 'success');
            riskForm.reset();
            riskEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
            riskEpicSelect.disabled = true;
        } catch (err) {
            showToast('Не удалось создать риск: ' + err.message, 'error');
        }
    });

    // Import Users Form
    const importForm = document.getElementById('form-import-users');
    importForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const teamId = document.getElementById('import-team-select').value;
        const usersData = document.getElementById('import-data').value.trim();

        try {
            const resp = await apiPost('/users/bulk', { csv: usersData, team_id: teamId });
            showToast(`Импортировано пользователей: ${resp.imported_count || resp.count || 0}`, 'success');
            importForm.reset();
        } catch (err) {
            showToast('Ошибка импорта пользователей: ' + err.message, 'error');
        }
    });
}

async function reloadTeams() {
    try {
        const data = await apiGet('/teams');
        state.set('teams', data.teams || []);
    } catch (e) {
        console.error('Failed to reload teams:', e);
    }
}

async function reloadEpics(teamId) {
    try {
        const data = await apiGet(`/epics?team_id=${teamId}`);
        state.set('epics', data.epics || []);
    } catch (e) {
        console.error('Failed to reload epics:', e);
    }
}
