package telegram

import (
	"context"
	"log/slog"
	"strings"

	"EpicScoreBot/internal/utils/logger/sl"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// handleMention handles messages where the bot is @mentioned.
// It strips the mention, sends the question to the AI client, and replies
// in the same chat/thread.
func (epicBot *Bot) handleMention(ctx context.Context, update *models.Update) {
	op := "telegram.handleMention"
	log := epicBot.log.With(slog.String("op", op))

	msg := update.Message
	if msg == nil {
		return
	}

	if epicBot.ai == nil {
		log.Error("AI client not initialized")
		return
	}

	question := extractQuestion(msg, epicBot.botUsername)
	if strings.TrimSpace(question) == "" {
		epicBot.sendReply(ctx, msg,
			"❓ Задайте вопрос после упоминания, например:\n@BotName кто не оценил EP-1?")
		return
	}

	log.Info("AI mention received",
		slog.String("username", msg.From.Username),
		slog.String("question", question),
	)

	// Typing indicator so user knows the bot is working.
	epicBot.b.SendChatAction(ctx, &tgbot.SendChatActionParams{
		ChatID: msg.Chat.ID,
		Action: models.ChatActionTyping,
	})

	answer, err := epicBot.ai.Ask(ctx, question)
	if err != nil {
		log.Error("AI ask failed", sl.Err(err))
		return
	}

	if _, err := epicBot.sendMarkdown(ctx, msg, answer); err != nil {
		// Fallback: send as plain text if markdown fails.
		epicBot.sendReply(ctx, msg, answer)
	}
}

// isBotMentioned reports whether the message contains a @mention targeting the bot.
func isBotMentioned(msg *models.Message, botUsername string) bool {
	if msg == nil || botUsername == "" {
		return false
	}
	for _, e := range msg.Entities {
		if e.Type != models.MessageEntityTypeMention {
			continue
		}
		runes := []rune(msg.Text)
		if e.Offset+e.Length > len(runes) {
			continue
		}
		mentioned := strings.TrimPrefix(string(runes[e.Offset:e.Offset+e.Length]), "@")
		if strings.EqualFold(mentioned, botUsername) {
			return true
		}
	}
	return false
}

// extractQuestion removes @BotUsername and leading/trailing whitespace from the message text.
func extractQuestion(msg *models.Message, botUsername string) string {
	text := msg.Text
	// Strip all mention entities that point to the bot.
	for _, e := range msg.Entities {
		if e.Type != models.MessageEntityTypeMention {
			continue
		}
		runes := []rune(text)
		if e.Offset+e.Length > len(runes) {
			continue
		}
		mentioned := strings.TrimPrefix(string(runes[e.Offset:e.Offset+e.Length]), "@")
		if strings.EqualFold(mentioned, botUsername) {
			text = string(runes[:e.Offset]) + string(runes[e.Offset+e.Length:])
		}
	}
	return strings.TrimSpace(text)
}
