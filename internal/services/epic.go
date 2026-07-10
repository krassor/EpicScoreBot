package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/google/uuid"
)

type epicService struct {
	log  *slog.Logger
	repo Repository
}

func NewEpicService(log *slog.Logger, repo Repository) EpicService {
	return &epicService{
		log:  log.With(slog.String("service", "epic")),
		repo: repo,
	}
}

func (s *epicService) CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error) {
	op := "epicService.CreateEpic"
	log := s.log.With(slog.String("op", op), slog.String("number", number))

	existing, err := s.repo.GetEpicByNumber(ctx, number)
	if err == nil && existing != nil {
		log.Warn("epic with number already exists")
		return nil, ErrEpicAlreadyExists
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Error("error checking existing epic", sl.Err(err))
		return nil, err
	}

	epic, err := s.repo.CreateEpic(ctx, number, name, description, teamID, year, quarter, epicType, evaluatingRoleIDs)
	if err != nil {
		log.Error("failed to create epic", sl.Err(err))
		return nil, err
	}

	log.Info("epic created successfully", slog.String("epic_id", epic.ID.String()))
	return epic, nil
}

func (s *epicService) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return s.repo.GetEpicByID(ctx, epicID)
}

func (s *epicService) GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error) {
	return s.repo.GetEpicByNumber(ctx, number)
}

func (s *epicService) GetEpicsByStatus(ctx context.Context, status domain.Status) ([]domain.Epic, error) {
	return s.repo.GetEpicsByStatus(ctx, status)
}

func (s *epicService) GetAllEpics(ctx context.Context) ([]domain.Epic, error) {
	return s.repo.GetAllEpics(ctx)
}

func (s *epicService) GetUnscoredEpicsByUser(ctx context.Context, userID, teamID uuid.UUID) ([]domain.Epic, error) {
	return s.repo.GetUnscoredEpicsByUser(ctx, userID, teamID)
}

func (s *epicService) GetUnscoredEpicsForUserAcrossTeams(ctx context.Context, userID uuid.UUID, telegramID string) ([]domain.Epic, error) {
	op := "epicService.GetUnscoredEpicsForUserAcrossTeams"
	log := s.log.With(slog.String("op", op), slog.String("telegram_id", telegramID))

	teams, err := s.repo.GetTeamsByUserTelegramID(ctx, telegramID)
	if err != nil {
		log.Error("failed to get teams by user telegram id", sl.Err(err))
		return nil, err
	}

	seen := make(map[uuid.UUID]bool)
	var allEpics []domain.Epic
	for _, team := range teams {
		epics, err := s.repo.GetUnscoredEpicsByUser(ctx, userID, team.ID)
		if err != nil {
			log.Error("error getting unscored epics", sl.Err(err), slog.String("team_id", team.ID.String()))
			continue
		}
		for _, e := range epics {
			if !seen[e.ID] {
				seen[e.ID] = true
				allEpics = append(allEpics, e)
			}
		}
	}

	return allEpics, nil
}

func (s *epicService) UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error {
	return s.repo.UpdateEpicStatus(ctx, epicID, status)
}

func (s *epicService) DeleteEpic(ctx context.Context, epicID uuid.UUID) error {
	return s.repo.DeleteEpic(ctx, epicID)
}

func (s *epicService) GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error) {
	return s.repo.GetEpicsByTeamIDAndStatus(ctx, teamID, status)
}

func (s *epicService) CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error {
	return s.repo.CreateEpicScore(ctx, epicID, userID, roleID, score)
}

func (s *epicService) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	return s.repo.HasUserScoredEpic(ctx, epicID, userID)
}

func (s *epicService) GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error) {
	return s.repo.GetUsersWhoScoredEpic(ctx, epicID)
}

func (s *epicService) GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error) {
	return s.repo.GetEpicRoleScoresByEpicID(ctx, epicID)
}

func (s *epicService) GetReportData(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error) {
	op := "epicService.GetReportData"
	log := s.log.With(slog.String("op", op), slog.String("team_id", teamID.String()), slog.Int("year", year), slog.Int("quarter", quarter))

	team, err := s.repo.GetTeamByID(ctx, teamID)
	if err != nil {
		log.Error("failed to get team", sl.Err(err))
		return nil, fmt.Errorf("team not found: %w", err)
	}

	allEpics, err := s.repo.GetEpicsByTeamYearQuarter(ctx, teamID, year, quarter)
	if err != nil {
		log.Error("failed to get epics for team", sl.Err(err))
		return nil, fmt.Errorf("failed to get epics: %w", err)
	}

	var epics []domain.Epic
	for _, e := range allEpics {
		if e.Status == domain.StatusScored {
			epics = append(epics, e)
		}
	}

	// 1. Get users in team and role capacities
	users, err := s.repo.GetUsersByTeamID(ctx, teamID)
	if err != nil {
		log.Error("failed to get team members", sl.Err(err))
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}

	roleMembersCount := make(map[uuid.UUID]int)
	roleNames := make(map[uuid.UUID]string)
	for _, u := range users {
		role, err := s.repo.GetRoleByUserID(ctx, u.ID)
		if err == nil && role != nil {
			roleMembersCount[role.ID]++
			roleNames[role.ID] = role.Name
		}
	}

	const capacityFactor = 8.0 * 6.0 * 0.838
	totalCapacity := float64(len(users)) * capacityFactor

	rolePlanned := make(map[string]float64)
	epicReportItems := make([]report.EpicReportData, 0, len(epics))
	var featureScoreSum float64
	var techScoreSum float64
	var totalFinalScore float64

	for _, e := range epics {
		epicData := report.EpicReportData{
			Number:        e.Number,
			Name:          e.Name,
			RoleScores:    []report.RoleScoreData{},
			RoleScoresMap: make(map[string]float64),
			Risks:         []report.RiskReportData{},
		}

		if e.FinalScore != nil {
			epicData.FinalScore = *e.FinalScore
			totalFinalScore += *e.FinalScore
			if e.Type == "feature" {
				featureScoreSum += *e.FinalScore
			} else if e.Type == "architecture" || e.Type == "techdebt" {
				techScoreSum += *e.FinalScore
			}
		}

		// Role scores
		roleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, e.ID)
		if err == nil {
			var totalScore float64
			for _, rs := range roleScores {
				roleName := rs.RoleID.String()
				if rName, exists := roleNames[rs.RoleID]; exists {
					roleName = rName
				} else {
					if r, err := s.repo.GetRoleByID(ctx, rs.RoleID); err == nil {
						roleName = r.Name
						roleNames[rs.RoleID] = r.Name
					}
				}
				epicData.RoleScores = append(epicData.RoleScores, report.RoleScoreData{
					RoleName:    roleName,
					WeightedAvg: rs.WeightedAvg,
				})
				epicData.RoleScoresMap[roleName] = rs.WeightedAvg
				totalScore += rs.WeightedAvg
				rolePlanned[roleName] += rs.WeightedAvg
			}
			epicData.TotalScore = totalScore
		} else {
			log.Error("failed to get role scores", sl.Err(err), slog.String("epic_id", e.ID.String()))
		}

		// Risks
		risks, err := s.repo.GetRisksByEpicID(ctx, e.ID)
		if err == nil {
			for _, r := range risks {
				riskScores, err := s.repo.GetRiskScoresByRiskID(ctx, r.ID)

				var probs []int
				var impacts []int
				if err == nil {
					for _, rs := range riskScores {
						probs = append(probs, rs.Probability)
						impacts = append(impacts, rs.Impact)
					}
				}

				var wScore float64
				var coeff float64
				if r.WeightedScore != nil {
					wScore = *r.WeightedScore
					coeff = scoring.RiskCoefficient(wScore)
				} else {
					coeff = 1.0
				}

				epicData.Risks = append(epicData.Risks, report.RiskReportData{
					Description:   r.Description,
					Probabilities: probs,
					Impacts:       impacts,
					WeightedScore: wScore,
					Coefficient:   coeff,
				})
			}
		} else {
			log.Error("failed to get risks", sl.Err(err), slog.String("epic_id", e.ID.String()))
		}

		epicReportItems = append(epicReportItems, epicData)
	}

	var roleCapacities []report.RoleCapacityReportData
	for rID, count := range roleMembersCount {
		rName := roleNames[rID]
		capVal := float64(count) * capacityFactor
		plannedVal := rolePlanned[rName]
		roleCapacities = append(roleCapacities, report.RoleCapacityReportData{
			RoleName: rName,
			Capacity: capVal,
			Planned:  plannedVal,
			Diff:     capVal - plannedVal,
		})
	}

	quotas := make(map[string]report.QuotaReportData)
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
	quotas["feature"] = report.QuotaReportData{
		LimitPercent:  40,
		ActualPercent: featurePercent,
		Status:        featureStatus,
	}

	techStatus := "OK"
	if techPercent > 60 {
		techStatus = "EXCEEDED"
	}
	quotas["tech_architecture"] = report.QuotaReportData{
		LimitPercent:  60,
		ActualPercent: techPercent,
		Status:        techStatus,
	}

	reportData := &report.ReportData{
		TeamName:       team.Name,
		Year:           year,
		Quarter:        quarter,
		TotalCapacity:  totalCapacity,
		RoleCapacities: roleCapacities,
		Epics:          epicReportItems,
		Quotas:         quotas,
		Generated:      time.Now(),
	}

	return reportData, nil
}

func (s *epicService) GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.GetEvaluatingRoleIDs(ctx, epicID)
}

func (s *epicService) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	return s.repo.GetEpicsByTeamYearQuarter(ctx, teamID, year, quarter)
}
