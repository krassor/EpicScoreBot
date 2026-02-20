package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// handleCallbackQuery dispatches inline keyboard callbacks.
func (bot *Bot) handleCallbackQuery(update *tgbotapi.Update) {
	op := "telegram.handleCallbackQuery"
	log := bot.log.With(slog.String("op", op))

	if update.CallbackQuery == nil {
		return
	}

	callback := update.CallbackQuery
	data := callback.Data

	// Acknowledge the callback immediately
	ack := tgbotapi.NewCallback(callback.ID, "")
	ack.ShowAlert = false
	if _, err := bot.tgbot.Request(ack); err != nil {
		log.Error("failed to ack callback", sl.Err(err))
	}

	ctx, cancel := context.WithTimeout(bot.ctx, 30*time.Second)
	defer cancel()

	chatID := callback.Message.Chat.ID
	telegramID := callback.From.ID

	switch {
	// team_<teamID> — show team's unscored epics
	case strings.HasPrefix(data, "team_"):
		teamIDStr := strings.TrimPrefix(data, "team_")
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID команды")
			return
		}
		bot.showTeamEpics(ctx, chatID, telegramID, teamID)

	// epic_<epicID> — show scoring options for an epic
	case strings.HasPrefix(data, "epic_"):
		epicIDStr := strings.TrimPrefix(data, "epic_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		bot.showEpicScoreOptions(ctx, chatID, telegramID, epicID)

	// score_epic_<epicID>_<value> — submit epic score
	case strings.HasPrefix(data, "score_epic_"):
		bot.handleEpicScoreSubmit(ctx, chatID, telegramID, data)

	// risks_<epicID> — show unscored risks for epic
	case strings.HasPrefix(data, "risks_"):
		epicIDStr := strings.TrimPrefix(data, "risks_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		bot.showEpicRisks(ctx, chatID, telegramID, epicID)

	// risk_<riskID> — show risk scoring form
	case strings.HasPrefix(data, "risk_") && !strings.HasPrefix(data, "riskprob_") && !strings.HasPrefix(data, "riskimp_"):
		riskIDStr := strings.TrimPrefix(data, "risk_")
		riskID, err := uuid.Parse(riskIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID риска")
			return
		}
		bot.showRiskScoreForm(ctx, chatID, riskID)

	// riskprob_<riskID>_<value> — submit risk probability (step 1),
	// then show impact buttons
	case strings.HasPrefix(data, "riskprob_"):
		bot.handleRiskProbability(ctx, chatID, telegramID, data)

	// riskimp_<riskID>_<prob>_<value> — submit risk impact (step 2)
	case strings.HasPrefix(data, "riskimp_"):
		bot.handleRiskImpact(ctx, chatID, telegramID, data)

	default:
		log.Warn("unknown callback data", slog.String("data", data))
	}
}

// showTeamEpics shows the list of unscored SCORING epics for the user in a team.
func (bot *Bot) showTeamEpics(ctx context.Context, chatID, telegramID int64, teamID uuid.UUID) {
	op := "bot.showTeamEpics()"
	log := bot.log.With(
		slog.String("op", op),
	)

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		log.Error("failed to find user", sl.Err(botErr))
		return
	}

	epics, err := bot.repo.GetUnscoredEpicsByUser(ctx, user.ID, teamID)
	if err != nil {
		botErr := bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
		log.Error("failed to get unscored epics", sl.Err(botErr))
		return
	}

	team, _ := bot.repo.GetTeamByID(ctx, teamID)
	teamName := "команда"
	if team != nil {
		teamName = team.Name
	}

	if len(epics) == 0 {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("✅ В команде «%s» нет неоценённых эпиков.", teamName))
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, epic := range epics {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("📝 #%s %s", epic.Number, epic.Name),
			fmt.Sprintf("epic_%s", epic.ID.String()))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("📋 Неоценённые эпики в команде «%s»:", teamName))
	msg.ReplyMarkup = keyboard
	_, botErr := bot.tgbot.Send(msg)
	log.Error("failed to send message", sl.Err(botErr))
}

// showEpicScoreOptions shows the score buttons (1–100) and Risks button.
func (bot *Bot) showEpicScoreOptions(ctx context.Context, chatID, telegramID int64, epicID uuid.UUID) {
	op := "bot.showEpicScoreOptions()"
	log := bot.log.With(
		slog.String("op", op),
	)

	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Эпик не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	// Check if already scored
	scored, err := bot.repo.HasUserScoredEpic(ctx, epicID, user.ID)
	if err == nil && scored {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("✅ Вы уже оценили эпик #%s.", epic.Number))
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	// Get user's role for this team
	roles, err := bot.repo.GetRolesByUserID(ctx, user.ID)
	if err != nil || len(roles) == 0 {
		botErr := bot.sendReply(chatID, "❌ У вас нет назначенной роли.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	roleName := roles[0].Name
	prefix := fmt.Sprintf("score_epic_%s_", epicID.String())

	// Score buttons: 1, 2, 3, 5, 8, 13, 21, 34, 55, 89
	fibValues := []int{1, 2, 3, 5, 8, 13, 21, 34, 55, 89}

	var btnRows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton
	for i, v := range fibValues {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			strconv.Itoa(v),
			fmt.Sprintf("%s%d", prefix, v))
		currentRow = append(currentRow, btn)
		if (i+1)%5 == 0 {
			btnRows = append(btnRows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		btnRows = append(btnRows, currentRow)
	}

	// Risks button
	risksBtn := tgbotapi.NewInlineKeyboardButtonData(
		"⚠️ Оценить риски",
		fmt.Sprintf("risks_%s", epicID.String()))
	btnRows = append(btnRows, tgbotapi.NewInlineKeyboardRow(risksBtn))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(btnRows...)
	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("📝 Эпик #%s «%s»\n\n%s\n\n"+
			"Ваша роль: *%s*\nВыберите оценку трудоёмкости:",
			epic.Number, epic.Name, epic.Description, roleName))
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	_, botErr := bot.tgbot.Send(msg)
	log.Error("failed to send message", sl.Err(botErr))
}

// handleEpicScoreSubmit processes an epic score submission.
// Format: score_epic_<epicID>_<value>
func (bot *Bot) handleEpicScoreSubmit(ctx context.Context, chatID, telegramID int64, data string) {
	op := "bot.handleEpicScoreSubmit()"
	log := bot.log.With(
		slog.String("op", op),
	)

	// Parse: score_epic_<uuid>_<int>
	trimmed := strings.TrimPrefix(data, "score_epic_")
	// Find the last underscore to separate UUID from value
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore < 0 {
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	epicIDStr := trimmed[:lastUnderscore]
	valueStr := trimmed[lastUnderscore+1:]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID эпика.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	score, err := strconv.Atoi(valueStr)
	if err != nil || score < 1 {
		botErr := bot.sendReply(chatID, "❌ Некорректная оценка.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	// Get user's role
	roles, err := bot.repo.GetRolesByUserID(ctx, user.ID)
	if err != nil || len(roles) == 0 {
		botErr := bot.sendReply(chatID, "❌ У вас нет назначенной роли.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}
	roleID := roles[0].ID

	if err := bot.repo.CreateEpicScore(ctx, epicID, user.ID, roleID, score); err != nil {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	epic, _ := bot.repo.GetEpicByID(ctx, epicID)
	epicNum := epicID.String()
	if epic != nil {
		epicNum = epic.Number
	}

	botErr := bot.sendReply(chatID,
		fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!",
			score, epicNum))
	log.Error("failed to send reply", sl.Err(botErr))

	// Try to auto-complete scoring
	if err := bot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
		bot.log.Error("failed to try complete epic scoring",
			slog.String("epicID", epicID.String()), sl.Err(err))
	}
}

// showEpicRisks shows unscored risks for an epic.
func (bot *Bot) showEpicRisks(ctx context.Context, chatID, telegramID int64, epicID uuid.UUID) {
	op := "bot.showEpicRisks()"
	log := bot.log.With(
		slog.String("op", op),
	)

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	risks, err := bot.repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)
	if err != nil {
		botErr := bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	if len(risks) == 0 {
		botErr := bot.sendReply(chatID, "✅ Все риски этого эпика уже оценены.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, risk := range risks {
		desc := risk.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("⚠️ %s", desc),
			fmt.Sprintf("risk_%s", risk.ID.String()))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(chatID,
		"⚠️ Неоценённые риски:\nВыберите риск для оценки:")
	msg.ReplyMarkup = keyboard
	_, botErr := bot.tgbot.Send(msg)
	log.Error("failed to send message", sl.Err(botErr))
}

// showRiskScoreForm shows probability buttons for a risk.
func (bot *Bot) showRiskScoreForm(ctx context.Context, chatID int64, riskID uuid.UUID) {
	op := "bot.showRiskScoreForm()"
	log := bot.log.With(
		slog.String("op", op),
	)

	risk, err := bot.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Риск не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	var probBtns []tgbotapi.InlineKeyboardButton
	for i := 1; i <= 4; i++ {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			strconv.Itoa(i),
			fmt.Sprintf("riskprob_%s_%d", riskID.String(), i))
		probBtns = append(probBtns, btn)
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(probBtns...),
	)
	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("⚠️ Риск: %s\n\n"+
			"Выберите *вероятность* риска (1–4):",
			risk.Description))
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	_, botErr := bot.tgbot.Send(msg)
	log.Error("failed to send message", sl.Err(botErr))
}

// handleRiskProbability processes risk probability selection.
// Format: riskprob_<riskID>_<value>
func (bot *Bot) handleRiskProbability(ctx context.Context, chatID, telegramID int64, data string) {
	op := "bot.handleRiskProbability()"
	log := bot.log.With(
		slog.String("op", op),
	)

	trimmed := strings.TrimPrefix(data, "riskprob_")
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore < 0 {
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	riskIDStr := trimmed[:lastUnderscore]
	probStr := trimmed[lastUnderscore+1:]

	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID риска.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	prob, err := strconv.Atoi(probStr)
	if err != nil || prob < 1 || prob > 4 {
		botErr := bot.sendReply(chatID, "❌ Вероятность должна быть от 1 до 4.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	// Show impact buttons, passing probability in callback data
	var impBtns []tgbotapi.InlineKeyboardButton
	for i := 1; i <= 4; i++ {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			strconv.Itoa(i),
			fmt.Sprintf("riskimp_%s_%d_%d", riskID.String(), prob, i))
		impBtns = append(impBtns, btn)
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(impBtns...),
	)

	risk, _ := bot.repo.GetRiskByID(ctx, riskID)
	desc := riskID.String()
	if risk != nil {
		desc = risk.Description
	}

	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("⚠️ Риск: %s\nВероятность: *%d*\n\n"+
			"Выберите *влияние* риска (1–4):",
			desc, prob))
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	_, botErr := bot.tgbot.Send(msg)
	log.Error("failed to send message", sl.Err(botErr))
}

// handleRiskImpact processes risk impact selection and saves the score.
// Format: riskimp_<riskID>_<probability>_<impact>
func (bot *Bot) handleRiskImpact(ctx context.Context, chatID, telegramID int64, data string) {
	op := "bot.handleRiskImpact()"
	log := bot.log.With(
		slog.String("op", op),
	)

	trimmed := strings.TrimPrefix(data, "riskimp_")

	// Parse: <uuid>_<prob>_<impact>
	// Find last two underscores
	parts := strings.Split(trimmed, "_")
	if len(parts) < 7 { // UUID has 5 parts separated by "-" → split by "_" gives uuid segments + prob + impact
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	// UUID is parts[0] through parts[4] joined by "-"
	// Actually, UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	// When split by "_", uuid parts are separated by "-", so the whole thing
	// is: <uuid>_<prob>_<impact> where uuid contains "-" not "_"
	// So we need a different approach

	// Let's find the last two underscores
	lastIdx := strings.LastIndex(trimmed, "_")
	if lastIdx < 0 {
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}
	impact, err := strconv.Atoi(trimmed[lastIdx+1:])
	if err != nil || impact < 1 || impact > 4 {
		botErr := bot.sendReply(chatID, "❌ Влияние должно быть от 1 до 4.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	rest := trimmed[:lastIdx]
	secondLastIdx := strings.LastIndex(rest, "_")
	if secondLastIdx < 0 {
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}
	prob, err := strconv.Atoi(rest[secondLastIdx+1:])
	if err != nil || prob < 1 || prob > 4 {
		botErr := bot.sendReply(chatID, "❌ Вероятность должна быть от 1 до 4.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	riskIDStr := rest[:secondLastIdx]
	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID риска.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, telegramID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	if err := bot.repo.CreateRiskScore(ctx, riskID, user.ID, prob, impact); err != nil {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка сохранения оценки риска: %v", err))
		log.Error("failed to send reply", sl.Err(botErr))
		return
	}

	riskScore := prob * impact
	coeff := scoring.RiskCoefficient(float64(riskScore))

	botErr := bot.sendReply(chatID,
		fmt.Sprintf("✅ Оценка риска сохранена!\n"+
			"Вероятность: %d, Влияние: %d\n"+
			"Результат: %d (коэфф: %.2f)",
			prob, impact, riskScore, coeff))
	log.Error("failed to send reply", sl.Err(botErr))

	// Try to auto-complete risk scoring
	if err := bot.scoring.TryCompleteRiskScoring(ctx, riskID); err != nil {
		bot.log.Error("failed to try complete risk scoring",
			slog.String("riskID", riskID.String()), sl.Err(err))
	}
}

// sendCallbackAlert sends a popup alert to a callback.
func (bot *Bot) sendCallbackAlert(callback *tgbotapi.CallbackQuery, text string) {
	op := "bot.sendCallbackAlert()"
	log := bot.log.With(
		slog.String("op", op),
	)

	alert := tgbotapi.NewCallback(callback.ID, text)
	alert.ShowAlert = true
	_, botErr := bot.tgbot.Request(alert)
	log.Error("failed to send callback alert", sl.Err(botErr))
}
