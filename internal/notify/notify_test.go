package notify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// fakeReminderRepository — тестовая реализация ReminderRepository с
// настраиваемыми данными и ошибками для конкретного пользователя/эпика.
type fakeReminderRepository struct {
	epic  *domain.Epic
	users []domain.User

	// scoredEffort — набор userID, проставивших оценку трудоёмкости эпика.
	scoredEffort map[uuid.UUID]bool
	// unscoredRisks — риски, оставшиеся неоценёнными для конкретного userID.
	unscoredRisks map[uuid.UUID][]domain.Risk

	getEpicErr  error
	getUsersErr error
}

func (f *fakeReminderRepository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	if f.getEpicErr != nil {
		return nil, f.getEpicErr
	}
	return f.epic, nil
}

func (f *fakeReminderRepository) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	if f.getUsersErr != nil {
		return nil, f.getUsersErr
	}
	return f.users, nil
}

func (f *fakeReminderRepository) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	return f.scoredEffort[userID], nil
}

func (f *fakeReminderRepository) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	return f.unscoredRisks[userID], nil
}

func TestBuildEpicScoringReminders(t *testing.T) {
	epicID := uuid.New()
	teamID := uuid.New()
	epic := &domain.Epic{ID: epicID, Number: "42", Name: "Тестовый эпик", TeamID: teamID}

	t.Run("участник не оценил ничего", func(t *testing.T) {
		userID := uuid.New()
		user := domain.User{ID: userID, LastName: "Иванов", TelegramID: "ivanov", ChatID: 111}
		risk := domain.Risk{ID: uuid.New(), Description: "Риск задержки"}

		repo := &fakeReminderRepository{
			epic:          epic,
			users:         []domain.User{user},
			scoredEffort:  map[uuid.UUID]bool{},
			unscoredRisks: map[uuid.UUID][]domain.Risk{userID: {risk}},
		}

		gotEpic, reminders, err := BuildEpicScoringReminders(context.Background(), repo, epicID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotEpic == nil || gotEpic.ID != epicID {
			t.Fatalf("expected epic %v, got %v", epicID, gotEpic)
		}
		if len(reminders) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(reminders))
		}
		r := reminders[0]
		if r.ChatID != 111 || r.TelegramID != "ivanov" {
			t.Errorf("unexpected reminder recipient: %+v", r)
		}
		if !strings.Contains(r.Text, "Трудоемкость эпика") {
			t.Errorf("expected text to mention effort, got: %s", r.Text)
		}
		if !strings.Contains(r.Text, "Риск задержки") {
			t.Errorf("expected text to mention unscored risk, got: %s", r.Text)
		}
	})

	t.Run("участник оценил только эпик", func(t *testing.T) {
		userID := uuid.New()
		user := domain.User{ID: userID, LastName: "Петров", TelegramID: "petrov", ChatID: 222}
		risk := domain.Risk{ID: uuid.New(), Description: "Риск интеграции"}

		repo := &fakeReminderRepository{
			epic:          epic,
			users:         []domain.User{user},
			scoredEffort:  map[uuid.UUID]bool{userID: true},
			unscoredRisks: map[uuid.UUID][]domain.Risk{userID: {risk}},
		}

		_, reminders, err := BuildEpicScoringReminders(context.Background(), repo, epicID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reminders) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(reminders))
		}
		r := reminders[0]
		if strings.Contains(r.Text, "Трудоемкость эпика") {
			t.Errorf("expected text NOT to mention effort (already scored), got: %s", r.Text)
		}
		if !strings.Contains(r.Text, "Риск интеграции") {
			t.Errorf("expected text to mention unscored risk, got: %s", r.Text)
		}
	})

	t.Run("участник полностью завершил оценку", func(t *testing.T) {
		userID := uuid.New()
		user := domain.User{ID: userID, LastName: "Сидоров", TelegramID: "sidorov", ChatID: 333}

		repo := &fakeReminderRepository{
			epic:          epic,
			users:         []domain.User{user},
			scoredEffort:  map[uuid.UUID]bool{userID: true},
			unscoredRisks: map[uuid.UUID][]domain.Risk{},
		}

		_, reminders, err := BuildEpicScoringReminders(context.Background(), repo, epicID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reminders) != 0 {
			t.Fatalf("expected 0 reminders for fully-scored user, got %d", len(reminders))
		}
	})

	t.Run("эпик без участников", func(t *testing.T) {
		repo := &fakeReminderRepository{
			epic:  epic,
			users: nil,
		}

		gotEpic, reminders, err := BuildEpicScoringReminders(context.Background(), repo, epicID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotEpic == nil {
			t.Fatal("expected non-nil epic")
		}
		if len(reminders) != 0 {
			t.Fatalf("expected 0 reminders, got %d", len(reminders))
		}
	})

	t.Run("ошибка получения эпика", func(t *testing.T) {
		repo := &fakeReminderRepository{getEpicErr: errors.New("db error")}

		_, _, err := BuildEpicScoringReminders(context.Background(), repo, epicID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestDeliverReminders(t *testing.T) {
	t.Run("ошибка отправки одному из адресатов не прерывает рассылку остальным", func(t *testing.T) {
		reminders := []EpicReminder{
			{ChatID: 1, TelegramID: "first", Text: "a"},
			{ChatID: 2, TelegramID: "second", Text: "b"},
			{ChatID: 3, TelegramID: "third", Text: "c"},
		}

		var calledChatIDs []int64
		send := func(ctx context.Context, chatID int64, text string) error {
			calledChatIDs = append(calledChatIDs, chatID)
			if chatID == 2 {
				return errors.New("telegram api error")
			}
			return nil
		}

		sentCount, failed := DeliverReminders(context.Background(), reminders, send)

		if sentCount != 2 {
			t.Errorf("expected sentCount=2, got %d", sentCount)
		}
		if len(failed) != 1 || failed[0] != "@second" {
			t.Errorf("expected failed=[@second], got %v", failed)
		}
		if len(calledChatIDs) != 3 {
			t.Errorf("expected send to be called for all 3 reminders, got %d calls", len(calledChatIDs))
		}
	})

	t.Run("получатель без ChatID считается неудачным без вызова send", func(t *testing.T) {
		reminders := []EpicReminder{
			{ChatID: 0, TelegramID: "no_chat", Text: "a"},
		}

		sendCalled := false
		send := func(ctx context.Context, chatID int64, text string) error {
			sendCalled = true
			return nil
		}

		sentCount, failed := DeliverReminders(context.Background(), reminders, send)

		if sendCalled {
			t.Error("expected send NOT to be called for recipient without ChatID")
		}
		if sentCount != 0 {
			t.Errorf("expected sentCount=0, got %d", sentCount)
		}
		if len(failed) != 1 || failed[0] != "@no_chat" {
			t.Errorf("expected failed=[@no_chat], got %v", failed)
		}
	})

	t.Run("пустой список напоминаний", func(t *testing.T) {
		sentCount, failed := DeliverReminders(context.Background(), nil, func(ctx context.Context, chatID int64, text string) error {
			return nil
		})
		if sentCount != 0 || len(failed) != 0 {
			t.Errorf("expected no sent/failed, got sentCount=%d failed=%v", sentCount, failed)
		}
	})
}
