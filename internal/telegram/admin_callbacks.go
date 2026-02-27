package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// ─── Callback data format ──────────────────────────────────────────────────
//
// adm_user_<action>_<userID>
// adm_role_<action>_<roleID>        (userID stored in session as pendingUserID)
// adm_team_<action>_<...>
//   assignteam flow:   adm_team_assignteam_<teamID>  (userID in session)
//   addepic    flow:   adm_team_addepic_<teamID>
//   removefromteam:    adm_team_removefromteam_<teamID> (userID in session)
// adm_epic_<action>_<epicID>
// adm_risk_<action>_<epicID>_<riskID>
// adm_confirm_<action>_<id>
// adm_deny_*

// handleAdmUserSelected handles when an admin picks a user from the user picker.
// data = "adm_user_<action>_<userID>"
func (epicBot *Bot) handleAdmUserSelected(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	op := "bot.handleAdmUserSelected"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("data", data),
	)

	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_user_")
	if len(rest) < 38 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	userIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	log.Debug("parsed", slog.String("user_id", userIDStr), slog.String("action", action))

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID пользователя.")
		return
	}

	user, err := epicBot.repo.GetUserByID(ctx, userID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден.")
		return
	}

	log.Debug("user found", slog.Any("user tg id", user.TelegramID))

	switch action {
	case "assignrole":
		epicBot.showRolePicker(ctx, chatID, threadID, "assignrole", userID.String())
	case "unassignrole":
		epicBot.showUserRolePicker(ctx, chatID, threadID, "unassignrole", userID)
	case "assignteam":
		epicBot.showTeamPickerForUser(ctx, chatID, threadID, "assignteam", user)
	case "removefromteam":
		epicBot.showUserTeamPicker(ctx, chatID, threadID, "removefromteam", user)
	case "deleteuser":
		kb := inlineKeyboard(inlineRow(
			inlineBtn("✅ Да, удалить", "adm_confirm_deleteuser_"+userID.String()),
			inlineBtn("❌ Отмена", "adm_deny_deleteuser"),
		))
		epicBot.sendWithKeyboard(ctx, chatID, threadID,
			fmt.Sprintf("⚠️ Удалить пользователя %s %s (@%s)?\n"+
				"Будут удалены все его роли, привязки к командам и оценки.\n"+
				"Это действие необратимо.",
				user.FirstName, user.LastName, user.TelegramID),
			kb)
	case "renameuser":
		epicBot.sessions.set(chatID, &Session{
			Step:     StepRenameUserFirstName,
			ThreadID: threadID,
			Data:     map[string]string{"pendingUserID": userID.String()},
		})
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✏️ Переименование пользователя %s %s (@%s).\n📝 Введите новое имя:",
				user.FirstName, user.LastName, user.TelegramID))
	case "changerate":
		epicBot.sessions.set(chatID, &Session{
			Step:     StepChangeRateWeight,
			ThreadID: threadID,
			Data:     map[string]string{"pendingUserID": userID.String()},
		})
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("⚖️ Изменение веса пользователя %s %s (@%s).\nТекущий вес: %d\n📝 Введите новый вес (0–100):",
				user.FirstName, user.LastName, user.TelegramID, user.Weight))
	default:
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// showTeamPickerForUser shows all teams for admin to assign a user to.
func (epicBot *Bot) showTeamPickerForUser(
	ctx context.Context,
	chatID int64,
	threadID int,
	action string,
	user *domain.User,
) error {
	op := "bot.showTeamPickerForUser"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
	)
	teams, err := epicBot.repo.GetAllTeams(ctx)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting all teams", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Команды не найдены.")
	}
	sess, _ := epicBot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = user.ID.String()
	epicBot.sessions.set(chatID, sess)

	var rows [][]models.InlineKeyboardButton
	for _, t := range teams {
		rows = append(rows, inlineRow(inlineBtn(
			"👥 "+t.Name,
			fmt.Sprintf("adm_team_%s_%s", action, t.ID.String()),
		)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("👥 Выберите команду для пользователя %s %s:", user.FirstName, user.LastName), kb)
}

// handleAdmRoleSelected handles role selection.
// data = "adm_role_<action>_<roleID>"
func (epicBot *Bot) handleAdmRoleSelected(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_role_")
	if len(rest) < 38 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	roleIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	sess, ok := epicBot.sessions.get(chatID)
	if !ok || sess == nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Сессия истекла. Повторите команду.")
		return
	}
	userIDStr, hasPending := sess.Data["pendingUserID"]
	if !hasPending || userIDStr == "" {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Сессия истекла. Повторите команду.")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID пользователя.")
		return
	}
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID роли.")
		return
	}

	user, err := epicBot.repo.GetUserByID(ctx, userID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден.")
		return
	}
	role, err := epicBot.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Роль не найдена.")
		return
	}

	delete(sess.Data, "pendingUserID")
	epicBot.sessions.set(chatID, sess)

	switch action {
	case "assignrole":
		if err := epicBot.repo.AssignUserRole(ctx, userID, roleID); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка назначения роли: %v", err))
			return
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Роль «%s» назначена пользователю %s %s.", role.Name, user.FirstName, user.LastName))
	case "unassignrole":
		if err := epicBot.repo.RemoveUserRole(ctx, userID, roleID); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка снятия роли: %v", err))
			return
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Роль «%s» снята у пользователя %s %s.", role.Name, user.FirstName, user.LastName))
	default:
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmTeamSelected handles team selection.
func (epicBot *Bot) handleAdmTeamSelected(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_team_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	lastID := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	switch action {
	case "addepic":
		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID команды.")
			return
		}
		epicBot.sessions.set(chatID, &Session{
			Step:     StepAddEpicNumber,
			ThreadID: threadID,
			Data:     map[string]string{"teamID": teamID.String()},
		})
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите номер эпика (например, EP-1):")

	case "assignteam", "removefromteam":
		sess, ok := epicBot.sessions.get(chatID)
		if !ok || sess == nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Сессия истекла. Повторите команду.")
			return
		}
		userIDStr, hasPending := sess.Data["pendingUserID"]
		if !hasPending || userIDStr == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Сессия истекла. Повторите команду.")
			return
		}

		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID команды.")
			return
		}
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID пользователя.")
			return
		}

		user, err := epicBot.repo.GetUserByID(ctx, userID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден.")
			return
		}
		team, err := epicBot.repo.GetTeamByID(ctx, teamID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Команда не найдена.")
			return
		}

		delete(sess.Data, "pendingUserID")
		epicBot.sessions.set(chatID, sess)

		switch action {
		case "assignteam":
			teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
			if err != nil {
				epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка получения команд пользователя.")
				return
			}
			for _, t := range teams {
				if t.ID == teamID {
					epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь уже состоит в этой команде.")
					return
				}
			}
			if err := epicBot.repo.AssignUserTeam(ctx, userID, teamID); err != nil {
				epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка добавления в команду.")
				return
			}
			epicBot.sendReply(ctx, chatID, threadID,
				fmt.Sprintf("✅ Пользователь %s %s добавлен в команду «%s».",
					user.FirstName, user.LastName, team.Name))
		case "removefromteam":
			if err := epicBot.repo.RemoveUserTeam(ctx, userID, teamID); err != nil {
				epicBot.sendReply(ctx, chatID, threadID,
					fmt.Sprintf("❌ Ошибка удаления из команды: %v", err))
				return
			}
			epicBot.sendReply(ctx, chatID, threadID,
				fmt.Sprintf("✅ Пользователь %s %s удалён из команды «%s».",
					user.FirstName, user.LastName, team.Name))
		}

	case "list":
		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID команды.")
			return
		}
		users, err := epicBot.repo.GetUsersByTeamID(ctx, teamID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка получения пользователей команды.")
			return
		}
		var msg strings.Builder
		for _, user := range users {
			role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
			roleName := "—"
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&msg, "@%s %s %s - %s\n", user.TelegramID, user.FirstName, user.LastName, roleName)
		}
		if msg.Len() == 0 {
			epicBot.sendReply(ctx, chatID, threadID, "❌ В команде нет пользователей.")
			return
		}
		epicBot.sendReply(ctx, chatID, threadID, msg.String())

	default:
		epicBot.sendReply(ctx, chatID, threadID, "❌ Неизвестное действие.")
	}
}

// handleAdmEpicSelected handles epic selection.
// data = "adm_epic_<action>_<epicID>"
func (epicBot *Bot) handleAdmEpicSelected(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_epic_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	epicIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID эпика.")
		return
	}

	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик не найден.")
		return
	}

	switch action {
	case "startscore":
		epicBot.execStartScore(ctx, chatID, threadID, epicID)

	case "results":
		epicBot.showEpicResults(ctx, chatID, threadID, epicID)

	case "epicstatus":
		epicBot.showEpicStatusReport(ctx, chatID, threadID, epicID)

	case "addrisk":
		epicBot.sessions.set(chatID, &Session{
			Step:     StepAddRiskDesc,
			ThreadID: threadID,
			Data:     map[string]string{"epicID": epicID.String()},
		})
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("📝 Введите описание риска для эпика #%s «%s»:", epic.Number, epic.Name))

	case "deleteepic":
		kb := inlineKeyboard(inlineRow(
			inlineBtn("✅ Да, удалить", "adm_confirm_deleteepic_"+epicID.String()),
			inlineBtn("❌ Отмена", "adm_deny_deleteepic"),
		))
		epicBot.sendWithKeyboard(ctx, chatID, threadID,
			fmt.Sprintf("⚠️ Удалить эпик #%s «%s» и все его риски и оценки?\nЭто действие необратимо.",
				epic.Number, epic.Name),
			kb)

	case "deleterisk":
		epicBot.showRiskPicker(ctx, chatID, threadID, "deleterisk", epic)

	default:
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmRiskSelected handles risk selection for deleterisk.
// data = "adm_risk_<action>_<epicID>_<riskID>"
func (epicBot *Bot) handleAdmRiskSelected(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_risk_")
	if len(rest) < 74 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	riskIDStr := rest[len(rest)-36:]
	rest2 := rest[:len(rest)-37]
	epicIDStr := rest2[len(rest2)-36:]
	action := rest2[:len(rest2)-37]

	if _, err := uuid.Parse(epicIDStr); err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID эпика.")
		return
	}
	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID риска.")
		return
	}

	risk, err := epicBot.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Риск не найден.")
		return
	}

	switch action {
	case "deleterisk":
		desc := risk.Description
		if len([]rune(desc)) > 60 {
			desc = string([]rune(desc)[:57]) + "..."
		}
		kb := inlineKeyboard(inlineRow(
			inlineBtn("✅ Да, удалить", "adm_confirm_deleterisk_"+riskID.String()),
			inlineBtn("❌ Отмена", "adm_deny_deleterisk"),
		))
		epicBot.sendWithKeyboard(ctx, chatID, threadID,
			fmt.Sprintf("⚠️ Удалить риск «%s»?\nЭто действие необратимо.", desc),
			kb)
	default:
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmConfirm handles confirmed destructive actions.
// data = "adm_confirm_<action>_<id>"
func (epicBot *Bot) handleAdmConfirm(
	ctx context.Context,
	chatID int64,
	threadID int,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isSuperAdminCallback(callback) {
		epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_confirm_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные.")
		return
	}
	idStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	id, err := uuid.Parse(idStr)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID.")
		return
	}

	switch action {
	case "deleteepic":
		epic, _ := epicBot.repo.GetEpicByID(ctx, id)
		if err := epicBot.repo.DeleteEpic(ctx, id); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка удаления эпика: %v", err))
			return
		}
		epicNum := id.String()
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("🗑️ Эпик #%s удалён.", epicNum))

	case "deleterisk":
		risk, _ := epicBot.repo.GetRiskByID(ctx, id)
		if err := epicBot.repo.DeleteRisk(ctx, id); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка удаления риска: %v", err))
			return
		}
		desc := id.String()
		if risk != nil {
			desc = risk.Description
			if len([]rune(desc)) > 60 {
				desc = string([]rune(desc)[:57]) + "..."
			}
		}
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("🗑️ Риск «%s» удалён.", desc))

	case "deleteuser":
		user, _ := epicBot.repo.GetUserByID(ctx, id)
		if err := epicBot.repo.DeleteUser(ctx, id); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка удаления пользователя: %v", err))
			return
		}
		userLabel := id.String()
		if user != nil {
			userLabel = fmt.Sprintf("%s %s (@%s)", user.FirstName, user.LastName, user.TelegramID)
		}
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("🗑️ Пользователь %s удалён.", userLabel))

	default:
		epicBot.sendReply(ctx, chatID, threadID, "❌ Неизвестное действие.")
	}
}
