package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"log/slog"

	"github.com/google/uuid"
)

// notifyTestRepo — репозиторий-заглушка для тестов NotifyEpicReminders,
// реализующая ровно то подмножество Repository, которое требуется
// notify.BuildEpicScoringReminders и team-scoped проверке admin.
type notifyTestRepo struct {
	Repository

	epic *domain.Epic

	teamAdminOf    bool
	teamAdminOfErr error

	users           []domain.User
	getUsersTeamErr error

	// scoredEffort — userID → проставлена ли оценка трудоёмкости эпика.
	scoredEffort  map[uuid.UUID]bool
	unscoredRisks map[uuid.UUID][]domain.Risk
}

func (r *notifyTestRepo) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error) {
	return r.epic, nil
}

func (r *notifyTestRepo) IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error) {
	return r.teamAdminOf, r.teamAdminOfErr
}

func (r *notifyTestRepo) GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error) {
	return r.users, r.getUsersTeamErr
}

func (r *notifyTestRepo) HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error) {
	return r.scoredEffort[userID], nil
}

func (r *notifyTestRepo) GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error) {
	return r.unscoredRisks[userID], nil
}

// notifyTestNotifier — заглушка TelegramNotifier, фиксирующая вызовы и
// умеющая эмулировать ошибку отправки конкретному chatID.
type notifyTestNotifier struct {
	sentTo  []int64
	failFor map[int64]bool
}

func (n *notifyTestNotifier) SendDirectMessage(ctx context.Context, chatID int64, text string) error {
	n.sentTo = append(n.sentTo, chatID)
	if n.failFor[chatID] {
		return errors.New("send failed")
	}
	return nil
}

func TestNotifyEpicReminders(t *testing.T) {
	cfg := config.BotConfig{SuperAdmins: []string{"root"}}
	epicID := uuid.New()
	teamID := uuid.New()

	userA := domain.User{ID: uuid.New(), LastName: "Иванов", TelegramID: "ivanov", ChatID: 111}
	userB := domain.User{ID: uuid.New(), LastName: "Петров", TelegramID: "petrov", ChatID: 222}

	newRepo := func() *notifyTestRepo {
		return &notifyTestRepo{
			epic:  &domain.Epic{ID: epicID, Number: "1", Name: "Epic", TeamID: teamID, Status: domain.StatusScoring},
			users: []domain.User{userA, userB},
			scoredEffort: map[uuid.UUID]bool{
				userA.ID: false, // непроголосовавший
				userB.ID: true,  // полностью завершил оценку
			},
			unscoredRisks:   map[uuid.UUID][]domain.Risk{},
			teamAdminOf:     true,
			getUsersTeamErr: nil,
		}
	}

	doRequest := func(t *testing.T, repo *notifyTestRepo, notifier *notifyTestNotifier, session *middleware.UserSession) *httptest.ResponseRecorder {
		t.Helper()
		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{"epic_id":"` + epicID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)
		return rr
	}

	t.Run("happy_path_team_admin", func(t *testing.T) {
		repo := newRepo()
		notifier := &notifyTestNotifier{failFor: map[int64]bool{}}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			SentCount         int      `json:"sent_count"`
			FailedTelegramIDs []string `json:"failed_telegram_ids"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.SentCount != 1 {
			t.Errorf("expected sent_count=1 (only userA is unscored), got %d", resp.SentCount)
		}
		if len(resp.FailedTelegramIDs) != 0 {
			t.Errorf("expected no failed ids, got %v", resp.FailedTelegramIDs)
		}
		if len(notifier.sentTo) != 1 || notifier.sentTo[0] != userA.ChatID {
			t.Errorf("expected message sent to userA's chat only, got %v", notifier.sentTo)
		}
	})

	t.Run("forbidden_team_admin_of_other_team", func(t *testing.T) {
		repo := newRepo()
		repo.teamAdminOf = false // team-admin другой команды
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_b"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(notifier.sentTo) != 0 {
			t.Error("expected no messages sent for a forbidden request")
		}
	})

	t.Run("forbidden_member_without_admin_rights", func(t *testing.T) {
		repo := newRepo()
		repo.teamAdminOf = false // обычный участник, не admin ни одной команды
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "2", Username: "regular_member"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(notifier.sentTo) != 0 {
			t.Error("expected no messages sent for a forbidden request")
		}
	})

	t.Run("bad_request_epic_not_in_scoring", func(t *testing.T) {
		repo := newRepo()
		repo.epic.Status = domain.StatusScored
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(notifier.sentTo) != 0 {
			t.Error("expected no messages sent when epic is not in SCORING status")
		}
	})

	t.Run("superadmin_bypasses_team_scope", func(t *testing.T) {
		repo := newRepo()
		repo.teamAdminOf = false
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "3", Username: "root"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for superadmin, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("send_error_for_one_recipient_does_not_interrupt_others", func(t *testing.T) {
		repo := newRepo()
		// Оба участника непроголосовавшие в этом кейсе.
		repo.scoredEffort[userB.ID] = false
		notifier := &notifyTestNotifier{failFor: map[int64]bool{userA.ChatID: true}}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		rr := doRequest(t, repo, notifier, session)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			SentCount         int      `json:"sent_count"`
			FailedTelegramIDs []string `json:"failed_telegram_ids"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.SentCount != 1 {
			t.Errorf("expected sent_count=1, got %d", resp.SentCount)
		}
		if len(resp.FailedTelegramIDs) != 1 || resp.FailedTelegramIDs[0] != "@ivanov" {
			t.Errorf("expected failed_telegram_ids=[@ivanov], got %v", resp.FailedTelegramIDs)
		}
	})

	t.Run("notifier_unavailable_returns_500", func(t *testing.T) {
		repo := newRepo()
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, nil)
		body := `{"epic_id":"` + epicID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 when notifier is nil, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid_json_body_returns_400", func(t *testing.T) {
		repo := newRepo()
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{invalid json}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid JSON, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid_epic_id_uuid_returns_400", func(t *testing.T) {
		repo := newRepo()
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{"epic_id":"not-a-uuid"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid UUID, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("epic_not_found_returns_404", func(t *testing.T) {
		repo := newRepo()
		repo.epic = nil // эпик не найден
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{"epic_id":"` + epicID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404 when epic not found, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("is_team_admin_of_error_returns_403", func(t *testing.T) {
		repo := newRepo()
		repo.teamAdminOfErr = errors.New("database error")
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{"epic_id":"` + epicID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 when IsTeamAdminOf fails, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("build_reminders_error_returns_500", func(t *testing.T) {
		repo := newRepo()
		repo.getUsersTeamErr = errors.New("database error")
		notifier := &notifyTestNotifier{}
		session := &middleware.UserSession{TelegramID: "1", Username: "team_admin_a"}

		handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, notifier)
		body := `{"epic_id":"` + epicID.String() + `"}`
		req := httptest.NewRequest("POST", "/api/gantt/epics/notify", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.NotifyEpicReminders(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 when BuildEpicScoringReminders fails, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(notifier.sentTo) != 0 {
			t.Error("expected no messages sent when build fails")
		}
	})
}
