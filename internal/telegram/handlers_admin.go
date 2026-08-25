package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"EpicScoreBot/internal/services"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
)

// ─── /addteam ─────────────────────────────────────────────────────────────

func (epicBot *Bot) handleAddTeam(ctx context.Context, msg *models.Message) error {
	op := "bot.handleAddTeam"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("username", msg.From.Username),
	)
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		_, err := epicBot.sendReply(ctx, msg, "⚠️ Использование: /addteam <название команды>")
		return err
	}

	team, err := epicBot.teamService.CreateTeam(ctx, args, "")
	if err != nil {
		if errors.Is(err, services.ErrTeamAlreadyExists) {
			_, retErr := epicBot.sendReply(ctx, msg, "❌ Команда с таким названием уже существует.")
			return retErr
		}
		log.Error("error creating team", sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Ошибка создания команды.")
		return retErr
	}
	_, retErr := epicBot.sendReply(ctx, msg,
		fmt.Sprintf("✅ Команда «%s» создана (ID: %s)", team.Name, team.ID))
	return retErr
}

// ─── /adduser ─────────────────────────────────────────────────────────────

func (epicBot *Bot) handleAddUser(ctx context.Context, msg *models.Message) error {
	if !epicBot.isTeamAdminAny(ctx, msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}

	args := strings.Fields(commandArguments(msg))
	if len(args) >= 4 {
		username := strings.TrimPrefix(args[0], "@")
		if username == "" {
			_, err := epicBot.sendReply(ctx, msg, "❌ Некорректный @username.")
			return err
		}
		weight, err := strconv.Atoi(args[3])
		if err != nil || weight < 0 || weight > 100 {
			_, retErr := epicBot.sendReply(ctx, msg, "❌ Вес должен быть числом от 0 до 100.")
			return retErr
		}

		user, err := epicBot.userService.CreateUser(ctx, args[1], args[2], username, weight)
		if err != nil {
			if errors.Is(err, services.ErrUserAlreadyExists) {
				_, retErr := epicBot.sendReply(ctx, msg, "❌ Пользователь с таким @username уже существует.")
				return retErr
			}
			_, retErr := epicBot.sendReply(ctx, msg, "❌ Ошибка создания пользователя.")
			return retErr
		}
		_, retErr := epicBot.sendReply(ctx, msg,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))
		return retErr
	}

	// Interactive form: start session — first message is sent normally.
	sk := sessionKey{ChatID: msg.Chat.ID, ThreadID: msg.MessageThreadID, Username: msg.From.Username}
	sent, err := epicBot.sendReply(ctx, msg, "👤 Введите @username пользователя:")
	if err != nil {
		return err
	}
	sess := &Session{
		Step:     StepAddUserUsername,
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

// ─── /assignrole — inline keyboard ────────────────────────────────────────

func (epicBot *Bot) handleAssignRole(ctx context.Context, msg *models.Message) error {
	if !epicBot.isTeamAdminAny(ctx, msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showUserPickerWithoutRole(ctx, msg)
}

// showUserPickerWithoutRole sends an inline keyboard with users who have no role assigned.
func (epicBot *Bot) showUserPickerWithoutRole(ctx context.Context, msg *models.Message) error {
	op := "bot.showUserPickerWithoutRole"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
	)
	users, err := epicBot.userService.GetAllUsers(ctx)
	if err != nil {
		log.Error("error getting all users", sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Ошибка получения пользователей.")
		return retErr
	}

	var rows [][]models.InlineKeyboardButton
	for _, u := range users {
		// Skip users who already have a role.
		if _, err := epicBot.roleService.GetRoleByUserID(ctx, u.ID); err == nil {
			continue
		}
		label := fmt.Sprintf("👤 %s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
		data := fmt.Sprintf("adm_user_assignrole_%s", u.ID.String())
		rows = append(rows, inlineRow(inlineBtn(label, data)))
	}

	if len(rows) == 0 {
		_, retErr := epicBot.sendReply(ctx, msg, "✅ У всех пользователей уже назначена роль.")
		return retErr
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

// ─── /assignteam — inline keyboard ────────────────────────────────────────

func (epicBot *Bot) handleAssignTeam(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "assignteam")
}

// ─── /unassignrole — inline keyboard ─────────────────────────────────────

func (epicBot *Bot) handleUnassignRole(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "unassignrole")
}

// ─── /removefromteam — inline keyboard ───────────────────────────────────

func (epicBot *Bot) handleRemoveFromTeam(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "removefromteam")
}

// ─── /deleteuser — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteUser(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для суперадминистраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "deleteuser")
}

// ─── /renameuser ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleRenameUser(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "renameuser")
}

// ─── /changerate ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleChangeRate(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showUserPickerInitial(ctx, msg, "changerate")
}

// ─── /addadmin ────────────────────────────────────────────────────────────
//
// Назначение team-admin переведено с правки config.yaml (BotConfig.Admins) на
// запись в БД (таблица team_admins), т.к. привязка теперь team-scoped, а не
// глобальной ролью. Флоу интерактивный (выбор команды → выбор пользователя →
// подтверждение), т.к. username нигде не хранится персистентно вне сессии
// Telegram-бота — см. handleAdmTeamSelected (case "addadmin") и
// handleAdmUserSelected (case "addadminconfirm") в admin_callbacks.go.

func (epicBot *Bot) handleAddAdmin(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "addadmin")
}

// ─── /removeadmin ─────────────────────────────────────────────────────────
//
// Аналогично /addadmin — интерактивный флоу (выбор команды → выбор одного из
// текущих team-admin этой команды → подтверждение снятия), см.
// handleAdmTeamSelected (case "removeadmin") и handleAdmUserSelected
// (case "removeadminconfirm") в admin_callbacks.go.

func (epicBot *Bot) handleRemoveAdmin(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "removeadmin")
}
