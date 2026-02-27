package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/repositories"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Bot is the Telegram bot for EpicScoreBot.
type Bot struct {
	b        *bot.Bot
	cfg      *config.Config
	repo     *repositories.Repository
	scoring  *scoring.Service
	sessions *sessionStore
	ctx      context.Context
	cancel   context.CancelFunc
	log      *slog.Logger
}

// New creates a new Bot instance.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	repo *repositories.Repository,
	scoringSvc *scoring.Service,
) *Bot {
	op := "telegram.New()"
	log := logger.With(slog.String("op", op))

	ctx, cancel := context.WithCancel(context.Background())

	epicBot := &Bot{
		cfg:      cfg,
		repo:     repo,
		scoring:  scoringSvc,
		sessions: newSessionStore(),
		ctx:      ctx,
		cancel:   cancel,
		log:      log,
	}

	b, err := bot.New(cfg.BotConfig.TgbotApiToken,
		bot.WithDefaultHandler(epicBot.defaultHandler),
	)
	if err != nil {
		log.Error("error auth telegram bot", sl.Err(err))
		cancel()
		return nil
	}

	epicBot.b = b

	log.Info("telegram bot created")
	return epicBot
}

// defaultHandler is the single entry point for all updates from go-telegram/bot.
func (epicBot *Bot) defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	op := "telegram.defaultHandler()"
	log := epicBot.log.With(slog.String("op", op))

	if update.Message != nil {
		log.Info("input message",
			slog.String("user_id", strconv.FormatInt(update.Message.From.ID, 10)),
			slog.String("user_name", update.Message.From.Username),
			slog.String("text", update.Message.Text),
		)
	}
	if update.CallbackQuery != nil {
		log.Info("input callback",
			slog.String("user_id", strconv.FormatInt(update.CallbackQuery.From.ID, 10)),
			slog.String("user_name", update.CallbackQuery.From.Username),
			slog.String("data", update.CallbackQuery.Data),
		)
	}

	switch {
	case update.Message != nil && isCommand(update.Message):
		if err := epicBot.commandHandler(ctx, update); err != nil {
			log.Error("command handler error", sl.Err(err))
		}
	case update.CallbackQuery != nil:
		epicBot.handleCallbackQuery(ctx, update)
	case update.Message != nil:
		epicBot.handleSessionInput(update)
	}
}

// isCommand reports whether msg is a bot command.
func isCommand(msg *models.Message) bool {
	if msg == nil || len(msg.Entities) == 0 {
		return false
	}
	for _, e := range msg.Entities {
		if e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0 {
			return true
		}
	}
	return false
}

// commandText extracts /command from a message (without @botname suffix).
func commandText(msg *models.Message) string {
	if msg == nil || len(msg.Entities) == 0 {
		return ""
	}
	for _, e := range msg.Entities {
		if e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0 {
			raw := []rune(msg.Text)[e.Offset : e.Offset+e.Length]
			cmd := string(raw)
			// strip leading slash
			if len(cmd) > 0 && cmd[0] == '/' {
				cmd = cmd[1:]
			}
			// strip @botname if present
			for i, c := range cmd {
				if c == '@' {
					cmd = cmd[:i]
					break
				}
			}
			return cmd
		}
	}
	return ""
}

// commandArguments returns the text that follows the first /command entity.
func commandArguments(msg *models.Message) string {
	if msg == nil || len(msg.Entities) == 0 {
		return ""
	}
	for _, e := range msg.Entities {
		if e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0 {
			end := e.Offset + e.Length
			runes := []rune(msg.Text)
			if end >= len(runes) {
				return ""
			}
			// skip one space after command
			rest := string(runes[end:])
			if len(rest) > 0 && rest[0] == ' ' {
				rest = rest[1:]
			}
			return rest
		}
	}
	return ""
}

// Start begins polling for Telegram updates.
func (epicBot *Bot) Start(_ int) {
	epicBot.log.Info("starting telegram bot polling")
	epicBot.b.Start(epicBot.ctx)
	epicBot.log.Info("telegram bot polling stopped")
}

// sendReply sends a plain-text reply to the given chat/topic.
func (epicBot *Bot) sendReply(ctx context.Context, chatID int64, threadID int, text string) error {
	chunks := splitTextIntoChunks(text, 4096)
	for _, chunk := range chunks {
		p := &bot.SendMessageParams{
			ChatID: chatID,
			Text:   chunk,
		}
		if threadID != 0 {
			p.MessageThreadID = threadID
		}
		if _, err := epicBot.b.SendMessage(ctx, p); err != nil {
			return fmt.Errorf("sendReply: %w", err)
		}
	}
	return nil
}

// sendMarkdown sends a Markdown-formatted reply to the given chat/topic.
func (epicBot *Bot) sendMarkdown(ctx context.Context, chatID int64, threadID int, text string) error {
	p := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	}
	if threadID != 0 {
		p.MessageThreadID = threadID
	}
	_, err := epicBot.b.SendMessage(ctx, p)
	return err
}

// sendWithKeyboard sends a plain-text reply with an inline keyboard.
func (epicBot *Bot) sendWithKeyboard(
	ctx context.Context,
	chatID int64,
	threadID int,
	text string,
	kb *models.InlineKeyboardMarkup,
) error {
	p := &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: kb,
	}
	if threadID != 0 {
		p.MessageThreadID = threadID
	}
	_, err := epicBot.b.SendMessage(ctx, p)
	return err
}

// sendMarkdownWithKeyboard sends a Markdown reply with an inline keyboard.
func (epicBot *Bot) sendMarkdownWithKeyboard(
	ctx context.Context,
	chatID int64,
	threadID int,
	text string,
	kb *models.InlineKeyboardMarkup,
) error {
	p := &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   models.ParseModeMarkdown,
		ReplyMarkup: kb,
	}
	if threadID != 0 {
		p.MessageThreadID = threadID
	}
	_, err := epicBot.b.SendMessage(ctx, p)
	return err
}

// inlineKeyboard builds an InlineKeyboardMarkup from rows of buttons.
func inlineKeyboard(rows ...[]models.InlineKeyboardButton) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// inlineRow builds a single row of inline keyboard buttons.
func inlineRow(btns ...models.InlineKeyboardButton) []models.InlineKeyboardButton {
	return btns
}

// inlineBtn creates an inline keyboard button with callback data.
func inlineBtn(text, data string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: data}
}

// splitTextIntoChunks splits text into chunks of the specified size.
func splitTextIntoChunks(text string, chunkSize int) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkSize {
		end := min(i+chunkSize, len(runes))
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// Shutdown gracefully stops the bot.
func (epicBot *Bot) Shutdown(_ context.Context) error {
	epicBot.cancel()
	return nil
}
