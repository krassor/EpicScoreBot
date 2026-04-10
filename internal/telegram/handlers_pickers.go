package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// ─── Inline picker helpers (Initial — send first message, save ID) ─────────

// showUserPickerInitial sends an inline keyboard with all registered users.
// The sent message ID is stored in a new session for editing later.
func (epicBot *Bot) showUserPickerInitial(ctx context.Context, msg *models.Message, action string) error {
	op := "bot.showUserPickerInitial"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
	)
	users, err := epicBot.repo.GetAllUsers(ctx)
	if err != nil || len(users) == 0 {
		if err != nil {
			log.Error("error getting all users", sl.Err(err))
		}
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Пользователи не найдены.")
		return retErr
	}
	var rows [][]models.InlineKeyboardButton
	for _, u := range users {
		label := fmt.Sprintf("👤 %s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
		data := fmt.Sprintf("adm_user_%s_%s", action, u.ID.String())
		rows = append(rows, inlineRow(inlineBtn(label, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)

	sent, err := epicBot.sendWithKeyboard(ctx, msg, "👤 Выберите пользователя:", kb)
	if err != nil {
		return err
	}
	// Save session with the message ID for future editing.
	sk := sessionKey{ChatID: msg.Chat.ID, ThreadID: msg.MessageThreadID, Username: msg.From.Username}
	sess := &Session{
		ThreadID: msg.MessageThreadID,
		Username: msg.From.Username,
		Data:     make(map[string]string),
	}
	if sent != nil {
		sess.MessageID = sent.ID
	}
	epicBot.sessions.set(sk, sess)
	return nil
}

// showTeamPickerInitial sends an inline keyboard with all teams.
func (epicBot *Bot) showTeamPickerInitial(ctx context.Context, msg *models.Message, action string) error {
	op := "bot.showTeamPickerInitial"
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
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Команды не найдены.")
		return retErr
	}
	var rows [][]models.InlineKeyboardButton
	for _, t := range teams {
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, inlineRow(inlineBtn("👥 "+t.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)

	sent, err := epicBot.sendWithKeyboard(ctx, msg, "👥 Выберите команду:", kb)
	if err != nil {
		return err
	}
	sk := sessionKey{ChatID: msg.Chat.ID, ThreadID: msg.MessageThreadID, Username: msg.From.Username}
	sess := &Session{
		ThreadID: msg.MessageThreadID,
		Username: msg.From.Username,
		Data:     make(map[string]string),
	}
	if sent != nil {
		sess.MessageID = sent.ID
	}
	epicBot.sessions.set(sk, sess)
	return nil
}

// showEpicPickerInitial sends an inline keyboard with epics, optionally filtered by status.
func (epicBot *Bot) showEpicPickerInitial(ctx context.Context, msg *models.Message, action, statusFilter string) error {
	op := "bot.showEpicPickerInitial"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
		slog.String("status_filter", statusFilter),
	)
	var epics []domain.Epic
	var err error
	if statusFilter != "" {
		epics, err = epicBot.repo.GetEpicsByStatus(ctx, domain.Status(statusFilter))
	} else {
		epics, err = epicBot.repo.GetAllEpics(ctx)
	}
	if err != nil || len(epics) == 0 {
		if err != nil {
			log.Error("error getting epics by status", sl.Err(err))
		}
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Эпики не найдены.")
		return retErr
	}
	var rows [][]models.InlineKeyboardButton
	for _, e := range epics {
		label := fmt.Sprintf("📝 #%s %s [%s]", e.Number, e.Name, string(e.Status))
		data := fmt.Sprintf("adm_epic_%s_%s", action, e.ID.String())
		rows = append(rows, inlineRow(inlineBtn(label, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)

	sent, err := epicBot.sendWithKeyboard(ctx, msg, "📝 Выберите эпик:", kb)
	if err != nil {
		return err
	}
	sk := sessionKey{ChatID: msg.Chat.ID, ThreadID: msg.MessageThreadID, Username: msg.From.Username}
	sess := &Session{
		ThreadID: msg.MessageThreadID,
		Username: msg.From.Username,
		Data:     make(map[string]string),
	}
	if sent != nil {
		sess.MessageID = sent.ID
	}
	epicBot.sessions.set(sk, sess)
	return nil
}

// showRolePicker sends an inline keyboard with all roles (editing existing message).
func (epicBot *Bot) showRolePicker(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	action, userIDStr string,
	msgID int,
) {
	op := "bot.showRolePicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
		slog.String("user_id", userIDStr),
	)

	roles, err := epicBot.repo.GetAllRoles(ctx)
	log.Debug("roles found", slog.Int("roles count", len(roles)))

	if err != nil || len(roles) == 0 {
		if err != nil {
			log.Error("error getting roles", sl.Err(err))
		}
		epicBot.editOrSend(ctx, msg, msgID, "❌ Роли не найдены.")
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
	sess.Data["pendingUserID"] = userIDStr
	sess.MessageID = msgID
	epicBot.sessions.set(sk, sess)

	var rows [][]models.InlineKeyboardButton
	for _, r := range roles {
		data := fmt.Sprintf("adm_role_%s_%s", action, r.ID.String())
		rows = append(rows, inlineRow(inlineBtn("🎭 "+r.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)

	log.Debug("rows created", slog.Int("rows count", len(rows)))

	epicBot.editOrSendWithKeyboard(ctx, msg, msgID, "🎭 Выберите роль:", kb)

	log.Debug("rows sent", slog.Int("rows count", len(rows)))
}

// showUserRolePicker sends roles currently assigned to a user.
func (epicBot *Bot) showUserRolePicker(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	action string,
	userID uuid.UUID,
	msgID int,
) {
	role, err := epicBot.repo.GetRoleByUserID(ctx, userID)
	if err != nil {
		epicBot.editOrSend(ctx, msg, msgID, "❌ У пользователя нет назначенных ролей.")
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
	sess.Data["pendingUserID"] = userID.String()
	sess.MessageID = msgID
	epicBot.sessions.set(sk, sess)

	data := fmt.Sprintf("adm_role_%s_%s", action, role.ID.String())
	kb := inlineKeyboard(
		inlineRow(inlineBtn("🎭 "+role.Name, data)),
		inlineRow(inlineBtn("❌ Отмена", "adm_cancel")),
	)
	epicBot.editOrSendWithKeyboard(ctx, msg, msgID, "🎭 Выберите роль для снятия:", kb)
}

// showUserTeamPicker sends teams to which the user belongs.
func (epicBot *Bot) showUserTeamPicker(
	ctx context.Context,
	msg *models.Message,
	callback *models.CallbackQuery,
	action string,
	user *domain.User,
	msgID int,
) {
	op := "bot.showUserTeamPicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("action", action),
		slog.String("user_id", user.ID.String()),
	)
	teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting teams by user telegram id", sl.Err(err))
		}
		epicBot.editOrSend(ctx, msg, msgID, "❌ Пользователь не состоит ни в одной команде.")
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
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, inlineRow(inlineBtn("👥 "+t.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	epicBot.editOrSendWithKeyboard(ctx, msg, msgID, "👥 Выберите команду:", kb)
}
