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
	Number     string
	Name       string
	RoleScores []RoleScoreData
	TotalScore float64 // sum of role weighted averages
	Risks      []RiskReportData
	FinalScore float64 // total score adjusted by risk coefficients
}

// ReportData is the top-level data structure for a team report.
type ReportData struct {
	TeamName  string
	Epics     []EpicReportData
	Generated time.Time
}
