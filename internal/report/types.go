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
