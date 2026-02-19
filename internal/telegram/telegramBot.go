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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot is the Telegram bot for EpicScoreBot.
type Bot struct {
	tgbot           *tgbotapi.BotAPI
	cfg             *config.Config
	repo            *repositories.Repository
	scoring         *scoring.Service
	shutdownChannel chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	log             *slog.Logger
}

// New creates a new Bot instance.
func New(logger *slog.Logger, cfg *config.Config, repo *repositories.Repository, scoringSvc *scoring.Service) *Bot {
	op := "telegram.New()"
	log := logger.With(slog.String("op", op))

	bot, err := tgbotapi.NewBotAPI(cfg.BotConfig.TgbotApiToken)
	if err != nil {
		log.Error("error auth telegram bot", sl.Err(err))
	}

	bot.Debug = false

	log.Info("authorized on account",
		slog.String("UserName", bot.Self.UserName))

	ctx, cancel := context.WithCancel(context.Background())

	return &Bot{
		tgbot:           bot,
		cfg:             cfg,
		repo:            repo,
		scoring:         scoringSvc,
		shutdownChannel: make(chan struct{}),
		ctx:             ctx,
		cancel:          cancel,
		log:             log,
	}
}

// Start begins polling for Telegram updates.
func (bot *Bot) Start(updateTimeout int) {
	op := "telegram.Start()"
	log := bot.log.With(slog.String("op", op))

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = updateTimeout

	_, err := bot.tgbot.MakeRequest("deleteWebhook",
		tgbotapi.Params{"drop_pending_updates": "false"})
	if err != nil {
		log.Error("failed to delete webhook", sl.Err(err))
	}

	updates := bot.tgbot.GetUpdatesChan(updateConfig)

	for update := range updates {
		log.Debug("received update")
		go bot.processUpdate(&update)
	}
	log.Info("exiting update processing loop")
}

// processUpdate routes an update to the appropriate handler.
func (bot *Bot) processUpdate(update *tgbotapi.Update) {
	op := "telegram.processUpdate()"
	log := bot.log.With(slog.String("op", op))

	if update.Message != nil {
		log.Info("input message",
			slog.String("user_id", strconv.FormatInt(update.Message.From.ID, 10)),
			slog.String("user_name", update.Message.From.UserName),
			slog.String("text", update.Message.Text),
		)
	}
	if update.CallbackQuery != nil {
		log.Info("input callback",
			slog.String("user_id", strconv.FormatInt(update.CallbackQuery.From.ID, 10)),
			slog.String("user_name", update.CallbackQuery.From.UserName),
			slog.String("data", update.CallbackQuery.Data),
		)
	}

	select {
	case <-bot.shutdownChannel:
		return
	case <-bot.ctx.Done():
		return
	default:
		switch {
		case update.Message != nil && update.Message.IsCommand():
			if err := bot.commandHandler(bot.ctx, update); err != nil {
				log.Error("command handler error", sl.Err(err))
			}
		case update.CallbackQuery != nil:
			bot.handleCallbackQuery(update)
		default:
			if update.Message != nil {
				log.Debug("unsupported message type")
			}
		}
	}
}

// sendReply sends a reply message to the same chat.
func (bot *Bot) sendReply(chatID int64, text string) error {
	chunks := splitTextIntoChunks(text, 4096)
	for _, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		if _, err := bot.tgbot.Send(msg); err != nil {
			return fmt.Errorf("sendReply: %w", err)
		}
	}
	return nil
}

// splitTextIntoChunks splits text into chunks of the specified size.
func splitTextIntoChunks(text string, chunkSize int) []string {
	var chunks []string
	for i := 0; i < len(text); i += chunkSize {
		end := min(i+chunkSize, len(text))
		chunks = append(chunks, text[i:end])
	}
	return chunks
}

// Shutdown gracefully stops the bot.
func (bot *Bot) Shutdown(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("exit telegram bot: %w", ctx.Err())
		default:
			close(bot.shutdownChannel)
			bot.cancel()
			bot.tgbot.StopReceivingUpdates()
			return nil
		}
	}
}
