package repositories

import (
	"context"

	"github.com/google/uuid"
)

// TeamAdminAuth адаптирует telegram_id-ориентированную идентификацию
// HTTP-сессии (middleware.UserSession.TelegramID) к UUID-ориентированным
// методам team_admins репозитория (Repository.IsTeamAdmin,
// Repository.IsTeamAdminOfAny, Repository.GetTeamIDsByAdminUserID),
// которыми также напрямую пользуется Telegram-бот, знающий userID.
//
// Встраивает *Repository, поэтому промоутит все его методы и таким образом
// реализует интерфейс handlers.Repository целиком; дополнительно объявляет
// три собственных метода с теми же именами, что и у Repository, но с
// telegram_id вместо userID — они "затеняют" промоутнутые одноимённые
// UUID-методы Repository на этом типе (доступны только через явное
// a.Repository.Метод(...)), что позволяет одному конкретному значению
// одновременно удовлетворять middleware.TeamAdminChecker/handlers.TeamAdminScoper
// (telegram_id) и оставаться полноценным Repository для остального кода.
type TeamAdminAuth struct {
	*Repository
}

// NewTeamAdminAuth создаёт адаптер поверх Repository для HTTP-транспорта.
func NewTeamAdminAuth(repo *Repository) *TeamAdminAuth {
	return &TeamAdminAuth{Repository: repo}
}

// IsTeamAdminOfAny сообщает, является ли пользователь (по telegram_id
// HTTP-сессии) team-admin хотя бы одной команды. Незарегистрированный
// пользователь считается не-admin (ошибка резолвинга не пробрасывается).
func (a *TeamAdminAuth) IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error) {
	user, err := a.Repository.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, nil
	}
	return a.Repository.IsTeamAdminOfAny(ctx, user.ID)
}

// IsTeamAdminOf сообщает, является ли пользователь (по telegram_id
// HTTP-сессии) team-admin конкретной команды.
func (a *TeamAdminAuth) IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error) {
	user, err := a.Repository.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, nil
	}
	return a.Repository.IsTeamAdmin(ctx, user.ID, teamID)
}

// AdminTeamIDs возвращает список ID команд, где пользователь (по telegram_id
// HTTP-сессии) назначен team-admin.
func (a *TeamAdminAuth) AdminTeamIDs(ctx context.Context, telegramID string) ([]uuid.UUID, error) {
	user, err := a.Repository.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	return a.Repository.GetTeamIDsByAdminUserID(ctx, user.ID)
}
