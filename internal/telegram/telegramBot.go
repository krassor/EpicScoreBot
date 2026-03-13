package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/utils/logger/sl"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Bot is the Telegram bot for EpicScoreBot.
type Bot struct {
	b           *bot.Bot
	cfg         *config.Config
	repo        Repository
	scoring     ScoringService
	ai          AIClient
	sessions    *sessionStore
	botUsername string
	ctx         context.Context
	cancel      context.CancelFunc
	log         *slog.Logger
}

// New creates a new Bot instance.
func New(
	logger *slog.Logger,
	cfg *config.Config,
	repo Repository,
	scoringSvc ScoringService,
	aiClient AIClient,
) *Bot {
	op := "telegram.New()"
	log := logger.With(slog.String("op", op))

	ctx, cancel := context.WithCancel(context.Background())

	epicBot := &Bot{
		cfg:      cfg,
		repo:     repo,
		scoring:  scoringSvc,
		ai:       aiClient,
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

	// Fetch bot username for mention detection.
	me, err := b.GetMe(ctx)
	if err != nil {
		log.Error("failed to get bot me", sl.Err(err))
	} else {
		epicBot.botUsername = me.Username
		log.Info("bot username", slog.String("username", me.Username))
	}

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
			//slog.String("text", update.Message.Text),
		)
	}
	if update.CallbackQuery != nil {
		log.Info("input callback",
			slog.String("user_id", strconv.FormatInt(update.CallbackQuery.From.ID, 10)),
			slog.String("user_name", update.CallbackQuery.From.Username),
			//slog.String("data", update.CallbackQuery.Data),
		)
	}

	switch {
	case update.Message != nil && isCommand(update.Message):
		if err := epicBot.commandHandler(ctx, update); err != nil {
			log.Error("command handler error", sl.Err(err))
		}
	case update.CallbackQuery != nil:
		epicBot.handleCallbackQuery(ctx, update)
	case update.Message != nil && isBotMentioned(update.Message, epicBot.botUsername):
		epicBot.handleMention(ctx, update)
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

// ─── Send methods (create new messages) ───────────────────────────────────

// sendReply sends a plain-text reply to the given chat/topic.
func (epicBot *Bot) sendReply(ctx context.Context, msg *models.Message, text string) (*models.Message, error) {
	chunks := splitTextIntoChunks(text, 4096)
	var lastMsg *models.Message
	for _, chunk := range chunks {
		p := &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   chunk,
		}
		if msg.MessageThreadID != 0 {
			p.MessageThreadID = msg.MessageThreadID
		}

		sent, err := epicBot.b.SendMessage(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("sendReply: %w", err)
		}
		lastMsg = sent
	}
	return lastMsg, nil
}

// sendMarkdown sends a Markdown-formatted reply to the given chat/topic.
func (epicBot *Bot) sendMarkdown(ctx context.Context, msg *models.Message, text string) (*models.Message, error) {
	p := &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	}
	if msg.MessageThreadID != 0 {
		p.MessageThreadID = msg.MessageThreadID
	}
	return epicBot.b.SendMessage(ctx, p)
}

// sendHTML sends an HTML-formatted reply to the given chat/topic.
// HTML is more reliable than Markdown in Telegram because special characters
// in usernames and text don't break the parser.
func (epicBot *Bot) sendHTML(ctx context.Context, msg *models.Message, text string) (*models.Message, error) {
	p := &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if msg.MessageThreadID != 0 {
		p.MessageThreadID = msg.MessageThreadID
	}
	return epicBot.b.SendMessage(ctx, p)
}

// sendWithKeyboard sends a plain-text reply with an inline keyboard.
func (epicBot *Bot) sendWithKeyboard(
	ctx context.Context,
	msg *models.Message,
	text string,
	kb *models.InlineKeyboardMarkup,
) (*models.Message, error) {
	p := &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text,
		ReplyMarkup: kb,
	}
	if msg.MessageThreadID != 0 {
		p.MessageThreadID = msg.MessageThreadID
	}
	return epicBot.b.SendMessage(ctx, p)
}

// sendMarkdownWithKeyboard sends a Markdown reply with an inline keyboard.
func (epicBot *Bot) sendMarkdownWithKeyboard(
	ctx context.Context,
	msg *models.Message,
	text string,
	kb *models.InlineKeyboardMarkup,
) (*models.Message, error) {
	p := &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text,
		ParseMode:   models.ParseModeMarkdown,
		ReplyMarkup: kb,
	}
	if msg.MessageThreadID != 0 {
		p.MessageThreadID = msg.MessageThreadID
	}
	return epicBot.b.SendMessage(ctx, p)
}

// ─── Edit methods (modify existing bot messages in-place) ─────────────────

// editReply edits the text of a previously sent bot message.
func (epicBot *Bot) editReply(ctx context.Context, chatID int64, messageID int, text string) error {
	_, err := epicBot.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
	return err
}

// editMarkdown edits a message with Markdown formatting.
func (epicBot *Bot) editMarkdown(ctx context.Context, chatID int64, messageID int, text string) error {
	_, err := epicBot.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
	})
	return err
}

// editWithKeyboard edits a message and replaces its inline keyboard.
func (epicBot *Bot) editWithKeyboard(
	ctx context.Context,
	chatID int64,
	messageID int,
	text string,
	kb *models.InlineKeyboardMarkup,
) error {
	_, err := epicBot.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: kb,
	})
	return err
}

// editMarkdownWithKeyboard edits a message with Markdown and inline keyboard.
func (epicBot *Bot) editMarkdownWithKeyboard(
	ctx context.Context,
	chatID int64,
	messageID int,
	text string,
	kb *models.InlineKeyboardMarkup,
) error {
	_, err := epicBot.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   models.ParseModeMarkdown,
		ReplyMarkup: kb,
	})
	return err
}

// ─── Delete method ────────────────────────────────────────────────────────

// deleteMessage deletes a bot message from the chat.
func (epicBot *Bot) deleteMessage(ctx context.Context, chatID int64, messageID int) error {
	_, err := epicBot.b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	return err
}

// ─── Helpers ──────────────────────────────────────────────────────────────

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

// escapeMarkdownV2 escapes all MarkdownV2 reserved characters in a string
// so it can be safely embedded in a MarkdownV2-formatted message.
func escapeMarkdownV2(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`_`, `\_`,
		`*`, `\*`,
		`[`, `\[`,
		`]`, `\]`,
		`(`, `\(`,
		`)`, `\)`,
		`~`, `\~`,
		"`", "\\`",
		`>`, `\>`,
		`#`, `\#`,
		`+`, `\+`,
		`-`, `\-`,
		`=`, `\=`,
		`|`, `\|`,
		`{`, `\{`,
		`}`, `\}`,
		`.`, `\.`,
		`!`, `\!`,
	)
	return replacer.Replace(s)
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
