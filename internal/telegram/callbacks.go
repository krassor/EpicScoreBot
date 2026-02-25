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
	username := callback.From.UserName

	switch {
	// ── User scoring flows ──────────────────────────────────────────────────

	// team_<teamID> — show team's unscored epics
	case strings.HasPrefix(data, "team_"):
		teamIDStr := strings.TrimPrefix(data, "team_")
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID команды")
			return
		}
		bot.showTeamEpics(ctx, chatID, username, teamID)

	// epic_<epicID> — show scoring options for an epic
	case strings.HasPrefix(data, "epic_"):
		epicIDStr := strings.TrimPrefix(data, "epic_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		bot.showEpicScoreOptions(ctx, chatID, username, epicID)

	// score_epic_<epicID>_<value> — submit epic score
	case strings.HasPrefix(data, "score_epic_"):
		bot.handleEpicScoreSubmit(ctx, chatID, username, data)

	// risks_<epicID> — show unscored risks for epic
	case strings.HasPrefix(data, "risks_"):
		epicIDStr := strings.TrimPrefix(data, "risks_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		bot.showEpicRisks(ctx, chatID, username, epicID)

	// risk_<riskID> — show risk scoring form
	case strings.HasPrefix(data, "risk_") && !strings.HasPrefix(data, "riskprob_") && !strings.HasPrefix(data, "riskimp_"):
		riskIDStr := strings.TrimPrefix(data, "risk_")
		riskID, err := uuid.Parse(riskIDStr)
		if err != nil {
			bot.sendCallbackAlert(callback, "❌ Ошибка парсинга ID риска")
			return
		}
		bot.showRiskScoreForm(ctx, chatID, riskID)

	// riskprob_<riskID>_<value> — submit risk probability (step 1)
	case strings.HasPrefix(data, "riskprob_"):
		bot.handleRiskProbability(ctx, chatID, data)

	// riskimp_<riskID>_<prob>_<value> — submit risk impact (step 2)
	case strings.HasPrefix(data, "riskimp_"):
		bot.handleRiskImpact(ctx, chatID, username, data)

	// ── Admin flows ─────────────────────────────────────────────────────────

	case data == "adm_cancel":
		bot.sessions.clear(chatID)
		bot.sendReply(chatID, "❌ Действие отменено.")

	// adm_user_<action>_<userID> — user selected in picker
	case strings.HasPrefix(data, "adm_user_"):
		bot.handleAdmUserSelected(ctx, chatID, callback, data)

	// adm_role_<action>_<userID>_<roleID> — role selected in picker
	case strings.HasPrefix(data, "adm_role_"):
		bot.handleAdmRoleSelected(ctx, chatID, callback, data)

	// adm_team_<action>_<...> — team selected in picker
	case strings.HasPrefix(data, "adm_team_"):
		bot.handleAdmTeamSelected(ctx, chatID, callback, data)

	// adm_epic_<action>_<epicID> — epic selected in picker
	case strings.HasPrefix(data, "adm_epic_"):
		bot.handleAdmEpicSelected(ctx, chatID, callback, data)

	// adm_risk_<action>_<epicID>_<riskID> — risk selected in picker
	case strings.HasPrefix(data, "adm_risk_"):
		bot.handleAdmRiskSelected(ctx, chatID, callback, data)

	// adm_confirm_<action>_<id> — confirm destructive action
	case strings.HasPrefix(data, "adm_confirm_"):
		bot.handleAdmConfirm(ctx, chatID, callback, data)

	// adm_deny_* — cancel destructive action
	case strings.HasPrefix(data, "adm_deny_"):
		bot.sessions.clear(chatID)
		bot.sendReply(chatID, "❌ Удаление отменено.")

	// epicstatus_<epicID> — handled in epic picker now via adm_epic_epicstatus_

	default:
		log.Warn("unknown callback data", slog.String("data", data))
	}
}

// showTeamEpics shows the list of unscored SCORING epics for the user in a team.
func (bot *Bot) showTeamEpics(ctx context.Context, chatID int64, username string, teamID uuid.UUID) {
	op := "bot.showTeamEpics()"
	log := bot.log.With(
		slog.String("op", op),
	)

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	epics, err := bot.repo.GetUnscoredEpicsByUser(ctx, user.ID, teamID)
	if err != nil {
		botErr := bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
	if botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// showEpicScoreOptions shows scoring options for a selected epic.
//
// Logic:
//   - If effort not yet scored → start a session and ask the user to type a
//     number (0–500). Validation and saving happen in handleSessionInput.
//   - If effort already scored but unscored risks remain → redirect to risk list.
//   - If both effort and all risks are scored → show "all done" message.
func (bot *Bot) showEpicScoreOptions(ctx context.Context, chatID int64, username string, epicID uuid.UUID) {
	op := "bot.showEpicScoreOptions()"
	log := bot.log.With(
		slog.String("op", op),
	)

	epic, err := bot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Эпик не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// Get user's role (required for effort scoring label).
	role, err := bot.repo.GetRoleByUserID(ctx, user.ID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ У вас нет назначенной роли.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// Check whether this user has already submitted an effort score.
	effortScored, _ := bot.repo.HasUserScoredEpic(ctx, epicID, user.ID)

	// Check whether there are any unscored risks for this user in this epic.
	unscoredRisks, _ := bot.repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)

	// Nothing left to score at all.
	if effortScored && len(unscoredRisks) == 0 {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("✅ Вы уже оценили эпик #%s и все его риски.", epic.Number))
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// Effort already scored but risks remain — go straight to risk list.
	if effortScored {
		bot.showEpicRisks(ctx, chatID, username, epicID)
		return
	}

	// Effort not yet scored — start a session and prompt for manual text input.
	bot.sessions.set(chatID, &Session{
		Step: StepScoreEpicEffort,
		Data: map[string]string{
			"epicID":   epicID.String(),
			"username": username,
		},
	})

	roleName := role.Name
	botErr := bot.sendReply(chatID,
		fmt.Sprintf("📝 Эпик #%s «%s»\n\n%s\n\nВаша роль: *%s*\n\nВведите оценку трудоёмкости (число от 0 до 500):",
			epic.Number, epic.Name, epic.Description, roleName))
	if botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}
}

// handleEpicScoreSubmit processes an epic score submission.
// Format: score_epic_<epicID>_<value>
func (bot *Bot) handleEpicScoreSubmit(ctx context.Context, chatID int64, username string, data string) {
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
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	epicIDStr := trimmed[:lastUnderscore]
	valueStr := trimmed[lastUnderscore+1:]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID эпика.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	score, err := strconv.Atoi(valueStr)
	if err != nil || score < 1 {
		botErr := bot.sendReply(chatID, "❌ Некорректная оценка.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// Get user's role
	role, err := bot.repo.GetRoleByUserID(ctx, user.ID)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ У вас нет назначенной роли.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}
	roleID := role.ID

	if err := bot.repo.CreateEpicScore(ctx, epicID, user.ID, roleID, score); err != nil {
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
	if botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}

	// Try to auto-complete scoring
	if err := bot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
		bot.log.Error("failed to try complete epic scoring",
			slog.String("epicID", epicID.String()), sl.Err(err))
	}
}

// showEpicRisks shows unscored risks for an epic.
func (bot *Bot) showEpicRisks(ctx context.Context, chatID int64, username string, epicID uuid.UUID) {
	op := "bot.showEpicRisks()"
	log := bot.log.With(
		slog.String("op", op),
	)

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	risks, err := bot.repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)
	if err != nil {
		botErr := bot.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if len(risks) == 0 {
		botErr := bot.sendReply(chatID, "✅ Все риски этого эпика уже оценены.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
	if botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
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
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
	if botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// handleRiskProbability processes risk probability selection.
// Format: riskprob_<riskID>_<value>
func (bot *Bot) handleRiskProbability(ctx context.Context, chatID int64, data string) {
	op := "bot.handleRiskProbability()"
	log := bot.log.With(
		slog.String("op", op),
	)

	trimmed := strings.TrimPrefix(data, "riskprob_")
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore < 0 {
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskIDStr := trimmed[:lastUnderscore]
	probStr := trimmed[lastUnderscore+1:]

	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID риска.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	prob, err := strconv.Atoi(probStr)
	if err != nil || prob < 1 || prob > 4 {
		botErr := bot.sendReply(chatID, "❌ Вероятность должна быть от 1 до 4.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
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
	if botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// handleRiskImpact processes risk impact selection and saves the score.
// Format: riskimp_<riskID>_<probability>_<impact>
func (bot *Bot) handleRiskImpact(ctx context.Context, chatID int64, username string, data string) {
	op := "bot.handleRiskImpact()"
	log := bot.log.With(
		slog.String("op", op),
	)

	log.Debug(
		"input data",
		slog.String("data", data),
	)

	trimmed := strings.TrimPrefix(data, "riskimp_")

	// Parse: <uuid>_<prob>_<impact>
	// Find last two underscores
	parts := strings.Split(trimmed, "_")
	if len(parts) != 3 { // UUID has 5 parts separated by "-" → split by "_" gives uuid segments + prob + impact
		log.Error("invalid callback data format", slog.String("len(parts) != 3", data))
		botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// UUID is parts[0] through parts[4] joined by "-"
	// Actually, UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	// When split by "_", uuid parts are separated by "-", so the whole thing
	// is: <uuid>_<prob>_<impact> where uuid contains "-" not "_"
	// So we need a different approach

	// Let's find the last two underscores
	// lastIdx := strings.LastIndex(trimmed, "_")
	// if lastIdx < 0 {
	// 	botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
	// 	if botErr != nil {
	// 		log.Error("failed to send reply", sl.Err(botErr))
	// 	}
	// 	return
	// }
	impact, err := strconv.Atoi(parts[2])
	if err != nil || impact < 1 || impact > 4 {
		log.Error("invalid impact", slog.String("impact", parts[2]))
		botErr := bot.sendReply(chatID, "❌ Влияние должно быть от 1 до 4.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	// rest := trimmed[:lastIdx]
	// secondLastIdx := strings.LastIndex(rest, "_")
	// if secondLastIdx < 0 {
	// 	botErr := bot.sendReply(chatID, "❌ Некорректные данные.")
	// 	if botErr != nil {
	// 		log.Error("failed to send reply", sl.Err(botErr))
	// 	}
	// 	return
	// }
	prob, err := strconv.Atoi(parts[1])
	if err != nil || prob < 1 || prob > 4 {
		log.Error("invalid probability", slog.String("prob", parts[1]))
		botErr := bot.sendReply(chatID, "❌ Вероятность должна быть от 1 до 4.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskIDStr := parts[0]
	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		log.Error("invalid risk id", slog.String("risk_id", riskIDStr))
		botErr := bot.sendReply(chatID, "❌ Ошибка парсинга ID риска.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := bot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		log.Error("user not found", slog.String("username", username))
		botErr := bot.sendReply(chatID, "❌ Пользователь не найден.")
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if err := bot.repo.CreateRiskScore(ctx, riskID, user.ID, prob, impact); err != nil {
		log.Error("failed to create risk score", sl.Err(err))
		botErr := bot.sendReply(chatID,
			fmt.Sprintf("❌ Ошибка сохранения оценки риска: %v", err))
		if botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskScore := prob * impact
	coeff := scoring.RiskCoefficient(float64(riskScore))

	botErr := bot.sendReply(chatID,
		fmt.Sprintf("✅ Оценка риска сохранена!\n"+
			"Вероятность: %d, Влияние: %d\n"+
			"Результат: %d (коэфф: %.2f)",
			prob, impact, riskScore, coeff))
	if botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}

	// Try to auto-complete risk scoring
	if err := bot.scoring.TryCompleteRiskScoring(ctx, riskID); err != nil {
		log.Error(
			"failed to try complete risk scoring",
			slog.String("riskID", riskID.String()),
			sl.Err(err))
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
	if botErr != nil {
		log.Error("failed to send callback alert", sl.Err(botErr))
	}
}
