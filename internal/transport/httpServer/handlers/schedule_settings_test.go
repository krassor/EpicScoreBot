package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// --- GET /api/gantt/teams/{id}/schedule-settings (backend §3.1) ---

func TestGetTeamScheduleSettings_ReturnsCurrentValue(t *testing.T) {
	teamID := uuid.New()

	for _, want := range []bool{false, true} {
		repo := &mockGanttRepo{
			getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
				return &domain.Team{ID: id, BackfillBlockPast: want}, nil
			},
		}
		svc := &mockGanttSvc{}
		handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

		req := newAssigneeRequest(http.MethodGet, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
			"id", teamID.String(), "", nil)
		w := httptest.NewRecorder()

		handler.GetTeamScheduleSettings(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("want=%v: expected 200, got %d, body: %s", want, w.Code, w.Body.String())
		}
		var resp struct {
			BackfillBlockPast bool `json:"backfill_block_past"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.BackfillBlockPast != want {
			t.Errorf("backfill_block_past = %v, want %v", resp.BackfillBlockPast, want)
		}
	}
}

func TestGetTeamScheduleSettings_InvalidTeamID(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	req := newAssigneeRequest(http.MethodGet, "/api/gantt/teams/not-a-uuid/schedule-settings", "id", "not-a-uuid", "", nil)
	w := httptest.NewRecorder()

	handler.GetTeamScheduleSettings(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
}

func TestGetTeamScheduleSettings_TeamNotFound(t *testing.T) {
	repo := &mockGanttRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return nil, errors.New("not found")
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	teamID := uuid.New()
	req := newAssigneeRequest(http.MethodGet, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), "", nil)
	w := httptest.NewRecorder()

	handler.GetTeamScheduleSettings(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
}

// --- PUT /api/gantt/teams/{id}/schedule-settings (backend §3.2) ---

func TestSetTeamScheduleSettings_Unauthorized(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	teamID := uuid.New()
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":true}`, nil)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTeamBackfillBlockPastCalled {
		t.Errorf("service must not be called without a session")
	}
}

func TestSetTeamScheduleSettings_RejectsMember(t *testing.T) {
	repo := &mockGanttRepo{
		isTeamAdminOfAny: false,
		findUserByTelegramIDFunc: func(ctx context.Context, telegramID string) (*domain.User, error) {
			return &domain.User{ID: uuid.New(), TelegramID: telegramID}, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	teamID := uuid.New()
	session := &middleware.UserSession{TelegramID: "member-1", Username: "plain_member"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":true}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member role, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTeamBackfillBlockPastCalled {
		t.Errorf("service must not be called when the request is rejected for role")
	}
}

func TestSetTeamScheduleSettings_RejectsAdminOfDifferentTeam(t *testing.T) {
	teamID := uuid.New()
	otherTeamID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: id}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, id uuid.UUID) (bool, error) {
			// Админ только другой команды, не той, что в запросе.
			return id == otherTeamID, nil
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_a_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":true}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin of a different team, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTeamBackfillBlockPastCalled {
		t.Errorf("service must not be called when the admin does not own this team")
	}
}

func TestSetTeamScheduleSettings_AdminOfOwnTeamSucceeds(t *testing.T) {
	teamID := uuid.New()

	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: id}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, id uuid.UUID) (bool, error) {
			return id == teamID, nil
		},
	}
	svc := &mockGanttSvc{
		setTeamBackfillBlockPastFunc: func(ctx context.Context, id uuid.UUID, blocked bool) ([]domain.GanttTask, error) {
			return []domain.GanttTask{{ID: uuid.New()}, {ID: uuid.New()}}, nil
		},
	}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":true}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTeamBackfillBlockPastCalled {
		t.Fatalf("expected service to be called")
	}
	if svc.setTeamBackfillBlockPastArgs.teamID != teamID || !svc.setTeamBackfillBlockPastArgs.blocked {
		t.Errorf("unexpected service args: %+v", svc.setTeamBackfillBlockPastArgs)
	}

	var resp struct {
		BackfillBlockPast bool `json:"backfill_block_past"`
		Count             int  `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.BackfillBlockPast {
		t.Errorf("backfill_block_past in response = false, want true")
	}
	if resp.Count != 2 {
		t.Errorf("count in response = %d, want 2", resp.Count)
	}
}

func TestSetTeamScheduleSettings_SuperAdminBypassesTeamScopeCheck(t *testing.T) {
	teamID := uuid.New()

	repo := &mockGanttRepo{
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return &domain.Team{ID: id}, nil
		},
		isTeamAdminOfFunc: func(ctx context.Context, telegramID string, id uuid.UUID) (bool, error) {
			return false, nil // superadmin не должен зависеть от этой проверки
		},
	}
	svc := &mockGanttSvc{
		setTeamBackfillBlockPastFunc: func(ctx context.Context, id uuid.UUID, blocked bool) ([]domain.GanttTask, error) {
			return nil, nil
		},
	}
	cfg := config.BotConfig{SuperAdmins: []string{"root_user"}}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "super-1", Username: "root_user"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":false}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for superadmin, got %d, body: %s", w.Code, w.Body.String())
	}
	if !svc.setTeamBackfillBlockPastCalled {
		t.Fatalf("expected service to be called for superadmin")
	}
}

func TestSetTeamScheduleSettings_InvalidTeamID(t *testing.T) {
	repo := &mockGanttRepo{}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/not-a-uuid/schedule-settings",
		"id", "not-a-uuid", `{"backfill_block_past":true}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
}

func TestSetTeamScheduleSettings_InvalidBody(t *testing.T) {
	repo := &mockGanttRepo{isTeamAdminOfAny: true}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	teamID := uuid.New()
	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `not-json`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTeamBackfillBlockPastCalled {
		t.Errorf("service must not be called with an invalid body")
	}
}

func TestSetTeamScheduleSettings_TeamNotFound(t *testing.T) {
	repo := &mockGanttRepo{
		isTeamAdminOfAny: true,
		getTeamByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
			return nil, errors.New("not found")
		},
	}
	svc := &mockGanttSvc{}
	handler := NewGanttHandler(slog.Default(), svc, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{}, &mockNotifier{})

	teamID := uuid.New()
	session := &middleware.UserSession{TelegramID: "admin-1", Username: "team_admin"}
	req := newAssigneeRequest(http.MethodPut, "/api/gantt/teams/"+teamID.String()+"/schedule-settings",
		"id", teamID.String(), `{"backfill_block_past":true}`, session)
	w := httptest.NewRecorder()

	handler.SetTeamScheduleSettings(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
	decodeError(t, w.Body.Bytes())
	if svc.setTeamBackfillBlockPastCalled {
		t.Errorf("service must not be called for an unknown team")
	}
}
