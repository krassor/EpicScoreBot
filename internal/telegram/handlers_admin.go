package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

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

	team, _ := epicBot.repo.GetTeamByName(ctx, args)
	if team != nil {
		_, err := epicBot.sendReply(ctx, msg, "❌ Команда с таким названием уже существует.")
		return err
	}

	team, err := epicBot.repo.CreateTeam(ctx, args, "")
	if err != nil {
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
	if !epicBot.isAdmin(msg) {
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

		user, _ := epicBot.repo.FindUserByTelegramID(ctx, username)
		if user != nil {
			_, retErr := epicBot.sendReply(ctx, msg, "❌ Пользователь с таким @username уже существует.")
			return retErr
		}

		user, err = epicBot.repo.CreateUser(ctx, args[1], args[2], username, weight)
		if err != nil {
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
	if !epicBot.isAdmin(msg) {
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
	users, err := epicBot.repo.GetAllUsers(ctx)
	if err != nil {
		log.Error("error getting all users", sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Ошибка получения пользователей.")
		return retErr
	}

	var rows [][]models.InlineKeyboardButton
	for _, u := range users {
		// Skip users who already have a role.
		if _, err := epicBot.repo.GetRoleByUserID(ctx, u.ID); err == nil {
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

func (epicBot *Bot) handleAddAdmin(ctx context.Context, msg *models.Message) error {
	op := "bot.handleAddAdmin"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", msg.Chat.ID),
	)

	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		_, err := epicBot.sendReply(ctx, msg, "⚠️ Использование: /addadmin <username>")
		return err
	}
	username := strings.TrimPrefix(args, "@")

	epicBot.cfg.BotConfig.Admins = append(epicBot.cfg.BotConfig.Admins, username)
	err := epicBot.cfg.Write()
	if err != nil {
		epicBot.cfg.BotConfig.Admins = epicBot.cfg.BotConfig.Admins[:len(epicBot.cfg.BotConfig.Admins)-1]
		log.Error("failed to add admin", slog.String("username", username), sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка добавления администратора: %v", err))
		return retErr
	}
	log.Info("admin added", slog.String("username", username))
	_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("✅ Администратор @%s добавлен.", username))
	return retErr
}

// ─── /removeadmin ─────────────────────────────────────────────────────────

func (epicBot *Bot) handleRemoveAdmin(ctx context.Context, msg *models.Message) error {
	op := "bot.handleRemoveAdmin"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", msg.Chat.ID),
	)

	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		_, err := epicBot.sendReply(ctx, msg, "⚠️ Использование: /removeadmin <username>")
		return err
	}
	username := strings.TrimPrefix(args, "@")

	idx := slices.Index(epicBot.cfg.BotConfig.Admins, username)
	if idx == -1 {
		_, err := epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Администратор @%s не найден.", username))
		return err
	}

	removed := epicBot.cfg.BotConfig.Admins[idx]
	epicBot.cfg.BotConfig.Admins = slices.Delete(epicBot.cfg.BotConfig.Admins, idx, idx+1)

	if err := epicBot.cfg.Write(); err != nil {
		epicBot.cfg.BotConfig.Admins = slices.Insert(epicBot.cfg.BotConfig.Admins, idx, removed)
		log.Error("failed to remove admin", slog.String("username", username), sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка удаления администратора: %v", err))
		return retErr
	}

	log.Info("admin removed", slog.String("username", username))
	_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("✅ Администратор @%s удалён.", username))
	return retErr
}
