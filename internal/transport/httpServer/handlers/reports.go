package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"EpicScoreBot/internal/models/domain"
	"github.com/google/uuid"
)

type RoleCapacityData struct {
	RoleName string  `json:"role_name"`
	Capacity float64 `json:"capacity"`
	Planned  float64 `json:"planned"`
	Diff     float64 `json:"diff"`
}

type QuotaData struct {
	LimitPercent  float64 `json:"limit_percent"`
	ActualPercent float64 `json:"actual_percent"`
	Status        string  `json:"status"` // "OK" or "EXCEEDED"
}

type EpicReportItem struct {
	ID         string             `json:"id"`
	Number     string             `json:"number"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Status     string             `json:"status"`
	FinalScore float64            `json:"final_score"`
	RoleScores map[string]float64 `json:"role_scores"`
}

type CapacityReportResponse struct {
	TeamName       string               `json:"team_name"`
	Year           int                  `json:"year"`
	Quarter        int                  `json:"quarter"`
	TotalCapacity  float64              `json:"total_capacity"`
	RoleCapacities []RoleCapacityData   `json:"role_capacities"`
	Epics          []EpicReportItem     `json:"epics"`
	Quotas         map[string]QuotaData `json:"quotas"`
}

func (h *GanttHandler) GetCapacityReport(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetCapacityReport"
	log := h.log.With(slog.String("op", op))

	teamIDStr := r.URL.Query().Get("team_id")
	yearStr := r.URL.Query().Get("year")
	quarterStr := r.URL.Query().Get("quarter")

	if teamIDStr == "" {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	teamUUID, err := uuid.Parse(teamIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	year := 2026
	if yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil {
			year = y
		}
	}

	quarter := 3
	if quarterStr != "" {
		if q, err := strconv.Atoi(quarterStr); err == nil && q >= 1 && q <= 4 {
			quarter = q
		}
	}

	// 1. Get team
	team, err := h.repo.GetTeamByID(r.Context(), teamUUID)
	if err != nil {
		log.Error("failed to get team", slog.String("error", err.Error()))
		writeError(w, http.StatusNotFound, "team not found")
		return
	}

	// 2. Get users in team
	users, err := h.repo.GetUsersByTeamID(r.Context(), teamUUID)
	if err != nil {
		log.Error("failed to get team members", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}

	// 3. Count members by role
	roleMembersCount := make(map[uuid.UUID]int)
	roleNames := make(map[uuid.UUID]string)

	for _, u := range users {
		role, err := h.repo.GetRoleByUserID(r.Context(), u.ID)
		if err == nil && role != nil {
			roleMembersCount[role.ID]++
			roleNames[role.ID] = role.Name
		}
	}

	// Calculate capacity: MembersCount * 8 * 6 * 0.838
	const capacityFactor = 8.0 * 6.0 * 0.838
	totalCapacity := float64(len(users)) * capacityFactor

	// 4. Get epics for the year/quarter
	epics, err := h.repo.GetEpicsByTeamYearQuarter(r.Context(), teamUUID, year, quarter)
	if err != nil {
		log.Error("failed to get epics", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get epics")
		return
	}

	rolePlanned := make(map[string]float64)
	var epicsList []EpicReportItem
	var featureScoreSum float64
	var techScoreSum float64
	var totalFinalScore float64

	for _, e := range epics {
		item := EpicReportItem{
			ID:         e.ID.String(),
			Number:     e.Number,
			Name:       e.Name,
			Type:       e.Type,
			Status:     string(e.Status),
			RoleScores: make(map[string]float64),
		}

		if e.FinalScore != nil {
			item.FinalScore = *e.FinalScore
		}

		if e.Status == domain.StatusScored {
			totalFinalScore += item.FinalScore
			if e.Type == "feature" {
				featureScoreSum += item.FinalScore
			} else if e.Type == "architecture" || e.Type == "techdebt" {
				techScoreSum += item.FinalScore
			}

			// Get role-level scores
			roleScores, err := h.repo.GetEpicRoleScoresByEpicID(r.Context(), e.ID)
			if err == nil {
				var baseScore float64
				for _, rs := range roleScores {
					baseScore += rs.WeightedAvg
				}

				riskFactor := 1.0
				if baseScore > 0 && e.Status == domain.StatusScored && e.FinalScore != nil {
					riskFactor = *e.FinalScore / baseScore
				}

				for _, rs := range roleScores {
					roleName := rs.RoleID.String()
					if rName, exists := roleNames[rs.RoleID]; exists {
						roleName = rName
					} else {
						if r, err := h.repo.GetRoleByID(r.Context(), rs.RoleID); err == nil {
							roleName = r.Name
							roleNames[rs.RoleID] = r.Name
						}
					}
					scaledScore := rs.WeightedAvg * riskFactor
					item.RoleScores[roleName] = scaledScore
					rolePlanned[roleName] += scaledScore
				}
			}
		}

		epicsList = append(epicsList, item)
	}

	// 5. Build role capacities slice
	var roleCapacities []RoleCapacityData
	for rID, count := range roleMembersCount {
		rName := roleNames[rID]
		capVal := float64(count) * capacityFactor
		plannedVal := rolePlanned[rName]
		roleCapacities = append(roleCapacities, RoleCapacityData{
			RoleName: rName,
			Capacity: capVal,
			Planned:  plannedVal,
			Diff:     capVal - plannedVal,
		})
	}

	// 6. Calculate quotas
	quotas := make(map[string]QuotaData)
	var featurePercent float64
	var techPercent float64

	if totalFinalScore > 0 {
		featurePercent = (featureScoreSum / totalFinalScore) * 100
		techPercent = (techScoreSum / totalFinalScore) * 100
	}

	featureStatus := "OK"
	if featurePercent > 40 {
		featureStatus = "EXCEEDED"
	}
	quotas["feature"] = QuotaData{
		LimitPercent:  40,
		ActualPercent: featurePercent,
		Status:        featureStatus,
	}

	techStatus := "OK"
	if techPercent > 60 {
		techStatus = "EXCEEDED"
	}
	quotas["tech_architecture"] = QuotaData{
		LimitPercent:  60,
		ActualPercent: techPercent,
		Status:        techStatus,
	}

	resp := CapacityReportResponse{
		TeamName:       team.Name,
		Year:           year,
		Quarter:        quarter,
		TotalCapacity:  totalCapacity,
		RoleCapacities: roleCapacities,
		Epics:          epicsList,
		Quotas:         quotas,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
