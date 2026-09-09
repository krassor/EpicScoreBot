// ── Admin Panel Module ────────────────────────────────────────────────

import { state } from './state.js';
import { apiPost, apiGet, apiPut, apiDelete } from './api.js';
import { showToast, handleApiError, withSubmitLock, renderTableState, openModal, closeModal } from './utils.js';

// ID команды, выбранной в разделе "Администраторы команд" (независим от глобального selectedTeamId)
let selectedTeamAdminsTeamId = '';
// ID пользователей, уже являющихся администраторами выбранной команды (для фильтрации формы назначения)
let currentTeamAdminUserIds = [];

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
            populateTeamAdminUserSelect(users, currentTeamAdminUserIds);
        } catch (e) {
            console.error('Ошибка в подписке на users:', e);
        }
    });

    // Подписка на профиль пользователя — раздел "Администраторы команд" виден только superadmin
    state.subscribe('userProfile', (profile) => {
        const card = document.getElementById('admin-card-team-admins');
        if (!card) return;
        if (profile && profile.role === 'superadmin') {
            card.classList.remove('hidden');
        } else {
            card.classList.add('hidden');
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
        document.getElementById('story-team-select'),
        document.getElementById('risk-team-select'),
        document.getElementById('import-team-select'),
        document.getElementById('team-admins-team-select')
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
            // Рендерим состояние ошибки НАПРЯМУЮ в таблицу, а не только тостом:
            // state.set здесь не вызывается, поэтому подписка на 'users' не
            // сработает и таблица иначе осталась бы в состоянии "Загрузка..."
            // навсегда (web-async-feedback, design.md Decision 8).
            renderTableState(document.getElementById('table-users-body'), 'error', {
                colspan: 7,
                errorText: 'Не удалось загрузить список пользователей.',
                onRetry: loadUsers,
            });
            handleApiError(err, { title: 'Не удалось загрузить список пользователей' });
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

        openModal(modal);
    } catch (err) {
        handleApiError(err, { title: 'Не удалось загрузить данные пользователя' });
    }
}

// Закрывает модальное окно редактирования
function closeEditUserModal() {
    const modal = document.getElementById('modal-edit-user');
    if (modal) {
        closeModal(modal);
    }
}

function setupFormListeners() {
    // Форма создания новой команды
    const teamForm = document.getElementById('form-create-team');
    teamForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const name = document.getElementById('team-name').value.trim();
        const description = document.getElementById('team-desc').value.trim();

        await withSubmitLock(teamForm, async () => {
            try {
                const newTeam = await apiPost('/teams', { name, description });
                showToast(`Команда "${newTeam.name || name}" успешно создана!`, 'success');
                teamForm.reset();
                reloadTeams();
            } catch (err) {
                handleApiError(err, { title: 'Не удалось создать команду' });
            }
        });
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

        await withSubmitLock(epicForm, async () => {
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
                handleApiError(err, { title: 'Не удалось создать эпик' });
            }
        });
    });

    // Выбор команды для историй в админке
    const storyTeamSelect = document.getElementById('story-team-select');
    const storyEpicSelect = document.getElementById('story-epic-select');
    
    storyTeamSelect?.addEventListener('change', async (e) => {
        const teamId = e.target.value;
        storyEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
        storyEpicSelect.disabled = true;
        if (!teamId) return;

        try {
            storyEpicSelect.innerHTML = '<option value="">Загрузка эпиков...</option>';
            const data = await apiGet(`/epics?team_id=${teamId}&all=true`);
            const epics = data.epics || [];
            
            storyEpicSelect.innerHTML = '<option value="">Выберите эпик...</option>';
            if (epics.length === 0) {
                storyEpicSelect.innerHTML = '<option value="">Нет эпиков в этой команде</option>';
            } else {
                epics.forEach(epic => {
                    const opt = document.createElement('option');
                    opt.value = epic.id;
                    opt.textContent = `${epic.number}: ${epic.name}`;
                    storyEpicSelect.appendChild(opt);
                });
                storyEpicSelect.disabled = false;
            }
        } catch (err) {
            handleApiError(err, { title: 'Не удалось загрузить эпики' });
        }
    });

    // Форма создания истории в админке
    const adminStoryForm = document.getElementById('form-create-story-admin');
    adminStoryForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const epicId = storyEpicSelect.value;
        const name = document.getElementById('story-name').value.trim();
        const description = document.getElementById('story-desc').value.trim();

        if (!epicId) {
            showToast('Пожалуйста, выберите эпик', 'error');
            return;
        }

        await withSubmitLock(adminStoryForm, async () => {
            try {
                await apiPost(`/epics/${epicId}/stories`, { name, description });
                showToast('История успешно создана!', 'success');
                adminStoryForm.reset();
                storyEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
                storyEpicSelect.disabled = true;

                // Оповещаем другие панели о создании истории
                window.dispatchEvent(new CustomEvent('story-created', { detail: { epicId } }));
            } catch (err) {
                handleApiError(err, { title: 'Не удалось создать историю' });
            }
        });
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
            handleApiError(err, { title: 'Не удалось загрузить эпики' });
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
            handleApiError(err, { title: 'Не удалось загрузить истории' });
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

        await withSubmitLock(riskForm, async () => {
            try {
                await apiPost('/risks', { description, epic_id: storyId });
                showToast('Риск успешно добавлен к истории!', 'success');
                riskForm.reset();
                riskEpicSelect.innerHTML = '<option value="">Сначала выберите команду...</option>';
                riskEpicSelect.disabled = true;
                riskStorySelect.innerHTML = '<option value="">Сначала выберите эпик...</option>';
                riskStorySelect.disabled = true;
            } catch (err) {
                handleApiError(err, { title: 'Не удалось добавить риск' });
            }
        });
    });

    // Форма импорта пользователей
    const importForm = document.getElementById('form-import-users');
    importForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const teamId = document.getElementById('import-team-select').value;
        const usersData = document.getElementById('import-data').value.trim();

        await withSubmitLock(importForm, async () => {
            try {
                const resp = await apiPost('/users/bulk', { csv: usersData, team_id: teamId });
                showToast(`Импортировано пользователей: ${resp.imported_count || resp.count || 0}`, 'success');
                importForm.reset();
                loadUsers(); // Перезагружаем список
            } catch (err) {
                handleApiError(err, { title: 'Ошибка импорта пользователей' });
            }
        });
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

        await withSubmitLock(singleUserForm, async () => {
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
                handleApiError(err, { title: 'Не удалось создать пользователя' });
            }
        });
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

        await withSubmitLock(editUserForm, async () => {
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
                // blocking: true — форма редактирования остаётся открытой, пользователю
                // нужно понять причину отказа (например, диапазон веса) и исправить
                // значение, а не потерять сообщение вместе с исчезающим тостом.
                handleApiError(err, { title: 'Не удалось обновить пользователя', blocking: true });
            }
        });
    });

    // Закрытие модального окна редактирования
    document.getElementById('edit-user-close')?.addEventListener('click', closeEditUserModal);
    document.getElementById('edit-user-cancel')?.addEventListener('click', closeEditUserModal);
    // Клик по фону (области dialog за пределами .modal-content) закрывает
    // модалку — замена клика по .modal-overlay при переходе на <dialog>
    // (design.md, Decision 3).
    document.getElementById('modal-edit-user')?.addEventListener('click', (e) => {
        if (!e.target.closest('.modal-content')) closeEditUserModal();
    });

    // Кнопка обновления списка пользователей
    document.getElementById('btn-refresh-users')?.addEventListener('click', () => {
        loadUsers();
    });

    // Раздел "Администраторы команд" (только superadmin)
    const teamAdminsTeamSelect = document.getElementById('team-admins-team-select');
    teamAdminsTeamSelect?.addEventListener('change', (e) => {
        selectedTeamAdminsTeamId = e.target.value;
        currentTeamAdminUserIds = [];
        if (selectedTeamAdminsTeamId) {
            loadTeamAdmins(selectedTeamAdminsTeamId);
        } else {
            renderTeamAdminsTable([]);
            populateTeamAdminUserSelect(state.get('users'), currentTeamAdminUserIds);
        }
    });

    // Форма назначения нового team-admin
    const assignTeamAdminForm = document.getElementById('form-assign-team-admin');
    assignTeamAdminForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        const userSelect = document.getElementById('team-admins-user-select');
        const userId = userSelect?.value;

        if (!selectedTeamAdminsTeamId) {
            showToast('Пожалуйста, выберите команду', 'error');
            return;
        }
        if (!userId) {
            showToast('Пожалуйста, выберите пользователя', 'error');
            return;
        }

        await withSubmitLock(assignTeamAdminForm, async () => {
            try {
                await apiPost('/admin/team-admins', { user_id: userId, team_id: selectedTeamAdminsTeamId });
                showToast('Администратор команды успешно назначен!', 'success');
                await loadTeamAdmins(selectedTeamAdminsTeamId);
            } catch (err) {
                handleApiError(err, { title: 'Не удалось назначить администратора команды' });
            }
        });
    });
}

// Заполняет список выбора пользователя для назначения team-admin,
// исключая пользователей, уже являющихся администраторами выбранной команды
function populateTeamAdminUserSelect(users, currentAdminUserIds = []) {
    const select = document.getElementById('team-admins-user-select');
    if (!select) return;

    if (!selectedTeamAdminsTeamId) {
        select.innerHTML = '<option value="">Сначала выберите команду...</option>';
        select.disabled = true;
        return;
    }

    const val = select.value;
    const availableUsers = (users || []).filter(u => !currentAdminUserIds.includes(u.id));

    select.innerHTML = '<option value="">Выберите пользователя...</option>';
    availableUsers.forEach(u => {
        const opt = document.createElement('option');
        opt.value = u.id;
        const fullName = [u.first_name, u.last_name].filter(Boolean).join(' ');
        opt.textContent = fullName ? `${u.telegram_id} (${fullName})` : u.telegram_id;
        select.appendChild(opt);
    });
    select.disabled = availableUsers.length === 0;

    if (availableUsers.some(u => u.id === val)) {
        select.value = val;
    }
}

// Загружает список администраторов выбранной команды
async function loadTeamAdmins(teamId) {
    const tbody = document.getElementById('table-team-admins-body');
    renderTableState(tbody, 'loading', { colspan: 4 });

    try {
        const data = await apiGet(`/admin/team-admins?team_id=${teamId}`);
        const admins = data.admins || [];
        currentTeamAdminUserIds = admins.map(a => a.id);
        renderTeamAdminsTable(admins);
        populateTeamAdminUserSelect(state.get('users'), currentTeamAdminUserIds);
    } catch (err) {
        // Ошибка и штатное "нет администраторов" — разные состояния (web-async-feedback,
        // «Пустой результат и ошибка различимы»): renderTeamAdminsTable([]) сюда НЕ
        // вызываем, иначе сетевой сбой выглядел бы как «у команды нет администраторов».
        currentTeamAdminUserIds = [];
        renderTableState(tbody, 'error', {
            colspan: 4,
            errorText: 'Не удалось загрузить администраторов команды. Проверьте соединение и повторите попытку.',
            onRetry: () => loadTeamAdmins(teamId),
        });
        handleApiError(err, { title: 'Не удалось загрузить администраторов команды' });
    }
}

// Рендерит таблицу текущих team-admin выбранной команды
function renderTeamAdminsTable(admins) {
    const tbody = document.getElementById('table-team-admins-body');
    if (!tbody) return;

    if (!admins || admins.length === 0) {
        // Штатное пустое состояние (не ошибка) — текст сохранён без изменений.
        renderTableState(tbody, 'empty', {
            colspan: 4,
            emptyText: selectedTeamAdminsTeamId ? 'У этой команды нет назначенных администраторов' : 'Выберите команду, чтобы увидеть список администраторов',
        });
        return;
    }

    tbody.innerHTML = '';

    admins.forEach(admin => {
        const tr = document.createElement('tr');

        const tdTg = document.createElement('td');
        tdTg.textContent = admin.telegram_id || '-';
        tr.appendChild(tdTg);

        const tdFirst = document.createElement('td');
        tdFirst.textContent = admin.first_name || '-';
        tr.appendChild(tdFirst);

        const tdLast = document.createElement('td');
        tdLast.textContent = admin.last_name || '-';
        tr.appendChild(tdLast);

        const tdActions = document.createElement('td');
        const btnRemove = document.createElement('button');
        btnRemove.className = 'btn btn-secondary btn-sm';
        btnRemove.textContent = '🗑️ Снять';
        btnRemove.addEventListener('click', () => openRemoveTeamAdminModal(admin));
        tdActions.appendChild(btnRemove);
        tr.appendChild(tdActions);

        tbody.appendChild(tr);
    });
}

// Возвращает название команды по id из уже загруженного списка команд.
function getTeamName(teamId) {
    const team = (state.get('teams') || []).find(t => t.id === teamId);
    return team ? team.name : '';
}

// Модальное окно подтверждения снятия администратора команды — кастомная
// модалка вместо window.confirm (design.md Decision 3, web-destructive-confirm):
// снятие прав необратимо влияет на доступ пользователя, запрос на сервер не
// должен уходить по одному клику на «Снять».
function openRemoveTeamAdminModal(admin) {
    let modal = document.getElementById('modal-remove-team-admin');
    if (!modal) {
        modal = document.createElement('dialog');
        modal.id = 'modal-remove-team-admin';
        modal.className = 'modal';
        modal.setAttribute('aria-labelledby', 'modal-remove-team-admin-title');
        document.body.appendChild(modal);
        // Клик по фону (области dialog за пределами .modal-content) закрывает
        // модалку — замена клика по .modal-overlay при переходе на <dialog>
        // (design.md, Decision 3). Слушатель вешается один раз при создании
        // элемента, а не при каждой перерисовке innerHTML.
        modal.addEventListener('click', (e) => {
            if (!e.target.closest('.modal-content')) closeModal(modal);
        });
    }

    const teamName = getTeamName(selectedTeamAdminsTeamId);
    const fullName = [admin.first_name, admin.last_name].filter(Boolean).join(' ') || admin.telegram_id;

    modal.innerHTML = `
        <div class="modal-content">
            <div class="modal-header">
                <h2 id="modal-remove-team-admin-title">Снятие администратора команды</h2>
                <button class="btn-icon btn-close-modal">✕</button>
            </div>
            <div class="modal-body">
                <p>
                    Снять права администратора команды <strong>${teamName}</strong> у пользователя <strong>${fullName}</strong>?
                </p>
                <p style="color: var(--color-danger);">
                    Пользователь потеряет доступ к администрированию этой команды. Права можно назначить заново.
                </p>
            </div>
            <div class="modal-footer modal-footer--destructive">
                <button type="button" class="btn btn-secondary btn-close-modal">Отмена</button>
                <button type="button" id="btn-confirm-remove-team-admin" class="btn btn-danger">Снять права</button>
            </div>
        </div>
    `;

    openModal(modal);

    modal.querySelectorAll('.btn-close-modal').forEach(btn => {
        btn.onclick = () => closeModal(modal);
    });

    const btnConfirm = modal.querySelector('#btn-confirm-remove-team-admin');
    btnConfirm.onclick = async () => {
        closeModal(modal);
        await removeTeamAdmin(admin.id);
    };
}

// Снимает пользователя с роли администратора выбранной команды
async function removeTeamAdmin(userId) {
    if (!selectedTeamAdminsTeamId) return;

    try {
        await apiDelete('/admin/team-admins', { user_id: userId, team_id: selectedTeamAdminsTeamId });
        showToast('Администратор команды успешно снят', 'success');
        await loadTeamAdmins(selectedTeamAdminsTeamId);
    } catch (err) {
        handleApiError(err, { title: 'Не удалось снять администратора команды' });
    }
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
