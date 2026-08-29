package report

import "time"

// RoleScoreData holds a single role's weighted average for an epic.
type RoleScoreData struct {
	RoleName    string
	WeightedAvg float64
}

// RiskReportData holds aggregated data for one risk.
type RiskReportData struct {
	Description   string
	Probabilities []int   // individual user probability scores (1–4)
	Impacts       []int   // individual user impact scores (1–4)
	WeightedScore float64 // weighted average of prob*impact
	Coefficient   float64 // risk multiplier coefficient
}

// EpicReportData holds all report data for a single epic.
type EpicReportData struct {
	Number        string
	Name          string
	Type          string
	RoleScores    []RoleScoreData
	RoleScoresMap map[string]float64 // Map for easy template lookups by role name
	TotalScore    float64            // sum of role weighted averages
	Risks         []RiskReportData
	FinalScore    float64            // total score adjusted by risk coefficients
}

type RoleCapacityReportData struct {
	RoleName string
	Capacity float64
	Planned  float64
	Diff     float64
}

type QuotaReportData struct {
	LimitPercent  float64
	ActualPercent float64
	Status        string
}

// ReportData is the top-level data structure for a team report.
type ReportData struct {
	TeamName       string
	Year           int
	Quarter        int
	TotalCapacity  float64
	RoleCapacities []RoleCapacityReportData
	Epics          []EpicReportData
	Quotas         map[string]QuotaReportData
	Generated      time.Time
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
