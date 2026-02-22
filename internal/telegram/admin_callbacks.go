package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"EpicScoreBot/internal/models/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
func (bot *Bot) handleAdmUserSelected(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	op := "bot.handleAdmUserSelected"
	log := bot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("data", data),
	)

	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	// strip prefix "adm_user_"
	rest := strings.TrimPrefix(data, "adm_user_")
	// rest = "<action>_<userID>"
	// action may itself contain '_' so find the last segment (UUID is fixed length)
	// UUID is always 36 chars; rest ends in "_<uuid>"
	if len(rest) < 38 { // minimum: 1 char action + "_" + 36 char uuid
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	userIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37] // cut trailing "_<uuid>"

	log.Debug("parsed", slog.String("user_id", userIDStr), slog.String("action", action))

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID пользователя.")
		return
	}

	user, err := bot.repo.GetUserByID(ctx, userID)
	if err != nil {
		bot.sendReply(chatID, "❌ Пользователь не найден.")
		return
	}

	log.Debug("user found", slog.Any("user tg id", user.TelegramID))

	switch action {
	case "assignrole":
		bot.showRolePicker(ctx, chatID, "assignrole", userID.String())
	case "unassignrole":
		bot.showUserRolePicker(ctx, chatID, "unassignrole", userID)
	case "assignteam":
		// show team picker; embed userID in callback
		bot.showTeamPickerForUser(ctx, chatID, "assignteam", user)
	case "removefromteam":
		bot.showUserTeamPicker(ctx, chatID, "removefromteam", user)
	default:
		bot.sendReply(chatID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// showTeamPickerForUser shows all teams for admin to assign a user to.
// user.ID is stored in the session; callback data carries only action + teamID
// to stay within Telegram's 64-byte callback-data limit.
func (bot *Bot) showTeamPickerForUser(ctx context.Context, chatID int64, action string, user *domain.User) error {
	teams, err := bot.repo.GetAllTeams(ctx)
	if err != nil || len(teams) == 0 {
		return bot.sendReply(chatID, "❌ Команды не найдены.")
	}
	// Persist userID in session so the callback handler can retrieve it.
	sess, _ := bot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = user.ID.String()
	bot.sessions.set(chatID, sess)

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range teams {
		// adm_team_assignteam_<teamID> — fits well under 64 bytes
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 "+t.Name, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("👥 Выберите команду для пользователя %s %s:", user.FirstName, user.LastName))
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// handleAdmRoleSelected handles role selection.
// data = "adm_role_<action>_<roleID>"
// userID is read from session key "pendingUserID" (set by the role picker).
func (bot *Bot) handleAdmRoleSelected(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_role_")
	// rest = "<action>_<roleID 36>"
	if len(rest) < 38 { // minimum: 1 char action + "_" + 36 char uuid
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	roleIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37] // cut trailing "_<uuid>"

	// Retrieve the target userID stored by the picker in the session.
	sess, ok := bot.sessions.get(chatID)
	if !ok || sess == nil {
		bot.sendReply(chatID, "❌ Сессия истекла. Повторите команду.")
		return
	}
	userIDStr, hasPending := sess.Data["pendingUserID"]
	if !hasPending || userIDStr == "" {
		bot.sendReply(chatID, "❌ Сессия истекла. Повторите команду.")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID пользователя.")
		return
	}
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID роли.")
		return
	}

	user, err := bot.repo.GetUserByID(ctx, userID)
	if err != nil {
		bot.sendReply(chatID, "❌ Пользователь не найден.")
		return
	}
	role, err := bot.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		bot.sendReply(chatID, "❌ Роль не найдена.")
		return
	}

	// Clean up session pendingUserID after successful lookup.
	delete(sess.Data, "pendingUserID")
	bot.sessions.set(chatID, sess)

	switch action {
	case "assignrole":
		if err := bot.repo.AssignUserRole(ctx, userID, roleID); err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка назначения роли: %v", err))
			return
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Роль «%s» назначена пользователю %s %s.",
				role.Name, user.FirstName, user.LastName))
	case "unassignrole":
		if err := bot.repo.RemoveUserRole(ctx, userID, roleID); err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка снятия роли: %v", err))
			return
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Роль «%s» снята у пользователя %s %s.",
				role.Name, user.FirstName, user.LastName))
	default:
		bot.sendReply(chatID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmTeamSelected handles team selection.
// data formats:
//
//	adm_team_addepic_<teamID>             — addepic: team picked, start session
//	adm_team_assignteam_<teamID>          — assign user (ID in session) to team
//	adm_team_removefromteam_<teamID>      — remove user (ID in session) from team
func (bot *Bot) handleAdmTeamSelected(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_team_")
	// Last segment is always a UUID (36 chars)
	if len(rest) < 37 {
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	lastID := rest[len(rest)-36:]
	action := rest[:len(rest)-37] // action name only (no embedded userID)

	switch action {
	case "addepic":
		// Start addepic session: teamID picked
		teamID, err := uuid.Parse(lastID)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка парсинга ID команды.")
			return
		}
		bot.sessions.set(chatID, &Session{
			Step: StepAddEpicNumber,
			Data: map[string]string{"teamID": teamID.String()},
		})
		bot.sendReply(chatID, "📝 Введите номер эпика (например, EP-1):")

	case "assignteam", "removefromteam":
		// Retrieve the target userID stored in the session by the team picker.
		sess, ok := bot.sessions.get(chatID)
		if !ok || sess == nil {
			bot.sendReply(chatID, "❌ Сессия истекла. Повторите команду.")
			return
		}
		userIDStr, hasPending := sess.Data["pendingUserID"]
		if !hasPending || userIDStr == "" {
			bot.sendReply(chatID, "❌ Сессия истекла. Повторите команду.")
			return
		}

		teamID, err := uuid.Parse(lastID)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка парсинга ID команды.")
			return
		}
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка парсинга ID пользователя.")
			return
		}
		user, err := bot.repo.GetUserByID(ctx, userID)
		if err != nil {
			bot.sendReply(chatID, "❌ Пользователь не найден.")
			return
		}
		team, err := bot.repo.GetTeamByID(ctx, teamID)
		if err != nil {
			bot.sendReply(chatID, "❌ Команда не найдена.")
			return
		}

		// Clean up session pendingUserID after successful lookup.
		delete(sess.Data, "pendingUserID")
		bot.sessions.set(chatID, sess)

		switch action {
		case "assignteam":
			if err := bot.repo.AssignUserTeam(ctx, userID, teamID); err != nil {
				bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка добавления в команду: %v", err))
				return
			}
			bot.sendReply(chatID,
				fmt.Sprintf("✅ Пользователь %s %s добавлен в команду «%s».",
					user.FirstName, user.LastName, team.Name))
		case "removefromteam":
			if err := bot.repo.RemoveUserTeam(ctx, userID, teamID); err != nil {
				bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка удаления из команды: %v", err))
				return
			}
			bot.sendReply(chatID,
				fmt.Sprintf("✅ Пользователь %s %s удалён из команды «%s».",
					user.FirstName, user.LastName, team.Name))
		}
	default:
		bot.sendReply(chatID, "❌ Неизвестное действие.")
	}
}

// handleAdmEpicSelected handles epic selection.
// data = "adm_epic_<action>_<epicID>"
func (bot *Bot) handleAdmEpicSelected(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_epic_")
	if len(rest) < 37 {
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	epicIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID эпика.")
		return
	}

	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		bot.sendReply(chatID, "❌ Эпик не найден.")
		return
	}

	switch action {
	case "startscore":
		bot.execStartScore(ctx, chatID, epicID)

	case "results":
		bot.showEpicResults(ctx, chatID, epicID)

	case "epicstatus":
		bot.showEpicStatusReport(ctx, chatID, epicID)

	case "addrisk":
		bot.sessions.set(chatID, &Session{
			Step: StepAddRiskDesc,
			Data: map[string]string{"epicID": epicID.String()},
		})
		bot.sendReply(chatID,
			fmt.Sprintf("📝 Введите описание риска для эпика #%s «%s»:", epic.Number, epic.Name))

	case "deleteepic":
		// Show confirmation
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", "adm_confirm_deleteepic_"+epicID.String()),
				tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_deny_deleteepic"),
			),
		)
		m := tgbotapi.NewMessage(chatID,
			fmt.Sprintf("⚠️ Удалить эпик #%s «%s» и все его риски и оценки?\nЭто действие необратимо.",
				epic.Number, epic.Name))
		m.ReplyMarkup = kb
		bot.tgbot.Send(m)

	case "deleterisk":
		// Need to pick a risk next
		bot.showRiskPicker(ctx, chatID, "deleterisk", epic)

	default:
		bot.sendReply(chatID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmRiskSelected handles risk selection for deleterisk.
// data = "adm_risk_<action>_<epicID>_<riskID>"
func (bot *Bot) handleAdmRiskSelected(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_risk_")
	// rest = "<action>_<epicID 36>_<riskID 36>"
	if len(rest) < 74 {
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	riskIDStr := rest[len(rest)-36:]
	rest2 := rest[:len(rest)-37]
	epicIDStr := rest2[len(rest2)-36:]
	action := rest2[:len(rest2)-37]

	_, err := uuid.Parse(epicIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID эпика.")
		return
	}
	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID риска.")
		return
	}

	risk, err := bot.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		bot.sendReply(chatID, "❌ Риск не найден.")
		return
	}

	switch action {
	case "deleterisk":
		desc := risk.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", "adm_confirm_deleterisk_"+riskID.String()),
				tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_deny_deleterisk"),
			),
		)
		m := tgbotapi.NewMessage(chatID,
			fmt.Sprintf("⚠️ Удалить риск «%s»?\nЭто действие необратимо.", desc))
		m.ReplyMarkup = kb
		bot.tgbot.Send(m)
	default:
		bot.sendReply(chatID, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmConfirm handles confirmed destructive actions.
// data = "adm_confirm_<action>_<id>"
func (bot *Bot) handleAdmConfirm(ctx context.Context, chatID int64, callback *tgbotapi.CallbackQuery, data string) {
	if !bot.isAdminCallback(callback) {
		bot.sendReply(chatID, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_confirm_")
	// rest = "<action>_<uuid>"
	if len(rest) < 37 {
		bot.sendReply(chatID, "❌ Некорректные данные.")
		return
	}
	idStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	id, err := uuid.Parse(idStr)
	if err != nil {
		bot.sendReply(chatID, "❌ Ошибка парсинга ID.")
		return
	}

	switch action {
	case "deleteepic":
		epic, _ := bot.repo.GetEpicByID(ctx, id)
		if err := bot.repo.DeleteEpic(ctx, id); err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка удаления эпика: %v", err))
			return
		}
		epicNum := id.String()
		if epic != nil {
			epicNum = epic.Number
		}
		bot.sendReply(chatID, fmt.Sprintf("🗑️ Эпик #%s удалён.", epicNum))

	case "deleterisk":
		risk, _ := bot.repo.GetRiskByID(ctx, id)
		if err := bot.repo.DeleteRisk(ctx, id); err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка удаления риска: %v", err))
			return
		}
		desc := id.String()
		if risk != nil {
			desc = risk.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
		}
		bot.sendReply(chatID, fmt.Sprintf("🗑️ Риск «%s» удалён.", desc))

	default:
		bot.sendReply(chatID, "❌ Неизвестное действие.")
	}
}
