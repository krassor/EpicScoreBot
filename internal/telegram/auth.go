package telegram

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// isSuperAdmin checks if the message sender is in the super admins list.
// Роль superadmin остаётся config-based (BotConfig.SuperAdmins) — не связана
// с team_admins.
func (epicBot *Bot) isSuperAdmin(msg *models.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	return epicBot.isSuperAdminUsername(msg.From.Username)
}

// isSuperAdminCallback checks if the callback sender is a super admin.
func (epicBot *Bot) isSuperAdminCallback(callback *models.CallbackQuery) bool {
	if callback == nil {
		return false
	}
	return epicBot.isSuperAdminUsername(callback.From.Username)
}

func (epicBot *Bot) isSuperAdminUsername(username string) bool {
	for _, superadmin := range epicBot.cfg.BotConfig.SuperAdmins {
		if strings.EqualFold(username, superadmin) {
			return true
		}
	}
	return false
}

// isTeamAdminAny сообщает, есть ли у отправителя сообщения права
// администратора хотя бы одной команды: superadmin (config-based) ИЛИ
// team-admin (team_admins в БД) хотя бы одной команды. Грубый гейт входа во
// flow, когда конкретная команда действия ещё не выбрана (например
// handleAddEpic, handleReport, handleList — до showTeamPickerInitial/
// showEpicPickerInitial).
func (epicBot *Bot) isTeamAdminAny(ctx context.Context, msg *models.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	return epicBot.isTeamAdminAnyUsername(ctx, msg.From.Username)
}

// isTeamAdminAnyCallback — то же самое для callback-сценариев.
func (epicBot *Bot) isTeamAdminAnyCallback(ctx context.Context, callback *models.CallbackQuery) bool {
	if callback == nil {
		return false
	}
	return epicBot.isTeamAdminAnyUsername(ctx, callback.From.Username)
}

func (epicBot *Bot) isTeamAdminAnyUsername(ctx context.Context, username string) bool {
	if epicBot.isSuperAdminUsername(username) {
		return true
	}
	user, err := epicBot.userService.FindUserByTelegramID(ctx, username)
	if err != nil {
		return false
	}
	isAdmin, err := epicBot.teamAdminService.IsTeamAdminOfAny(ctx, user.ID)
	return err == nil && isAdmin
}

// isTeamAdminFor сообщает, является ли пользователь с данным telegram-username
// (в этом проекте это фактически идентификатор в users.telegram_id) team-admin
// конкретной команды, либо superadmin. Используется, когда команда действия
// уже известна: финальное подтверждение внутри callback-флоу (создание
// эпика/риска, старт скоринга и т.п.) и действия над уже существующей
// сущностью с известным TeamID.
func (epicBot *Bot) isTeamAdminFor(ctx context.Context, telegramID string, teamID uuid.UUID) bool {
	if epicBot.isSuperAdminUsername(telegramID) {
		return true
	}
	user, err := epicBot.userService.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false
	}
	isAdmin, err := epicBot.teamAdminService.IsTeamAdmin(ctx, user.ID, teamID)
	return err == nil && isAdmin
}
