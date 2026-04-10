package telegram

import (
	"context"
	"fmt"
	"strings"

	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot/models"
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
	case "report":
		return epicBot.handleReport(ctx, msg)
	case "list":
		return epicBot.handleList(ctx, msg)
	case "epicnotify":
		return epicBot.handleEpicNotify(ctx, msg)
	case "epicinfo":
		return epicBot.handleEpicInfo(ctx, msg)
	case "gantt":
		return epicBot.handleGantt(ctx, msg)
	default:
		_, err := epicBot.sendReply(ctx, msg,
			fmt.Sprintf("❓ Неизвестная команда: /%s\nИспользуйте /help для списка команд.",
				commandText(msg)))
		return err
	}
}

// ─── /start ───────────────────────────────────────────────────────────────

func (epicBot *Bot) handleStart(ctx context.Context, msg *models.Message) error {
	username := msg.From.Username
	if username != "" {
		user, err := epicBot.repo.FindUserByTelegramID(ctx, username)
		if err == nil && user != nil {
			if user.ChatID != msg.From.ID {
				if updateErr := epicBot.repo.UpdateUserChatID(ctx, user.ID, msg.From.ID); updateErr != nil {
					epicBot.log.Error("failed to update user ChatID on /start", sl.Err(updateErr))
				}
			}
		}
	}

	text := fmt.Sprintf("👋 Привет, %s!\n\n"+
		"Я бот для оценки трудоёмкости эпиков и рисков.\n"+
		"Используйте /help для получения списка команд.",
		msg.From.FirstName)
	_, err := epicBot.sendReply(ctx, msg, text)
	return err
}

// ─── /help ────────────────────────────────────────────────────────────────

func (epicBot *Bot) handleHelp(ctx context.Context, msg *models.Message) error {
	var sb strings.Builder
	sb.WriteString("<b>Для получения уведомлений необходимо в личном чате с ботом @EpicScoreBot pзапустить команду /start</b>\n\n")
	sb.WriteString("📋 <b>Команды бота</b>\n\n")
	sb.WriteString("<b>👤 Для всех:</b>\n")
	sb.WriteString("/score — меню оценки эпиков и рисков\n")
	sb.WriteString("/epicinfo — информация по неоценённым эпикам\n")

	if epicBot.isAdmin(msg) {
		sb.WriteString("\n<b>🔧 Для администраторов:</b>\n")
		sb.WriteString("/addteam &lt;название&gt; — создать команду\n")
		sb.WriteString("/adduser — добавить пользователя\n")
		sb.WriteString("/assignrole — назначить роль пользователю\n")
		sb.WriteString("/addepic — создать эпик\n")
		sb.WriteString("/addrisk — добавить риск к эпику\n")
		sb.WriteString("/startscore — запустить оценку эпика\n")
		sb.WriteString("/epicstatus — статус оценки эпика\n")
		sb.WriteString("/results — показать результаты эпика\n")
		sb.WriteString("/list — список участников команды\n")
		sb.WriteString("/epicnotify — отправить напоминания об оценке\n")
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
