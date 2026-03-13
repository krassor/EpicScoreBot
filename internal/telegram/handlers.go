package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// ─── Command dispatcher ────────────────────────────────────────────────────

// commandHandler dispatches bot commands.
func (epicBot *Bot) commandHandler(ctx context.Context, update *models.Update) error {
	msg := update.Message
	// Starting a new command cancels any pending session for this user/chat.
	sk := sessionKey{
		ChatID:   msg.Chat.ID,
		ThreadID: msg.MessageThreadID,
		Username: msg.From.Username,
	}
	sess, ok := epicBot.sessions.get(sk)
	if ok && sess.MessageID > 0 {
		epicBot.deleteMessage(ctx, msg.Chat.ID, sess.MessageID)
	}
	epicBot.sessions.clear(sk)

	switch commandText(msg) {
	case "start":
		return epicBot.handleStart(ctx, msg)
	case "help":
		return epicBot.handleHelp(ctx, msg)
	case "addteam":
		return epicBot.handleAddTeam(ctx, msg)
	case "adduser":
		return epicBot.handleAddUser(ctx, msg)
	case "renameuser":
		return epicBot.handleRenameUser(ctx, msg)
	case "assignrole":
		return epicBot.handleAssignRole(ctx, msg)
	case "assignteam":
		return epicBot.handleAssignTeam(ctx, msg)
	case "addepic":
		return epicBot.handleAddEpic(ctx, msg)
	case "addrisk":
		return epicBot.handleAddRisk(ctx, msg)
	case "startscore":
		return epicBot.handleStartScore(ctx, msg)
	case "results":
		return epicBot.handleResults(ctx, msg)
	case "epicstatus":
		return epicBot.handleEpicStatus(ctx, msg)
	case "score":
		return epicBot.handleScoreMenu(ctx, msg)
	case "unassignrole":
		return epicBot.handleUnassignRole(ctx, msg)
	case "removefromteam":
		return epicBot.handleRemoveFromTeam(ctx, msg)
	case "deleteepic":
		return epicBot.handleDeleteEpic(ctx, msg)
	case "deleterisk":
		return epicBot.handleDeleteRisk(ctx, msg)
	case "deleteuser":
		return epicBot.handleDeleteUser(ctx, msg)
	case "changerate":
		return epicBot.handleChangeRate(ctx, msg)
	case "addadmin":
		return epicBot.handleAddAdmin(ctx, msg)
	case "removeadmin":
		return epicBot.handleRemoveAdmin(ctx, msg)
	case "list":
		return epicBot.handleList(ctx, msg)
	default:
		_, err := epicBot.sendReply(ctx, msg,
			fmt.Sprintf("❓ Неизвестная команда: /%s\nИспользуйте /help для списка команд.",
				commandText(msg)))
		return err
	}
}

// ─── /start ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleStart(ctx context.Context, msg *models.Message) error {
	text := fmt.Sprintf("👋 Привет, %s!\n\n"+
		"Я бот для оценки трудоёмкости эпиков и рисков.\n"+
		"Используйте /help для списка команд.",
		msg.From.FirstName)
	_, err := epicBot.sendReply(ctx, msg, text)
	return err
}

// ─── /help ────────────────────────────────────────────────────────────────

func (epicBot *Bot) handleHelp(ctx context.Context, msg *models.Message) error {
	var sb strings.Builder
	sb.WriteString("📋 <b>Команды бота</b>\n\n")
	sb.WriteString("<b>👤 Для всех:</b>\n")
	sb.WriteString("/score — меню оценки эпиков и рисков\n")
	sb.WriteString("/epicstatus — статус оценки эпика\n")

	if epicBot.isAdmin(msg) {
		sb.WriteString("\n<b>🔧 Для администраторов:</b>\n")
		sb.WriteString("/addteam &lt;название&gt; — создать команду\n")
		sb.WriteString("/adduser — добавить пользователя\n")
		sb.WriteString("/assignrole — назначить роль пользователю\n")
		sb.WriteString("/addepic — создать эпик\n")
		sb.WriteString("/addrisk — добавить риск к эпику\n")
		sb.WriteString("/startscore — запустить оценку эпика\n")
		sb.WriteString("/results — показать результаты эпика\n")
		sb.WriteString("/list — список участников команды\n")
	}

	if epicBot.isSuperAdmin(msg) {
		sb.WriteString("\n<b>⚡ Для супер-администраторов:</b>\n")
		sb.WriteString("/assignteam — добавить пользователя в команду\n")
		sb.WriteString("/renameuser — переименовать пользователя\n")
		sb.WriteString("/changerate — изменить вес пользователя\n")
		sb.WriteString("/unassignrole — снять роль у пользователя\n")
		sb.WriteString("/removefromteam — удалить из команды\n")
		sb.WriteString("/deleteepic — удалить эпик\n")
		sb.WriteString("/deleterisk — удалить риск\n")
		sb.WriteString("/deleteuser — удалить пользователя\n")
		sb.WriteString("/addadmin — добавить администратора\n")
		sb.WriteString("/removeadmin — удалить администратора\n")
	}

	if !epicBot.isAdmin(msg) {
		sb.WriteString("\nДля управления — обратитесь к администратору.")
	}

	_, err := epicBot.sendHTML(ctx, msg, sb.String())
	return err
}

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

// ─── /addepic — inline keyboard then session ──────────────────────────────

func (epicBot *Bot) handleAddEpic(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "addepic")
}

// ─── /addrisk — inline keyboard then session ──────────────────────────────

func (epicBot *Bot) handleAddRisk(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showEpicPickerInitial(ctx, msg, "addrisk", "")
}

// ─── /startscore — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleStartScore(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showEpicPickerInitial(ctx, msg, "startscore", string(domain.StatusNew))
}

// ─── /results — inline keyboard ──────────────────────────────────────────

func (epicBot *Bot) handleResults(ctx context.Context, msg *models.Message) error {
	return epicBot.showEpicPickerInitial(ctx, msg, "results", "")
}

// ─── /epicstatus — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleEpicStatus(ctx context.Context, msg *models.Message) error {
	return epicBot.showEpicPickerInitial(ctx, msg, "epicstatus", "")
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

// ─── /deleteepic — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteEpic(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showEpicPickerInitial(ctx, msg, "deleteepic", "")
}

// ─── /deleterisk — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteRisk(ctx context.Context, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для супер-администраторов.")
		return err
	}
	return epicBot.showEpicPickerInitial(ctx, msg, "deleterisk", "")
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

// ─── /list ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleList(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "list")
}

// ─── /score ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleScoreMenu(ctx context.Context, msg *models.Message) error {
	op := "bot.handleScoreMenu"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
	)
	username := msg.From.Username
	if username == "" {
		_, err := epicBot.sendReply(ctx, msg,
			"❌ У вас не задан @username в Telegram. Установите его в настройках профиля.")
		return err
	}

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, retErr := epicBot.sendReply(ctx, msg,
				"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
			return retErr
		}
		_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка: %v", err))
		return retErr
	}

	teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, username)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting teams by user telegram id", sl.Err(err))
		}
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Вы не состоите ни в одной команде.")
		return retErr
	}

	var rows [][]models.InlineKeyboardButton
	for _, team := range teams {
		rows = append(rows, inlineRow(inlineBtn(
			fmt.Sprintf("👥 %s", team.Name),
			fmt.Sprintf("team_%s", team.ID.String()),
		)))
	}
	kb := inlineKeyboard(rows...)
	_, retErr := epicBot.sendWithKeyboard(ctx, msg,
		fmt.Sprintf("👤 %s %s, выберите команду:", user.FirstName, user.LastName), kb)
	return retErr
}

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

// ─── /results logic (called by callback) ──────────────────────────────────

func (epicBot *Bot) showEpicResults(ctx context.Context, msg *models.Message, epicID uuid.UUID) {
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Результаты эпика \\#%s «%s»*\n", escapeMarkdownV2(epic.Number), escapeMarkdownV2(epic.Name))
	fmt.Fprintf(&sb, "Статус: %s\n\n", escapeMarkdownV2(string(epic.Status)))

	roleScores, err := epicBot.repo.GetEpicRoleScoresByEpicID(ctx, epic.ID)
	if err == nil && len(roleScores) > 0 {
		sb.WriteString("📋 *Оценки по ролям:*\n")
		for _, rs := range roleScores {
			role, err := epicBot.repo.GetRoleByID(ctx, rs.RoleID)
			roleName := rs.RoleID.String()
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&sb, "  • %s: %s\n", escapeMarkdownV2(roleName), escapeMarkdownV2(fmt.Sprintf("%.2f", rs.WeightedAvg)))
		}
		sb.WriteString("\n")
	}

	risks, err := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err == nil && len(risks) > 0 {
		sb.WriteString("⚠️ *Риски:*\n")
		for _, risk := range risks {
			coeff := ""
			if risk.WeightedScore != nil {
				c := scoring.RiskCoefficient(*risk.WeightedScore)
				coeff = fmt.Sprintf(" \\(оценка: %s, коэфф: %s\\)",
					escapeMarkdownV2(fmt.Sprintf("%.2f", *risk.WeightedScore)),
					escapeMarkdownV2(fmt.Sprintf("%.2f", c)))
			}
			fmt.Fprintf(&sb, "  • %s \\[%s\\]%s\n", escapeMarkdownV2(risk.Description), escapeMarkdownV2(string(risk.Status)), coeff)
		}
		sb.WriteString("\n")
	}

	if epic.FinalScore != nil {
		fmt.Fprintf(&sb, "🏆 *Итоговая оценка: %s*\n", escapeMarkdownV2(fmt.Sprintf("%.0f", *epic.FinalScore)))
	} else {
		sb.WriteString("⏳ Итоговая оценка ещё не рассчитана\\.\n")
	}

	epicBot.sendMarkdown(ctx, msg, sb.String())
}

// ─── /epicstatus logic (called by callback) ───────────────────────────────

func (epicBot *Bot) showEpicStatusReport(ctx context.Context, msg *models.Message, epicID uuid.UUID) {
	op := "bot.showEpicStatusReport"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
		slog.String("epic_id", epicID.String()),
	)
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}
	log.Debug(
		"epic found",
		slog.String("epic", epic.Number),
	)

	teamMembers, err := epicBot.repo.GetUsersByTeamID(ctx, epic.TeamID)
	if err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	log.Debug(
		"team members found",
		slog.Int("count", len(teamMembers)),
	)

	scoredEpic, _ := epicBot.repo.GetUsersWhoScoredEpic(ctx, epic.ID)
	scoredSet := make(map[uuid.UUID]bool)
	for _, u := range scoredEpic {
		scoredSet[u.ID] = true
	}

	log.Debug(
		"scored epic",
		slog.Int("count", len(scoredEpic)),
	)

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Статус оценки эпика \\#%s «%s»*\n\n",
		escapeMarkdownV2(epic.Number), escapeMarkdownV2(epic.Name))

	sb.WriteString("📋 *Трудоёмкость — не оценили:*\n")
	missing := 0
	for _, u := range teamMembers {
		if !scoredSet[u.ID] {
			fmt.Fprintf(&sb, "  • %s %s \\(@%s\\)\n",
				escapeMarkdownV2(u.FirstName), escapeMarkdownV2(u.LastName), escapeMarkdownV2(u.TelegramID))
			missing++
		}
	}
	if missing == 0 {
		sb.WriteString("  ✅ Все оценили\n")
	}

	risks, _ := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if len(risks) > 0 {
		sb.WriteString("\n⚠️ *Риски:*\n")
		for _, risk := range risks {
			scoredRisk, _ := epicBot.repo.GetUsersWhoScoredRisk(ctx, risk.ID)
			riskScoredSet := make(map[uuid.UUID]bool)
			for _, u := range scoredRisk {
				riskScoredSet[u.ID] = true
			}
			desc := risk.Description
			if len([]rune(desc)) > 40 {
				desc = string([]rune(desc)[:37]) + "..."
			}
			fmt.Fprintf(&sb, "\n*%s* \\[%s\\] — не оценили:\n",
				escapeMarkdownV2(desc), escapeMarkdownV2(string(risk.Status)))
			riskMissing := 0
			for _, u := range teamMembers {
				if !riskScoredSet[u.ID] {
					fmt.Fprintf(&sb, "  • %s %s \\(@%s\\)\n",
						escapeMarkdownV2(u.FirstName), escapeMarkdownV2(u.LastName), escapeMarkdownV2(u.TelegramID))
					riskMissing++
				}
			}
			if riskMissing == 0 {
				sb.WriteString("  ✅ Все оценили\n")
			}
		}
	}

	log.Debug(
		"status report",
		slog.String("report", sb.String()),
	)

	epicBot.sendMarkdown(ctx, msg, sb.String())
}

// ─── Session input handler ────────────────────────────────────────────────

// handleSessionInput handles plain-text messages that continue a multi-step flow.
func (epicBot *Bot) handleSessionInput(update *models.Update) {
	op := "bot.handleSessionInput"
	log := epicBot.log.With(slog.String("op", op))

	if update.Message == nil {
		return
	}
	msg := update.Message
	chatID := msg.Chat.ID
	text := msg.Text

	log.Debug(
		"text input",
		slog.String("text", text),
		slog.String("username", msg.From.Username),
		slog.Int64("chat_id", chatID),
		slog.Int("message_thread_id", msg.MessageThreadID),
	)

	// Find session by chatID + threadID (username is checked inside).
	sess, sk, ok := epicBot.sessions.findByChat(chatID, msg.MessageThreadID)
	if !ok {
		// No active session — ignore silently.
		log.Debug("no active session")
		return
	}

	// Verify the message sender is the session owner.
	if sess.Username != "" && !strings.EqualFold(sess.Username, msg.From.Username) {
		log.Debug("text input from non-owner, ignoring",
			slog.String("session_owner", sess.Username),
			slog.String("sender", msg.From.Username),
		)
		return
	}

	epicBot.sessions.touch(sk)

	ctx := epicBot.ctx
	msgID := sess.MessageID

	log.Debug(
		"session found",
		slog.String("step", string(sess.Step)),
	)

	switch sess.Step {

	// ── /adduser interactive steps ─────────────────────────────────────

	case StepAddUserUsername:
		username := strings.TrimPrefix(text, "@")
		if username == "" {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Некорректный @username. Попробуйте ещё раз:")
			return
		}
		sess.Data["username"] = username
		sess.Step = StepAddUserFirstName
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите имя:")

	case StepAddUserFirstName:
		if text == "" {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Имя не может быть пустым. Введите имя:")
			return
		}
		sess.Data["firstName"] = text
		sess.Step = StepAddUserLastName
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите фамилию:")

	case StepAddUserLastName:
		if text == "" {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Фамилия не может быть пустой. Введите фамилию:")
			return
		}
		sess.Data["lastName"] = text
		sess.Step = StepAddUserWeight
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите вес пользователя (0–100):")

	case StepAddUserWeight:
		weight, err := strconv.Atoi(text)
		if err != nil || weight < 0 || weight > 100 {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Вес должен быть числом от 0 до 100. Введите ещё раз:")
			return
		}
		user, err := epicBot.repo.CreateUser(ctx,
			sess.Data["firstName"], sess.Data["lastName"],
			sess.Data["username"], weight)
		epicBot.sessions.clear(sk)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))

	// ── /renameuser interactive steps ──────────────────────────────────

	case StepRenameUserFirstName:
		if text == "" {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Имя не может быть пустым. Введите новое имя:")
			return
		}
		sess.Data["firstName"] = text
		sess.Step = StepRenameUserLastName
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите новую фамилию:")

	case StepRenameUserLastName:
		if text == "" {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Фамилия не может быть пустой. Введите новую фамилию:")
			return
		}
		userIDStr := sess.Data["pendingUserID"]
		epicBot.sessions.clear(sk)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка: неверный ID пользователя.")
			return
		}
		if err := epicBot.repo.UpdateUserName(ctx, userID, sess.Data["firstName"], text); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка переименования.")
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Пользователь переименован: %s %s", sess.Data["firstName"], text))

	// ── /changerate interactive steps ─────────────────────────────────

	case StepChangeRateWeight:
		weight, err := strconv.Atoi(text)
		if err != nil || weight < 0 || weight > 100 {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Вес должен быть числом от 0 до 100. Введите ещё раз:")
			return
		}
		userIDStr := sess.Data["pendingUserID"]
		epicBot.sessions.clear(sk)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка: неверный ID пользователя.")
			return
		}
		if err := epicBot.repo.UpdateUserWeight(ctx, userID, weight); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка изменения веса.")
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("✅ Вес пользователя изменён на %d", weight))

	// ── /addepic interactive steps ─────────────────────────────────────

	case StepAddEpicNumber:
		sess.Data["number"] = text
		epic, _ := epicBot.repo.GetEpicByNumber(ctx, sess.Data["number"])
		// if err != nil {
		// 	epicBot.editOrSend(ctx, msg, msgID, "❌ Ошибка поиска эпика.")
		// 	return
		// }
		if epic != nil {
			epicBot.editOrSend(ctx, msg, msgID, "❌ Эпик с таким номером уже существует.")
			return
		}

		sess.Step = StepAddEpicName
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите название эпика:")

	case StepAddEpicName:
		sess.Data["name"] = text
		sess.Step = StepAddEpicDesc
		epicBot.sessions.set(sk, sess)
		epicBot.editOrSend(ctx, msg, msgID, "📝 Введите описание эпика (или напишите «-» чтобы пропустить):")

	case StepAddEpicDesc:
		desc := text
		if desc == "-" {
			desc = ""
		}
		teamIDStr := sess.Data["teamID"]
		epicBot.sessions.clear(sk)
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка: неверный ID команды.")
			return
		}

		epic, _ := epicBot.repo.GetEpicByNumber(ctx, sess.Data["number"])
		// if err != nil {
		// 	epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка поиска эпика.")
		// 	return
		// }
		if epic != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Эпик с таким номером уже существует.")
			return
		}

		epic, err = epicBot.repo.CreateEpic(ctx, sess.Data["number"], sess.Data["name"], desc, teamID)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка создания эпика.")
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Эпик #%s «%s» создан (статус: NEW)", epic.Number, epic.Name))

	// ── /addrisk interactive steps ─────────────────────────────────────

	case StepAddRiskDesc:
		epicIDStr := sess.Data["epicID"]
		epicBot.sessions.clear(sk)
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка: неверный ID эпика.")
			return
		}
		risk, err := epicBot.repo.CreateRisk(ctx, text, epicID)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка создания риска: %v", err))
			return
		}
		epic, _ := epicBot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicID.String()
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Риск создан для эпика #%s (ID: %s)", epicNum, risk.ID))

	// ── /score epic effort text-input step ────────────────────────────

	case StepScoreEpicEffort:
		score, err := strconv.Atoi(text)
		if err != nil || score < 0 || score > 500 {
			epicBot.editOrSend(ctx, msg, msgID,
				"❌ Некорректный ввод. Введите целое число от 0 до 500:")
			return
		}

		epicIDStr := sess.Data["epicID"]
		username := sess.Data["username"]
		epicBot.sessions.clear(sk)

		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка: неверный ID эпика.")
			return
		}

		user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Пользователь не найден.")
			return
		}

		role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ У вас нет назначенной роли.")
			return
		}

		if err := epicBot.repo.CreateEpicScore(ctx, epicID, user.ID, role.ID, score); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
			return
		}

		epic, _ := epicBot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicIDStr
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!", score, epicNum))

		if err := epicBot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
			epicBot.log.Error("failed to try complete epic scoring",
				slog.String("epicID", epicID.String()), sl.Err(err))
		}

		// Show unscored risks if any remain.
		epicBot.showEpicRisks(ctx, msg, username, epicID)

	default:
		epicBot.sessions.clear(sk)
	}
}

// ─── /startscore execution (called by callback) ───────────────────────────

func (epicBot *Bot) execStartScore(ctx context.Context, msg *models.Message, epicID uuid.UUID) {
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}
	if epic.Status != domain.StatusNew {
		epicBot.sendReply(ctx, msg,
			fmt.Sprintf("⚠️ Эпик #%s уже в статусе %s.", epic.Number, string(epic.Status)))
		return
	}
	if err := epicBot.repo.UpdateEpicStatus(ctx, epic.ID, domain.StatusScoring); err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка смены статуса эпика: %v", err))
		return
	}
	risks, err := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка получения рисков: %v", err))
		return
	}
	for _, risk := range risks {
		if err := epicBot.repo.UpdateRiskStatus(ctx, risk.ID, domain.StatusScoring); err != nil {
			epicBot.log.Error("failed to update risk status",
				slog.String("riskID", risk.ID.String()), sl.Err(err))
		}
	}
	epicBot.sendReply(ctx, msg,
		fmt.Sprintf("🚀 Эпик #%s «%s» и %d рисков отправлены на оценку!",
			epic.Number, epic.Name, len(risks)))
}

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
