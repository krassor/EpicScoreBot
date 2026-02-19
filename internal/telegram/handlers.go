package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/scoring"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// commandHandler dispatches bot commands.
func (bot *Bot) commandHandler(ctx context.Context, update *tgbotapi.Update) error {
	chatID := update.Message.Chat.ID

	switch update.Message.Command() {
	case "start":
		return bot.handleStart(chatID, update.Message)

	case "help":
		return bot.handleHelp(chatID)

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

	case "score":
		return bot.handleScoreMenu(ctx, chatID, update.Message)

	default:
		return bot.sendReply(chatID,
			fmt.Sprintf("❓ Неизвестная команда: /%s\nИспользуйте /help для списка команд.",
				update.Message.Command()))
	}
}

// handleStart greets the user.
func (bot *Bot) handleStart(chatID int64, msg *tgbotapi.Message) error {
	text := fmt.Sprintf("👋 Привет, %s!\n\n"+
		"Я бот для оценки трудоёмкости эпиков и рисков.\n"+
		"Используйте /help для списка команд.",
		msg.From.FirstName)
	return bot.sendReply(chatID, text)
}

// handleHelp shows available commands.
func (bot *Bot) handleHelp(chatID int64) error {
	text := `📋 *Команды бота*

*Для администратора:*
/addteam <название> — создать команду
/adduser <tgID> <имя> <фамилия> <вес> — добавить пользователя
/assignrole <tgID> <название роли> — назначить роль
/assignteam <tgID> <название команды> — добавить в команду
/addepic <команда> | <номер> | <название> | <описание> — создать эпик
/addrisk <номер эпика> | <описание риска> — добавить риск
/startscore <номер эпика> — отправить эпик на оценку
/results <номер эпика> — показать результаты

*Для участников:*
/score — меню оценки эпиков и рисков`

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := bot.tgbot.Send(msg)
	return err
}

// handleAddTeam creates a team. Admin only.
// Usage: /addteam TeamName
func (bot *Bot) handleAddTeam(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		return bot.sendReply(chatID,
			"⚠️ Использование: /addteam <название команды>")
	}

	team, err := bot.repo.CreateTeam(ctx, args, "")
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка создания команды: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Команда «%s» создана (ID: %s)", team.Name, team.ID))
}

// handleAddUser creates a user. Admin only.
// Usage: /adduser <telegramID> <firstName> <lastName> <weight>
func (bot *Bot) handleAddUser(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := strings.Fields(msg.CommandArguments())
	if len(args) < 4 {
		return bot.sendReply(chatID,
			"⚠️ Использование: /adduser <telegramID> <имя> <фамилия> <вес>")
	}

	tgID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return bot.sendReply(chatID, "❌ Некорректный telegramID.")
	}

	weight, err := strconv.Atoi(args[3])
	if err != nil || weight < 0 || weight > 100 {
		return bot.sendReply(chatID,
			"❌ Вес должен быть числом от 0 до 100.")
	}

	user, err := bot.repo.CreateUser(ctx, args[1], args[2], tgID, weight)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Пользователь %s %s создан (вес: %d%%)",
			user.FirstName, user.LastName, user.Weight))
}

// handleAssignRole assigns a role to a user. Admin only.
// Usage: /assignrole <telegramID> <roleName>
func (bot *Bot) handleAssignRole(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := msg.CommandArguments()
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		return bot.sendReply(chatID,
			"⚠️ Использование: /assignrole <telegramID> <название роли>")
	}

	tgID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return bot.sendReply(chatID, "❌ Некорректный telegramID.")
	}

	roleName := strings.TrimSpace(parts[1])

	user, err := bot.repo.FindUserByTelegramID(ctx, tgID)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Пользователь с TG ID %d не найден.", tgID))
	}

	role, err := bot.repo.GetRoleByName(ctx, roleName)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Роль «%s» не найдена.", roleName))
	}

	if err := bot.repo.AssignUserRole(ctx, user.ID, role.ID); err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка назначения роли: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Роль «%s» назначена пользователю %s %s.",
			role.Name, user.FirstName, user.LastName))
}

// handleAssignTeam assigns a user to a team. Admin only.
// Usage: /assignteam <telegramID> <teamName>
func (bot *Bot) handleAssignTeam(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := msg.CommandArguments()
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		return bot.sendReply(chatID,
			"⚠️ Использование: /assignteam <telegramID> <название команды>")
	}

	tgID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return bot.sendReply(chatID, "❌ Некорректный telegramID.")
	}

	teamName := strings.TrimSpace(parts[1])

	user, err := bot.repo.FindUserByTelegramID(ctx, tgID)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Пользователь с TG ID %d не найден.", tgID))
	}

	team, err := bot.repo.GetTeamByName(ctx, teamName)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Команда «%s» не найдена.", teamName))
	}

	if err := bot.repo.AssignUserTeam(ctx, user.ID, team.ID); err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка добавления в команду: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Пользователь %s %s добавлен в команду «%s».",
			user.FirstName, user.LastName, team.Name))
}

// handleAddEpic creates an epic. Admin only.
// Usage: /addepic teamName | epicNumber | epicName | description
func (bot *Bot) handleAddEpic(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := msg.CommandArguments()
	parts := strings.SplitN(args, "|", 4)
	if len(parts) < 3 {
		return bot.sendReply(chatID,
			"⚠️ Использование: /addepic <команда> | <номер> | <название> | <описание>")
	}

	teamName := strings.TrimSpace(parts[0])
	epicNumber := strings.TrimSpace(parts[1])
	epicName := strings.TrimSpace(parts[2])
	description := ""
	if len(parts) == 4 {
		description = strings.TrimSpace(parts[3])
	}

	team, err := bot.repo.GetTeamByName(ctx, teamName)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Команда «%s» не найдена.", teamName))
	}

	epic, err := bot.repo.CreateEpic(ctx, epicNumber, epicName, description, team.ID)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка создания эпика: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Эпик #%s «%s» создан для команды «%s» (статус: NEW)",
			epic.Number, epic.Name, team.Name))
}

// handleAddRisk creates a risk for an epic. Admin only.
// Usage: /addrisk epicNumber | riskDescription
func (bot *Bot) handleAddRisk(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	args := msg.CommandArguments()
	parts := strings.SplitN(args, "|", 2)
	if len(parts) < 2 {
		return bot.sendReply(chatID,
			"⚠️ Использование: /addrisk <номер эпика> | <описание риска>")
	}

	epicNumber := strings.TrimSpace(parts[0])
	riskDesc := strings.TrimSpace(parts[1])

	epic, err := bot.repo.GetEpicByNumber(ctx, epicNumber)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Эпик #%s не найден.", epicNumber))
	}

	risk, err := bot.repo.CreateRisk(ctx, riskDesc, epic.ID)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка создания риска: %v", err))
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("✅ Риск создан для эпика #%s (ID: %s)",
			epic.Number, risk.ID))
}

// handleStartScore moves an epic and its risks to SCORING. Admin only.
// Usage: /startscore epicNumber
func (bot *Bot) handleStartScore(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	if !bot.isAdmin(msg) {
		return bot.sendReply(chatID, "⛔ Только для администраторов.")
	}

	epicNumber := strings.TrimSpace(msg.CommandArguments())
	if epicNumber == "" {
		return bot.sendReply(chatID,
			"⚠️ Использование: /startscore <номер эпика>")
	}

	epic, err := bot.repo.GetEpicByNumber(ctx, epicNumber)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Эпик #%s не найден.", epicNumber))
	}

	if epic.Status != domain.StatusNew {
		return bot.sendReply(chatID,
			fmt.Sprintf("⚠️ Эпик #%s уже в статусе %s.",
				epic.Number, string(epic.Status)))
	}

	if err := bot.repo.UpdateEpicStatus(ctx, epic.ID, domain.StatusScoring); err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка смены статуса эпика: %v", err))
	}

	// Move all risks to SCORING as well
	risks, err := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка получения рисков: %v", err))
	}

	for _, risk := range risks {
		if err := bot.repo.UpdateRiskStatus(ctx, risk.ID, domain.StatusScoring); err != nil {
			bot.log.Error("failed to update risk status",
				slog.String("riskID", risk.ID.String()),
				slog.String("error", err.Error()))
		}
	}

	return bot.sendReply(chatID,
		fmt.Sprintf("🚀 Эпик #%s «%s» и %d рисков отправлены на оценку!",
			epic.Number, epic.Name, len(risks)))
}

// handleResults shows the scoring results for an epic.
// Usage: /results epicNumber
func (bot *Bot) handleResults(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	epicNumber := strings.TrimSpace(msg.CommandArguments())
	if epicNumber == "" {
		return bot.sendReply(chatID,
			"⚠️ Использование: /results <номер эпика>")
	}

	epic, err := bot.repo.GetEpicByNumber(ctx, epicNumber)
	if err != nil {
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Эпик #%s не найден.", epicNumber))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Результаты эпика #%s «%s»*\n", epic.Number, epic.Name)
	fmt.Fprintf(&sb, "Статус: %s\n\n", string(epic.Status))

	// Role scores
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

	// Risks
	risks, err := bot.repo.GetRisksByEpicID(ctx, epic.ID)
	if err == nil && len(risks) > 0 {
		sb.WriteString("⚠️ *Риски:*\n")
		for _, risk := range risks {
			coeff := ""
			if risk.WeightedScore != nil {
				c := scoring.RiskCoefficient(*risk.WeightedScore)
				coeff = fmt.Sprintf(
					" (оценка: %.2f, коэфф: %.2f)",
					*risk.WeightedScore, c)
			}
			fmt.Fprintf(&sb, "  • %s [%s]%s\n",
				risk.Description, string(risk.Status), coeff)
		}
		sb.WriteString("\n")
	}

	// Final score
	if epic.FinalScore != nil {
		fmt.Fprintf(&sb, "🏆 *Итоговая оценка: %.0f*\n", *epic.FinalScore)
	} else {
		sb.WriteString("⏳ Итоговая оценка ещё не рассчитана.\n")
	}

	resultMsg := tgbotapi.NewMessage(chatID, sb.String())
	resultMsg.ParseMode = tgbotapi.ModeMarkdown
	_, err = bot.tgbot.Send(resultMsg)
	return err
}

// handleScoreMenu shows the scoring menu for the user.
// Usage: /score
func (bot *Bot) handleScoreMenu(ctx context.Context, chatID int64, msg *tgbotapi.Message) error {
	telegramID := msg.From.ID

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.sendReply(chatID,
				"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
		}
		return bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка: %v", err))
	}

	// Get user's teams
	teams, err := bot.repo.GetTeamsByUserTelegramID(ctx, telegramID)
	if err != nil || len(teams) == 0 {
		return bot.sendReply(chatID,
			"❌ Вы не состоите ни в одной команде.")
	}

	// Build inline keyboard with teams
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, team := range teams {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("👥 %s", team.Name),
			fmt.Sprintf("team_%s", team.ID.String()))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	replyMsg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("👤 %s %s, выберите команду:",
			user.FirstName, user.LastName))
	replyMsg.ReplyMarkup = keyboard
	_, err = bot.tgbot.Send(replyMsg)
	return err
}
