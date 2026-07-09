import { state } from './state.js';
import { apiGet } from './api.js';
import { showToast } from './utils.js';

export function initReportsPanel() {
    console.log('initReportsPanel: Инициализация панели отчетов...');

    // Установка значений по умолчанию для года и квартала при инициализации
    const reportsYearInput = document.getElementById('reports-year');
    const reportsQuarterSelect = document.getElementById('reports-quarter');
    
    if (reportsYearInput && !reportsYearInput.value) {
        reportsYearInput.value = new Date().getFullYear();
    }
    
    if (reportsQuarterSelect && !reportsQuarterSelect.value) {
        // Определяем текущий квартал
        const currentMonth = new Date().getMonth(); // 0-11
        const currentQuarter = Math.floor(currentMonth / 3) + 1;
        reportsQuarterSelect.value = currentQuarter;
    }

    // Подписка на изменение списка команд для заполнения селектора
    state.subscribe('teams', (teams) => {
        console.log('initReportsPanel: Получен список команд для отчетов:', teams);
        try {
            populateReportsTeamSelect(teams);
        } catch (e) {
            console.error('Ошибка при заполнении команд в отчетах:', e);
        }
    });

    // Синхронизация выбранной команды с глобальным состоянием
    state.subscribe('selectedTeamId', (teamId) => {
        const select = document.getElementById('reports-team-select');
        if (select && teamId && select.value !== teamId) {
            select.value = teamId;
            if (state.get('activeTab') === 'reports') {
                loadCapacityReport();
            }
        }
    });

    // Подписка на изменение активной вкладки
    state.subscribe('activeTab', (tabName) => {
        if (tabName === 'reports') {
            // Синхронизируем селектор команды, если глобально выбрана команда
            const globalTeamId = state.get('selectedTeamId');
            const select = document.getElementById('reports-team-select');
            if (select && globalTeamId && select.value !== globalTeamId) {
                select.value = globalTeamId;
            }
            loadCapacityReport();
        }
    });

    // Добавление слушателей на изменение фильтров
    document.getElementById('reports-team-select')?.addEventListener('change', (e) => {
        const teamId = e.target.value;
        if (teamId) {
            state.set('selectedTeamId', teamId);
        }
        loadCapacityReport();
    });

    document.getElementById('reports-year')?.addEventListener('change', () => {
        loadCapacityReport();
    });

    document.getElementById('reports-quarter')?.addEventListener('change', () => {
        loadCapacityReport();
    });
}

function populateReportsTeamSelect(teams) {
    const select = document.getElementById('reports-team-select');
    if (!select) return;

    const val = select.value;
    select.innerHTML = '<option value="">Выберите команду...</option>';
    
    teams.forEach(team => {
        const opt = document.createElement('option');
        opt.value = team.id;
        opt.textContent = team.name;
        select.appendChild(opt);
    });

    if (teams.some(t => t.id === val)) {
        select.value = val;
    } else if (state.get('selectedTeamId') && teams.some(t => t.id === state.get('selectedTeamId'))) {
        select.value = state.get('selectedTeamId');
    }
}

async function loadCapacityReport() {
    const teamId = document.getElementById('reports-team-select')?.value;
    const year = document.getElementById('reports-year')?.value;
    const quarter = document.getElementById('reports-quarter')?.value;

    const capacityContainer = document.getElementById('reports-capacity-table-container');
    const quotasContainer = document.getElementById('reports-quotas-table-container');

    if (!teamId) {
        if (capacityContainer) {
            capacityContainer.innerHTML = '<p style="color: var(--text-muted); padding: 10px; margin: 0;">Выберите команду для формирования отчета.</p>';
        }
        if (quotasContainer) {
            quotasContainer.innerHTML = '<p style="color: var(--text-muted); padding: 10px; margin: 0;">Выберите команду для формирования отчета.</p>';
        }
        return;
    }

    if (capacityContainer) {
        capacityContainer.innerHTML = '<div class="loader-container" style="display: flex; justify-content: center; padding: 20px;"><div class="loader"></div></div>';
    }
    if (quotasContainer) {
        quotasContainer.innerHTML = '<div class="loader-container" style="display: flex; justify-content: center; padding: 20px;"><div class="loader"></div></div>';
    }

    try {
        const url = `/reports/capacity?team_id=${teamId}&year=${year}&quarter=${quarter}`;
        console.log(`loadCapacityReport: Запрос GET ${url}`);
        const data = await apiGet(url);
        
        renderCapacityTable(data);
        renderQuotasTable(data);
    } catch (err) {
        console.error('Ошибка при загрузке отчета о вместимости:', err);
        const errMsg = `<p style="color: var(--color-danger); padding: 10px; margin: 0;">Ошибка загрузки данных: ${err.message}</p>`;
        if (capacityContainer) capacityContainer.innerHTML = errMsg;
        if (quotasContainer) quotasContainer.innerHTML = errMsg;
    }
}

function renderCapacityTable(data) {
    const container = document.getElementById('reports-capacity-table-container');
    if (!container) return;

    const epics = data.epics || [];
    const roleCapacities = data.role_capacities || [];

    if (roleCapacities.length === 0) {
        container.innerHTML = '<p style="color: var(--text-muted); padding: 10px; margin: 0;">Нет данных о вместимости участников для выбранного периода.</p>';
        return;
    }

    // Собираем все уникальные эпики
    const sortedEpics = [...epics].sort((a, b) => a.number.localeCompare(b.number));

    let html = `
        <table class="admin-table" style="width: 100%; border-collapse: collapse;">
            <thead>
                <tr>
                    <th style="text-align: left;">Роль</th>
                    <th style="text-align: center;">Доступно (чд)</th>
    `;

    // Добавляем колонки для каждого эпика
    sortedEpics.forEach(epic => {
        const typeClass = `badge-type-${epic.type}`;
        // Добавим класс стиля для бейджей типов, если нужно
        html += `
            <th style="text-align: center; min-width: 100px;" title="${epic.name} (${epic.status})">
                <div style="font-size: 11px; color: var(--text-muted); font-weight: normal; margin-bottom: 2px;">${epic.type}</div>
                <div>${epic.number}</div>
            </th>
        `;
    });

    html += `
                    <th style="text-align: center;">Запланировано (чд)</th>
                    <th style="text-align: center;">Разница (чд)</th>
                </tr>
            </thead>
            <tbody>
    `;

    // Вычисляем суммарные показатели по колонкам
    let totalCapacity = data.total_capacity || 0;
    let totalPlannedSum = 0;
    const epicSums = {};
    sortedEpics.forEach(epic => {
        epicSums[epic.id] = 0;
    });

    // Отрисовываем строки для каждой роли
    roleCapacities.forEach(rc => {
        const isOverplanned = rc.diff < 0;
        const diffStyle = isOverplanned ? 'color: var(--color-danger); font-weight: bold;' : 'color: var(--color-success);';
        
        html += `
            <tr>
                <td style="font-weight: 500;">${rc.role_name}</td>
                <td style="text-align: center; font-weight: 500;">${rc.capacity.toFixed(1)}</td>
        `;

        // Оценки по эпикам
        sortedEpics.forEach(epic => {
            const score = epic.role_scores ? (epic.role_scores[rc.role_name] || 0) : 0;
            epicSums[epic.id] += score;
            html += `
                <td style="text-align: center; color: ${score > 0 ? 'var(--color-text)' : 'var(--color-text-muted)'};">
                    ${score > 0 ? score.toFixed(1) : '-'}
                </td>
            `;
        });

        totalPlannedSum += rc.planned;

        html += `
                <td style="text-align: center; font-weight: 500;">${rc.planned.toFixed(1)}</td>
                <td style="text-align: center; ${diffStyle}">${rc.diff.toFixed(1)}</td>
            </tr>
        `;
    });

    // Отрисовываем итоговую строку
    const totalDiff = totalCapacity - totalPlannedSum;
    const isTotalOverplanned = totalDiff < 0;
    const totalDiffStyle = isTotalOverplanned ? 'color: var(--color-danger); font-weight: bold;' : 'color: var(--color-success); font-weight: bold;';

    html += `
            <tr style="border-top: 2px solid var(--color-border); background-color: rgba(255, 255, 255, 0.02); font-weight: bold;">
                <td>Итого</td>
                <td style="text-align: center;">${totalCapacity.toFixed(1)}</td>
    `;

    // Суммы по эпикам в итоговой строке
    sortedEpics.forEach(epic => {
        const sum = epicSums[epic.id] || 0;
        html += `
            <td style="text-align: center;">${sum > 0 ? sum.toFixed(1) : '-'}</td>
        `;
    });

    html += `
                <td style="text-align: center;">${totalPlannedSum.toFixed(1)}</td>
                <td style="text-align: center; ${totalDiffStyle}">${totalDiff.toFixed(1)}</td>
            </tr>
        </tbody>
    </table>
    `;

    container.innerHTML = html;
}

function renderQuotasTable(data) {
    const container = document.getElementById('reports-quotas-table-container');
    if (!container) return;

    const quotas = data.quotas || {};
    
    const featureQuota = quotas.feature || { limit_percent: 40, actual_percent: 0, status: 'OK' };
    const techQuota = quotas.tech_architecture || { limit_percent: 60, actual_percent: 0, status: 'OK' };

    const getStatusBadge = (status) => {
        if (status === 'EXCEEDED') {
            return `<span class="badge" style="background-color: var(--color-danger); color: white; padding: 4px 8px; border-radius: 4px; font-weight: bold;">Превышено</span>`;
        }
        return `<span class="badge" style="background-color: var(--color-success); color: white; padding: 4px 8px; border-radius: 4px; font-weight: bold;">В пределах квоты</span>`;
    };

    const getRowStyle = (status) => {
        return status === 'EXCEEDED' ? 'background-color: rgba(239, 68, 68, 0.05);' : '';
    };

    let html = `
        <table class="admin-table" style="width: 100%; border-collapse: collapse;">
            <thead>
                <tr>
                    <th style="text-align: left; width: 40%;">Тип задач</th>
                    <th style="text-align: center; width: 20%;">Целевой лимит</th>
                    <th style="text-align: center; width: 20%;">Фактический процент</th>
                    <th style="text-align: center; width: 20%;">Статус</th>
                </tr>
            </thead>
            <tbody>
                <tr style="${getRowStyle(featureQuota.status)}">
                    <td style="font-weight: 500;">
                        <span style="display: inline-block; width: 12px; height: 12px; background-color: var(--color-primary); border-radius: 3px; margin-right: 8px; vertical-align: middle;"></span>
                        Feature (Бизнес-фичи)
                    </td>
                    <td style="text-align: center;">&le; ${featureQuota.limit_percent}%</td>
                    <td style="text-align: center; font-weight: 600; ${featureQuota.status === 'EXCEEDED' ? 'color: var(--color-danger);' : ''}">
                        ${featureQuota.actual_percent.toFixed(1)}%
                    </td>
                    <td style="text-align: center;">${getStatusBadge(featureQuota.status)}</td>
                </tr>
                <tr style="${getRowStyle(techQuota.status)}">
                    <td style="font-weight: 500;">
                        <span style="display: inline-block; width: 12px; height: 12px; background-color: var(--color-role-it-leader); border-radius: 3px; margin-right: 8px; vertical-align: middle;"></span>
                        Architecture + Techdebt (Архитектура и техдолг)
                    </td>
                    <td style="text-align: center;">&le; ${techQuota.limit_percent}%</td>
                    <td style="text-align: center; font-weight: 600; ${techQuota.status === 'EXCEEDED' ? 'color: var(--color-danger);' : ''}">
                        ${techQuota.actual_percent.toFixed(1)}%
                    </td>
                    <td style="text-align: center;">${getStatusBadge(techQuota.status)}</td>
                </tr>
            </tbody>
        </table>
    `;

    container.innerHTML = html;
}
