package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"EpicScoreBot/internal/services"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

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
		user, err := epicBot.userService.CreateUser(ctx,
			sess.Data["firstName"], sess.Data["lastName"],
			sess.Data["username"], weight)
		epicBot.sessions.clear(sk)
		if err != nil {
			if errors.Is(err, services.ErrUserAlreadyExists) {
				epicBot.deleteAndSend(ctx, msg, msgID, "❌ Пользователь с таким @username уже существует.")
				return
			}
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
		if err := epicBot.userService.UpdateUserName(ctx, userID, sess.Data["firstName"], text); err != nil {
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
		if err := epicBot.userService.UpdateUserWeight(ctx, userID, weight); err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, "❌ Ошибка изменения веса.")
			return
		}
		epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("✅ Вес пользователя изменён на %d", weight))

	// ── /addepic interactive steps ─────────────────────────────────────

	case StepAddEpicNumber:
		sess.Data["number"] = text
		epic, _ := epicBot.epicService.GetEpicByNumber(ctx, sess.Data["number"])
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

		epic, err := epicBot.epicService.CreateEpic(ctx, sess.Data["number"], sess.Data["name"], desc, teamID)
		if err != nil {
			if errors.Is(err, services.ErrEpicAlreadyExists) {
				epicBot.deleteAndSend(ctx, msg, msgID, "❌ Эпик с таким номером уже существует.")
				return
			}
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
		risk, err := epicBot.riskService.CreateRisk(ctx, text, epicID)
		if err != nil {
			epicBot.deleteAndSend(ctx, msg, msgID, fmt.Sprintf("❌ Ошибка создания риска: %v", err))
			return
		}
		epic, _ := epicBot.epicService.GetEpicByID(ctx, epicID)
		epicNum := epicID.String()
		if epic != nil {
			epicNum = epic.Number
		}
		epicBot.deleteAndSend(ctx, msg, msgID,
			fmt.Sprintf("✅ Риск создан для эпика #%s (ID: %s)", epicNum, risk.ID))

	// ── /score epic effort text-input step ────────────────────────────

	case StepScoreEpicEffort:
		promptMsgID := sess.MessageID
		epicBot.deleteMessage(ctx, msg.Chat.ID, msg.ID) // Delete user message

		score, err := strconv.Atoi(text)
		if err != nil || score < 0 || score > 500 {
			if sent, _ := epicBot.sendReply(ctx, msg, "❌ Некорректный ввод. Укажите число от 0 до 500:"); sent != nil {
				go func() {
					time.Sleep(3 * time.Second)
					epicBot.deleteMessage(context.Background(), sent.Chat.ID, sent.ID)
				}()
			}
			return
		}

		epicIDStr := sess.Data["epicID"]
		username := sess.Data["username"]
		epicBot.sessions.clear(sk)

		epicID, err := uuid.Parse(epicIDStr)
		if err != nil {
			epicBot.editReply(ctx, msg.Chat.ID, promptMsgID, "❌ Ошибка: неверный ID эпика.")
			return
		}

		user, err := epicBot.userService.FindUserByTelegramID(ctx, username)
		if err != nil {
			epicBot.editReply(ctx, msg.Chat.ID, promptMsgID, "❌ Пользователь не найден.")
			return
		}

		role, err := epicBot.roleService.GetRoleByUserID(ctx, user.ID)
		if err != nil {
			epicBot.editReply(ctx, msg.Chat.ID, promptMsgID, "❌ У вас нет назначенной роли.")
			return
		}

		if err := epicBot.epicService.CreateEpicScore(ctx, epicID, user.ID, role.ID, score); err != nil {
			epicBot.editReply(ctx, msg.Chat.ID, promptMsgID, fmt.Sprintf("❌ Ошибка сохранения оценки: %v", err))
			return
		}

		epic, _ := epicBot.epicService.GetEpicByID(ctx, epicID)
		epicNum := epicIDStr
		if epic != nil {
			epicNum = epic.Number
		}

		successText := fmt.Sprintf("✅ Оценка %d для эпика #%s сохранена!", score, epicNum)

		if err := epicBot.scoring.TryCompleteEpicScoring(ctx, epicID); err != nil {
			epicBot.log.Error("failed to try complete epic scoring",
				slog.String("epicID", epicID.String()), sl.Err(err))
		}

		msg.ID = promptMsgID
		epicBot.showEpicRisks(ctx, msg, username, epicID, successText)

	default:
		epicBot.sessions.clear(sk)
	}
}
