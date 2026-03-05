package ai

import (
	"context"
	"fmt"
	"log/slog"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/repositories"
	"EpicScoreBot/internal/scoring"
	"EpicScoreBot/internal/utils/logger/sl"

	openrouter "github.com/revrost/go-openrouter"
)

const maxToolRounds = 5

// telegramFormatSuffix is appended to the system prompt so the LLM produces
// text compatible with Telegram's Markdown parser.
const telegramFormatSuffix = `

IMPORTANT — formatting rules (Telegram Markdown):
- Use *bold* for emphasis (single asterisks, NOT double **).
- Use _italic_ for secondary emphasis.
- Use ` + "`" + `code` + "`" + ` for inline code.
- NEVER use markdown tables (| ... |). Instead, format structured data as
  bullet-point lists, one item per line, for example:
  • Иванов Иван (@ivan) — Аналитик
  • Петров Пётр (@petr) — Разработчик
- Use blank lines to separate sections.
- Keep the answer concise and readable in a mobile Telegram chat.`

// Client wraps the OpenRouter API and provides Ask() for Q&A over project data.
type Client struct {
	log      *slog.Logger
	cfg      *config.Config
	repo     *repositories.Repository
	scoring  *scoring.Service
	orClient *openrouter.Client
	tools    []openrouter.Tool
}

// New creates an AI Client. Returns nil when AIApiToken is empty (AI disabled).
func New(logger *slog.Logger, cfg *config.Config, repo *repositories.Repository, scoringSvc *scoring.Service) *Client {
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
		scoring:  scoringSvc,
		orClient: openrouter.NewClient(cfg.BotConfig.AI.AIApiToken),
		tools:    tools,
	}
}

// Ask sends a question to the LLM, executing tool calls as needed, and returns
// the final natural-language answer.
func (c *Client) Ask(ctx context.Context, question string) (string, error) {
	op := "ai.Ask()"
	log := c.log.With(slog.String("op", op))

	requestCtx, cancel := context.WithTimeout(ctx, c.cfg.BotConfig.AI.GetTimeout())
	defer cancel()

	systemPrompt := c.cfg.BotConfig.AI.SystemRolePrompt + telegramFormatSuffix

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
