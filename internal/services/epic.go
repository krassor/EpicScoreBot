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
			Type:          e.Type,
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

		// Сначала запрашиваем истории эпика
		stories, err := s.repo.GetStoriesByEpicID(ctx, e.ID)
		if err == nil && len(stories) > 0 {
			var totalScore float64
			for _, story := range stories {
				if story.Status != domain.StatusScored {
					continue
				}
				storyRoleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, story.ID)
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
							if r, err := s.repo.GetRoleByID(ctx, rs.RoleID); err == nil {
								roleName = r.Name
								roleNames[rs.RoleID] = r.Name
							}
						}
						scaledScore := rs.WeightedAvg * storyRiskFactor
						epicData.RoleScoresMap[roleName] += scaledScore
						totalScore += scaledScore
						rolePlanned[roleName] += scaledScore
					}
				}
			}
			// Заполняем срез RoleScores из карты RoleScoresMap
			for rName, score := range epicData.RoleScoresMap {
				epicData.RoleScores = append(epicData.RoleScores, report.RoleScoreData{
					RoleName:    rName,
					WeightedAvg: score,
				})
			}
			epicData.TotalScore = totalScore
		} else {
			// Fallback логика: если сторей нет, берем оценки ролей самого эпика
			roleScores, err := s.repo.GetEpicRoleScoresByEpicID(ctx, e.ID)
			if err == nil {
				var baseScore float64
				for _, rs := range roleScores {
					baseScore += rs.WeightedAvg
				}

				riskFactor := 1.0
				if baseScore > 0 && e.FinalScore != nil {
					riskFactor = *e.FinalScore / baseScore
				}

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
					scaledScore := rs.WeightedAvg * riskFactor
					epicData.RoleScores = append(epicData.RoleScores, report.RoleScoreData{
						RoleName:    roleName,
						WeightedAvg: scaledScore,
					})
					epicData.RoleScoresMap[roleName] = scaledScore
					totalScore += scaledScore
					rolePlanned[roleName] += scaledScore
				}
				epicData.TotalScore = totalScore
			} else {
				log.Error("failed to get role scores", sl.Err(err), slog.String("epic_id", e.ID.String()))
			}
		}

		// Risks
		var risks []domain.Risk
		stories, err = s.repo.GetStoriesByEpicID(ctx, e.ID)
		if err == nil {
			for _, story := range stories {
				storyRisks, err := s.repo.GetRisksByEpicID(ctx, story.ID)
				if err == nil {
					risks = append(risks, storyRisks...)
				}
			}
		}
		epicRisks, err := s.repo.GetRisksByEpicID(ctx, e.ID)
		if err == nil {
			risks = append(risks, epicRisks...)
		}

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

// GetCapacityReport делегирует агрегацию отчёта о вместимости команды
// BuildCapacityReport, используя собственный (широкий) Repository, который
// удовлетворяет узкому CapacityReportRepository структурно.
func (s *epicService) GetCapacityReport(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.CapacityReportResponse, error) {
	return BuildCapacityReport(ctx, s.log, s.repo, teamID, year, quarter)
}

func (s *epicService) GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.GetEvaluatingRoleIDs(ctx, epicID)
}

func (s *epicService) GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error) {
	return s.repo.GetEpicsByTeamYearQuarter(ctx, teamID, year, quarter)
}

func (s *epicService) CreateStory(ctx context.Context, epicID uuid.UUID, name, description string) (*domain.Epic, error) {
	op := "epicService.CreateStory"
	log := s.log.With(slog.String("op", op), slog.String("epic_id", epicID.String()))

	parent, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		log.Error("failed to get parent epic", sl.Err(err))
		return nil, err
	}

	if parent.ParentEpicID != nil {
		log.Warn("cannot decompose a story")
		return nil, errors.New("cannot decompose a story")
	}

	count, err := s.repo.CountStoriesByEpicID(ctx, epicID)
	if err != nil {
		log.Error("failed to count stories", sl.Err(err))
		return nil, err
	}

	storyNumber := fmt.Sprintf("%s-S%d", parent.Number, count+1)

	story, err := s.repo.CreateStory(ctx, epicID, storyNumber, name, description, parent.TeamID, parent.Year, parent.Quarter, parent.Type, parent.EvaluatingRoleIDs)
	if err != nil {
		log.Error("failed to create story", sl.Err(err))
		return nil, err
	}

	log.Info("story created successfully", slog.String("story_id", story.ID.String()), slog.String("number", storyNumber))
	return story, nil
}

func (s *epicService) GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error) {
	return s.repo.GetStoriesByEpicID(ctx, epicID)
}

func (s *epicService) DeleteStory(ctx context.Context, storyID uuid.UUID) error {
	op := "epicService.DeleteStory"
	log := s.log.With(slog.String("op", op), slog.String("story_id", storyID.String()))

	story, err := s.repo.GetEpicByID(ctx, storyID)
	if err != nil {
		log.Error("failed to get story", sl.Err(err))
		return nil
	}

	if story.ParentEpicID == nil {
		return errors.New("cannot delete epic via DeleteStory")
	}

	parent, err := s.repo.GetEpicByID(ctx, *story.ParentEpicID)
	if err == nil && parent.Status != domain.StatusNew {
		log.Warn("cannot delete story of non-new epic", slog.String("parent_status", string(parent.Status)))
		return errors.New("cannot delete story of an epic that is already in progress or scored")
	}

	err = s.repo.DeleteEpic(ctx, storyID)
	if err != nil {
		log.Error("failed to delete story", sl.Err(err))
		return err
	}

	log.Info("story deleted successfully")
	return nil
}

func (s *epicService) StartEpicScoring(ctx context.Context, epicID uuid.UUID) error {
	op := "epicService.StartEpicScoring"
	log := s.log.With(slog.String("op", op), slog.String("epic_id", epicID.String()))

	epic, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		log.Error("failed to get epic", sl.Err(err))
		return err
	}

	if epic.ParentEpicID != nil {
		return errors.New("cannot start scoring of a story directly, start parent epic scoring")
	}

	if epic.Status != domain.StatusNew {
		return fmt.Errorf("epic already in status %s", epic.Status)
	}

	stories, err := s.repo.GetStoriesByEpicID(ctx, epicID)
	if err != nil {
		log.Error("failed to get stories", sl.Err(err))
		return err
	}

	if len(stories) == 0 {
		return errors.New("epic must have at least one story")
	}

	if err := s.repo.UpdateEpicStatus(ctx, epicID, domain.StatusScoring); err != nil {
		log.Error("failed to update parent epic status", sl.Err(err))
		return err
	}

	for _, story := range stories {
		if err := s.repo.StartEpicScoring(ctx, story.ID); err != nil {
			log.Error("failed to start story scoring", slog.String("story_id", story.ID.String()), sl.Err(err))
			return err
		}
	}

	log.Info("scoring started successfully for epic and all its stories")
	return nil
}
func (s *epicService) UpdateEpic(ctx context.Context, id uuid.UUID, req domain.UpdateEpicReq) (*domain.Epic, error) {
	op := "epicService.UpdateEpic"
	log := s.log.With(slog.String("op", op), slog.String("epic_id", id.String()))

	if req.Number == "" || req.Name == "" {
		return nil, errors.New("epic number and name are required")
	}

	epic, err := s.repo.GetEpicByID(ctx, id)
	if err != nil {
		log.Error("failed to find epic", sl.Err(err))
		return nil, fmt.Errorf("epic not found: %w", err)
	}

	if epic.ParentEpicID != nil {
		return nil, errors.New("cannot update story using epic endpoint")
	}

	// Status restrictions
	if epic.Status != domain.StatusNew {
		if req.TeamID != epic.TeamID {
			return nil, fmt.Errorf("cannot change team when epic status is %s", epic.Status)
		}
		if !sameRoleIDs(epic.EvaluatingRoleIDs, req.EvaluatingRoleIDs) {
			return nil, fmt.Errorf("cannot change evaluating roles when epic status is %s", epic.Status)
		}
	}

	oldNumber := epic.Number
	epic.Number = req.Number
	epic.Name = req.Name
	epic.Description = req.Description
	epic.TeamID = req.TeamID
	epic.Year = req.Year
	epic.Quarter = req.Quarter
	epic.Type = req.Type

	if err := s.repo.UpdateEpic(ctx, epic, req.EvaluatingRoleIDs, oldNumber); err != nil {
		log.Error("failed to update epic in repository", sl.Err(err))
		return nil, err
	}

	log.Info("epic updated successfully", slog.String("number", epic.Number))
	return s.repo.GetEpicByID(ctx, id)
}

func (s *epicService) UpdateStory(ctx context.Context, id uuid.UUID, req domain.UpdateStoryReq) (*domain.Epic, error) {
	op := "epicService.UpdateStory"
	log := s.log.With(slog.String("op", op), slog.String("story_id", id.String()))

	if req.Number == "" || req.Name == "" {
		return nil, errors.New("story number and name are required")
	}

	story, err := s.repo.GetEpicByID(ctx, id)
	if err != nil {
		log.Error("failed to find story", sl.Err(err))
		return nil, fmt.Errorf("story not found: %w", err)
	}

	if story.ParentEpicID == nil {
		return nil, errors.New("cannot update epic using story endpoint")
	}

	// Check parent epic change
	if req.ParentEpicID != nil && *req.ParentEpicID != *story.ParentEpicID {
		if story.Status != domain.StatusNew {
			return nil, fmt.Errorf("cannot change parent epic when story status is %s", story.Status)
		}
		parentEpic, err := s.repo.GetEpicByID(ctx, *req.ParentEpicID)
		if err != nil {
			return nil, fmt.Errorf("parent epic not found: %w", err)
		}
		story.ParentEpicID = req.ParentEpicID
		story.TeamID = parentEpic.TeamID
		story.Year = parentEpic.Year
		story.Quarter = parentEpic.Quarter
		story.Type = parentEpic.Type
		story.EvaluatingRoleIDs = parentEpic.EvaluatingRoleIDs
	}

	story.Number = req.Number
	story.Name = req.Name
	story.Description = req.Description

	if err := s.repo.UpdateStory(ctx, story); err != nil {
		log.Error("failed to update story in repository", sl.Err(err))
		return nil, err
	}

	log.Info("story updated successfully", slog.String("number", story.Number))
	return s.repo.GetEpicByID(ctx, id)
}

func sameRoleIDs(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[uuid.UUID]bool)
	for _, id := range a {
		m[id] = true
	}
	for _, id := range b {
		if !m[id] {
			return false
		}
	}
	return true
}
