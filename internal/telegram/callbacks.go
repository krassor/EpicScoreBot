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

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// handleCallbackQuery dispatches inline keyboard callbacks.
func (epicBot *Bot) handleCallbackQuery(ctx context.Context, update *models.Update) {
	op := "telegram.handleCallbackQuery"
	log := epicBot.log.With(slog.String("op", op))

	if update.CallbackQuery == nil {
		return
	}

	callback := update.CallbackQuery
	data := callback.Data

	// Acknowledge the callback immediately.
	if _, err := epicBot.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		ShowAlert:       false,
	}); err != nil {
		log.Error("failed to ack callback", sl.Err(err))
	}

	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	chatID := callback.Message.Message.Chat.ID
	threadID := callback.Message.Message.MessageThreadID
	username := callback.From.Username

	switch {
	// ── User scoring flows ──────────────────────────────────────────────────

	// team_<teamID> — show team's unscored epics
	case strings.HasPrefix(data, "team_"):
		teamIDStr := strings.TrimPrefix(data, "team_")
		teamID, err := uuid.Parse(teamIDStr)
		if err != nil {
			epicBot.sendCallbackAlert(rctx, callback, "❌ Ошибка парсинга ID команды")
			return
		}
		epicBot.showTeamEpics(rctx, chatID, threadID, username, teamID)

	// epic_<epicID> — show scoring options for an epic
	case strings.HasPrefix(data, "epic_"):
		epicIDStr := strings.TrimPrefix(data, "epic_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.sendCallbackAlert(rctx, callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		epicBot.showEpicScoreOptions(rctx, chatID, threadID, username, epicID)

	// score_epic_<epicID>_<value> — submit epic score
	case strings.HasPrefix(data, "score_epic_"):
		epicBot.handleEpicScoreSubmit(rctx, chatID, threadID, username, data)

	// risks_<epicID> — show unscored risks for epic
	case strings.HasPrefix(data, "risks_"):
		epicIDStr := strings.TrimPrefix(data, "risks_")
		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.sendCallbackAlert(rctx, callback, "❌ Ошибка парсинга ID эпика")
			return
		}
		epicBot.showEpicRisks(rctx, chatID, threadID, username, epicID)

	// risk_<riskID> — show risk scoring form
	case strings.HasPrefix(data, "risk_") &&
		!strings.HasPrefix(data, "riskprob_") &&
		!strings.HasPrefix(data, "riskimp_"):
		riskIDStr := strings.TrimPrefix(data, "risk_")
		riskID, err := uuid.Parse(riskIDStr)
		if err != nil {
			epicBot.sendCallbackAlert(rctx, callback, "❌ Ошибка парсинга ID риска")
			return
		}
		epicBot.showRiskScoreForm(rctx, chatID, threadID, riskID)

	// riskprob_<riskID>_<value> — submit risk probability (step 1)
	case strings.HasPrefix(data, "riskprob_"):
		epicBot.handleRiskProbability(rctx, chatID, threadID, data)

	// riskimp_<riskID>_<prob>_<value> — submit risk impact (step 2)
	case strings.HasPrefix(data, "riskimp_"):
		epicBot.handleRiskImpact(rctx, chatID, threadID, username, data)

	// ── Admin flows ─────────────────────────────────────────────────────────

	case data == "adm_cancel":
		epicBot.sessions.clear(chatID)
		epicBot.sendReply(rctx, chatID, threadID, "❌ Действие отменено.")

	// adm_user_<action>_<userID> — user selected in picker
	case strings.HasPrefix(data, "adm_user_"):
		epicBot.handleAdmUserSelected(rctx, chatID, threadID, callback, data)

	// adm_role_<action>_<roleID> — role selected in picker
	case strings.HasPrefix(data, "adm_role_"):
		epicBot.handleAdmRoleSelected(rctx, chatID, threadID, callback, data)

	// adm_team_<action>_<...> — team selected in picker
	case strings.HasPrefix(data, "adm_team_"):
		epicBot.handleAdmTeamSelected(rctx, chatID, threadID, callback, data)

	// adm_epic_<action>_<epicID> — epic selected in picker
	case strings.HasPrefix(data, "adm_epic_"):
		epicBot.handleAdmEpicSelected(rctx, chatID, threadID, callback, data)

	// adm_risk_<action>_<epicID>_<riskID> — risk selected in picker
	case strings.HasPrefix(data, "adm_risk_"):
		epicBot.handleAdmRiskSelected(rctx, chatID, threadID, callback, data)

	// adm_confirm_<action>_<id> — confirm destructive action
	case strings.HasPrefix(data, "adm_confirm_"):
		epicBot.handleAdmConfirm(rctx, chatID, threadID, callback, data)

	// adm_deny_* — cancel destructive action
	case strings.HasPrefix(data, "adm_deny_"):
		epicBot.sessions.clear(chatID)
		epicBot.sendReply(rctx, chatID, threadID, "❌ Удаление отменено.")

	default:
		log.Warn("unknown callback data", slog.String("data", data))
	}
}

// showTeamEpics shows the list of unscored SCORING epics for the user in a team.
func (epicBot *Bot) showTeamEpics(
	ctx context.Context,
	chatID int64,
	threadID int,
	username string,
	teamID uuid.UUID,
) {
	op := "bot.showTeamEpics()"
	log := epicBot.log.With(slog.String("op", op))

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	epics, err := epicBot.repo.GetUnscoredEpicsByUser(ctx, user.ID, teamID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка: %v", err)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	team, _ := epicBot.repo.GetTeamByID(ctx, teamID)
	teamName := "команда"
	if team != nil {
		teamName = team.Name
	}

	if len(epics) == 0 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ В команде «%s» нет неоценённых эпиков.", teamName)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	var rows [][]models.InlineKeyboardButton
	for _, epic := range epics {
		rows = append(rows, inlineRow(inlineBtn(
			fmt.Sprintf("📝 #%s %s", epic.Number, epic.Name),
			fmt.Sprintf("epic_%s", epic.ID.String()),
		)))
	}
	kb := inlineKeyboard(rows...)

	if botErr := epicBot.sendWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("📋 Неоценённые эпики в команде «%s»:", teamName), kb); botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// showEpicScoreOptions shows scoring options for a selected epic.
func (epicBot *Bot) showEpicScoreOptions(
	ctx context.Context,
	chatID int64,
	threadID int,
	username string,
	epicID uuid.UUID,
) {
	op := "bot.showEpicScoreOptions()"
	log := epicBot.log.With(slog.String("op", op))

	epic, err := epicBot.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Эпик не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ У вас нет назначенной роли."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	effortScored, _ := epicBot.repo.HasUserScoredEpic(ctx, epicID, user.ID)
	unscoredRisks, _ := epicBot.repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)

	if effortScored && len(unscoredRisks) == 0 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("✅ Вы уже оценили эпик #%s и все его риски.", epic.Number)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if effortScored {
		epicBot.showEpicRisks(ctx, chatID, threadID, username, epicID)
		return
	}

	// Start a session and prompt for manual text input.
	epicBot.sessions.set(chatID, &Session{
		Step:     StepScoreEpicEffort,
		ThreadID: threadID,
		Data: map[string]string{
			"epicID":   epicID.String(),
			"username": username,
		},
	})

	if botErr := epicBot.sendMarkdown(ctx, chatID, threadID,
		fmt.Sprintf("📝 Эпик #%s «%s»\n\n%s\n\nВаша роль: *%s*\n\nВведите оценку трудоёмкости (число от 0 до 500):",
			epic.Number, epic.Name, epic.Description, role.Name)); botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}
}

// handleEpicScoreSubmit processes an epic score submission.
// Format: score_epic_<epicID>_<value>
func (epicBot *Bot) handleEpicScoreSubmit(
	ctx context.Context,
	chatID int64,
	threadID int,
	username string,
	data string,
) {
	op := "bot.handleEpicScoreSubmit()"
	log := epicBot.log.With(slog.String("op", op))

	trimmed := strings.TrimPrefix(data, "score_epic_")
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore < 0 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	epicIDStr := trimmed[:lastUnderscore]
	valueStr := trimmed[lastUnderscore+1:]

	epicID, err := uuid.Parse(epicIDStr)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID эпика."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	score, err := strconv.Atoi(valueStr)
	if err != nil || score < 1 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректная оценка."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	role, err := epicBot.repo.GetRoleByUserID(ctx, user.ID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ У вас нет назначенной роли."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if err := epicBot.repo.CreateEpicScore(ctx, epicID, user.ID, role.ID, score); err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	epic, _ := epicBot.repo.GetEpicByID(ctx, epicID)
	epicNum := epicID.String()
	if epic != nil {
		epicNum = epic.Number
	}

	if botErr := epicBot.sendReply(ctx, chatID, threadID,
		fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!", score, epicNum)); botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}

	if err := epicBot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
		epicBot.log.Error("failed to try complete epic scoring",
			slog.String("epicID", epicID.String()), sl.Err(err))
	}
}

// showEpicRisks shows unscored risks for an epic.
func (epicBot *Bot) showEpicRisks(
	ctx context.Context,
	chatID int64,
	threadID int,
	username string,
	epicID uuid.UUID,
) {
	op := "bot.showEpicRisks()"
	log := epicBot.log.With(slog.String("op", op))

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	risks, err := epicBot.repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, fmt.Sprintf("❌ Ошибка: %v", err)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if len(risks) == 0 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "✅ Все риски этого эпика уже оценены."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	var rows [][]models.InlineKeyboardButton
	for _, risk := range risks {
		desc := risk.Description
		if len([]rune(desc)) > 50 {
			desc = string([]rune(desc)[:47]) + "..."
		}
		rows = append(rows, inlineRow(inlineBtn(
			fmt.Sprintf("⚠️ %s", desc),
			fmt.Sprintf("risk_%s", risk.ID.String()),
		)))
	}
	kb := inlineKeyboard(rows...)

	if botErr := epicBot.sendWithKeyboard(ctx, chatID, threadID,
		"⚠️ Неоценённые риски:\nВыберите риск для оценки:", kb); botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// showRiskScoreForm shows probability buttons for a risk.
func (epicBot *Bot) showRiskScoreForm(ctx context.Context, chatID int64, threadID int, riskID uuid.UUID) {
	op := "bot.showRiskScoreForm()"
	log := epicBot.log.With(slog.String("op", op))

	risk, err := epicBot.repo.GetRiskByID(ctx, riskID)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Риск не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	var probBtns []models.InlineKeyboardButton
	for i := 1; i <= 4; i++ {
		probBtns = append(probBtns, inlineBtn(
			strconv.Itoa(i),
			fmt.Sprintf("riskprob_%s_%d", riskID.String(), i),
		))
	}
	kb := inlineKeyboard(inlineRow(probBtns...))

	if botErr := epicBot.sendMarkdownWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("⚠️ Риск: %s\n\nВыберите *вероятность* риска (1–4):", risk.Description),
		kb); botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// handleRiskProbability processes risk probability selection.
// Format: riskprob_<riskID>_<value>
func (epicBot *Bot) handleRiskProbability(ctx context.Context, chatID int64, threadID int, data string) {
	op := "bot.handleRiskProbability()"
	log := epicBot.log.With(slog.String("op", op))

	trimmed := strings.TrimPrefix(data, "riskprob_")
	lastUnderscore := strings.LastIndex(trimmed, "_")
	if lastUnderscore < 0 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskIDStr := trimmed[:lastUnderscore]
	probStr := trimmed[lastUnderscore+1:]

	riskID, err := uuid.Parse(riskIDStr)
	if err != nil {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID риска."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	prob, err := strconv.Atoi(probStr)
	if err != nil || prob < 1 || prob > 4 {
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Вероятность должна быть от 1 до 4."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	var impBtns []models.InlineKeyboardButton
	for i := 1; i <= 4; i++ {
		impBtns = append(impBtns, inlineBtn(
			strconv.Itoa(i),
			fmt.Sprintf("riskimp_%s_%d_%d", riskID.String(), prob, i),
		))
	}
	kb := inlineKeyboard(inlineRow(impBtns...))

	risk, _ := epicBot.repo.GetRiskByID(ctx, riskID)
	desc := riskID.String()
	if risk != nil {
		desc = risk.Description
	}

	if botErr := epicBot.sendMarkdownWithKeyboard(ctx, chatID, threadID,
		fmt.Sprintf("⚠️ Риск: %s\nВероятность: *%d*\n\nВыберите *влияние* риска (1–4):", desc, prob),
		kb); botErr != nil {
		log.Error("failed to send message", sl.Err(botErr))
	}
}

// handleRiskImpact processes risk impact selection and saves the score.
// Format: riskimp_<riskID>_<probability>_<impact>
func (epicBot *Bot) handleRiskImpact(
	ctx context.Context,
	chatID int64,
	threadID int,
	username string,
	data string,
) {
	op := "bot.handleRiskImpact()"
	log := epicBot.log.With(slog.String("op", op))
	log.Debug("input data", slog.String("data", data))

	trimmed := strings.TrimPrefix(data, "riskimp_")
	parts := strings.Split(trimmed, "_")
	if len(parts) != 3 {
		log.Error("invalid callback data format", slog.String("data", data))
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Некорректные данные."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	impact, err := strconv.Atoi(parts[2])
	if err != nil || impact < 1 || impact > 4 {
		log.Error("invalid impact", slog.String("impact", parts[2]))
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Влияние должно быть от 1 до 4."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	prob, err := strconv.Atoi(parts[1])
	if err != nil || prob < 1 || prob > 4 {
		log.Error("invalid probability", slog.String("prob", parts[1]))
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Вероятность должна быть от 1 до 4."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskID, err := uuid.Parse(parts[0])
	if err != nil {
		log.Error("invalid risk id", slog.String("risk_id", parts[0]))
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Ошибка парсинга ID риска."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
	if err != nil {
		log.Error("user not found", slog.String("username", username))
		if botErr := epicBot.sendReply(ctx, chatID, threadID, "❌ Пользователь не найден."); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	if err := epicBot.repo.CreateRiskScore(ctx, riskID, user.ID, prob, impact); err != nil {
		log.Error("failed to create risk score", sl.Err(err))
		if botErr := epicBot.sendReply(ctx, chatID, threadID,
			fmt.Sprintf("❌ Ошибка сохранения оценки риска: %v", err)); botErr != nil {
			log.Error("failed to send reply", sl.Err(botErr))
		}
		return
	}

	riskScore := prob * impact
	coeff := scoring.RiskCoefficient(float64(riskScore))

	if botErr := epicBot.sendReply(ctx, chatID, threadID,
		fmt.Sprintf("✅ Оценка риска сохранена!\nВероятность: %d, Влияние: %d\nРезультат: %d (коэфф: %.2f)",
			prob, impact, riskScore, coeff)); botErr != nil {
		log.Error("failed to send reply", sl.Err(botErr))
	}

	if err := epicBot.scoring.TryCompleteRiskScoring(ctx, riskID); err != nil {
		log.Error("failed to try complete risk scoring",
			slog.String("riskID", riskID.String()), sl.Err(err))
	}
}

// sendCallbackAlert sends a popup alert to a callback query.
func (epicBot *Bot) sendCallbackAlert(ctx context.Context, callback *models.CallbackQuery, text string) {
	op := "bot.sendCallbackAlert()"
	log := epicBot.log.With(slog.String("op", op))

	if _, err := epicBot.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            text,
		ShowAlert:       true,
	}); err != nil {
		log.Error("failed to send callback alert", sl.Err(err))
	}
}
