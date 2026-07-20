// ── Admin Panel Module ────────────────────────────────────────────────

import { state } from './state.js';
import { apiPost, apiGet, apiPut } from './api.js';
import { showToast } from './utils.js';

export function initAdminPanel() {
    console.log('initAdminPanel: Инициализация панели администратора...');

    // Подписка на изменение списка команд для заполнения выпадающих списков и чекбоксов
    state.subscribe('teams', (teams) => {
        console.log('initAdminPanel: Получен список команд:', teams);
        try {
            populateTeamSelects(teams);
            renderCheckboxList('single-user-teams-container', teams, 'team_ids');
            renderCheckboxList('edit-user-teams-container', teams, 'team_ids');
        } catch (e) {
            console.error('Ошибка в подписке на teams:', e);
        }
    });

    // Подписка на изменение списка ролей для заполнения списков чекбоксов
    state.subscribe('roles', (roles) => {
        console.log('initAdminPanel: Получен список ролей:', roles);
        try {
            renderCheckboxList('single-user-roles-container', roles, 'role_ids');
            renderCheckboxList('edit-user-roles-container', roles, 'role_ids');
            renderCheckboxList('epic-evaluating-roles-container', roles, 'evaluating_role_ids');
        } catch (e) {
            console.error('Ошибка в подписке на roles:', e);
        }
    });

    // Подписка на изменение списка пользователей для рендеринга таблицы
    state.subscribe('users', (users) => {
        console.log('initAdminPanel: Получен список пользователей:', users);
        try {
            renderUsersTable(users);
        } catch (e) {
            console.error('Ошибка в подписке на users:', e);
        }
    });

    // Загрузка пользователей при открытии вкладки админки
    state.subscribe('activeTab', (tabName) => {
        console.log('initAdminPanel: Вкладка изменена на:', tabName);
        if (tabName === 'admin') {
            console.log('initAdminPanel: Запуск загрузки пользователей...');
            loadUsers();
        }
    });

    const epicYearInput = document.getElementById('epic-year');
    if (epicYearInput && !epicYearInput.value) {
        epicYearInput.value = new Date().getFullYear();
    }

    try {
        setupFormListeners();
    } catch (e) {
        console.error('Ошибка при настройке слушателей форм:', e);
    }
}

// Заполняет стандартные выпадающие списки команд
function populateTeamSelects(teams) {
    const selects = [
        document.getElementById('epic-team-select'),
        document.getElementById('risk-team-select'),
        document.getElementById('import-team-select')
    ];

    selects.forEach(select => {
        if (!select) return;
        
        // Сохраняем текущее выбранное значение
        const val = select.value;
        
        select.innerHTML = '<option value="">Выберите команду...</option>';
        teams.forEach(team => {
            const opt = document.createElement('option');
            opt.value = team.id;
            opt.textContent = team.name;
            select.appendChild(opt);
        });
        
        // Восстанавливаем значение, если оно всё ещё валидно
        if (teams.some(t => t.id === val)) {
            select.value = val;
        }
    });
}

// Рендерит список элементов (команд или ролей) в виде списка чекбоксов
function renderCheckboxList(containerId, items, nameAttr) {
    const container = document.getElementById(containerId);
    if (!container) return;

    container.innerHTML = '';
    if (!items || items.length === 0) {
        container.innerHTML = '<div style="font-size: 12px; color: var(--text-muted);">Нет доступных элементов</div>';
        return;
    }

    items.forEach(item => {
        const div = document.createElement('div');
        div.className = 'checkbox-item';

        const cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.name = nameAttr;
        cb.value = item.id;
        cb.id = `${containerId}-${item.id}`;

        const label = document.createElement('label');
        label.htmlFor = cb.id;
        label.textContent = item.name;

        div.appendChild(cb);
        div.appendChild(label);
        container.appendChild(div);
    });
}

// Загружает список пользователей с бэкенда
async function loadUsers() {
    console.log('loadUsers: Вызов apiGet("/admin/users")...');
    try {
        const users = await apiGet('/admin/users');
        console.log('loadUsers: Пользователи успешно загружены с сервера:', users);
        state.set('users', users || []);
    } catch (err) {
        console.error('loadUsers: Ошибка при загрузке пользователей:', err);
        if (err.message !== 'UNAUTHORIZED' && err.message !== 'FORBIDDEN') {
            showToast('Не удалось загрузить пользователей: ' + err.message, 'error');
        }
    }
}

// Рендерит таблицу пользователей во вкладке Admin
function renderUsersTable(users) {
    const tbody = document.getElementById('table-users-body');
    if (!tbody) return;

    tbody.innerHTML = '';
    if (!users || users.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7" style="text-align: center; color: var(--text-muted); padding: 20px;">
                    Нет зарегистрированных пользователей
                </td>
            </tr>`;
        return;
    }

    users.forEach(user => {
        const tr = document.createElement('tr');

        // Telegram ID
        const tdTg = document.createElement('td');
        tdTg.textContent = user.telegram_id || '-';
        tr.appendChild(tdTg);

        // Имя
        const tdFirst = document.createElement('td');
        tdFirst.textContent = user.first_name || '-';
        tr.appendChild(tdFirst);

        // Фамилия
        const tdLast = document.createElement('td');
        tdLast.textContent = user.last_name || '-';
        tr.appendChild(tdLast);

        // Роли (бейджи)
        const tdRoles = document.createElement('td');
        const rolesDiv = document.createElement('div');
        rolesDiv.className = 'table-badge-list';
        (user.user_roles || []).forEach(r => {
            const span = document.createElement('span');
            span.className = 'table-badge';
            span.textContent = r.name;
            rolesDiv.appendChild(span);
        });
        if (rolesDiv.children.length === 0) {
            rolesDiv.innerHTML = '<span style="color: var(--text-muted); font-size: 11px;">Нет ролей</span>';
        }
        tdRoles.appendChild(rolesDiv);
        tr.appendChild(tdRoles);

        // Команды (бейджи)
        const tdTeams = document.createElement('td');
        const teamsDiv = document.createElement('div');
        teamsDiv.className = 'table-badge-list';
        (user.user_teams || []).forEach(t => {
            const span = document.createElement('span');
            span.className = 'table-badge';
            span.textContent = t.name;
            teamsDiv.appendChild(span);
        });
        if (teamsDiv.children.length === 0) {
            teamsDiv.innerHTML = '<span style="color: var(--text-muted); font-size: 11px;">Нет команд</span>';
        }
        tdTeams.appendChild(teamsDiv);
        tr.appendChild(tdTeams);

        // Вес
        const tdWeight = document.createElement('td');
        tdWeight.textContent = user.weight !== undefined ? user.weight : '100';
        tr.appendChild(tdWeight);

        // Действия
        const tdActions = document.createElement('td');
        const btnEdit = document.createElement('button');
        btnEdit.className = 'btn btn-secondary btn-sm';
        btnEdit.textContent = '✏️ Редактировать';
        btnEdit.addEventListener('click', () => openEditUserModal(user.id));
        tdActions.appendChild(btnEdit);

        tr.appendChild(tdActions);
        tbody.appendChild(tr);
    });
}

// Открывает модальное окно детального просмотра и редактирования
async function openEditUserModal(userId) {
    const modal = document.getElementById('modal-edit-user');
    if (!modal) return;

    try {
        const user = await apiGet(`/admin/users/${userId}`);

        // Заполнение полей формы
        document.getElementById('edit-user-id').value = user.id;
        document.getElementById('edit-user-telegram-id').value = user.telegram_id || '';
        document.getElementById('edit-user-first-name').value = user.first_name || '';
        document.getElementById('edit-user-last-name').value = user.last_name || '';
        document.getElementById('edit-user-weight').value = user.weight !== undefined ? user.weight : 100;

        // Отметка чекбоксов ролей
        const roleCbs = document.querySelectorAll('#edit-user-roles-container input[type="checkbox"]');
        roleCbs.forEach(cb => {
            cb.checked = (user.role_ids || []).includes(cb.value);
        });

        // Отметка чекбоксов команд
        const teamCbs = document.querySelectorAll('#edit-user-teams-container input[type="checkbox"]');
        teamCbs.forEach(cb => {
            cb.checked = (user.team_ids || []).includes(cb.value);
        });

        modal.classList.remove('hidden');
    } catch (err) {
        showToast('Не удалось загрузить данные пользователя: ' + err.message, 'error');
    }
}

// Закрывает модальное окно редактирования
function closeEditUserModal() {
    const modal = document.getElementById('modal-edit-user');
    if (modal) {
        modal.classList.add('hidden');
    }
}

function setupFormListeners() {
    // Форма создания новой команды
    const teamForm = document.getElementById('form-create-team');
    teamForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const name = document.getElementById('team-name').value.trim();
        const description = document.getElementById('team-desc').value.trim();

        try {
            const newTeam = await apiPost('/teams', { name, description });
            showToast(`Команда "${newTeam.name || name}" успешно создана!`, 'success');
            teamForm.reset();
            reloadTeams();
        } catch (err) {
            showToast('Не удалось создать команду: ' + err.message, 'error');
        }
    });

    // Форма создания эпика
    const epicForm = document.getElementById('form-create-epic');
    epicForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const teamId = document.getElementById('epic-team-select').value;
        const number = document.getElementById('epic-number').value.trim();
        const name = document.getElementById('epic-name').value.trim();
        const description = document.getElementById('epic-desc').value.trim();
        const year = parseInt(document.getElementById('epic-year').value, 10);
        const quarter = parseInt(document.getElementById('epic-quarter').value, 10);
        const type = document.getElementById('epic-type').value;

        // Сбор выбранных ролей-оценщиков
        const roleCbs = document.querySelectorAll('#epic-evaluating-roles-container input[type="checkbox"]:checked');
        const evaluatingRoleIds = Array.from(roleCbs).map(cb => cb.value);

        try {
            await apiPost('/epics', { 
                team_id: teamId, 
                number, 
                name, 
                description,
                year,
                quarter,
                type,
                evaluating_role_ids: evaluatingRoleIds
            });
            showToast(`Эпик "${number}: ${name}" успешно создан!`, 'success');
            epicForm.reset();
            
            const epicYearInput = document.getElementById('epic-year');
            if (epicYearInput) {
                epicYearInput.value = new Date().getFullYear();
            }
            
            if (teamId === state.get('selectedTeamId')) {
                reloadEpics(teamId);
            }
        } catch (err) {
            showToast('Не удалось создать эпик: ' + err.message, 'error');
        }
    });

    // Выбор команды для рисков (динамическая загрузка эпиков)
    const riskTeamSelect = document.getElementById('risk-team-select');
    const riskEpicSelect = document.getElementById('risk-epic-select');
    const riskStorySelect = document.getElementById('risk-story-select');
    
    riskTeamSelect?.addEventListener('change', async (e) => {
        const teamId = e.target.value;
        riskEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
        riskEpicSelect.disabled = true;
        riskStorySelect.innerHTML = '<option value="">Сначала выберите эпик...</option>';
        riskStorySelect.disabled = true;
        if (!teamId) return;

        try {
            riskEpicSelect.innerHTML = '<option value="">Загрузка эпиков...</option>';
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

    // Динамическая загрузка сторей (историй) при выборе эпика
    riskEpicSelect?.addEventListener('change', async (e) => {
        const epicId = e.target.value;
        riskStorySelect.innerHTML = '<option value="">Сначала выберите эпик...</option>';
        riskStorySelect.disabled = true;
        if (!epicId) return;

        try {
            riskStorySelect.innerHTML = '<option value="">Загрузка историй...</option>';
            const stories = await apiGet(`/epics/${epicId}/stories`);
            
            riskStorySelect.innerHTML = '<option value="">Выберите историю (сторю)...</option>';
            if (!stories || stories.length === 0) {
                riskStorySelect.innerHTML = '<option value="">Нет историй в этом эпике</option>';
            } else {
                stories.forEach(story => {
                    const opt = document.createElement('option');
                    opt.value = story.id;
                    opt.textContent = `${story.number}: ${story.name}`;
                    riskStorySelect.appendChild(opt);
                });
                riskStorySelect.disabled = false;
            }
        } catch (err) {
            showToast('Не удалось загрузить истории: ' + err.message, 'error');
        }
    });

    // Форма добавления риска
    const riskForm = document.getElementById('form-create-risk');
    riskForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const storyId = riskStorySelect.value;
        const description = document.getElementById('risk-desc').value.trim();

        if (!storyId) {
            showToast('Пожалуйста, выберите историю', 'error');
            return;
        }

        try {
            await apiPost('/risks', { description, epic_id: storyId });
            showToast('Риск успешно добавлен к истории!', 'success');
            riskForm.reset();
            riskEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
            riskEpicSelect.disabled = true;
            riskStorySelect.innerHTML = '<option value="">Сначала выберите эпик...</option>';
            riskStorySelect.disabled = true;
        } catch (err) {
            showToast('Не удалось добавить риск: ' + err.message, 'error');
        }
    });

    // Форма импорта пользователей
    const importForm = document.getElementById('form-import-users');
    importForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const teamId = document.getElementById('import-team-select').value;
        const usersData = document.getElementById('import-data').value.trim();

        try {
            const resp = await apiPost('/users/bulk', { csv: usersData, team_id: teamId });
            showToast(`Импортировано пользователей: ${resp.imported_count || resp.count || 0}`, 'success');
            importForm.reset();
            loadUsers(); // Перезагружаем список
        } catch (err) {
            showToast('Ошибка импорта пользователей: ' + err.message, 'error');
        }
    });

    // Форма создания одиночного пользователя
    const singleUserForm = document.getElementById('form-single-user');
    singleUserForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const telegramId = document.getElementById('single-user-telegram-id').value.trim();
        const firstName = document.getElementById('single-user-first-name').value.trim();
        const lastName = document.getElementById('single-user-last-name').value.trim();
        const weight = parseInt(document.getElementById('single-user-weight').value, 10) || 100;

        // Сбор выбранных ролей
        const roleCbs = document.querySelectorAll('#single-user-roles-container input[type="checkbox"]:checked');
        const roleIds = Array.from(roleCbs).map(cb => cb.value);

        // Сбор выбранных команд
        const teamCbs = document.querySelectorAll('#single-user-teams-container input[type="checkbox"]:checked');
        const teamIds = Array.from(teamCbs).map(cb => cb.value);

        try {
            await apiPost('/admin/users', {
                telegram_id: telegramId,
                first_name: firstName,
                last_name: lastName,
                weight: weight,
                role_ids: roleIds,
                team_ids: teamIds
            });

            showToast(`Пользователь @${telegramId} успешно создан!`, 'success');
            singleUserForm.reset();
            loadUsers();
        } catch (err) {
            showToast('Не удалось создать пользователя: ' + err.message, 'error');
        }
    });

    // Форма редактирования пользователя
    const editUserForm = document.getElementById('form-edit-user');
    editUserForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const userId = document.getElementById('edit-user-id').value;
        const firstName = document.getElementById('edit-user-first-name').value.trim();
        const lastName = document.getElementById('edit-user-last-name').value.trim();
        const weight = parseInt(document.getElementById('edit-user-weight').value, 10) || 100;

        // Сбор выбранных ролей
        const roleCbs = document.querySelectorAll('#edit-user-roles-container input[type="checkbox"]:checked');
        const roleIds = Array.from(roleCbs).map(cb => cb.value);

        // Сбор выбранных команд
        const teamCbs = document.querySelectorAll('#edit-user-teams-container input[type="checkbox"]:checked');
        const teamIds = Array.from(teamCbs).map(cb => cb.value);

        try {
            await apiPut(`/admin/users/${userId}`, {
                first_name: firstName,
                last_name: lastName,
                weight: weight,
                role_ids: roleIds,
                team_ids: teamIds
            });

            showToast('Данные пользователя успешно обновлены!', 'success');
            closeEditUserModal();
            loadUsers();
        } catch (err) {
            showToast('Не удалось обновить пользователя: ' + err.message, 'error');
        }
    });

    // Закрытие модального окна редактирования
    document.getElementById('edit-user-close')?.addEventListener('click', closeEditUserModal);
    document.getElementById('edit-user-cancel')?.addEventListener('click', closeEditUserModal);

    // Кнопка обновления списка пользователей
    document.getElementById('btn-refresh-users')?.addEventListener('click', () => {
        loadUsers();
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
