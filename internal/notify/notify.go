// Package notify содержит нейтральную (не зависящую ни от Telegram, ни от
// HTTP-транспорта) бизнес-логику рассылки напоминаний участникам команды,
// ещё не завершившим оценку эпика. Используется как Telegram-ботом
// (команда /epicnotify), так и HTTP-обработчиком веб-панели — единый
// критерий "кто ещё не проголосовал" и единый текст напоминания.
package notify

import (
	"context"
	"fmt"
	"strings"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// ReminderRepository — минимальный набор методов доступа к данным,
// необходимый для вычисления списка непроголосовавших участников эпика.
// Реализуется адаптером поверх сервисов Telegram-бота (internal/telegram)
// и напрямую интерфейсом handlers.Repository в HTTP-слое.
type ReminderRepository interface {
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
}

// EpicReminder — одно напоминание конкретному участнику команды эпика.
type EpicReminder struct {
	// ChatID — Telegram chat ID получателя. Может быть равен 0, если
	// участник ещё не начинал диалог с ботом — такие получатели не
	// попадают в список напоминаний (см. BuildEpicScoringReminders),
	// а сразу учитываются как неудачные.
	ChatID int64
	// TelegramID — username участника (без "@"), используется только для
	// отчёта об успешной/неудачной отправке.
	TelegramID string
	// Text — итоговый текст напоминания.
	Text string
}

// BuildEpicScoringReminders вычисляет список напоминаний участникам команды
// эпика, не завершившим оценку (трудоёмкость эпика и/или риски эпика).
// Участник, полностью завершивший обе оценки, в список не попадает.
// Возвращает эпик (для формирования сводки на стороне вызывающего кода) и
// срез напоминаний, включая напоминания участникам без известного ChatID
// (ChatID == 0) — их дальнейшая обработка (учёт как неудачных) — задача
// DeliverReminders.
func BuildEpicScoringReminders(
	ctx context.Context,
	repo ReminderRepository,
	epicID uuid.UUID,
) (*domain.Epic, []EpicReminder, error) {
	op := "notify.BuildEpicScoringReminders"

	epic, err := repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}
	if epic == nil {
		return nil, nil, fmt.Errorf("%s: эпик не найден", op)
	}

	users, err := repo.GetUsersByTeamID(ctx, epic.TeamID)
	if err != nil {
		return epic, nil, fmt.Errorf("%s: %w", op, err)
	}

	var reminders []EpicReminder
	for _, user := range users {
		effortScored, err1 := repo.HasUserScoredEpic(ctx, epicID, user.ID)
		unscoredRisks, err2 := repo.GetUnscoredRisksByUser(ctx, user.ID, epicID)

		if err1 != nil || err2 != nil {
			continue // пропускаем участника при ошибке БД
		}

		if effortScored && len(unscoredRisks) == 0 {
			continue // участник полностью завершил оценку
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "👋 Привет, %s! У тебя есть незаконченные оценки для эпика #%s «%s».\n\nЧто осталось оценить:\n",
			user.LastName, epic.Number, epic.Name)

		if !effortScored {
			sb.WriteString("  • Трудоемкость эпика\n")
		}

		for _, risk := range unscoredRisks {
			fmt.Fprintf(&sb, "  • Риск: %s\n", risk.Description)
		}

		sb.WriteString("\nДля оценки используй команду /score")

		reminders = append(reminders, EpicReminder{
			ChatID:     user.ChatID,
			TelegramID: user.TelegramID,
			Text:       sb.String(),
		})
	}

	return epic, reminders, nil
}

// DeliverReminders отправляет каждое напоминание через send (например,
// обёртку над Telegram Bot API), не прерывая рассылку остальным при ошибке
// отправки одному из адресатов. Участники без известного ChatID (ChatID == 0)
// считаются неудачными без попытки вызова send. Возвращает число успешно
// отправленных сообщений и список telegram_id получателей, которым отправить
// не удалось.
func DeliverReminders(
	ctx context.Context,
	reminders []EpicReminder,
	send func(ctx context.Context, chatID int64, text string) error,
) (sentCount int, failedTelegramIDs []string) {
	for _, reminder := range reminders {
		if reminder.ChatID == 0 {
			failedTelegramIDs = append(failedTelegramIDs, "@"+reminder.TelegramID)
			continue
		}

		if err := send(ctx, reminder.ChatID, reminder.Text); err != nil {
			failedTelegramIDs = append(failedTelegramIDs, "@"+reminder.TelegramID)
			continue
		}

		sentCount++
	}

	return sentCount, failedTelegramIDs
}
