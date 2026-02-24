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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ─── Command dispatcher ────────────────────────────────────────────────────

// commandHandler dispatches bot commands.
func (bot *Bot) commandHandler(ctx context.Context, update *tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	// Starting a new command cancels any pending session.
	bot.sessions.clear(chatID)

	switch update.Message.Command() {
	case "start":
		return bot.handleStart(chatID, update.Message)
	case "help":
		return bot.handleHelp(chatID, update.Message)
	case "addteam":
		return bot.handleAddTeam(ctx, chatID, update.Message)
	case "adduser":
		return bot.handleAddUser(ctx, chatID, update.Message)
	case "assignrole":
		return bot.handleAssignRole(ctx, chatID, update.Message)
	case "assignteam":
		return bot.handleAssignTeam(ctx, chatID, update.Message)
	case "addepic":
		return bot.handleAddEpic(ctx, chatID, update.Message)
	case "addrisk":
		return bot.handleAddRisk(ctx, chatID, update.Message)
	case "startscore":
		return bot.handleStartScore(ctx, chatID, update.Message)
	case "results":
		return bot.handleResults(ctx, chatID, update.Message)
	case "epicstatus":
		return bot.handleEpicStatus(ctx, chatID, update.Message)
	case "score":
		return bot.handleScoreMenu(ctx, chatID, update.Message)
	case "unassignrole":
		return bot.handleUnassignRole(ctx, chatID, update.Message)
	case "removefromteam":
		return bot.handleRemoveFromTeam(ctx, chatID, update.Message)
	case "deleteepic":
		return bot.handleDeleteEpic(ctx, chatID, update.Message)
	case "deleterisk":
		return bot.handleDeleteRisk(ctx, chatID, update.Message)
	case "addadmin":
		return bot.handleAddAdmin(ctx, chatID, update.Message)
	case "removeadmin":
		return bot.handleRemoveAdmin(ctx, chatID, update.Message)
	default:
		return bot.sendReply(chatID,
			fmt.Sprintf("❓ Неизвестная команда: /%s\nИспользуйте /help для списка команд.",
				update.Message.Command()))
	}
}

// ─── /start ───────────────────────────────────────────────────────────────

func (bot *Bot) handleStart(chatID int64, msg *tgbotapi.Message) error {
	text := fmt.Sprintf("👋 Привет, %s!\n\n"+
		"Я бот для оценки трудоёмкости эпиков и рисков.\n"+
		"Используйте /help для списка команд.",
		msg.From.FirstName)
	return bot.sendReply(chatID, text)
}

// ─── /help ────────────────────────────────────────────────────────────────

func (bot *Bot) handleHelp(chatID int64, msg *tgbotapi.Message) error {
	var text string
	if bot.isAdmin(msg) {
		text = `📋 *Команды бота*

*👤 Для всех:*
/score — меню оценки эпиков и рисков
/epicstatus — статус оценки эпика

*🔧 Для администраторов:*
/addteam <название> — создать команду
/adduser [@username имя фамилия вес] — добавить пользователя
/assignrole — назначить роль пользователю
/assignteam — добавить пользователя в команду
/addepic — создать эпик (интерактивно)
/addrisk — добавить риск к эпику (интерактивно)
/startscore — запустить оценку эпика
/results — показать результаты эпика
/unassignrole — снять роль у пользователя
/removefromteam — удалить пользователя из команды
/deleteepic — удалить эпик
/deleterisk — удалить риск`
	} else {
		text = `📋 *Команды бота*

/score — меню оценки эпиков и рисков
/epicstatus — статус оценки эпика

Для управления командой и эпиками — обратитесь к администратору.`
	}
	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = tgbotapi.ModeMarkdown
	_, err := bot.tgbot.Send(m)
	return err
}

// ─── /addteam ─────────────────────────────────────────────────────────────

func (bot *Bot) handleAddTeam(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return bot.sendReply(chatID, "⚠️ Использование: /addteam <название команды>")
	}
	team, err := bot.repo.CreateTeam(ctx, args, "")
	if err != nil {
		return bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка создания команды: %v", err))
	}
	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Команда «%s» создана (ID: %s)", team.Name, team.ID))
}

// ─── /adduser ─────────────────────────────────────────────────────────────

// handleAddUser creates a user.
// With args: /adduser @username имя фамилия вес  → immediate create
// Without args: interactive session (ask @username, then name, surname, weight)
func (bot *Bot) handleAddUser(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := strings.Fields(msg.CommandArguments())
	if len(args) >= 4 {
		// Direct form: /adduser @username имя фамилия вес
		username := strings.TrimPrefix(args[0], "@")
		if username == "" {
			return bot.sendReply(chatID, "❌ Некорректный @username.")
		}
		weight, err := strconv.Atoi(args[3])
		if err != nil || weight < 0 || weight > 100 {
			return bot.sendReply(chatID, "❌ Вес должен быть числом от 0 до 100.")
		}
		user, err := bot.repo.CreateUser(ctx, args[1], args[2], username, weight)
		if err != nil {
			return bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
		}
		return bot.sendReply(chatID,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))
	}

	// Interactive form: start session
	bot.sessions.set(chatID, &Session{
		Step: StepAddUserUsername,
		Data: make(map[string]string),
	})
	return bot.sendReply(chatID, "👤 Введите @username пользователя (без @):")
}

// ─── /assignrole — inline keyboard ────────────────────────────────────────

func (bot *Bot) handleAssignRole(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showUserPicker(ctx, chatID, "assignrole")
}

// ─── /assignteam — inline keyboard ────────────────────────────────────────

func (bot *Bot) handleAssignTeam(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showUserPicker(ctx, chatID, "assignteam")
}

// ─── /addepic — inline keyboard then session ──────────────────────────────

func (bot *Bot) handleAddEpic(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showTeamPicker(ctx, chatID, "addepic")
}

// ─── /addrisk — inline keyboard then session ──────────────────────────────

func (bot *Bot) handleAddRisk(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showEpicPicker(ctx, chatID, "addrisk", "")
}

// ─── /startscore — inline keyboard ───────────────────────────────────────

func (bot *Bot) handleStartScore(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showEpicPicker(ctx, chatID, "startscore", string(domain.StatusNew))
}

// ─── /results — inline keyboard ──────────────────────────────────────────

func (bot *Bot) handleResults(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	return bot.showEpicPicker(ctx, chatID, "results", "")
}

// ─── /epicstatus — inline keyboard ───────────────────────────────────────

func (bot *Bot) handleEpicStatus(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	return bot.showEpicPicker(ctx, chatID, "epicstatus", "")
}

// ─── /unassignrole — inline keyboard ─────────────────────────────────────

func (bot *Bot) handleUnassignRole(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showUserPicker(ctx, chatID, "unassignrole")
}

// ─── /removefromteam — inline keyboard ───────────────────────────────────

func (bot *Bot) handleRemoveFromTeam(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showUserPicker(ctx, chatID, "removefromteam")
}

// ─── /deleteepic — inline keyboard ───────────────────────────────────────

func (bot *Bot) handleDeleteEpic(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showEpicPicker(ctx, chatID, "deleteepic", "")
}

// ─── /deleterisk — inline keyboard ───────────────────────────────────────

func (bot *Bot) handleDeleteRisk(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	return bot.showEpicPicker(ctx, chatID, "deleterisk", "")
}

// ─── /score ───────────────────────────────────────────────────────────────

func (bot *Bot) handleScoreMenu(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	username := msg.From.UserName
	if username == "" {
		return bot.sendReply(chatID,
			"❌ У вас не задан @username в Telegram. Установите его в настройках профиля.")
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.sendReply(chatID,
				"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
		}
		return bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
	}

	teams, err := bot.repo.GetTeamsByUserTelegramID(ctx, username)
	if err != nil || len(teams) == 0 {
		return bot.sendReply(chatID, "❌ Вы не состоите ни в одной команде.")
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, team := range teams {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("👥 %s", team.Name),
			fmt.Sprintf("team_%s", team.ID.String()))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	replyMsg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("👤 %s %s, выберите команду:", user.FirstName, user.LastName))
	replyMsg.ReplyMarkup = keyboard
	_, err = bot.tgbot.Send(replyMsg)
	return err
}

// ─── Inline picker helpers ─────────────────────────────────────────────────

// showUserPicker sends an inline keyboard with all registered users.
// action is embedded in the callback data so the callback handler knows the flow.
func (bot *Bot) showUserPicker(ctx context.Context, chatID int64, action string) error {
	users, err := bot.repo.GetAllUsers(ctx)
	if err != nil || len(users) == 0 {
		return bot.sendReply(chatID, "❌ Пользователи не найдены.")
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, u := range users {
		label := fmt.Sprintf("👤 %s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
		data := fmt.Sprintf("adm_user_%s_%s", action, u.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "👤 Выберите пользователя:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// showTeamPicker sends an inline keyboard with all teams.
func (bot *Bot) showTeamPicker(ctx context.Context, chatID int64, action string) error {
	teams, err := bot.repo.GetAllTeams(ctx)
	if err != nil || len(teams) == 0 {
		return bot.sendReply(chatID, "❌ Команды не найдены.")
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range teams {
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 "+t.Name, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "👥 Выберите команду:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// showEpicPicker sends an inline keyboard with epics, optionally filtered by status.
func (bot *Bot) showEpicPicker(ctx context.Context, chatID int64, action, statusFilter string) error {
	var epics []domain.Epic
	var err error
	if statusFilter != "" {
		epics, err = bot.repo.GetEpicsByStatus(ctx, domain.Status(statusFilter))
	} else {
		epics, err = bot.repo.GetAllEpics(ctx)
	}
	if err != nil || len(epics) == 0 {
		return bot.sendReply(chatID, "❌ Эпики не найдены.")
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, e := range epics {
		label := fmt.Sprintf("📝 #%s %s [%s]", e.Number, e.Name, string(e.Status))
		data := fmt.Sprintf("adm_epic_%s_%s", action, e.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "📝 Выберите эпик:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// showRolePicker sends an inline keyboard with all roles.
// userIDStr is stored in the session by the caller; callback data carries only
// action + roleID to stay within Telegram's 64-byte callback-data limit.
func (bot *Bot) showRolePicker(ctx context.Context, chatID int64, action, userIDStr string) error {
	op := "bot.showRolePicker"
	log := bot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", chatID),
		slog.String("action", action),
		slog.String("user_id", userIDStr),
	)

	roles, err := bot.repo.GetAllRoles(ctx)

	log.Debug("roles found", slog.Int("roles count", len(roles)))

	if err != nil || len(roles) == 0 {
		return bot.sendReply(chatID, "❌ Роли не найдены.")
	}

	// Persist userID in the session so the callback handler can retrieve it
	// without embedding it in callback data (two UUIDs exceed the 64-byte limit).
	sess, _ := bot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = userIDStr
	bot.sessions.set(chatID, sess)

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, r := range roles {
		// callback: adm_role_<action>_<roleID>  — fits well under 64 bytes
		data := fmt.Sprintf("adm_role_%s_%s", action, r.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎭 "+r.Name, data)))
	}

	log.Debug("rows created", slog.Int("rows count", len(rows)))

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))

	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "🎭 Выберите роль:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)

	if err != nil {
		log.Error("error sending rows", slog.String("error", err.Error()))
	} else {
		log.Debug(
			"rows sent",
			slog.Int("rows count", len(rows)),
		)
	}

	return err
}

// showUserRolePicker sends roles currently assigned to a user.
// userID is stored in the session; callback data carries only action + roleID.
func (bot *Bot) showUserRolePicker(ctx context.Context, chatID int64, action string, userID uuid.UUID) error {
	roles, err := bot.repo.GetRolesByUserID(ctx, userID)
	if err != nil || len(roles) == 0 {
		return bot.sendReply(chatID, "❌ У пользователя нет назначенных ролей.")
	}
	// Persist userID in session so the callback handler can retrieve it.
	sess, _ := bot.sessions.get(chatID)
	if sess == nil {
		sess = &Session{Data: make(map[string]string)}
	}
	sess.Data["pendingUserID"] = userID.String()
	bot.sessions.set(chatID, sess)

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, r := range roles {
		// callback: adm_role_<action>_<roleID>  — fits well under 64 bytes
		data := fmt.Sprintf("adm_role_%s_%s", action, r.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎭 "+r.Name, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "🎭 Выберите роль для снятия:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// showUserTeamPicker sends teams to which the user belongs.
// user.ID is stored in the session; callback data carries only action + teamID.
func (bot *Bot) showUserTeamPicker(ctx context.Context, chatID int64, action string, user *domain.User) error {
	teams, err := bot.repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
	if err != nil || len(teams) == 0 {
		return bot.sendReply(chatID, "❌ Пользователь не состоит ни в одной команде.")
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
		// callback: adm_team_<action>_<teamID>  — fits well under 64 bytes
		data := fmt.Sprintf("adm_team_%s_%s", action, t.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 "+t.Name, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID, "👥 Выберите команду:")
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// showRiskPicker sends risks for an epic.
func (bot *Bot) showRiskPicker(ctx context.Context, chatID int64, action string, epic *domain.Epic) error {
	risks, err := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil || len(risks) == 0 {
		return bot.sendReply(chatID, "❌ Риски не найдены для выбранного эпика.")
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, r := range risks {
		desc := r.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		data := fmt.Sprintf("adm_risk_%s_%s_%s", action, epic.ID.String(), r.ID.String())
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚠️ "+desc, data)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "adm_cancel")))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("⚠️ Выберите риск для эпика #%s «%s»:", epic.Number, epic.Name))
	m.ReplyMarkup = kb
	_, err = bot.tgbot.Send(m)
	return err
}

// ─── /results logic (called by callback) ──────────────────────────────────

// showEpicResults sends the full result report for an epic.
func (bot *Bot) showEpicResults(ctx context.Context, chatID int64, epicID uuid.UUID) {
	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		bot.sendReply(chatID, "❌ Эпик не найден.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Результаты эпика #%s «%s»*\n", epic.Number, epic.Name)
	fmt.Fprintf(&sb, "Статус: %s\n\n", string(epic.Status))

	roleScores, err := bot.repo.GetEpicRoleScoresByEpicID(ctx, epic.ID)
	if err == nil && len(roleScores) > 0 {
		sb.WriteString("📋 *Оценки по ролям:*\n")
		for _, rs := range roleScores {
			role, err := bot.repo.GetRoleByID(ctx, rs.RoleID)
			roleName := rs.RoleID.String()
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&sb, "  • %s: %.2f\n", roleName, rs.WeightedAvg)
		}
		sb.WriteString("\n")
	}

	risks, err := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err == nil && len(risks) > 0 {
		sb.WriteString("⚠️ *Риски:*\n")
		for _, risk := range risks {
			coeff := ""
			if risk.WeightedScore != nil {
				c := scoring.RiskCoefficient(*risk.WeightedScore)
				coeff = fmt.Sprintf(" (оценка: %.2f, коэфф: %.2f)", *risk.WeightedScore, c)
			}
			fmt.Fprintf(&sb, "  • %s [%s]%s\n",
				risk.Description, string(risk.Status), coeff)
		}
		sb.WriteString("\n")
	}

	if epic.FinalScore != nil {
		fmt.Fprintf(&sb, "🏆 *Итоговая оценка: %.0f*\n", *epic.FinalScore)
	} else {
		sb.WriteString("⏳ Итоговая оценка ещё не рассчитана.\n")
	}

	m := tgbotapi.NewMessage(chatID, sb.String())
	m.ParseMode = tgbotapi.ModeMarkdown
	bot.tgbot.Send(m)
}

// ─── /epicstatus logic (called by callback) ───────────────────────────────

// showEpicStatusReport shows who has not yet scored an epic and its risks.
func (bot *Bot) showEpicStatusReport(ctx context.Context, chatID int64, epicID uuid.UUID) {
	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		bot.sendReply(chatID, "❌ Эпик не найден.")
		return
	}

	teamMembers, err := bot.repo.GetUsersByTeamID(ctx, epic.TeamID)
	if err != nil {
		bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	scoredEpic, _ := bot.repo.GetUsersWhoScoredEpic(ctx, epic.ID)
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

	risks, _ := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if len(risks) > 0 {
		sb.WriteString("\n⚠️ *Риски:*\n")
		for _, risk := range risks {
			scoredRisk, _ := bot.repo.GetUsersWhoScoredRisk(ctx, risk.ID)
			riskScoredSet := make(map[uuid.UUID]bool)
			for _, u := range scoredRisk {
				riskScoredSet[u.ID] = true
			}
			desc := risk.Description
			if len(desc) > 40 {
				desc = desc[:37] + "..."
			}
			fmt.Fprintf(&sb, "\n*%s* [%s] — не оценили:\n", desc, string(risk.Status))
			riskMissing := 0
			for _, u := range teamMembers {
				if !riskScoredSet[u.ID] {
					fmt.Fprintf(&sb, "  • %s %s (@%s)\n",
						u.FirstName, u.LastName, u.TelegramID)
					riskMissing++
				}
			}
			if riskMissing == 0 {
				sb.WriteString("  ✅ Все оценили\n")
			}
		}
	}

	m := tgbotapi.NewMessage(chatID, sb.String())
	m.ParseMode = tgbotapi.ModeMarkdown
	bot.tgbot.Send(m)
}

// ─── Session input handler ────────────────────────────────────────────────

// handleSessionInput handles plain-text messages that continue a multi-step flow.
func (bot *Bot) handleSessionInput(update *tgbotapi.Update) {
	if update.Message == nil {
		return
	}
	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	sess, ok := bot.sessions.get(chatID)
	if !ok {
		// No active session — ignore silently
		return
	}
	bot.sessions.touch(chatID)

	ctx := bot.ctx

	switch sess.Step {

	// ── /adduser interactive steps ─────────────────────────────────────

	case StepAddUserUsername:
		username := strings.TrimPrefix(text, "@")
		if username == "" {
			bot.sendReply(chatID, "❌ Некорректный @username. Попробуйте ещё раз:")
			return
		}
		sess.Data["username"] = username
		sess.Step = StepAddUserFirstName
		bot.sessions.set(chatID, sess)
		bot.sendReply(chatID, "📝 Введите имя:")

	case StepAddUserFirstName:
		if text == "" {
			bot.sendReply(chatID, "❌ Имя не может быть пустым. Введите имя:")
			return
		}
		sess.Data["firstName"] = text
		sess.Step = StepAddUserLastName
		bot.sessions.set(chatID, sess)
		bot.sendReply(chatID, "📝 Введите фамилию:")

	case StepAddUserLastName:
		if text == "" {
			bot.sendReply(chatID, "❌ Фамилия не может быть пустой. Введите фамилию:")
			return
		}
		sess.Data["lastName"] = text
		sess.Step = StepAddUserWeight
		bot.sessions.set(chatID, sess)
		bot.sendReply(chatID, "📝 Введите вес пользователя (0–100):")

	case StepAddUserWeight:
		weight, err := strconv.Atoi(text)
		if err != nil || weight < 0 || weight > 100 {
			bot.sendReply(chatID, "❌ Вес должен быть числом от 0 до 100. Введите ещё раз:")
			return
		}
		user, err := bot.repo.CreateUser(ctx,
			sess.Data["firstName"], sess.Data["lastName"],
			sess.Data["username"], weight)
		bot.sessions.clear(chatID)
		if err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
			return
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Пользователь %s %s (@%s) создан",
				user.FirstName, user.LastName, user.TelegramID))

	// ── /addepic interactive steps ─────────────────────────────────────

	case StepAddEpicNumber:
		sess.Data["number"] = text
		sess.Step = StepAddEpicName
		bot.sessions.set(chatID, sess)
		bot.sendReply(chatID, "📝 Введите название эпика:")

	case StepAddEpicName:
		sess.Data["name"] = text
		sess.Step = StepAddEpicDesc
		bot.sessions.set(chatID, sess)
		bot.sendReply(chatID, "📝 Введите описание эпика (или напишите «-» чтобы пропустить):")

	case StepAddEpicDesc:
		desc := text
		if desc == "-" {
			desc = ""
		}
		teamIDStr := sess.Data["teamID"]
		bot.sessions.clear(chatID)
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка: неверный ID команды.")
			return
		}
		epic, err := bot.repo.CreateEpic(ctx, sess.Data["number"], sess.Data["name"], desc, teamID)
		if err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка создания эпика: %v", err))
			return
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Эпик #%s «%s» создан (статус: NEW)", epic.Number, epic.Name))

	// ── /addrisk interactive steps ─────────────────────────────────────

	case StepAddRiskDesc:
		epicIDStr := sess.Data["epicID"]
		bot.sessions.clear(chatID)
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка: неверный ID эпика.")
			return
		}
		risk, err := bot.repo.CreateRisk(ctx, text, epicID)
		if err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка создания риска: %v", err))
			return
		}
		epic, _ := bot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicID.String()
		if epic != nil {
			epicNum = epic.Number
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Риск создан для эпика #%s (ID: %s)", epicNum, risk.ID))

	// ── /score epic effort text-input step ────────────────────────────

	case StepScoreEpicEffort:
		score, err := strconv.Atoi(text)
		if err != nil || score < 0 || score > 500 {
			bot.sendReply(chatID,
				"❌ Некорректный ввод. Введите целое число от 0 до 500:")
			return
		}

		epicIDStr := sess.Data["epicID"]
		username := sess.Data["username"]
		bot.sessions.clear(chatID)

		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendReply(chatID, "❌ Ошибка: неверный ID эпика.")
			return
		}

		user, err := bot.repo.FindUserByTelegramID(ctx, username)
		if err != nil {
			bot.sendReply(chatID, "❌ Пользователь не найден.")
			return
		}

		roles, err := bot.repo.GetRolesByUserID(ctx, user.ID)
		if err != nil || len(roles) == 0 {
			bot.sendReply(chatID, "❌ У вас нет назначенной роли.")
			return
		}

		if err := bot.repo.CreateEpicScore(ctx, epicID, user.ID, roles[0].ID, score); err != nil {
			bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
			return
		}

		epic, _ := bot.repo.GetEpicByID(ctx, epicID)
		epicNum := epicIDStr
		if epic != nil {
			epicNum = epic.Number
		}
		bot.sendReply(chatID,
			fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!", score, epicNum))

		if err := bot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
			bot.log.Error("failed to try complete epic scoring",
				slog.String("epicID", epicID.String()), sl.Err(err))
		}

	default:
		bot.sessions.clear(chatID)
	}
}

// ─── /startscore execution (called by callback) ───────────────────────────

// execStartScore moves an epic and its risks to SCORING.
func (bot *Bot) execStartScore(ctx context.Context, chatID int64, epicID uuid.UUID) {
	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		bot.sendReply(chatID, "❌ Эпик не найден.")
		return
	}
	if epic.Status != domain.StatusNew {
		bot.sendReply(chatID,
			fmt.Sprintf("⚠️ Эпик #%s уже в статусе %s.", epic.Number, string(epic.Status)))
		return
	}
	if err := bot.repo.UpdateEpicStatus(ctx, epic.ID, domain.StatusScoring); err != nil {
		bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка смены статуса эпика: %v", err))
		return
	}
	risks, err := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil {
		bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения рисков: %v", err))
		return
	}
	for _, risk := range risks {
		if err := bot.repo.UpdateRiskStatus(ctx, risk.ID, domain.StatusScoring); err != nil {
			bot.log.Error("failed to update risk status",
				slog.String("riskID", risk.ID.String()), sl.Err(err))
		}
	}
	bot.sendReply(chatID,
		fmt.Sprintf("🚀 Эпик #%s «%s» и %d рисков отправлены на оценку!",
			epic.Number, epic.Name, len(risks)))
}

func (bot *Bot) handleAddAdmin(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	op := "bot.handleAddAdmin"
	log := bot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", chatID),
	)

	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return bot.sendReply(chatID, "⚠️ Использование: /addadmin <username>")
	}
	username := strings.TrimPrefix(args, "@")

	bot.cfg.BotConfig.Admins = append(bot.cfg.BotConfig.Admins, username)
	err := bot.cfg.Write()
	if err != nil {
		bot.cfg.BotConfig.Admins = bot.cfg.BotConfig.Admins[:len(bot.cfg.BotConfig.Admins)-1]
		log.Error("failed to add admin", slog.String("username", username), sl.Err(err))
		return bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка добавления администратора: %v", err))
	}
	log.Info("admin added", slog.String("username", username))
	return bot.sendReply(chatID, fmt.Sprintf("✅ Администратор @%s добавлен.", username))
}

func (bot *Bot) handleRemoveAdmin(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	op := "bot.handleRemoveAdmin"
	log := bot.log.With(
		slog.String("op", op),
		slog.Int64("chatID", chatID),
	)

	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return bot.sendReply(chatID, "⚠️ Использование: /removeadmin <username>")
	}
	username := strings.TrimPrefix(args, "@")

	idx := slices.Index(bot.cfg.BotConfig.Admins, username)
	if idx == -1 {
		return bot.sendReply(chatID, fmt.Sprintf("❌ Администратор @%s не найден.", username))
	}

	removed := bot.cfg.BotConfig.Admins[idx]
	bot.cfg.BotConfig.Admins = slices.Delete(bot.cfg.BotConfig.Admins, idx, idx+1)

	if err := bot.cfg.Write(); err != nil {
		// rollback
		bot.cfg.BotConfig.Admins = slices.Insert(bot.cfg.BotConfig.Admins, idx, removed)
		log.Error("failed to remove admin", slog.String("username", username), sl.Err(err))
		return bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка удаления администратора: %v", err))
	}

	log.Info("admin removed", slog.String("username", username))
	return bot.sendReply(chatID, fmt.Sprintf("✅ Администратор @%s удалён.", username))
}
