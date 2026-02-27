package telegram

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

// isAdmin checks if the message sender is in the admins list.
func (epicBot *Bot) isAdmin(msg *models.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	for _, admin := range epicBot.cfg.BotConfig.Admins {
		if strings.EqualFold(msg.From.Username, admin) {
			return true
		}
	}
	for _, superadmin := range epicBot.cfg.BotConfig.SuperAdmins {
		if strings.EqualFold(msg.From.Username, superadmin) {
			return true
		}
	}
	return false
}

// isSuperAdmin checks if the message sender is in the super admins list.
func (epicBot *Bot) isSuperAdmin(msg *models.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	for _, superadmin := range epicBot.cfg.BotConfig.SuperAdmins {
		if strings.EqualFold(msg.From.Username, superadmin) {
			return true
		}
	}
	return false
}

// isAdminCallback checks if the callback sender is an admin.
func (epicBot *Bot) isAdminCallback(callback *models.CallbackQuery) bool {
	if callback == nil {
		return false
	}
	for _, admin := range epicBot.cfg.BotConfig.Admins {
		if strings.EqualFold(callback.From.Username, admin) {
			return true
		}
	}
	for _, superadmin := range epicBot.cfg.BotConfig.SuperAdmins {
		if strings.EqualFold(callback.From.Username, superadmin) {
			return true
		}
	}
	return false
}

// isSuperAdminCallback checks if the callback sender is a super admin.
func (epicBot *Bot) isSuperAdminCallback(callback *models.CallbackQuery) bool {
	if callback == nil {
		return false
	}
	for _, superadmin := range epicBot.cfg.BotConfig.SuperAdmins {
		if strings.EqualFold(callback.From.Username, superadmin) {
			return true
		}
	}
	return false
}
