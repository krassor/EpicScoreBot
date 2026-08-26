package telegram

import (
	"context"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/services"

	"github.com/google/uuid"
)

// reminderRepositoryAdapter реализует notify.ReminderRepository поверх трёх
// узких сервисных интерфейсов бота (epicService/userService/riskService) —
// без изменения самих сервисов и без переноса бота на прямую работу с
// репозиторием.
type reminderRepositoryAdapter struct {
	epicService services.EpicService
	userService services.UserService
	riskService services.RiskService
}

func (a *reminderRepositoryAdapter) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return a.epicService.GetEpicByID(ctx, epicID)
}

func (a *reminderRepositoryAdapter) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	return a.userService.GetUsersByTeamID(ctx, teamID)
}

func (a *reminderRepositoryAdapter) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	return a.epicService.HasUserScoredEpic(ctx, epicID, userID)
}

func (a *reminderRepositoryAdapter) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	return a.riskService.GetUnscoredRisksByUser(ctx, userID, epicID)
}

// reminderRepository возвращает notify.ReminderRepository для этого бота.
func (epicBot *Bot) reminderRepository() *reminderRepositoryAdapter {
	return &reminderRepositoryAdapter{
		epicService: epicBot.epicService,
		userService: epicBot.userService,
		riskService: epicBot.riskService,
	}
}
