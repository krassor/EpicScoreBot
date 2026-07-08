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

	"github.com/go-chi/chi/v5"
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
	createUserWithRelationsFunc func(ctx context.Context, user *domain.User, teamUUIDs, roleUUIDs []uuid.UUID) error
	updateUserWithRelationsFunc func(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs, roleUUIDs []uuid.UUID) error
	getUserRelationsFunc        func(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error)
	getAllUsersFunc             func(ctx context.Context) ([]domain.User, error)
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

func (m *mockRepository) CreateUserWithRelations(ctx context.Context, user *domain.User, teamUUIDs, roleUUIDs []uuid.UUID) error {
	if m.createUserWithRelationsFunc != nil {
		return m.createUserWithRelationsFunc(ctx, user, teamUUIDs, roleUUIDs)
	}
	return nil
}

func (m *mockRepository) GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	return nil, nil
}

func (m *mockRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error) {
	return nil, nil
}

func (m *mockRepository) UpdateUserWithRelations(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs, roleUUIDs []uuid.UUID) error {
	if m.updateUserWithRelationsFunc != nil {
		return m.updateUserWithRelationsFunc(ctx, userID, firstName, lastName, weight, teamUUIDs, roleUUIDs)
	}
	return nil
}

func (m *mockRepository) GetUserRelations(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	if m.getUserRelationsFunc != nil {
		return m.getUserRelationsFunc(ctx, userID)
	}
	return nil, nil, nil
}

func (m *mockRepository) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == userID {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	if m.getAllUsersFunc != nil {
		return m.getAllUsersFunc(ctx)
	}
	var res []domain.User
	for _, u := range m.users {
		res = append(res, *u)
	}
	return res, nil
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

func TestSingleUserCRUD(t *testing.T) {
	repo := &mockRepository{
		users: make(map[string]*domain.User),
	}
	handler := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, config.BotConfig{})

	// 1. Тест создания пользователя (CreateSingleUser)
	createBody := `{"telegram_id":"test_user","first_name":"Test","last_name":"User","weight":90,"team_ids":["00000000-0000-0000-0000-000000000001"],"role_ids":["00000000-0000-0000-0000-000000000002"]}`
	reqCreate := httptest.NewRequest("POST", "/api/admin/users", strings.NewReader(createBody))
	rrCreate := httptest.NewRecorder()

	var createdUser *domain.User
	var passedTeams []uuid.UUID
	var passedRoles []uuid.UUID

	repo.createUserWithRelationsFunc = func(ctx context.Context, user *domain.User, teamUUIDs, roleUUIDs []uuid.UUID) error {
		createdUser = user
		passedTeams = teamUUIDs
		passedRoles = roleUUIDs
		repo.users[user.TelegramID] = user
		return nil
	}

	handler.CreateSingleUser(rrCreate, reqCreate)
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", rrCreate.Code, rrCreate.Body.String())
	}

	if createdUser == nil || createdUser.TelegramID != "test_user" || createdUser.FirstName != "Test" || createdUser.Weight != 90 {
		t.Errorf("created user mismatch: %+v", createdUser)
	}
	if len(passedTeams) != 1 || passedTeams[0].String() != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected 1 team uuid, got %v", passedTeams)
	}
	if len(passedRoles) != 1 || passedRoles[0].String() != "00000000-0000-0000-0000-000000000002" {
		t.Errorf("expected 1 role uuid, got %v", passedRoles)
	}

	// 2. Тест детальной информации пользователя (GetUserDetails)
	userID := createdUser.ID
	repo.getUserRelationsFunc = func(ctx context.Context, uID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
		if uID != userID {
			t.Errorf("expected user ID %v, got %v", userID, uID)
		}
		return passedTeams, passedRoles, nil
	}

	reqGet := httptest.NewRequest("GET", "/api/admin/users/"+userID.String(), nil)
	// Добавляем URL-параметр в chi.Context
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", userID.String())
	reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), chi.RouteCtxKey, chiCtx))

	rrGet := httptest.NewRecorder()
	handler.GetUserDetails(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rrGet.Code)
	}

	var userDetails map[string]any
	json.Unmarshal(rrGet.Body.Bytes(), &userDetails)
	if userDetails["telegram_id"] != "test_user" || userDetails["first_name"] != "Test" {
		t.Errorf("invalid user details: %v", userDetails)
	}

	// 3. Тест редактирования пользователя (UpdateUser)
	updateBody := `{"first_name":"Updated","last_name":"User2","weight":85,"team_ids":["00000000-0000-0000-0000-000000000003"],"role_ids":["00000000-0000-0000-0000-000000000004"]}`
	reqUpdate := httptest.NewRequest("PUT", "/api/admin/users/"+userID.String(), strings.NewReader(updateBody))
	reqUpdate = reqUpdate.WithContext(context.WithValue(reqUpdate.Context(), chi.RouteCtxKey, chiCtx))

	var updateCalled bool
	repo.updateUserWithRelationsFunc = func(ctx context.Context, uID uuid.UUID, firstName, lastName string, weight int, teamUUIDs, roleUUIDs []uuid.UUID) error {
		if uID != userID {
			t.Errorf("expected user ID %v, got %v", userID, uID)
		}
		if firstName != "Updated" || weight != 85 {
			t.Errorf("expected updated name/weight, got %s/%d", firstName, weight)
		}
		if len(teamUUIDs) != 1 || teamUUIDs[0].String() != "00000000-0000-0000-0000-000000000003" {
			t.Errorf("expected team 00000000-0000-0000-0000-000000000003, got %v", teamUUIDs)
		}
		updateCalled = true
		return nil
	}

	rrUpdate := httptest.NewRecorder()
	handler.UpdateUser(rrUpdate, reqUpdate)
	if rrUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rrUpdate.Code, rrUpdate.Body.String())
	}
	if !updateCalled {
		t.Error("expected repo.UpdateUserWithRelations to be called")
	}
}
