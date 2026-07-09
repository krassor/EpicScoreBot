package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

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
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showEpicPickerInitial(ctx, msg, "epicstatus", "")
}

// ─── /report — inline keyboard ───────────────────────────────────────────

func (epicBot *Bot) handleReport(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Эта команда доступна только администраторам.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "report")
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

// ─── /list ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleList(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	return epicBot.showTeamPickerInitial(ctx, msg, "list")
}

// ─── /epicnotify ──────────────────────────────────────────────────────────

func (epicBot *Bot) handleEpicNotify(ctx context.Context, msg *models.Message) error {
	if !epicBot.isAdmin(msg) {
		_, err := epicBot.sendReply(ctx, msg, "⛔ Только для администраторов.")
		return err
	}
	// The user requested standard flow like "/epicresult". /results uses showEpicPickerInitial directly.
	return epicBot.showEpicPickerInitial(ctx, msg, "epicnotify", string(domain.StatusScoring))
}

// ─── /epicinfo ────────────────────────────────────────────────────────────

func (epicBot *Bot) handleEpicInfo(ctx context.Context, msg *models.Message) error {
	op := "bot.handleEpicInfo"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
	)

	if msg.Chat.Type != models.ChatTypePrivate {
		botName := "EpicScoreBot"
		if epicBot.botUsername != "" {
			botName = epicBot.botUsername
		}
		sentMsg, err := epicBot.sendReply(ctx, msg, "Информация по эпикам доступна в личном чате @"+botName)
		if err == nil && sentMsg != nil {
			go func(chatID int64, messageID int) {
				time.Sleep(5 * time.Second)
				_ = epicBot.deleteMessage(context.Background(), chatID, messageID)
			}(msg.Chat.ID, sentMsg.ID)
		}
	}

	username := msg.From.Username
	if username == "" {
		_, err := epicBot.sendReply(ctx, msg,
			"❌ У вас не задан @username в Telegram. Установите его в настройках профиля.")
		return err
	}

	user, err := epicBot.userService.FindUserByTelegramID(ctx, username)
	if err != nil {
		_, retErr := epicBot.sendReply(ctx, msg,
			"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
		return retErr
	}

	if user.ChatID != msg.From.ID {
		if err := epicBot.userService.UpdateUserChatID(ctx, user.ID, msg.From.ID); err != nil {
			log.Error("failed to update user ChatID", sl.Err(err))
		}
	}

	teams, err := epicBot.teamService.GetTeamsByUserTelegramID(ctx, username)
	if err != nil || len(teams) == 0 {
		if err != nil {
			log.Error("error getting teams by user telegram id", sl.Err(err))
		}
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Вы не состоите ни в одной команде.")
		return retErr
	}

	allEpics, err := epicBot.epicService.GetUnscoredEpicsForUserAcrossTeams(ctx, user.ID, username)
	if err != nil {
		log.Error("error getting unscored epics", sl.Err(err))
		_, retErr := epicBot.sendReply(ctx, msg, "❌ Ошибка получения неоценённых эпиков.")
		return retErr
	}

	if len(allEpics) == 0 {
		_, retErr := epicBot.sendReply(ctx, msg, "✅ У вас нет неоценённых эпиков.")
		return retErr
	}

	var rows [][]models.InlineKeyboardButton
	for _, epic := range allEpics {
		rows = append(rows, inlineRow(inlineBtn(
			fmt.Sprintf("📝 #%s %s", epic.Number, epic.Name),
			fmt.Sprintf("epicinfo_%s", epic.ID.String()),
		)))
	}
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "score_cancel")))
	kb := inlineKeyboard(rows...)

	var retErr error
	if msg.Chat.Type != models.ChatTypePrivate {
		_, retErr = epicBot.sendWithKeyboardToUser(ctx, msg,
			fmt.Sprintf("📋 %s %s, ваши неоценённые эпики:", user.FirstName, user.LastName), kb)
	} else {
		_, retErr = epicBot.sendWithKeyboard(ctx, msg,
			fmt.Sprintf("📋 %s %s, ваши неоценённые эпики:", user.FirstName, user.LastName), kb)
	}
	return retErr
}

// ─── /score ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleScoreMenu(ctx context.Context, msg *models.Message) error {
	op := "bot.handleScoreMenu"
	log := epicBot.log.With(
		slog.String("op", op),
		slog.Int64("chat_id", msg.Chat.ID),
	)

	if msg.Chat.Type != models.ChatTypePrivate {
		botName := "EpicScoreBot"
		if epicBot.botUsername != "" {
			botName = epicBot.botUsername
		}
		sentMsg, err := epicBot.sendReply(ctx, msg, "Оценка проводится в личном чате @"+botName)
		if err == nil && sentMsg != nil {
			go func(chatID int64, messageID int) {
				time.Sleep(5 * time.Second)
				_ = epicBot.deleteMessage(context.Background(), chatID, messageID)
			}(msg.Chat.ID, sentMsg.ID)
		}
	}

	username := msg.From.Username
	if username == "" {
		_, err := epicBot.sendReply(ctx, msg,
			"❌ У вас не задан @username в Telegram. Установите его в настройках профиля.")
		return err
	}

	user, err := epicBot.userService.FindUserByTelegramID(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, retErr := epicBot.sendReply(ctx, msg,
				"❌ Вы не зарегистрированы в системе. Обратитесь к администратору.")
			return retErr
		}
		_, retErr := epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка: %v", err))
		return retErr
	}

	if user.ChatID != msg.From.ID {
		if err := epicBot.userService.UpdateUserChatID(ctx, user.ID, msg.From.ID); err != nil {
			log.Error("failed to update user ChatID", sl.Err(err))
		}
	}

	teams, err := epicBot.teamService.GetTeamsByUserTelegramID(ctx, username)
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
	rows = append(rows, inlineRow(inlineBtn("❌ Отмена", "score_cancel")))
	kb := inlineKeyboard(rows...)

	var retErr error
	if msg.Chat.Type != models.ChatTypePrivate {
		_, retErr = epicBot.sendWithKeyboardToUser(ctx, msg,
			fmt.Sprintf("👤 %s %s, выберите команду:", user.FirstName, user.LastName), kb)
	} else {
		_, retErr = epicBot.sendWithKeyboard(ctx, msg,
			fmt.Sprintf("👤 %s %s, выберите команду:", user.FirstName, user.LastName), kb)
	}
	return retErr
}

// ─── /results logic (called by callback) ──────────────────────────────────

func (epicBot *Bot) showEpicResults(ctx context.Context, msg *models.Message, epicID uuid.UUID) {
	// Attempt to compute risk scores first
	risks, err := epicBot.riskService.GetRisksByEpicID(ctx, epicID)
	if err == nil {
		for _, risk := range risks {
			if risk.Status != domain.StatusScored {
				_ = epicBot.scoring.TryCompleteRiskScoring(ctx, risk.ID)
			}
		}
	}

	// Attempt to compute epic score
	epicInit, errInit := epicBot.epicService.GetEpicByID(ctx, epicID)
	if errInit == nil && epicInit != nil && epicInit.Status != domain.StatusScored {
		_ = epicBot.scoring.TryCompleteEpicScoring(ctx, epicID)
	}

	// Now reload the epic to get the fresh status and scores
	epic, err := epicBot.epicService.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 *Результаты эпика \\#%s «%s»*\n", escapeMarkdownV2(epic.Number), escapeMarkdownV2(epic.Name))
	fmt.Fprintf(&sb, "Статус: %s\n\n", escapeMarkdownV2(string(epic.Status)))

	roleScores, err := epicBot.epicService.GetEpicRoleScoresByEpicID(ctx, epic.ID)
	if err == nil && len(roleScores) > 0 {
		sb.WriteString("📋 *Оценки по ролям:*\n")
		for _, rs := range roleScores {
			role, err := epicBot.roleService.GetRoleByID(ctx, rs.RoleID)
			roleName := rs.RoleID.String()
			if err == nil {
				roleName = role.Name
			}
			fmt.Fprintf(&sb, "  • %s: %s\n", escapeMarkdownV2(roleName), escapeMarkdownV2(fmt.Sprintf("%.2f", rs.WeightedAvg)))
		}
		sb.WriteString("\n")
	}

	risks, err = epicBot.riskService.GetRisksByEpicID(ctx, epic.ID)
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
	epic, err := epicBot.epicService.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}
	log.Debug(
		"epic found",
		slog.String("epic", epic.Number),
	)

	teamMembers, err := epicBot.userService.GetUsersByTeamID(ctx, epic.TeamID)
	if err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка получения участников: %v", err))
		return
	}

	evalRoleIDs, err := epicBot.epicService.GetEvaluatingRoleIDs(ctx, epic.ID)
	if err == nil && len(evalRoleIDs) > 0 {
		evalRoleSet := make(map[uuid.UUID]bool)
		for _, rid := range evalRoleIDs {
			evalRoleSet[rid] = true
		}

		var filteredMembers []domain.User
		for _, u := range teamMembers {
			role, err := epicBot.roleService.GetRoleByUserID(ctx, u.ID)
			if err == nil && role != nil && evalRoleSet[role.ID] {
				filteredMembers = append(filteredMembers, u)
			}
		}
		teamMembers = filteredMembers
	}

	log.Debug(
		"team members found",
		slog.Int("count", len(teamMembers)),
	)

	scoredEpic, _ := epicBot.epicService.GetUsersWhoScoredEpic(ctx, epic.ID)
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

	sb.WriteString("📋 *Трудоёмкость*\n")
	sb.WriteString("👉 Ждём оценку от:\n")
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

	risks, _ := epicBot.riskService.GetRisksByEpicID(ctx, epic.ID)
	if len(risks) > 0 {
		sb.WriteString("\n⚠️ *Риски:*\n")
		for _, risk := range risks {
			scoredRisk, _ := epicBot.riskService.GetUsersWhoScoredRisk(ctx, risk.ID)
			riskScoredSet := make(map[uuid.UUID]bool)
			for _, u := range scoredRisk {
				riskScoredSet[u.ID] = true
			}
			desc := risk.Description
			// if len([]rune(desc)) > 40 {
			// 	desc = string([]rune(desc)[:37]) + "..."
			// }
			fmt.Fprintf(&sb, "\n*%s*\n",
				escapeMarkdownV2(desc))
			sb.WriteString("👉 *Ждём оценку от:* ")
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

	sb.WriteString("\nДля оценки трудоёмкости и рисков используйте команду /score\n")

	log.Debug(
		"status report",
		slog.String("report", sb.String()),
	)

	epicBot.sendMarkdown(ctx, msg, sb.String())
}

// ─── /startscore execution (called by callback) ───────────────────────────

func (epicBot *Bot) execStartScore(ctx context.Context, msg *models.Message, epicID uuid.UUID) {
	epic, err := epicBot.epicService.GetEpicByID(ctx, epicID)
	if err != nil {
		epicBot.sendReply(ctx, msg, "❌ Эпик не найден.")
		return
	}
	if epic.Status != domain.StatusNew {
		epicBot.sendReply(ctx, msg,
			fmt.Sprintf("⚠️ Эпик #%s уже в статусе %s.", epic.Number, string(epic.Status)))
		return
	}
	if err := epicBot.epicService.UpdateEpicStatus(ctx, epic.ID, domain.StatusScoring); err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка смены статуса эпика: %v", err))
		return
	}
	risks, err := epicBot.riskService.GetRisksByEpicID(ctx, epic.ID)
	if err != nil {
		epicBot.sendReply(ctx, msg, fmt.Sprintf("❌ Ошибка получения рисков: %v", err))
		return
	}
	for _, risk := range risks {
		if err := epicBot.riskService.UpdateRiskStatus(ctx, risk.ID, domain.StatusScoring); err != nil {
			epicBot.log.Error("failed to update risk status",
				slog.String("riskID", risk.ID.String()), sl.Err(err))
		}
	}
	epicBot.sendReply(ctx, msg,
		fmt.Sprintf("🚀 Эпик #%s «%s» и %d рисков отправлены на оценку!",
			epic.Number, epic.Name, len(risks)))
}
