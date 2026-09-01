package report

import "math"

// RoundCapacityCell округляет вверх до целого человеко-дня риск-
// скорректированную трудоёмкость одной ячейки матрицы «эпик × роль» —
// минимальный дискрет планирования в отчёте о вместимости команды (см.
// design.md change simplify-capacity-report, раздел Decisions п.3).
// Округление касается только отображения отчёта (PDF/XLSX/веб-таблица) и
// не должно применяться к final_score/role_scores, используемым для
// расчёта квот по типам задач и в остальной бизнес-логике скоринга.
func RoundCapacityCell(score float64) int {
	return int(math.Ceil(score))
}

// RoundedCapacityMatrix — поячеечно округлённая вверх матрица «эпик × роль»
// риск-скорректированной трудоёмкости, общая для PDF- и XLSX-генераторов
// отчёта о вместимости команды (см. RoundCapacityMatrix).
type RoundedCapacityMatrix struct {
	// Cells[i][roleName] = RoundCapacityCell(epics[i].RoleScores[roleName]).
	Cells []map[string]int
	// EpicTotals[i] — «Итого (чд)» по эпику epics[i]: сумма уже округлённых
	// ячеек строки (по всем ролям), а НЕ отдельно округлённая точная сумма.
	EpicTotals []int
	// RolePlanned[roleName] — «Запланировано» по роли: сумма уже
	// округлённых ячеек колонки (по всем эпикам).
	RolePlanned map[string]int
}

// RoundCapacityMatrix строит поячеечно округлённую вверх матрицу риск-
// скорректированной трудоёмкости «эпик × роль» по срезу эпиков epics
// (используются их EpicReportItem.RoleScores — риск-скорректированные, а не
// RawRoleScores) и заданному порядку ролей roleNames. Роли, отсутствующие в
// epics[i].RoleScores, считаются нулевой трудоёмкостью.
//
// «Итого» по эпику и «Запланировано» по роли вычисляются как суммы уже
// округлённых ячеек — благодаря этому любая отображаемая в отчёте сумма
// равна сумме отображаемых слагаемых (см. design.md change
// simplify-capacity-report, раздел Decisions п.3). Округление не затрагивает
// исходные значения role_scores/final_score — они используются как есть,
// без изменений, для расчёта квот и остальной бизнес-логики.
func RoundCapacityMatrix(epics []EpicReportItem, roleNames []string) RoundedCapacityMatrix {
	m := RoundedCapacityMatrix{
		Cells:       make([]map[string]int, len(epics)),
		EpicTotals:  make([]int, len(epics)),
		RolePlanned: make(map[string]int, len(roleNames)),
	}

	for i, e := range epics {
		row := make(map[string]int, len(roleNames))
		var rowTotal int
		for _, roleName := range roleNames {
			cell := RoundCapacityCell(e.RoleScores[roleName])
			row[roleName] = cell
			rowTotal += cell
			m.RolePlanned[roleName] += cell
		}
		m.Cells[i] = row
		m.EpicTotals[i] = rowTotal
	}

	return m
}
