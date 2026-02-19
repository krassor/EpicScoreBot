package telegram

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// isAdmin checks if the message sender is in the admins list.
func (bot *Bot) isAdmin(msg *tgbotapi.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	for _, admin := range bot.cfg.BotConfig.Admins {
		if strings.EqualFold(msg.From.UserName, admin) {
			return true
		}
	}
	return false
}

// isAdminCallback checks if the callback sender is an admin.
func (bot *Bot) isAdminCallback(callback *tgbotapi.CallbackQuery) bool {
	if callback == nil || callback.From == nil {
		return false
	}
	for _, admin := range bot.cfg.BotConfig.Admins {
		if strings.EqualFold(callback.From.UserName, admin) {
			return true
		}
	}
	return false
}
