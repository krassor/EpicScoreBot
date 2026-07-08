package handlers

import (
	"context"
	"encoding/json"
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

type mockRepository struct {
	Repository
	users          map[string]*domain.User
	teams          map[uuid.UUID]*domain.Team
	bulkCreated    []domain.User
	bulkTeamID     *uuid.UUID
	bulkRoleID     *uuid.UUID
	findUserFunc   func(ctx context.Context, telegramID string) (*domain.User, error)
	bulkCreateFunc func(ctx context.Context, users []domain.User, teamID *uuid.UUID, roleID *uuid.UUID) error
}

func (m *mockRepository) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	if m.findUserFunc != nil {
		return m.findUserFunc(ctx, telegramID)
	}
	u, ok := m.users[telegramID]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockRepository) BulkCreateUsers(ctx context.Context, users []domain.User, teamID *uuid.UUID, roleID *uuid.UUID) error {
	if m.bulkCreateFunc != nil {
		return m.bulkCreateFunc(ctx, users, teamID, roleID)
	}
	m.bulkCreated = append(m.bulkCreated, users...)
	m.bulkTeamID = teamID
	m.bulkRoleID = roleID
	return nil
}

type mockGanttService struct {
	GanttService
}

type mockScoringService struct {
	ScoringService
}

type mockAIClient struct {
	AIClient
}

func TestGetProfile(t *testing.T) {
	repo := &mockRepository{
		users: map[string]*domain.User{
			"12345": {
				ID:         uuid.New(),
				TelegramID: "12345",
				FirstName:  "Ivan",
			},
		},
	}

	cfg := config.BotConfig{
		SuperAdmins: []string{"super"},
		Admins:      []string{"admin_user"},
	}

	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg)

	// Case 1: Unauthorized (no session)
	req := httptest.NewRequest("GET", "/api/gantt/profile", nil)
	rr := httptest.NewRecorder()
	handler.GetProfile(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	// Case 2: Member user
	session := &middleware.UserSession{
		TelegramID: "12345",
		Username:   "ivan_tg",
		FirstName:  "Ivan",
	}
	ctx := context.WithValue(req.Context(), middleware.UserSessionKey, session)
	req = req.WithContext(ctx)

	rr = httptest.NewRecorder()
	handler.GetProfile(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["role"] != "member" {
		t.Errorf("expected member role, got %v", resp["role"])
	}

	// Case 3: Admin
	session.Username = "admin_user"
	rr = httptest.NewRecorder()
	handler.GetProfile(rr, req)
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["role"] != "admin" {
		t.Errorf("expected admin role, got %v", resp["role"])
	}

	// Case 4: SuperAdmin
	session.Username = "super"
	rr = httptest.NewRecorder()
	handler.GetProfile(rr, req)
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["role"] != "superadmin" {
		t.Errorf("expected superadmin role, got %v", resp["role"])
	}
}

func TestBulkCreateUsers(t *testing.T) {
	repo := &mockRepository{}
	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{})

	body := `{"users":[{"telegram_id":"tg1","first_name":"User1","weight":50}],"csv":"tg2;User2;Last2;80","team_id":"00000000-0000-0000-0000-000000000001"}`
	req := httptest.NewRequest("POST", "/api/admin/users/bulk", strings.NewReader(body))
	rr := httptest.NewRecorder()

	handler.BulkCreateUsers(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if len(repo.bulkCreated) != 2 {
		t.Errorf("expected 2 created users, got %d", len(repo.bulkCreated))
	}

	if repo.bulkCreated[0].TelegramID != "tg1" || repo.bulkCreated[0].FirstName != "User1" || repo.bulkCreated[0].Weight != 50 {
		t.Errorf("first user mismatched: %+v", repo.bulkCreated[0])
	}

	if repo.bulkCreated[1].TelegramID != "tg2" || repo.bulkCreated[1].FirstName != "User2" || repo.bulkCreated[1].LastName != "Last2" || repo.bulkCreated[1].Weight != 80 {
		t.Errorf("second user mismatched: %+v", repo.bulkCreated[1])
	}
}
