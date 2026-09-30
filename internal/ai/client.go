package ai

import (
	"context"
	"fmt"
	"log/slog"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/utils/logger/sl"

	openrouter "github.com/revrost/go-openrouter"
)

const maxToolRounds = 5

// Channel определяет, куда уйдёт ответ, и тем самым — допустимую разметку.
type Channel int

const (
	// ChannelTelegram — сообщение Telegram с parse_mode=HTML.
	ChannelTelegram Channel = iota
	// ChannelWeb — AI-чат веб-интерфейса (web/gantt/js/ai-chat.js).
	ChannelWeb
)

// telegramFormatSuffix дописывается к системному промпту для ответов в
// Telegram: сообщение уходит с parse_mode=HTML, который понимает лишь
// ограниченный набор тегов.
const telegramFormatSuffix = `

Ответ отправляется в Telegram с parse_mode=HTML, поэтому оформляй его только HTML-тегами <b>, <i> и <code>. Markdown (**, *, #, таблицы с |) Telegram не рендерит — пользователь увидит служебные символы. Символы <, > и & вне тегов пиши как &lt;, &gt; и &amp;, иначе Telegram отклонит сообщение целиком.
Структурированные данные выводи списком, по одному элементу на строку, например:
• Иванов Иван (@ivan) — Аналитик
• Петров Пётр (@petr) — Разработчик
Разделяй смысловые блоки пустой строкой. Ответ читают в мобильном чате — пиши кратко.`

// webFormatSuffix дописывается к системному промпту для веб-чата: он
// экранирует HTML и поддерживает только **жирный** и переносы строк.
const webFormatSuffix = `

Ответ показывается в веб-чате, который отображает текст как есть: HTML-теги и прочий Markdown выводятся буквально. Для выделения используй только **жирный**, структурированные данные — списком по одному элементу на строку, смысловые блоки разделяй пустой строкой.`

func formatSuffix(ch Channel) string {
	if ch == ChannelWeb {
		return webFormatSuffix
	}
	return telegramFormatSuffix
}

// Client wraps the OpenRouter API and provides Ask() for Q&A over project data.
type Client struct {
	log      *slog.Logger
	cfg      *config.Config
	repo     Repository
	orClient *openrouter.Client
	tools    []openrouter.Tool
}

// New creates an AI Client. Returns nil when AIApiToken is empty (AI disabled).
func New(logger *slog.Logger, cfg *config.Config, repo Repository) *Client {
	op := "ai.New()"
	log := logger.With(slog.String("op", op))

	if cfg.BotConfig.AI.AIApiToken == "" {
		log.Warn("AI API token not set — AI mention handler disabled")
		return nil
	}

	tools, err := buildTools()
	if err != nil {
		log.Error("failed to build AI tools", sl.Err(err))
		return nil
	}

	log.Info("AI client created", slog.String("model", cfg.BotConfig.AI.ModelName))

	return &Client{
		log:      log,
		cfg:      cfg,
		repo:     repo,
		orClient: openrouter.NewClient(cfg.BotConfig.AI.AIApiToken),
		tools:    tools,
	}
}

// Ask sends a question to the LLM, executing tool calls as needed, and returns
// the final natural-language answer formatted for the given channel.
func (c *Client) Ask(ctx context.Context, question string, ch Channel) (string, error) {
	op := "ai.Ask()"
	log := c.log.With(slog.String("op", op))

	requestCtx, cancel := context.WithTimeout(ctx, c.cfg.BotConfig.AI.GetTimeout())
	defer cancel()

	systemPrompt := c.cfg.BotConfig.AI.SystemRolePrompt + formatSuffix(ch)

	messages := []openrouter.ChatCompletionMessage{
		openrouter.SystemMessage(systemPrompt),
		openrouter.UserMessage(question),
	}

	for round := range maxToolRounds {
		req := openrouter.ChatCompletionRequest{
			Model:    c.cfg.BotConfig.AI.ModelName,
			Messages: messages,
			Tools:    c.tools,
		}

		resp, err := c.orClient.CreateChatCompletion(requestCtx, req)
		if err != nil {
			return "", fmt.Errorf("openrouter request (round %d): %w", round, err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response from LLM")
		}

		// Учёт расхода токенов — без него не измерить эффект правок промпта.
		if resp.Usage != nil {
			log.Info("AI usage",
				slog.Int("round", round),
				slog.Int("prompt_tokens", resp.Usage.PromptTokens),
				slog.Int("completion_tokens", resp.Usage.CompletionTokens),
				slog.Float64("cost", resp.Usage.Cost),
			)
		}

		choice := resp.Choices[0]

		// No tool calls — this is the final answer.
		if len(choice.Message.ToolCalls) == 0 {
			log.Debug("AI final answer received", slog.Int("rounds", round+1))
			return choice.Message.Content.Text, nil
		}

		// Append assistant's message with its tool calls.
		messages = append(messages, choice.Message)

		// Execute every tool call in this round.
		for _, tc := range choice.Message.ToolCalls {
			log.Debug("executing tool",
				slog.String("name", tc.Function.Name),
				slog.String("args", tc.Function.Arguments),
			)

			result, err := executeTool(requestCtx, c.repo, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				log.Error("tool execution failed",
					slog.String("tool", tc.Function.Name),
					sl.Err(err),
				)
				result = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}

			messages = append(messages, openrouter.ToolMessage(tc.ID, result))
		}
	}

	return "", fmt.Errorf("exceeded max tool rounds (%d)", maxToolRounds)
}
