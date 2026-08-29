package services

import (
	"context"
	"fmt"
	"log/slog"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/google/uuid"
)

// CapacityReportRepository — минимальный набор методов хранилища,
// необходимый для агрегации отчёта о вместимости команды (см.
// BuildCapacityReport). Выделен в узкий интерфейс, а не завязан на широкий
// Repository, чтобы агрегатор мог использоваться напрямую HTTP-слоем
// (handlers.Repository — надмножество этих методов с идентичными
// сигнатурами) без необходимости заводить там полноценный EpicService.
type CapacityReportRepository interface {
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)
	GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
}

// BuildCapacityReport агрегирует вместимость по ролям, план/факт, квоты по
// типам задач и список эпиков команды за указанные год/квартал —
// используется и JSON-эндпоинтом GET /api/gantt/reports/capacity
// (handlers.GetCapacityReport), и XLSX-выгрузкой
// (handlers.ExportTeamReport, format=xlsx), чтобы не дублировать логику
// подсчёта (см. design.md change add-web-report, решение 3).
//
// Возвращает ErrTeamNotFound, если команда с teamID не найдена.
func BuildCapacityReport(ctx context.Context, log *slog.Logger, repo CapacityReportRepository, teamID uuid.UUID, year, quarter int) (*report.CapacityReportResponse, error) {
	op := "services.BuildCapacityReport"
	log = log.With(slog.String("op", op), slog.String("team_id", teamID.String()), slog.Int("year", year), slog.Int("quarter", quarter))

	// 1. Get team.
	team, err := repo.GetTeamByID(ctx, teamID)
	if err != nil {
		log.Error("failed to get team", sl.Err(err))
		return nil, fmt.Errorf("%s: %w: %w", op, ErrTeamNotFound, err)
	}

	// 2. Get users in team.
	users, err := repo.GetUsersByTeamID(ctx, teamID)
	if err != nil {
		log.Error("failed to get team members", sl.Err(err))
		return nil, fmt.Errorf("%s: failed to get team members: %w", op, err)
	}

	// 3. Count members by role.
	roleMembersCount := make(map[uuid.UUID]int)
	roleNames := make(map[uuid.UUID]string)

	for _, u := range users {
		role, err := repo.GetRoleByUserID(ctx, u.ID)
		if err == nil && role != nil {
			roleMembersCount[role.ID]++
			roleNames[role.ID] = role.Name
		}
	}

	// Calculate capacity: MembersCount * 8 * 6 * 0.838.
	const capacityFactor = 8.0 * 6.0 * 0.838
	totalCapacity := float64(len(users)) * capacityFactor

	// 4. Get epics for the year/quarter.
	epics, err := repo.GetEpicsByTeamYearQuarter(ctx, teamID, year, quarter)
	if err != nil {
		log.Error("failed to get epics", sl.Err(err))
		return nil, fmt.Errorf("%s: failed to get epics: %w", op, err)
	}

	rolePlanned := make(map[string]float64)
	var epicsList []report.EpicReportItem
	var featureScoreSum float64
	var techScoreSum float64
	var totalFinalScore float64

	for _, e := range epics {
		item := report.EpicReportItem{
			ID:            e.ID.String(),
			Number:        e.Number,
			Name:          e.Name,
			Type:          e.Type,
			Status:        string(e.Status),
			RoleScores:    make(map[string]float64),
			RawRoleScores: make(map[string]float64),
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

			// Сначала пытаемся получить оценки со сторей (историй) эпика.
			stories, err := repo.GetStoriesByEpicID(ctx, e.ID)
			if err == nil && len(stories) > 0 {
				// Если стори есть, суммируем их ролевые оценки с учетом рисков каждой стори.
				for _, story := range stories {
					if story.Status != domain.StatusScored {
						continue
					}
					storyRoleScores, err := repo.GetEpicRoleScoresByEpicID(ctx, story.ID)
					if err == nil {
						var storyBaseScore float64
						for _, rs := range storyRoleScores {
							storyBaseScore += rs.WeightedAvg
						}

						storyRiskFactor := 1.0
						if storyBaseScore > 0 && story.FinalScore != nil {
							storyRiskFactor = *story.FinalScore / storyBaseScore
						}

						for _, rs := range storyRoleScores {
							roleName := rs.RoleID.String()
							if rName, exists := roleNames[rs.RoleID]; exists {
								roleName = rName
							} else {
								if r, err := repo.GetRoleByID(ctx, rs.RoleID); err == nil {
									roleName = r.Name
									roleNames[rs.RoleID] = r.Name
								}
							}
							scaledScore := rs.WeightedAvg * storyRiskFactor
							item.RoleScores[roleName] += scaledScore
							item.RawRoleScores[roleName] += rs.WeightedAvg
							rolePlanned[roleName] += scaledScore
						}
					}
				}
			} else {
				// Fallback логика: если сторей нет, берем оценки ролей самого эпика.
				roleScores, err := repo.GetEpicRoleScoresByEpicID(ctx, e.ID)
				if err == nil {
					var baseScore float64
					for _, rs := range roleScores {
						baseScore += rs.WeightedAvg
					}

					riskFactor := 1.0
					if baseScore > 0 && e.FinalScore != nil {
						riskFactor = *e.FinalScore / baseScore
					}

					for _, rs := range roleScores {
						roleName := rs.RoleID.String()
						if rName, exists := roleNames[rs.RoleID]; exists {
							roleName = rName
						} else {
							if r, err := repo.GetRoleByID(ctx, rs.RoleID); err == nil {
								roleName = r.Name
								roleNames[rs.RoleID] = r.Name
							}
						}
						scaledScore := rs.WeightedAvg * riskFactor
						item.RoleScores[roleName] = scaledScore
						item.RawRoleScores[roleName] = rs.WeightedAvg
						rolePlanned[roleName] += scaledScore
					}
				}
			}
		}

		epicsList = append(epicsList, item)
	}

	// 5. Build role capacities slice.
	var roleCapacities []report.RoleCapacityData
	for rID, count := range roleMembersCount {
		rName := roleNames[rID]
		capVal := float64(count) * capacityFactor
		plannedVal := rolePlanned[rName]
		roleCapacities = append(roleCapacities, report.RoleCapacityData{
			RoleName: rName,
			Capacity: capVal,
			Planned:  plannedVal,
			Diff:     capVal - plannedVal,
		})
	}

	// 6. Calculate quotas.
	quotas := make(map[string]report.QuotaData)
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
	quotas["feature"] = report.QuotaData{
		LimitPercent:  40,
		ActualPercent: featurePercent,
		Status:        featureStatus,
	}

	techStatus := "OK"
	if techPercent > 60 {
		techStatus = "EXCEEDED"
	}
	quotas["tech_architecture"] = report.QuotaData{
		LimitPercent:  60,
		ActualPercent: techPercent,
		Status:        techStatus,
	}

	return &report.CapacityReportResponse{
		TeamName:       team.Name,
		Year:           year,
		Quarter:        quarter,
		TotalCapacity:  totalCapacity,
		RoleCapacities: roleCapacities,
		Epics:          epicsList,
		Quotas:         quotas,
	}, nil
}
