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

// sessionKeyFromCallback builds a sessionKey from callback context.
func sessionKeyFromCallback(msg *models.Message, callback *models.CallbackQuery) sessionKey {
	return sessionKey{
		ChatID:   msg.Chat.ID,
		ThreadID: msg.MessageThreadID,
		Username: callback.From.Username,
	}
}

// editOrSend edits the session message if messageID is set, otherwise sends a new message.
func (epicBot *Bot) editOrSend(ctx context.Context, msg *models.Message, messageID int, text string) {
	op := "bot.editOrSend"
	log := epicBot.log.With(slog.String("op", op))

	if messageID > 0 {
		if err := epicBot.editReply(ctx, msg.Chat.ID, messageID, text); err != nil {
			log.Error("failed to edit message, falling back to send", sl.Err(err))
			_, err := epicBot.sendReply(ctx, msg, text)
			if err != nil {
				log.Error("failed to send reply", sl.Err(err))
			}
		}
		return
	}
	if _, err := epicBot.sendReply(ctx, msg, text); err != nil {
		log.Error("failed to send reply", sl.Err(err))
	}
}

// editOrSendWithKeyboard edits the session message with keyboard if messageID is set.
func (epicBot *Bot) editOrSendWithKeyboard(
	ctx context.Context,
	msg *models.Message,
	messageID int,
	text string,
	kb *models.InlineKeyboardMarkup,
) {
	op := "bot.editOrSendWithKeyboard"
	log := epicBot.log.With(slog.String("op", op))

	if messageID > 0 {
		if err := epicBot.editWithKeyboard(ctx, msg.Chat.ID, messageID, text, kb); err != nil {
			log.Error("failed to edit message, falling back to send", sl.Err(err))
			_, err := epicBot.sendWithKeyboard(ctx, msg, text, kb)
			if err != nil {
				log.Error("failed to send reply", sl.Err(err))
			}
		}
		return
	}
	_, err := epicBot.sendWithKeyboard(ctx, msg, text, kb)
	if err != nil {
		log.Error("failed to send reply", sl.Err(err))
	}
}

// deleteAndSend deletes the session message and sends a final result message.
func (epicBot *Bot) deleteAndSend(ctx context.Context, msg *models.Message, messageID int, text string) {
	if messageID > 0 {
		if err := epicBot.deleteMessage(ctx, msg.Chat.ID, messageID); err != nil {
			epicBot.log.Error("failed to delete message", sl.Err(err))
		}
	}
	epicBot.sendReply(ctx, msg, text)
}

// handleAdmUserSelected handles when an admin picks a user from the user picker.
// data = "adm_user_<action>_<userID>"
func (epicBot *Bot) handleAdmUserSelected(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	op := "bot.handleAdmUserSelected"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("data", data),
	)

	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_user_")
	if len(rest) < 38 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	userIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	log.Debug("parsed", slog.String("user_id", userIDStr), slog.String("action", action))

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID пользователя.")
		return
	}

	user, err := epicBot.repo.GetUserByID(ctx, userID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Пользователь не найден.")
		return
	}

	log.Debug("user found", slog.Any("user tg id", user.TelegramID))

	sk := sessionKeyFromCallback(msg, callback)
	sess, _ := epicBot.sessions.get(sk)
	msgID := 0
	if sess != nil {
		msgID = sess.MessageID
	}

	switch action {
	case "assignrole":
		epicBot.showRolePicker(ctx, msg, callback, "assignrole", userID.String(), msgID)
	case "unassignrole":
		epicBot.showUserRolePicker(ctx, msg, callback, "unassignrole", userID, msgID)
	case "assignteam":
		epicBot.showTeamPickerForUser(ctx, msg, callback, "assignteam", user, msgID)
	case "removefromteam":
		epicBot.showUserTeamPicker(ctx, msg, callback, "removefromteam", user, msgID)
	case "deleteuser":
		kb := inlineKeyboard(inlineRow(
			inlineBtn("✅ Да, удалить", "adm_confirm_deleteuser_"+userID.String()),
			inlineBtn("❌ Отмена", "adm_deny_deleteuser"),
		))
		epicBot.editOrSendWithKeyboard(ctx, msg, msgID,
			fmt.Sprintf("⚠️ Удалить пользователя %s %s (@%s)?\n"+
				"Будут удалены все его роли, привязки к командам и оценки.\n"+
				"Это действие необратимо.",
				user.FirstName, user.LastName, user.TelegramID),
			kb)
	case "renameuser":
		epicBot.sessions.set(sk, &Session{
			Step:      StepRenameUserFirstName,
			ThreadID:  msg.MessageThreadID,
			Username:  callback.From.Username,
			MessageID: msgID,
			Data:      map[string]string{"pendingUserID": userID.String()},
		})
		epicBot.editOrSend(ctx, msg, msgID,
			fmt.Sprintf("✏️ Переименование пользователя %s %s (@%s).\n📝 Введите новое имя:",
				user.FirstName, user.LastName, user.TelegramID))
	case "changerate":
		epicBot.sessions.set(sk, &Session{
			Step:      StepChangeRateWeight,
			ThreadID:  msg.MessageThreadID,
			Username:  callback.From.Username,
			MessageID: msgID,
			Data:      map[string]string{"pendingUserID": userID.String()},
		})
		epicBot.editOrSend(ctx, msg, msgID,
			fmt.Sprintf("⚖️ Изменение веса пользователя %s %s (@%s).\nТекущий вес: %d\n📝 Введите новый вес (0–100):",
				user.FirstName, user.LastName, user.TelegramID, user.Weight))
	default:
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// showTeamPickerForUser shows all teams for admin to assign a user to.
func (epicBot *Bot) showTeamPickerForUser(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	action string,
	user *domain.User,
	msgID int,
) {
	op := "bot.showTeamPickerForUser"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
	)
	teams, err := epicBot.repo.GetAllTeams(ctx)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting all teams", sl.Err(err))
		}
		epicBot.editOrSend(ctx, msg, msgID, "❌ Команды не найдены.")
		return
	}
	sk := sessionKeyFromCallback(msg, callback)
	sess, _ := epicBot.sessions.get(sk)
	if sess == nil {
		sess = &Session{
			Data:     make(map[string]string),
			Username: callback.From.Username,
		}
	}
	sess.Data["pendingUserID"] = user.ID.String()
	sess.MessageID = msgID
	epicBot.sessions.set(sk, sess)

	var rows [][]models.InlineKeyboardButton
	for _, t := range teams {
		rows = append(rows, inlineRow(inlineBtn(
			"👥 "+t.Name,
			fmt.Sprintf("adm_team_%s_%s", action, t.ID.String()),
		)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	epicBot.editOrSendWithKeyboard(ctx, msg, msgID,
		fmt.Sprintf("👥 Выберите команду для пользователя %s %s:", user.FirstName, user.LastName), kb)
}

// handleAdmRoleSelected handles role selection.
// data = "adm_role_<action>_<roleID>"
func (epicBot *Bot) handleAdmRoleSelected(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_role_")
	if len(rest) < 38 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	roleIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	sk := sessionKeyFromCallback(msg, callback)
	sess, ok := epicBot.sessions.get(sk)
	if !ok || sess == nil {
		epicBot.sendReply(ctx, msg, "❌ Сессия истекла. Повторите команду.")
		return
	}
	userIDStr, hasPending := sess.Data["pendingUserID"]
	if !hasPending || userIDStr == "" {
		epicBot.sendReply(ctx, msg, "❌ Сессия истекла. Повторите команду.")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID пользователя.")
		return
	}
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID роли.")
		return
	}

	user, err := epicBot.repo.GetUserByID(ctx, userID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Пользователь не найден.")
		return
	}
	role, err := epicBot.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Роль не найдена.")
		return
	}

	msgID := sess.MessageID
	delete(sess.Data, "pendingUserID")
	epicBot.sessions.clear(sk)

	switch action {
	case "assignrole":
		if err := epicBot.repo.AssignUserRole(ctx, userID, roleID); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка назначения роли: %v", err))
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Роль «%s» назначена пользователю %s %s.", role.Name, user.FirstName, user.LastName))
	case "unassignrole":
		if err := epicBot.repo.RemoveUserRole(ctx, userID, roleID); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка снятия роли: %v", err))
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Роль «%s» снята у пользователя %s %s.", role.Name, user.FirstName, user.LastName))
	default:
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmTeamSelected handles team selection.
func (epicBot *Bot) handleAdmTeamSelected(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_team_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	lastID := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	sk := sessionKeyFromCallback(msg, callback)

	switch action {
	case "addepic":
		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID команды.")
			return
		}
		sess, _ := epicBot.sessions.get(sk)
		msgID := 0
		if sess != nil {
			msgID = sess.MessageID
		}
		epicBot.sessions.set(sk, &Session{
			Step:      StepAddEpicNumber,
			ThreadID:  msg.MessageThreadID,
			Username:  callback.From.Username,
			MessageID: msgID,
			Data:      map[string]string{"teamID": teamID.String()},
		})
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите номер эпика (например, EP-1):")

	case "assignteam", "removefromteam":
		sess, ok := epicBot.sessions.get(sk)
		if !ok || sess == nil {
			epicBot.sendReply(ctx, msg, "❌ Сессия истекла. Повторите команду.")
			return
		}
		userIDStr, hasPending := sess.Data["pendingUserID"]
		if !hasPending || userIDStr == "" {
			epicBot.sendReply(ctx, msg, "❌ Сессия истекла. Повторите команду.")
			return
		}

		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID команды.")
			return
		}
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID пользователя.")
			return
		}

		user, err := epicBot.repo.GetUserByID(ctx, userID)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Пользователь не найден.")
			return
		}
		team, err := epicBot.repo.GetTeamByID(ctx, teamID)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Команда не найдена.")
			return
		}

		msgID := sess.MessageID
		delete(sess.Data, "pendingUserID")
		epicBot.sessions.clear(sk)

		switch action {
		case "assignteam":
			teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
			if err != nil {
				epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка получения команд пользователя.")
				return
			}
			for _, t := range teams {
				if t.ID == teamID {
					epicBot.deleteAndSend(ctx, msg, msgID, "❌ Пользователь уже состоит в этой команде.")
					return
				}
			}
			if err := epicBot.repo.AssignUserTeam(ctx, userID, teamID); err != nil {
				epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка добавления в команду.")
				return
			}
			epicBot.deleteAndSend(ctx, msg, msgID,
				fmt.Sprintf("✅ Пользователь %s %s добавлен в команду «%s».",
					user.FirstName, user.LastName, team.Name))
		case "removefromteam":
			if err := epicBot.repo.RemoveUserTeam(ctx, userID, teamID); err != nil {
				epicBot.deleteAndSend(ctx, msg, msgID,
					fmt.Sprintf("❌ Ошибка удаления из команды: %v", err))
				return
			}
			epicBot.deleteAndSend(ctx, msg, msgID,
				fmt.Sprintf("✅ Пользователь %s %s удалён из команды «%s».",
					user.FirstName, user.LastName, team.Name))
		}

	case "list":
		teamID, err := uuid.Parse(lastID)
		if err != nil {
			epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID команды.")
			return
		}
		sess, _ := epicBot.sessions.get(sk)
		msgID := 0
		if sess != nil {
			msgID = sess.MessageID
		}
		epicBot.sessions.clear(sk)

		users, err := epicBot.repo.GetUsersByTeamID(ctx, teamID)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка получения пользователей команды.")
			return
		}
		var sb strings.Builder
		for _, user := range users {
			role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
			roleName := "—"
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&sb, "@%s %s %s - %s\n", user.TelegramID, user.FirstName, user.LastName, roleName)
		}
		if sb.Len() == 0 {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ В команде нет пользователей.")
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID, sb.String())

	default:
		epicBot.sendReply(ctx, msg, "❌ Неизвестное действие.")
	}
}

// handleAdmEpicSelected handles epic selection.
// data = "adm_epic_<action>_<epicID>"
func (epicBot *Bot) handleAdmEpicSelected(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_epic_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	epicIDStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID эпика.")
		return
	}

	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}

	sk := sessionKeyFromCallback(msg, callback)
	sess, _ := epicBot.sessions.get(sk)
	msgID := 0
	if sess != nil {
		msgID = sess.MessageID
	}

	switch action {
	case "startscore":
		epicBot.sessions.clear(sk)
		epicBot.deleteAndSendStartScore(ctx, msg, epicID, msgID)

	case "results":
		epicBot.sessions.clear(sk)
		epicBot.showEpicResultsAndClean(ctx, msg, epicID, msgID)

	case "epicstatus":
		epicBot.sessions.clear(sk)
		epicBot.showEpicStatusReportAndClean(ctx, msg, epicID, msgID)

	case "addrisk":
		epicBot.sessions.set(sk, &Session{
			Step:      StepAddRiskDesc,
			ThreadID:  msg.MessageThreadID,
			Username:  callback.From.Username,
			MessageID: msgID,
			Data:      map[string]string{"epicID": epicID.String()},
		})
		epicBot.editOrSend(ctx, msg, msgID,
			fmt.Sprintf("📝 Введите описание риска для эпика #%s «%s»:", epic.Number, epic.Name))

	case "deleteepic":
		kb := inlineKeyboard(inlineRow(
			inlineBtn("✅ Да, удалить", "adm_confirm_deleteepic_"+epicID.String()),
			inlineBtn("❌ Отмена", "adm_deny_deleteepic"),
		))
		epicBot.editOrSendWithKeyboard(ctx, msg, msgID,
			fmt.Sprintf("⚠️ Удалить эпик #%s «%s» и все его риски и оценки?\nЭто действие необратимо.",
				epic.Number, epic.Name),
			kb)

	case "deleterisk":
		epicBot.showRiskPickerEditing(ctx, msg, callback, "deleterisk", epic, msgID)

	default:
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmRiskSelected handles risk selection for deleterisk.
// data = "adm_risk_<action>_<epicID>_<riskID>"
func (epicBot *Bot) handleAdmRiskSelected(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_risk_")
	if len(rest) < 74 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	riskIDStr := rest[len(rest)-36:]
	rest2 := rest[:len(rest)-37]
	epicIDStr := rest2[len(rest2)-36:]
	action := rest2[:len(rest2)-37]

	if _, err := uuid.Parse(epicIDStr); err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID эпика.")
		return
	}
	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID риска.")
		return
	}

	risk, err := epicBot.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Риск не найден.")
		return
	}

	sk := sessionKeyFromCallback(msg, callback)
	sess, _ := epicBot.sessions.get(sk)
	msgID := 0
	if sess != nil {
		msgID = sess.MessageID
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
		epicBot.editOrSendWithKeyboard(ctx, msg, msgID,
			fmt.Sprintf("⚠️ Удалить риск «%s»?\nЭто действие необратимо.", desc),
			kb)
	default:
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Неизвестное действие: %s", action))
	}
}

// handleAdmConfirm handles confirmed destructive actions.
// data = "adm_confirm_<action>_<id>"
func (epicBot *Bot) handleAdmConfirm(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	data string,
) {
	if !epicBot.isSuperAdminCallback(callback) {
		epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return
	}
	rest := strings.TrimPrefix(data, "adm_confirm_")
	if len(rest) < 37 {
		epicBot.sendReply(ctx, msg, "❌ Некорректные данные.")
		return
	}
	idStr := rest[len(rest)-36:]
	action := rest[:len(rest)-37]

	id, err := uuid.Parse(idStr)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Ошибка парсинга ID.")
		return
	}

	sk := sessionKeyFromCallback(msg, callback)
	sess, _ := epicBot.sessions.get(sk)
	msgID := 0
	if sess != nil {
		msgID = sess.MessageID
	}
	epicBot.sessions.clear(sk)

	switch action {
	case "deleteepic":
		epic, _ := epicBot.repo.GetEpicByID(ctx, id)
		if err := epicBot.repo.DeleteEpic(ctx, id); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка удаления эпика: %v", err))
			return
		}
		epicNum := id.String()
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("🗑️ Эпик #%s удалён.", epicNum))

	case "deleterisk":
		risk, _ := epicBot.repo.GetRiskByID(ctx, id)
		if err := epicBot.repo.DeleteRisk(ctx, id); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка удаления риска: %v", err))
			return
		}
		desc := id.String()
		if risk != nil {
			desc = risk.Description
			if len([]rune(desc)) > 60 {
				desc = string([]rune(desc)[:57]) + "..."
			}
		}
		epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("🗑️ Риск «%s» удалён.", desc))

	case "deleteuser":
		user, _ := epicBot.repo.GetUserByID(ctx, id)
		if err := epicBot.repo.DeleteUser(ctx, id); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка удаления пользователя: %v", err))
			return
		}
		userLabel := id.String()
		if user != nil {
			userLabel = fmt.Sprintf("%s %s (@%s)", user.FirstName, user.LastName, user.TelegramID)
		}
		epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("🗑️ Пользователь %s удалён.", userLabel))

	default:
		epicBot.sendReply(ctx, msg, "❌ Неизвестное действие.")
	}
}

// showRiskPickerEditing sends risks picker editing the existing message.
func (epicBot *Bot) showRiskPickerEditing(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	action string,
	epic *domain.Epic,
	msgID int,
) {
	op := "bot.showRiskPickerEditing"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
		slog.String("epic_id", epic.ID.String()),
	)
	risks, err := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil || len(risks) == 0 {
		if err != nil {
			log.Error("error getting risks by epic id", sl.Err(err))
		}
		epicBot.editOrSend(ctx, msg, msgID, "❌ Риски не найдены для выбранного эпика.")
		return
	}
	var rows [][]models.InlineKeyboardButton
	for _, r := range risks {
		desc := r.Description
		if len([]rune(desc)) > 50 {
			desc = string([]rune(desc)[:47]) + "..."
		}
		data := fmt.Sprintf("adm_risk_%s_%s_%s", action, epic.ID.String(), r.ID.String())
		rows = append(rows, inlineRow(inlineBtn("⚠️ "+desc, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	epicBot.editOrSendWithKeyboard(ctx, msg, msgID,
		fmt.Sprintf("⚠️ Выберите риск для эпика #%s «%s»:", epic.Number, epic.Name), kb)
}

// deleteAndSendStartScore deletes the picker message and runs startscore logic.
func (epicBot *Bot) deleteAndSendStartScore(ctx context.Context, msg *models.Message, epicID uuid.UUID, msgID int) {
	if msgID > 0 {
		epicBot.deleteMessage(ctx, msg.Chat.ID, msgID)
	}
	epicBot.execStartScore(ctx, msg, epicID)
}

// showEpicResultsAndClean deletes picker message and shows results.
func (epicBot *Bot) showEpicResultsAndClean(ctx context.Context, msg *models.Message, epicID uuid.UUID, msgID int) {
	if msgID > 0 {
		epicBot.deleteMessage(ctx, msg.Chat.ID, msgID)
	}
	epicBot.showEpicResults(ctx, msg, epicID)
}

// showEpicStatusReportAndClean deletes picker message and shows status.
func (epicBot *Bot) showEpicStatusReportAndClean(ctx context.Context, msg *models.Message, epicID uuid.UUID, msgID int) {
	if msgID > 0 {
		epicBot.deleteMessage(ctx, msg.Chat.ID, msgID)
	}
	epicBot.showEpicStatusReport(ctx, msg, epicID)
}
