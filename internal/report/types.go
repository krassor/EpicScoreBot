package report

import "time"

// RiskReportData holds aggregated data for one risk.
type RiskReportData struct {
	Description   string
	Probabilities []int   // individual user probability scores (1–4)
	Impacts       []int   // individual user impact scores (1–4)
	WeightedScore float64 // weighted average of prob*impact
	Coefficient   float64 // risk multiplier coefficient
}

// EpicReportData оборачивает EpicReportItem — общее для JSON `/reports/capacity`,
// XLSX-выгрузки и PDF ядро агрегации отчёта о вместимости (см.
// services.BuildCapacityReport) — добавляя единственное, чего в общем ядре
// нет, но нужно PDF: риски эпика (для таблицы рисков и SVG-диаграмм, см.
// report/svg.go). Номер/название/тип/статус/итоговая и ролевые оценки
// (риск-скорректированные RoleScores и сырые RawRoleScores) наследуются от
// EpicReportItem через embedding — раздельного пересчёта в TotalScore/
// RoleScoresMap (как было раньше) больше нет, что исключает рассинхронизацию
// с web/XLSX (см. design.md change simplify-capacity-report, решение 1).
type EpicReportData struct {
	EpicReportItem
	Risks []RiskReportData
}

// ReportData — данные для PDF-отчёта о вместимости команды за квартал.
// Общие с JSON `/reports/capacity` и XLSX-выгрузкой поля (команда, период,
// ёмкость, роль-капасити, квоты) продвигаются через embedding
// CapacityReportResponse (см. services.BuildCapacityReport) — PDF поверх
// добавляет только то, чего в общем ядре нет: риски по каждому эпику, задачи
// диаграммы Ганта для отдельного листа PDF и время генерации отчёта.
type ReportData struct {
	CapacityReportResponse
	// Epics переопределяет одноимённое поле, продвинутое embedding'ом
	// CapacityReportResponse ([]EpicReportItem) — PDF-у дополнительно нужны
	// риски каждого эпика, которых нет в общем ядре агрегации.
	Epics []EpicReportData
	// GanttTasks — дерево задач диаграммы Ганта команды (эпики → стори →
	// ролевые задачи) для листа PDF-отчёта (см. gantt_svg.go,
	// openspec/changes/add-gantt-page-to-pdf-report). Не добавляется в
	// CapacityReportResponse: это общее ядро с JSON `/reports/capacity` и
	// XLSX-выгрузкой, формат которых менять нельзя (design.md, Решение 4).
	// Заполняется services.GetReportData из gantt.Service.GetTeamTasks;
	// ошибка источника или отсутствие задач оставляют срез пустым и не
	// прерывают формирование остального отчёта (design.md Решение 5) — см.
	// также Generated ниже, которое всегда заполняется вне зависимости от
	// исхода этого шага.
	GanttTasks []GanttReportRow
	Generated  time.Time
}

// GanttReportLevel различает уровень строки дерева диаграммы Ганта в
// PDF-отчёте: эпик, стори внутри эпика или ролевая задача внутри стори (либо
// внутри эпика напрямую — legacy-эпики без сторей, см. GanttReportRow.Depth).
// Используется построителем SVG (gantt_svg.go) для отступов и кодирования
// состояний (design.md Решение 6).
type GanttReportLevel string

const (
	GanttReportLevelEpic  GanttReportLevel = "epic"
	GanttReportLevelStory GanttReportLevel = "story"
	GanttReportLevelRole  GanttReportLevel = "role"
)

// GanttReportRow — одна строка диаграммы Ганта для PDF-отчёта: эпик, стори
// или ролевая задача, уже с текстовыми полями, готовыми для печати — в
// отличие от domain.GanttTask, где роль и исполнитель — UUID-ссылки,
// разрешаемые в services.GetReportData (см. также handlers.GetTasks,
// использующий тот же приём резолвинга имени исполнителя). Срез, в котором
// лежат такие строки, обязан сохранять порядок вкладки «Гант» — очередь
// эпиков, стори внутри эпика, роли внутри стори (см.
// gantt.Service.GetTeamTasks/orderTasksHierarchically); построитель SVG
// (gantt_svg.go) этот порядок не пересортировывает.
type GanttReportRow struct {
	Level GanttReportLevel
	// Depth — глубина отступа строки: 0 для эпика, 1 для стори (и для
	// ролевой задачи legacy-эпика без сторей, чьи ролевые задачи
	// принадлежат напрямую эпику), 2 для ролевой задачи внутри стори.
	Depth int
	// Name — имя эпика/стори либо название роли для ролевой задачи; у
	// ролевых задач domain.GanttTask.Name уже равно названию роли
	// ("BE разработчик" и т.п. — см. handlers.roleToCSS), поэтому
	// отдельного поля для роли не требуется.
	Name      string
	StartDate time.Time
	EndDate   time.Time
	// Completed — завершённость работы: ActualEndDate задан либо Progress
	// достиг 100% (см. domain.GanttTask). Кодируется на диаграмме отдельным
	// от заливки признаком (design.md Решение 6).
	Completed bool
	// HasAssignee/AssigneeName — исполнитель ролевой задачи. Всегда false/""
	// для строк уровня GanttReportLevelEpic/GanttReportLevelStory.
	// HasAssignee=false для ролевой задачи означает отсутствие кандидатов
	// роли в команде (см. domain.GanttTask.AssigneeID) — состояние,
	// которое SHALL быть различимо на диаграмме (design.md Решение 6).
	HasAssignee  bool
	AssigneeName string
}

// ── Capacity report (JSON `/reports/capacity` + XLSX export) ───────────────
//
// Типы ниже намеренно отделены от ReportData/EpicReportData (используемых
// PDF-генератором через Gotenberg, см. GenerateReport): агрегируются они
// иначе (см. services.BuildCapacityReport) и обязаны сохранять JSON-формат,
// который уже отдаёт GET /api/gantt/reports/capacity — см. design.md
// (add-web-report), решение 3.

// RoleCapacityData holds a single role's capacity/planned/diff figures for
// the capacity report.
type RoleCapacityData struct {
	RoleName string  `json:"role_name"`
	Capacity float64 `json:"capacity"`
	Planned  float64 `json:"planned"`
	Diff     float64 `json:"diff"`
}

// QuotaData holds the limit/actual/status figures for a single task-type
// quota (feature / tech_architecture).
type QuotaData struct {
	LimitPercent  float64 `json:"limit_percent"`
	ActualPercent float64 `json:"actual_percent"`
	Status        string  `json:"status"` // "OK" or "EXCEEDED"
}

// EpicReportItem holds a single epic's identification and role scores for
// the capacity report.
type EpicReportItem struct {
	ID         string  `json:"id"`
	Number     string  `json:"number"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	FinalScore float64 `json:"final_score"`
	// RoleScores — риск-скорректированные ролевые оценки (WeightedAvg * riskFactor),
	// используются для расчёта плановой загрузки по ролям (role_capacities[].planned).
	RoleScores map[string]float64 `json:"role_scores"`
	// RawRoleScores — сырые ролевые оценки (WeightedAvg без умножения на риск-фактор эпика/историй),
	// показывают трудоёмкость эпика без риск-буфера.
	RawRoleScores map[string]float64 `json:"raw_role_scores"`
}

// CapacityReportResponse aggregates team capacity/quota/epics data, shared
// by the JSON `/reports/capacity` endpoint and the XLSX export
// (`/reports/export?format=xlsx`) — see services.BuildCapacityReport.
type CapacityReportResponse struct {
	TeamName       string               `json:"team_name"`
	Year           int                  `json:"year"`
	Quarter        int                  `json:"quarter"`
	TotalCapacity  float64              `json:"total_capacity"`
	RoleCapacities []RoleCapacityData   `json:"role_capacities"`
	Epics          []EpicReportItem     `json:"epics"`
	Quotas         map[string]QuotaData `json:"quotas"`
}
