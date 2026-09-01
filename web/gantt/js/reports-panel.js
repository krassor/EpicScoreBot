import { state } from './state.js';
import { apiGet } from './api.js';
import { showToast } from './utils.js';

const API_BASE = '/api/gantt';

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
            updateExportButtonsState();
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

    // Кнопки скачивания отчета — прямая навигация браузера по ссылке с query-параметрами,
    // авторизация уходит автоматически через cookie tg_sys_auth (см. design.md решение 5).
    document.getElementById('btn-export-report-pdf')?.addEventListener('click', () => {
        downloadReport('pdf');
    });

    document.getElementById('btn-export-report-xlsx')?.addEventListener('click', () => {
        downloadReport('xlsx');
    });

    updateExportButtonsState();
}

// Формирует URL выгрузки отчета с текущими значениями фильтров вкладки «Отчеты»
// и инициирует скачивание обычной навигацией браузера (без fetch+blob).
function downloadReport(format) {
    const teamId = document.getElementById('reports-team-select')?.value;
    if (!teamId) {
        return;
    }
    const year = document.getElementById('reports-year')?.value;
    const quarter = document.getElementById('reports-quarter')?.value;

    const params = new URLSearchParams({ team_id: teamId, format });
    if (year) params.set('year', year);
    if (quarter) params.set('quarter', quarter);

    window.location.href = `${API_BASE}/reports/export?${params.toString()}`;
}

// Синхронизирует доступность кнопок скачивания с выбором команды —
// активны ровно тогда, когда доступен вызов loadCapacityReport().
function updateExportButtonsState() {
    const teamId = document.getElementById('reports-team-select')?.value;
    const btnPdf = document.getElementById('btn-export-report-pdf');
    const btnXlsx = document.getElementById('btn-export-report-xlsx');

    const enabled = Boolean(teamId);
    if (btnPdf) btnPdf.disabled = !enabled;
    if (btnXlsx) btnXlsx.disabled = !enabled;
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

    updateExportButtonsState();
}

async function loadCapacityReport() {
    const teamId = document.getElementById('reports-team-select')?.value;
    const year = document.getElementById('reports-year')?.value;
    const quarter = document.getElementById('reports-quarter')?.value;

    updateExportButtonsState();

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

// Поячеечно округляет вверх до целого человеко-дня риск-скорректированную
// трудоёмкость матрицы «эпик × роль» (epic.role_scores) и считает итоги по
// уже округлённым значениям — зеркалирует internal/report/rounding.go
// (RoundCapacityMatrix) на бэкенде, чтобы веб-таблица не расходилась с
// PDF/XLSX-выгрузками того же отчёта.
// Возвращает:
//   cells[i][roleName]  — округлённая ячейка эпика epics[i] по роли roleName;
//   epicTotals[i]        — «Итого (чд)» по эпику: сумма округлённых ячеек строки;
//   rolePlanned[roleName] — «Запланировано» по роли: сумма округлённых ячеек колонки;
//   grandTotal            — общий итог: сумма epicTotals (= сумма rolePlanned).
function roundCapacityMatrix(epics, roleCapacities) {
    const cells = [];
    const epicTotals = [];
    const rolePlanned = {};
    roleCapacities.forEach(rc => { rolePlanned[rc.role_name] = 0; });

    let grandTotal = 0;
    epics.forEach((epic, i) => {
        const row = {};
        let rowTotal = 0;
        roleCapacities.forEach(rc => {
            const rawScore = epic.role_scores ? (epic.role_scores[rc.role_name] || 0) : 0;
            const cell = Math.ceil(rawScore);
            row[rc.role_name] = cell;
            rowTotal += cell;
            rolePlanned[rc.role_name] += cell;
        });
        cells[i] = row;
        epicTotals[i] = rowTotal;
        grandTotal += rowTotal;
    });

    return { cells, epicTotals, rolePlanned, grandTotal };
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

    const sortedEpics = [...epics].sort((a, b) => a.number.localeCompare(b.number));

    // Округлённая вверх поячеечно риск-скорректированная матрица «эпик × роль»
    // и итоги по ней — единственный источник отображаемых значений трудоёмкости.
    const matrix = roundCapacityMatrix(sortedEpics, roleCapacities);

    let html = `
        <table class="admin-table" style="width: 100%; border-collapse: collapse;">
            <thead>
                <tr>
                    <th style="text-align: left; min-width: 180px;">Задача</th>
                    <th style="text-align: center; width: 120px;">Тип</th>
    `;

    // Добавляем роли как колонки в шапку
    roleCapacities.forEach(rc => {
        html += `<th style="text-align: center; min-width: 100px;">${rc.role_name}</th>`;
    });

    html += `
                    <th style="text-align: center; width: 110px;">Итого (чд)</th>
                </tr>
            </thead>
            <tbody>
    `;

    // Заполняем строки (эпики)
    sortedEpics.forEach((epic, i) => {
        html += `
            <tr>
                <td style="font-weight: 500; text-align: left;">
                    <span style="color: var(--color-primary); font-weight: bold; margin-right: 6px;">#${epic.number}</span>
                    <span>${epic.name}</span>
                </td>
                <td style="text-align: center;">
                    <span class="badge badge-type-${epic.type}" style="font-size: 11px;">${epic.type}</span>
                </td>
        `;

        // Риск-скорректированные оценки по ролям, округлённые вверх до целого чд
        roleCapacities.forEach(rc => {
            const cell = matrix.cells[i][rc.role_name];
            html += `
                <td style="text-align: center; color: ${cell > 0 ? 'var(--color-text)' : 'var(--color-text-muted)'};">
                    ${cell > 0 ? cell : '-'}
                </td>
            `;
        });

        html += `
                <td style="text-align: center; font-weight: bold; color: var(--color-primary);">${matrix.epicTotals[i]}</td>
            </tr>
        `;
    });

    // Итоговая строка: Запланировано (Planned)
    html += `
        <tr style="border-top: 2px solid var(--color-border); background-color: rgba(255, 255, 255, 0.02); font-weight: bold;">
            <td colspan="2" style="text-align: left; padding-left: 12px;">Запланировано (трудоемкость), чд</td>
    `;
    // «Запланировано» по роли — сумма уже округлённых ячеек колонки матрицы
    // (не rc.planned/ceil(rc.planned)), чтобы сумма строки визуально сходилась
    // с суммой отображаемых ячеек эпиков.
    roleCapacities.forEach(rc => {
        html += `<td style="text-align: center;">${matrix.rolePlanned[rc.role_name]}</td>`;
    });

    html += `
            <td style="text-align: center; color: var(--color-primary);">${matrix.grandTotal}</td>
        </tr>
    `;

    // Итоговая строка: Доступно (Capacity) — формула ёмкости, без округления
    let totalCapacity = data.total_capacity || 0;
    html += `
        <tr style="background-color: rgba(255, 255, 255, 0.02); font-weight: bold;">
            <td colspan="2" style="text-align: left; padding-left: 12px;">Доступно (емкость), чд</td>
    `;
    roleCapacities.forEach(rc => {
        html += `<td style="text-align: center;">${rc.capacity.toFixed(1)}</td>`;
    });
    html += `
            <td style="text-align: center;">${totalCapacity.toFixed(1)}</td>
        </tr>
    `;

    // Итоговая строка: Разница (Diff) — формула ёмкости, без округления
    html += `
        <tr style="background-color: rgba(255, 255, 255, 0.02); font-weight: bold; border-bottom: 2px solid var(--color-border);">
            <td colspan="2" style="text-align: left; padding-left: 12px;">Разница (недо-/перепланирование), чд</td>
    `;
    roleCapacities.forEach(rc => {
        const isOverplanned = rc.diff < 0;
        const diffStyle = isOverplanned ? 'color: var(--color-danger); font-weight: bold;' : 'color: var(--color-success); font-weight: bold;';
        html += `<td style="text-align: center; ${diffStyle}">${rc.diff.toFixed(1)}</td>`;
    });
    let totalPlannedSum = 0;
    roleCapacities.forEach(rc => { totalPlannedSum += rc.planned; });
    const totalDiff = totalCapacity - totalPlannedSum;
    const isTotalOverplanned = totalDiff < 0;
    const totalDiffStyle = isTotalOverplanned ? 'color: var(--color-danger); font-weight: bold;' : 'color: var(--color-success); font-weight: bold;';
    html += `
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
