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
	chatID := msg.Chat.ID
	threadID := msg.MessageThreadID
	// Starting a new command cancels any pending session.
	epicBot.sessions.clear(chatID)

	switch commandText(msg) {
	case "start":
		return epicBot.handleStart(ctx, chatID, threadID, msg)
	case "help":
		return epicBot.handleHelp(ctx, chatID, threadID, msg)
	case "addteam":
		return epicBot.handleAddTeam(ctx, chatID, threadID, msg)
	case "adduser":
		return epicBot.handleAddUser(ctx, chatID, threadID, msg)
	case "renameuser":
		return epicBot.handleRenameUser(ctx, chatID, threadID, msg)
	case "assignrole":
		return epicBot.handleAssignRole(ctx, chatID, threadID, msg)
	case "assignteam":
		return epicBot.handleAssignTeam(ctx, chatID, threadID, msg)
	case "addepic":
		return epicBot.handleAddEpic(ctx, chatID, threadID, msg)
	case "addrisk":
		return epicBot.handleAddRisk(ctx, chatID, threadID, msg)
	case "startscore":
		return epicBot.handleStartScore(ctx, chatID, threadID, msg)
	case "results":
		return epicBot.handleResults(ctx, chatID, threadID, msg)
	case "epicstatus":
		return epicBot.handleEpicStatus(ctx, chatID, threadID, msg)
	case "score":
		return epicBot.handleScoreMenu(ctx, chatID, threadID, msg)
	case "unassignrole":
		return epicBot.handleUnassignRole(ctx, chatID, threadID, msg)
	case "removefromteam":
		return epicBot.handleRemoveFromTeam(ctx, chatID, threadID, msg)
	case "deleteepic":
		return epicBot.handleDeleteEpic(ctx, chatID, threadID, msg)
	case "deleterisk":
		return epicBot.handleDeleteRisk(ctx, chatID, threadID, msg)
	case "deleteuser":
		return epicBot.handleDeleteUser(ctx, chatID, threadID, msg)
	case "changerate":
		return epicBot.handleChangeRate(ctx, chatID, threadID, msg)
	case "addadmin":
		return epicBot.handleAddAdmin(ctx, chatID, threadID, msg)
	case "removeadmin":
		return epicBot.handleRemoveAdmin(ctx, chatID, threadID, msg)
	case "list":
		return epicBot.handleList(ctx, chatID, threadID, msg)
	default:
		return epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("❓ Неизвестная команда: /%s\nИспользуйте /help для списка команд.",
				commandText(msg)))
	}
}

// ─── /start ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleStart(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	text := fmt.Sprintf("👋 Привет, %s!\n\n"+
		"Я бот для оценки трудоёмкости эпиков и рисков.\n"+
		"Используйте /help для списка команд.",
		msg.From.FirstName)
	return epicBot.sendReply(ctx, chatID, threadID, text)
}

// ─── /help ────────────────────────────────────────────────────────────────

func (epicBot *Bot) handleHelp(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	var sb strings.Builder
	sb.WriteString("📋 *Команды бота*\n\n")
	sb.WriteString("*👤 Для всех:*\n")
	sb.WriteString("/score — меню оценки эпиков и рисков\n")
	sb.WriteString("/epicstatus — статус оценки эпика\n")

	if epicBot.isAdmin(msg) {
		sb.WriteString("\n*🔧 Для администраторов:*\n")
		sb.WriteString("/addteam <название> — создать команду\n")
		sb.WriteString("/adduser — добавить пользователя\n")
		sb.WriteString("/assignrole — назначить роль пользователю\n")
		sb.WriteString("/addepic — создать эпик\n")
		sb.WriteString("/addrisk — добавить риск к эпику\n")
		sb.WriteString("/startscore — запустить оценку эпика\n")
		sb.WriteString("/results — показать результаты эпика\n")
		sb.WriteString("/list — список участников команды\n")
	}

	if epicBot.isSuperAdmin(msg) {
		sb.WriteString("\n*⚡ Для супер-администраторов:*\n")
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

	return epicBot.sendMarkdown(ctx, chatID, threadID, sb.String())
}

// ─── /addteam ─────────────────────────────────────────────────────────────

func (epicBot *Bot) handleAddTeam(
	ctx context.Context,
	chatID int64,
	threadID int,
	msg *models.Message,
) error {
	op := "bot.handleAddTeam"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("username", msg.From.Username),
	)
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		return epicBot.sendReply(ctx, chatID, threadID, "⚠️ Использование: /addteam <название команды>")
	}

	team, _ := epicBot.repo.GetTeamByName(ctx, args)
	if team != nil {
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Команда с таким названием уже существует.")
	}

	team, err := epicBot.repo.CreateTeam(ctx, args, "")
	if err != nil {
		log.Error("error creating team", sl.Err(err))
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка создания команды.")
	}
	return epicBot.sendReply(ctx, chatID, threadID,
		fmt.Sprintf("✅ Команда «%s» создана (ID: %s)", team.Name, team.ID))
}

// ─── /adduser ─────────────────────────────────────────────────────────────

func (epicBot *Bot) handleAddUser(
	ctx context.Context,
	chatID int64,
	threadID int,
	msg *models.Message,
) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}

	args := strings.Fields(commandArguments(msg))
	if len(args) >= 4 {
		username := strings.TrimPrefix(args[0], "@")
		if username == "" {
			return epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректный @username.")
		}
		weight, err := strconv.Atoi(args[3])
		if err != nil || weight < 0 || weight > 100 {
			return epicBot.sendReply(ctx, chatID, threadID, "❌ Вес должен быть числом от 0 до 100.")
		}

		user, _ := epicBot.repo.FindUserByTelegramID(ctx, username)
		if user != nil {
			return epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь с таким @username уже существует.")
		}

		user, err = epicBot.repo.CreateUser(ctx, args[1], args[2], username, weight)
		if err != nil {
			return epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка создания пользователя.")
		}
		return epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))
	}

	// Interactive form: start session
	epicBot.sessions.set(chatID, &Session{
		Step:     StepAddUserUsername,
		ThreadID: threadID,
		Data:     make(map[string]string),
	})
	return epicBot.sendReply(ctx, chatID, threadID, "👤 Введите @username пользователя:")
}

// ─── /assignrole — inline keyboard ────────────────────────────────────────

func (epicBot *Bot) handleAssignRole(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "assignrole")
}

// ─── /assignteam — inline keyboard ────────────────────────────────────────

func (epicBot *Bot) handleAssignTeam(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "assignteam")
}

// ─── /addepic — inline keyboard then session ──────────────────────────────

func (epicBot *Bot) handleAddEpic(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}
	return epicBot.showTeamPicker(ctx, chatID, threadID, "addepic")
}

// ─── /addrisk — inline keyboard then session ──────────────────────────────

func (epicBot *Bot) handleAddRisk(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}
	return epicBot.showEpicPicker(ctx, chatID, threadID, "addrisk", "")
}

// ─── /startscore — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleStartScore(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}
	return epicBot.showEpicPicker(ctx, chatID, threadID, "startscore", string(domain.StatusNew))
}

// ─── /results — inline keyboard ──────────────────────────────────────────

func (epicBot *Bot) handleResults(ctx context.Context, chatID int64, threadID int, _ *models.Message) error {
	return epicBot.showEpicPicker(ctx, chatID, threadID, "results", "")
}

// ─── /epicstatus — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleEpicStatus(ctx context.Context, chatID int64, threadID int, _ *models.Message) error {
	return epicBot.showEpicPicker(ctx, chatID, threadID, "epicstatus", "")
}

// ─── /unassignrole — inline keyboard ─────────────────────────────────────

func (epicBot *Bot) handleUnassignRole(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "unassignrole")
}

// ─── /removefromteam — inline keyboard ───────────────────────────────────

func (epicBot *Bot) handleRemoveFromTeam(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "removefromteam")
}

// ─── /deleteepic — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteEpic(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showEpicPicker(ctx, chatID, threadID, "deleteepic", "")
}

// ─── /deleterisk — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteRisk(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showEpicPicker(ctx, chatID, threadID, "deleterisk", "")
}

// ─── /deleteuser — inline keyboard ───────────────────────────────────────

func (epicBot *Bot) handleDeleteUser(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для суперадминистраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "deleteuser")
}

// ─── /renameuser ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleRenameUser(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "renameuser")
}

// ─── /changerate ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleChangeRate(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	return epicBot.showUserPicker(ctx, chatID, threadID, "changerate")
}

// ─── /list ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleList(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для администраторов.")
	}
	return epicBot.showTeamPicker(ctx, chatID, threadID, "list")
}

// ─── /score ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleScoreMenu(
	ctx context.Context,
	chatID int64,
	threadID int,
	msg *models.Message,
) error {
	op := "bot.handleScoreMenu"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
	)
	username := msg.From.Username
	if username == "" {
		return epicBot.sendReply(ctx, chatID, threadID,
			"❌ У вас не задан @username в Telegram. Установите его в настройках профиля.")
	}

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return epicBot.sendReply(ctx, chatID, threadID,
				"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
		}
		return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка: %v", err))
	}

	teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, username)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting teams by user telegram id", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Вы не состоите ни в одной команде.")
	}

	var rows [][]models.InlineKeyboardButton
	for _, team := range teams {
		rows = append(rows, inlineRow(inlineBtn(
			fmt.Sprintf("👥 %s", team.Name),
			fmt.Sprintf("team_%s", team.ID.String()),
		)))
	}
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("👤 %s %s, выберите команду:", user.FirstName, user.LastName), kb)
}

// ─── Inline picker helpers ─────────────────────────────────────────────────

// showUserPicker sends an inline keyboard with all registered users.
func (epicBot *Bot) showUserPicker(ctx context.Context, chatID int64, threadID int, action string) error {
	op := "bot.showUserPicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
	)
	users, err := epicBot.repo.GetAllUsers(ctx)
	if err != nil || len(users) == 0 {
		if err != nil {
			log.Error("error getting all users", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователи не найдены.")
	}
	var rows [][]models.InlineKeyboardButton
	for _, u := range users {
		label := fmt.Sprintf("👤 %s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
		data := fmt.Sprintf("adm_user_%s_%s", action, u.ID.String())
		rows = append(rows, inlineRow(inlineBtn(label, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID, "👤 Выберите пользователя:", kb)
}

// showTeamPicker sends an inline keyboard with all teams.
func (epicBot *Bot) showTeamPicker(ctx context.Context, chatID int64, threadID int, action string) error {
	op := "bot.showTeamPicker"
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
	var rows [][]models.InlineKeyboardButton
	for _, t := range teams {
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, inlineRow(inlineBtn("👥 "+t.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID, "👥 Выберите команду:", kb)
}

// showEpicPicker sends an inline keyboard with epics, optionally filtered by status.
func (epicBot *Bot) showEpicPicker(
	ctx context.Context,
	chatID int64,
	threadID int,
	action, statusFilter string,
) error {
	op := "bot.showEpicPicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
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
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Эпики не найдены.")
	}
	var rows [][]models.InlineKeyboardButton
	for _, e := range epics {
		label := fmt.Sprintf("📝 #%s %s [%s]", e.Number, e.Name, string(e.Status))
		data := fmt.Sprintf("adm_epic_%s_%s", action, e.ID.String())
		rows = append(rows, inlineRow(inlineBtn(label, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID, "📝 Выберите эпик:", kb)
}

// showRolePicker sends an inline keyboard with all roles.
func (epicBot *Bot) showRolePicker(
	ctx context.Context,
	chatID int64,
	threadID int,
	action, userIDStr string,
) error {
	op := "bot.showRolePicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
		slog.String("user_id", userIDStr),
	)

	roles, err := epicBot.repo.GetAllRoles(ctx)
	log.Debug("roles found", slog.Int("roles count", len(roles)))

	if err != nil || len(roles) == 0 {
		if err != nil {
			log.Error("error getting roles", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Роли не найдены.")
	}

	sess, _ := epicBot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = userIDStr
	epicBot.sessions.set(chatID, sess)

	var rows [][]models.InlineKeyboardButton
	for _, r := range roles {
		data := fmt.Sprintf("adm_role_%s_%s", action, r.ID.String())
		rows = append(rows, inlineRow(inlineBtn("🎭 "+r.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)

	log.Debug("rows created", slog.Int("rows count", len(rows)))

	if err := epicBot.sendWithKeyboard(ctx, chatID, threadID, "🎭 Выберите роль:", kb); err != nil {
		log.Error("error sending rows", slog.String("error", err.Error()))
		return err
	}
	log.Debug("rows sent", slog.Int("rows count", len(rows)))
	return nil
}

// showUserRolePicker sends roles currently assigned to a user.
func (epicBot *Bot) showUserRolePicker(
	ctx context.Context,
	chatID int64,
	threadID int,
	action string,
	userID uuid.UUID,
) error {
	role, err := epicBot.repo.GetRoleByUserID(ctx, userID)
	if err != nil {
		return epicBot.sendReply(ctx, chatID, threadID, "❌ У пользователя нет назначенных ролей.")
	}
	sess, _ := epicBot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = userID.String()
	epicBot.sessions.set(chatID, sess)

	data := fmt.Sprintf("adm_role_%s_%s", action, role.ID.String())
	kb := inlineKeyboard(
		inlineRow(inlineBtn("🎭 "+role.Name, data)),
		inlineRow(inlineBtn("❌ Отмена", "adm_cancel")),
	)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID, "🎭 Выберите роль для снятия:", kb)
}

// showUserTeamPicker sends teams to which the user belongs.
func (epicBot *Bot) showUserTeamPicker(
	ctx context.Context,
	chatID int64,
	threadID int,
	action string,
	user *domain.User,
) error {
	op := "bot.showUserTeamPicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
		slog.String("user_id", user.ID.String()),
	)
	teams, err := epicBot.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting teams by user telegram id", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не состоит ни в одной команде.")
	}
	sess, _ := epicBot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = user.ID.String()
	epicBot.sessions.set(chatID, sess)

	var rows [][]models.InlineKeyboardButton
	for _, t := range teams {
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, inlineRow(inlineBtn("👥 "+t.Name, data)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "adm_cancel")))
	kb := inlineKeyboard(rows...)
	return epicBot.sendWithKeyboard(ctx, chatID, threadID, "👥 Выберите команду:", kb)
}

// showRiskPicker sends risks for an epic.
func (epicBot *Bot) showRiskPicker(
	ctx context.Context,
	chatID int64,
	threadID int,
	action string,
	epic *domain.Epic,
) error {
	op := "bot.showRiskPicker"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
		slog.String("epic_id", epic.ID.String()),
	)
	risks, err := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil || len(risks) == 0 {
		if err != nil {
			log.Error("error getting risks by epic id", sl.Err(err))
		}
		return epicBot.sendReply(ctx, chatID, threadID, "❌ Риски не найдены для выбранного эпика.")
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
	return epicBot.sendWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("⚠️ Выберите риск для эпика #%s «%s»:", epic.Number, epic.Name), kb)
}

// ─── /results logic (called by callback) ──────────────────────────────────

func (epicBot *Bot) showEpicResults(ctx context.Context, chatID int64, threadID int, epicID uuid.UUID) {
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик не найден.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Результаты эпика #%s «%s»*\n", epic.Number, epic.Name)
	fmt.Fprintf(&sb, "Статус: %s\n\n", string(epic.Status))

	roleScores, err := epicBot.repo.GetEpicRoleScoresByEpicID(ctx, epic.ID)
	if err == nil && len(roleScores) > 0 {
		sb.WriteString("📋 *Оценки по ролям:*\n")
		for _, rs := range roleScores {
			role, err := epicBot.repo.GetRoleByID(ctx, rs.RoleID)
			roleName := rs.RoleID.String()
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&sb, "  • %s: %.2f\n", roleName, rs.WeightedAvg)
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
				coeff = fmt.Sprintf(" (оценка: %.2f, коэфф: %.2f)", *risk.WeightedScore, c)
			}
			fmt.Fprintf(&sb, "  • %s [%s]%s\n", risk.Description, string(risk.Status), coeff)
		}
		sb.WriteString("\n")
	}

	if epic.FinalScore != nil {
		fmt.Fprintf(&sb, "🏆 *Итоговая оценка: %.0f*\n", *epic.FinalScore)
	} else {
		sb.WriteString("⏳ Итоговая оценка ещё не рассчитана.\n")
	}

	epicBot.sendMarkdown(ctx, chatID, threadID, sb.String())
}

// ─── /epicstatus logic (called by callback) ───────────────────────────────

func (epicBot *Bot) showEpicStatusReport(ctx context.Context, chatID int64, threadID int, epicID uuid.UUID) {
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик не найден.")
		return
	}

	teamMembers, err := epicBot.repo.GetUsersByTeamID(ctx, epic.TeamID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	scoredEpic, _ := epicBot.repo.GetUsersWhoScoredEpic(ctx, epic.ID)
	scoredSet := make(map[uuid.UUID]bool)
	for _, u := range scoredEpic {
		scoredSet[u.ID] = true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Статус оценки эпика #%s «%s»*\n\n", epic.Number, epic.Name)

	sb.WriteString("📋 *Трудоёмкость — не оценили:*\n")
	missing := 0
	for _, u := range teamMembers {
		if !scoredSet[u.ID] {
			fmt.Fprintf(&sb, "  • %s %s (@%s)\n", u.FirstName, u.LastName, u.TelegramID)
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
			fmt.Fprintf(&sb, "\n*%s* [%s] — не оценили:\n", desc, string(risk.Status))
			riskMissing := 0
			for _, u := range teamMembers {
				if !riskScoredSet[u.ID] {
					fmt.Fprintf(&sb, "  • %s %s (@%s)\n", u.FirstName, u.LastName, u.TelegramID)
					riskMissing++
				}
			}
			if riskMissing == 0 {
				sb.WriteString("  ✅ Все оценили\n")
			}
		}
	}

	epicBot.sendMarkdown(ctx, chatID, threadID, sb.String())
}

// ─── Session input handler ────────────────────────────────────────────────

// handleSessionInput handles plain-text messages that continue a multi-step flow.
func (epicBot *Bot) handleSessionInput(update *models.Update) {
	if update.Message == nil {
		return
	}
	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	sess, ok := epicBot.sessions.get(chatID)
	if !ok {
		// No active session — ignore silently.
		return
	}
	epicBot.sessions.touch(chatID)

	ctx := epicBot.ctx
	// Use the thread from the session (set when the session was first created).
	threadID := sess.ThreadID

	switch sess.Step {

	// ── /adduser interactive steps ─────────────────────────────────────

	case StepAddUserUsername:
		username := strings.TrimPrefix(text, "@")
		if username == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректный @username. Попробуйте ещё раз:")
			return
		}
		sess.Data["username"] = username
		sess.Step = StepAddUserFirstName
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите имя:")

	case StepAddUserFirstName:
		if text == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Имя не может быть пустым. Введите имя:")
			return
		}
		sess.Data["firstName"] = text
		sess.Step = StepAddUserLastName
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите фамилию:")

	case StepAddUserLastName:
		if text == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Фамилия не может быть пустой. Введите фамилию:")
			return
		}
		sess.Data["lastName"] = text
		sess.Step = StepAddUserWeight
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите вес пользователя (0–100):")

	case StepAddUserWeight:
		weight, err := strconv.Atoi(text)
		if err != nil || weight < 0 || weight > 100 {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Вес должен быть числом от 0 до 100. Введите ещё раз:")
			return
		}
		user, err := epicBot.repo.CreateUser(ctx,
			sess.Data["firstName"], sess.Data["lastName"],
			sess.Data["username"], weight)
		epicBot.sessions.clear(chatID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
			return
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))

	// ── /renameuser interactive steps ──────────────────────────────────

	case StepRenameUserFirstName:
		if text == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Имя не может быть пустым. Введите новое имя:")
			return
		}
		sess.Data["firstName"] = text
		sess.Step = StepRenameUserLastName
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите новую фамилию:")

	case StepRenameUserLastName:
		if text == "" {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Фамилия не может быть пустой. Введите новую фамилию:")
			return
		}
		userIDStr := sess.Data["pendingUserID"]
		epicBot.sessions.clear(chatID)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка: неверный ID пользователя.")
			return
		}
		if err := epicBot.repo.UpdateUserName(ctx, userID, sess.Data["firstName"], text); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка переименования.")
			return
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Пользователь переименован: %s %s", sess.Data["firstName"], text))

	// ── /changerate interactive steps ─────────────────────────────────

	case StepChangeRateWeight:
		weight, err := strconv.Atoi(text)
		if err != nil || weight < 0 || weight > 100 {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Вес должен быть числом от 0 до 100. Введите ещё раз:")
			return
		}
		userIDStr := sess.Data["pendingUserID"]
		epicBot.sessions.clear(chatID)
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка: неверный ID пользователя.")
			return
		}
		if err := epicBot.repo.UpdateUserWeight(ctx, userID, weight); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка изменения веса.")
			return
		}
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("✅ Вес пользователя изменён на %d", weight))

	// ── /addepic interactive steps ─────────────────────────────────────

	case StepAddEpicNumber:
		epic, err := epicBot.repo.GetEpicByNumber(ctx, sess.Data["number"])
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка поиска эпика.")
			return
		}
		if epic != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик с таким номером уже существует.")
			return
		}
		sess.Data["number"] = text
		sess.Step = StepAddEpicName
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите название эпика:")

	case StepAddEpicName:
		sess.Data["name"] = text
		sess.Step = StepAddEpicDesc
		epicBot.sessions.set(chatID, sess)
		epicBot.sendReply(ctx, chatID, threadID, "📝 Введите описание эпика (или напишите «-» чтобы пропустить):")

	case StepAddEpicDesc:
		desc := text
		if desc == "-" {
			desc = ""
		}
		teamIDStr := sess.Data["teamID"]
		epicBot.sessions.clear(chatID)
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка: неверный ID команды.")
			return
		}

		epic, err := epicBot.repo.GetEpicByNumber(ctx, sess.Data["number"])
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка поиска эпика.")
			return
		}
		if epic != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик с таким номером уже существует.")
			return
		}

		epic, err = epicBot.repo.CreateEpic(ctx, sess.Data["number"], sess.Data["name"], desc, teamID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка создания эпика.")
			return
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Эпик #%s «%s» создан (статус: NEW)", epic.Number, epic.Name))

	// ── /addrisk interactive steps ─────────────────────────────────────

	case StepAddRiskDesc:
		epicIDStr := sess.Data["epicID"]
		epicBot.sessions.clear(chatID)
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка: неверный ID эпика.")
			return
		}
		risk, err := epicBot.repo.CreateRisk(ctx, text, epicID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка создания риска: %v", err))
			return
		}
		epic, _ := epicBot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicID.String()
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Риск создан для эпика #%s (ID: %s)", epicNum, risk.ID))

	// ── /score epic effort text-input step ────────────────────────────

	case StepScoreEpicEffort:
		score, err := strconv.Atoi(text)
		if err != nil || score < 0 || score > 500 {
			epicBot.sendReply(ctx, chatID, threadID,
				"❌ Некорректный ввод. Введите целое число от 0 до 500:")
			return
		}

		epicIDStr := sess.Data["epicID"]
		username := sess.Data["username"]
		epicBot.sessions.clear(chatID)

		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка: неверный ID эпика.")
			return
		}

		user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден.")
			return
		}

		role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
		if err != nil {
			epicBot.sendReply(ctx, chatID, threadID, "❌ У вас нет назначенной роли.")
			return
		}

		if err := epicBot.repo.CreateEpicScore(ctx, epicID, user.ID, role.ID, score); err != nil {
			epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
			return
		}

		epic, _ := epicBot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicIDStr
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!", score, epicNum))

		if err := epicBot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
			epicBot.log.Error("failed to try complete epic scoring",
				slog.String("epicID", epicID.String()), sl.Err(err))
		}

	default:
		epicBot.sessions.clear(chatID)
	}
}

// ─── /startscore execution (called by callback) ───────────────────────────

func (epicBot *Bot) execStartScore(ctx context.Context, chatID int64, threadID int, epicID uuid.UUID) {
	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик не найден.")
		return
	}
	if epic.Status != domain.StatusNew {
		epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("⚠️ Эпик #%s уже в статусе %s.", epic.Number, string(epic.Status)))
		return
	}
	if err := epicBot.repo.UpdateEpicStatus(ctx, epic.ID, domain.StatusScoring); err != nil {
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка смены статуса эпика: %v", err))
		return
	}
	risks, err := epicBot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil {
		epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка получения рисков: %v", err))
		return
	}
	for _, risk := range risks {
		if err := epicBot.repo.UpdateRiskStatus(ctx, risk.ID, domain.StatusScoring); err != nil {
			epicBot.log.Error("failed to update risk status",
				slog.String("riskID", risk.ID.String()), sl.Err(err))
		}
	}
	epicBot.sendReply(ctx, chatID, threadID,
		fmt.Sprintf("🚀 Эпик #%s «%s» и %d рисков отправлены на оценку!",
			epic.Number, epic.Name, len(risks)))
}

func (epicBot *Bot) handleAddAdmin(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	op := "bot.handleAddAdmin"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", chatID),
	)

	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		return epicBot.sendReply(ctx, chatID, threadID, "⚠️ Использование: /addadmin <username>")
	}
	username := strings.TrimPrefix(args, "@")

	epicBot.cfg.BotConfig.Admins = append(epicBot.cfg.BotConfig.Admins, username)
	err := epicBot.cfg.Write()
	if err != nil {
		epicBot.cfg.BotConfig.Admins = epicBot.cfg.BotConfig.Admins[:len(epicBot.cfg.BotConfig.Admins)-1]
		log.Error("failed to add admin", slog.String("username", username), sl.Err(err))
		return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка добавления администратора: %v", err))
	}
	log.Info("admin added", slog.String("username", username))
	return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("✅ Администратор @%s добавлен.", username))
}

func (epicBot *Bot) handleRemoveAdmin(ctx context.Context, chatID int64, threadID int, msg *models.Message) error {
	op := "bot.handleRemoveAdmin"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", chatID),
	)

	if !epicBot.isSuperAdmin(msg) {
		return epicBot.sendReply(ctx, chatID, threadID, "⛔ Только для супер-администраторов.")
	}
	args := strings.TrimSpace(commandArguments(msg))
	if args == "" {
		return epicBot.sendReply(ctx, chatID, threadID, "⚠️ Использование: /removeadmin <username>")
	}
	username := strings.TrimPrefix(args, "@")

	idx := slices.Index(epicBot.cfg.BotConfig.Admins, username)
	if idx == -1 {
		return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Администратор @%s не найден.", username))
	}

	removed := epicBot.cfg.BotConfig.Admins[idx]
	epicBot.cfg.BotConfig.Admins = slices.Delete(epicBot.cfg.BotConfig.Admins, idx, idx+1)

	if err := epicBot.cfg.Write(); err != nil {
		epicBot.cfg.BotConfig.Admins = slices.Insert(epicBot.cfg.BotConfig.Admins, idx, removed)
		log.Error("failed to remove admin", slog.String("username", username), sl.Err(err))
		return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка удаления администратора: %v", err))
	}

	log.Info("admin removed", slog.String("username", username))
	return epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("✅ Администратор @%s удалён.", username))
}
