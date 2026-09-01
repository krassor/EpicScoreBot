package report

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/xuri/excelize/v2"
)

// GenerateCapacityXLSX рендерит CapacityReportResponse (см.
// services.BuildCapacityReport) в виде XLSX-книги из трёх листов:
//   - «Вместимость» — таблица вместимости/плана по ролям и матрица
//     сырых ролевых оценок по эпикам, зеркалирующая таблицу вкладки
//     «Отчёты» веб-интерфейса (см. web/gantt/js/reports-panel.js,
//     renderCapacityTable);
//   - «Квоты» — лимиты/факт по типам задач (см. renderQuotasTable);
//   - «Эпики» — плоский список эпикоd с их итоговыми и ролевыми оценками
//     (включая риск-скорректированные role_scores) для полного соответствия
//     данным JSON-эндпоинта GET /api/gantt/reports/capacity.
//
// В отличие от PDF (GenerateReport, Gotenberg), генерируется полностью
// in-process, без внешнего сервиса — возвращает содержимое файла в памяти.
func GenerateCapacityXLSX(data CapacityReportResponse) ([]byte, error) {
	op := "report.GenerateCapacityXLSX"

	f := excelize.NewFile()
	defer func() {
		_ = f.Close()
	}()

	roleCapacities := sortedRoleCapacities(data.RoleCapacities)

	if err := writeCapacitySheet(f, data, roleCapacities); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := writeQuotasSheet(f, data); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := writeEpicsSheet(f, data, roleCapacities); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// excelize по умолчанию создаёт "Sheet1" — переименовываем её в первый
	// содержательный лист и удаляем как отдельную сущность не нужно, т.к.
	// writeCapacitySheet уже переиспользует её через SetSheetName.
	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("%s: write workbook: %w", op, err)
	}

	return buf.Bytes(), nil
}

// sortedRoleCapacities возвращает копию roleCapacities, отсортированную по
// названию роли — сама агрегация (services.BuildCapacityReport) строит срез
// из обхода map (порядок недетерминирован), а для файла отчёта нужен
// стабильный порядок колонок/строк.
func sortedRoleCapacities(roleCapacities []RoleCapacityData) []RoleCapacityData {
	sorted := make([]RoleCapacityData, len(roleCapacities))
	copy(sorted, roleCapacities)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].RoleName < sorted[j].RoleName
	})
	return sorted
}

// sortedEpics возвращает копию epics, отсортированную по номеру задачи —
// аналогично сортировке sortedEpics в renderCapacityTable (reports-panel.js).
func sortedEpics(epics []EpicReportItem) []EpicReportItem {
	sorted := make([]EpicReportItem, len(epics))
	copy(sorted, epics)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Number < sorted[j].Number
	})
	return sorted
}

// writeCapacitySheet заполняет лист «Вместимость»: шапка команды/периода,
// таблица эпиков × ролей (риск-скорректированные оценки, округлённые вверх
// поячеечно до целого человеко-дня — см. RoundCapacityMatrix) с итоговыми
// строками Запланировано/Доступно/Разница — зеркалирует renderCapacityTable
// (web/gantt/js/reports-panel.js). Одна колонка «Итого с рисками (чд)» на
// эпик — без отдельной «С рисками (чд)» (см. design.md change
// simplify-capacity-report, решение 2). Доступная ёмкость округляется вниз
// поролево (см. RoundRoleCapacities, решение 5) — «Доступно»/«Разница»
// отображаются целыми числами.
func writeCapacitySheet(f *excelize.File, data CapacityReportResponse, roleCapacities []RoleCapacityData) error {
	const sheet = "Вместимость"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}

	epics := sortedEpics(data.Epics)

	// Доступная ёмкость, округлённая вниз поролево — общий итог считается
	// как сумма уже округлённых величин по ролям (см. RoundRoleCapacities),
	// а не отдельно от общей численности команды.
	capacities := RoundRoleCapacities(roleCapacities)

	row := 1
	if err := setRow(f, sheet, row, "Команда:", data.TeamName); err != nil {
		return err
	}
	row++
	if err := setRow(f, sheet, row, "Период:", fmt.Sprintf("Q%d %d", data.Quarter, data.Year)); err != nil {
		return err
	}
	row++
	if err := setRow(f, sheet, row, "Итоговая вместимость команды, чд:", capacities.Total); err != nil {
		return err
	}
	row += 2

	// Header row.
	header := []interface{}{"Задача", "Тип"}
	for _, rc := range roleCapacities {
		header = append(header, rc.RoleName)
	}
	header = append(header, "Итого с рисками (чд)")
	if err := setRow(f, sheet, row, header...); err != nil {
		return err
	}
	row++

	// Матрица риск-скорректированной трудоёмкости, округлённой вверх
	// поячеечно до целого человеко-дня (см. RoundCapacityMatrix) — «Итого»
	// по эпику и «Запланировано» по роли ниже вычисляются как суммы уже
	// округлённых ячеек этой матрицы, а не отдельно округлённых точных сумм.
	roleNames := make([]string, len(roleCapacities))
	for i, rc := range roleCapacities {
		roleNames[i] = rc.RoleName
	}
	matrix := RoundCapacityMatrix(epics, roleNames)

	// Epic rows: риск-скорректированные ролевые оценки (role_scores),
	// округлённые вверх поячеечно.
	for i, e := range epics {
		values := []interface{}{fmt.Sprintf("#%s %s", e.Number, e.Name), e.Type}
		for _, roleName := range roleNames {
			values = append(values, matrix.Cells[i][roleName])
		}
		values = append(values, matrix.EpicTotals[i])
		if err := setRow(f, sheet, row, values...); err != nil {
			return err
		}
		row++
	}

	// Итоговая строка: Запланировано — сумма округлённых ячеек по роли
	// (matrix.RolePlanned), не пересчитывается отдельно другим путём.
	plannedValues := []interface{}{"Запланировано (трудоемкость), чд", ""}
	var totalRoundedPlanned int
	for _, roleName := range roleNames {
		plannedValues = append(plannedValues, matrix.RolePlanned[roleName])
		totalRoundedPlanned += matrix.RolePlanned[roleName]
	}
	plannedValues = append(plannedValues, totalRoundedPlanned)
	if err := setRow(f, sheet, row, plannedValues...); err != nil {
		return err
	}
	row++

	// Итоговая строка: Доступно (Capacity) — округлённая вниз поролево
	// доступная ёмкость (capacities, см. RoundRoleCapacities выше); общий
	// итог — сумма этих округлённых величин.
	capacityValues := []interface{}{"Доступно (емкость), чд", ""}
	for _, rc := range roleCapacities {
		capacityValues = append(capacityValues, capacities.RoleCapacity[rc.RoleName])
	}
	capacityValues = append(capacityValues, capacities.Total)
	if err := setRow(f, sheet, row, capacityValues...); err != nil {
		return err
	}
	row++

	// Итоговая строка: Разница — Capacity(округлённая вниз) − Запланировано
	// (округлённое вверх, matrix.RolePlanned); оба слагаемых уже целые,
	// поэтому результат целый без отдельного правила округления (см.
	// design.md change simplify-capacity-report, Decision 3/5).
	diffValues := []interface{}{"Разница (недо-/перепланирование), чд", ""}
	var totalDiff int
	for _, rc := range roleCapacities {
		diff := capacities.RoleCapacity[rc.RoleName] - matrix.RolePlanned[rc.RoleName]
		diffValues = append(diffValues, diff)
		totalDiff += diff
	}
	diffValues = append(diffValues, totalDiff)
	if err := setRow(f, sheet, row, diffValues...); err != nil {
		return err
	}

	return f.SetColWidth(sheet, "A", "A", 40)
}

// writeQuotasSheet заполняет лист «Квоты» — лимиты и фактические проценты
// по типам задач (feature / tech_architecture), см. renderQuotasTable
// (web/gantt/js/reports-panel.js).
func writeQuotasSheet(f *excelize.File, data CapacityReportResponse) error {
	const sheet = "Квоты"
	if _, err := f.NewSheet(sheet); err != nil {
		return fmt.Errorf("create sheet %q: %w", sheet, err)
	}

	if err := setRow(f, sheet, 1, "Тип задач", "Лимит, %", "Факт, %", "Статус"); err != nil {
		return err
	}

	type quotaRow struct {
		key   string
		label string
	}
	rows := []quotaRow{
		{key: "feature", label: "Feature (бизнес-фичи)"},
		{key: "tech_architecture", label: "Architecture + Techdebt (архитектура и техдолг)"},
	}

	row := 2
	for _, qr := range rows {
		q, ok := data.Quotas[qr.key]
		if !ok {
			continue
		}
		status := "В пределах квоты"
		if q.Status == "EXCEEDED" {
			status = "Превышено"
		}
		if err := setRow(f, sheet, row, qr.label, q.LimitPercent, q.ActualPercent, status); err != nil {
			return err
		}
		row++
	}

	return f.SetColWidth(sheet, "A", "A", 45)
}

// writeEpicsSheet заполняет лист «Эпики» — плоский список эпиков команды за
// период с итоговой оценкой и ролевыми оценками (и риск-скорректированными
// role_scores, и сырыми raw_role_scores), для полного соответствия данным
// JSON-эндпоинта GET /api/gantt/reports/capacity.
func writeEpicsSheet(f *excelize.File, data CapacityReportResponse, roleCapacities []RoleCapacityData) error {
	const sheet = "Эпики"
	if _, err := f.NewSheet(sheet); err != nil {
		return fmt.Errorf("create sheet %q: %w", sheet, err)
	}

	header := []interface{}{"Номер", "Название", "Тип", "Статус", "Итоговая оценка"}
	for _, rc := range roleCapacities {
		header = append(header, fmt.Sprintf("План: %s", rc.RoleName), fmt.Sprintf("Сырая: %s", rc.RoleName))
	}
	if err := setRow(f, sheet, 1, header...); err != nil {
		return err
	}

	row := 2
	for _, e := range sortedEpics(data.Epics) {
		values := []interface{}{e.Number, e.Name, e.Type, e.Status, e.FinalScore}
		for _, rc := range roleCapacities {
			values = append(values, e.RoleScores[rc.RoleName], e.RawRoleScores[rc.RoleName])
		}
		if err := setRow(f, sheet, row, values...); err != nil {
			return err
		}
		row++
	}

	return f.SetColWidth(sheet, "B", "B", 35)
}

// setRow записывает срез значений в строку row листа sheet, начиная с
// колонки A — тонкая обёртка над excelize.SetSheetRow с формированием
// адреса первой ячейки.
func setRow(f *excelize.File, sheet string, row int, values ...interface{}) error {
	cell, err := excelize.CoordinatesToCellName(1, row)
	if err != nil {
		return fmt.Errorf("resolve cell for row %d: %w", row, err)
	}
	if err := f.SetSheetRow(sheet, cell, &values); err != nil {
		return fmt.Errorf("write row %d: %w", row, err)
	}
	return nil
}
