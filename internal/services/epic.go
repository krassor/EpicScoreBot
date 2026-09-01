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

// GetReportData собирает данные для PDF-отчёта о вместимости команды за
// квартал. Ядро (команда/период/ёмкость/квоты/эпики с риск-скорректированными
// role_scores и сырыми raw_role_scores) строит общий агрегатор
// BuildCapacityReport — тот же, что использует JSON-эндпоинт
// GetCapacityReport и XLSX-выгрузка, — и встраивается в report.ReportData
// через embedding (см. report.ReportData), а не пересчитывается вручную:
// это исключает расхождение чисел между PDF и web/XLSX (см. design.md
// change simplify-capacity-report, решение 1). Поверх ядра дозапрашиваются
// только данные, которых в CapacityReportResponse нет, но нужны PDF: риски
// по каждому эпику (для таблицы рисков и SVG-диаграмм) и время генерации.
//
// В отчёт попадают только эпики со статусом StatusScored — как и раньше,
// до этого рефакторинга (в отличие от JSON/XLSX, показывающих все эпики
// периода независимо от статуса).
func (s *epicService) GetReportData(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error) {
	op := "epicService.GetReportData"
	log := s.log.With(slog.String("op", op), slog.String("team_id", teamID.String()), slog.Int("year", year), slog.Int("quarter", quarter))

	core, err := BuildCapacityReport(ctx, s.log, s.repo, teamID, year, quarter)
	if err != nil {
		log.Error("failed to build capacity report core", sl.Err(err))
		return nil, err
	}

	scoredEpics := make([]report.EpicReportItem, 0, len(core.Epics))
	for _, e := range core.Epics {
		if e.Status == string(domain.StatusScored) {
			scoredEpics = append(scoredEpics, e)
		}
	}
	core.Epics = scoredEpics

	epicReportItems := make([]report.EpicReportData, 0, len(scoredEpics))
	for _, item := range scoredEpics {
		var risks []report.RiskReportData
		if epicID, parseErr := uuid.Parse(item.ID); parseErr == nil {
			risks = s.collectEpicRisks(ctx, epicID, log)
		} else {
			log.Error("failed to parse epic id", sl.Err(parseErr), slog.String("epic_id", item.ID))
		}

		epicReportItems = append(epicReportItems, report.EpicReportData{
			EpicReportItem: item,
			Risks:          risks,
		})
	}

	reportData := &report.ReportData{
		CapacityReportResponse: *core,
		Epics:                  epicReportItems,
		Generated:              time.Now(),
	}

	return reportData, nil
}

// collectEpicRisks собирает риски эпика (включая риски его историй) и
// строит по каждому риску отчётные данные (индивидуальные оценки
// вероятности/влияния, средневзвешенную оценку и коэффициент риска) — то,
// чего нет в CapacityReportResponse (см. BuildCapacityReport), но нужно
// PDF-отчёту для карточки эпика и SVG-диаграмм (internal/report/svg.go).
// Ошибки хранилища логируются, но не прерывают формирование отчёта — как и
// в прежней реализации GetReportData.
func (s *epicService) collectEpicRisks(ctx context.Context, epicID uuid.UUID, log *slog.Logger) []report.RiskReportData {
	var risks []domain.Risk

	if stories, err := s.repo.GetStoriesByEpicID(ctx, epicID); err == nil {
		for _, story := range stories {
			if storyRisks, err := s.repo.GetRisksByEpicID(ctx, story.ID); err == nil {
				risks = append(risks, storyRisks...)
			} else {
				log.Error("failed to get story risks", sl.Err(err), slog.String("story_id", story.ID.String()))
			}
		}
	}

	epicRisks, err := s.repo.GetRisksByEpicID(ctx, epicID)
	if err != nil {
		log.Error("failed to get epic risks", sl.Err(err), slog.String("epic_id", epicID.String()))
	} else {
		risks = append(risks, epicRisks...)
	}

	reportRisks := make([]report.RiskReportData, 0, len(risks))
	for _, r := range risks {
		var probs []int
		var impacts []int
		if riskScores, err := s.repo.GetRiskScoresByRiskID(ctx, r.ID); err == nil {
			for _, rs := range riskScores {
				probs = append(probs, rs.Probability)
				impacts = append(impacts, rs.Impact)
			}
		} else {
			log.Error("failed to get risk scores", sl.Err(err), slog.String("risk_id", r.ID.String()))
		}

		var wScore float64
		coeff := 1.0
		if r.WeightedScore != nil {
			wScore = *r.WeightedScore
			coeff = scoring.RiskCoefficient(wScore)
		}

		reportRisks = append(reportRisks, report.RiskReportData{
			Description:   r.Description,
			Probabilities: probs,
			Impacts:       impacts,
			WeightedScore: wScore,
			Coefficient:   coeff,
		})
	}

	return reportRisks
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
