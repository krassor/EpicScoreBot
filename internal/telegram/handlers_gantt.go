package telegram

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot/models"
)

// handleGantt sends a link to the Gantt chart web UI.
func (epicBot *Bot) handleGantt(ctx context.Context, msg *models.Message) error {
	addr := fmt.Sprintf("%s:%s",
		epicBot.cfg.HttpServer.Address,
		epicBot.cfg.HttpServer.Port,
	)

	// Build the gantt URL. In production the address should be a
	// publicly reachable hostname; for local dev "localhost" works.
	host := epicBot.cfg.GanttHost
	if host == "" {
		host = addr
	}

	url := fmt.Sprintf("http://%s/gantt/", host)

	kb := inlineKeyboard(
		inlineRow(models.InlineKeyboardButton{
			Text: "📊 Открыть диаграмму Ганта",
			URL:  url,
		}),
	)

	text := "📊 *Диаграмма Ганта*\n\n" +
		"Для просмотра и редактирования " +
		"диаграммы Ганта откройте веб-интерфейс:"

	_, err := epicBot.sendMarkdownWithKeyboard(ctx, msg, text, kb)
	return err
}
