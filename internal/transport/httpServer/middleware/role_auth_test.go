package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

type mockUserFinder struct {
	user *domain.User
}

func (m *mockUserFinder) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	return m.user, nil
}

// mockTeamAdminChecker — конфигурируемая заглушка TeamAdminChecker для
// тестов team-scoped роли admin.
type mockTeamAdminChecker struct {
	isAdmin bool
	err     error
}

func (m *mockTeamAdminChecker) IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error) {
	return m.isAdmin, m.err
}

func TestRoleAuth(t *testing.T) {
	cfg := config.BotConfig{
		SuperAdmins: []string{"super"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Case 1: SuperAdmin has access to superadmin, admin and member
	{
		finder := &mockUserFinder{}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mw := RoleAuth(finder, teamAdmin, cfg, "superadmin")(nextHandler)
		req := httptest.NewRequest("GET", "/", nil)
		session := &UserSession{TelegramID: "1", Username: "super"}
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("SuperAdmin failed superadmin role check: got status %d", rr.Code)
		}
	}

	// Case 2: Team-admin (team_admins в БД) не имеет доступа к superadmin, но
	// имеет доступ к admin и member.
	{
		finder := &mockUserFinder{}
		teamAdmin := &mockTeamAdminChecker{isAdmin: true}
		mwSuper := RoleAuth(finder, teamAdmin, cfg, "superadmin")(nextHandler)
		mwAdmin := RoleAuth(finder, teamAdmin, cfg, "admin")(nextHandler)
		session := &UserSession{TelegramID: "2", Username: "admin_user"}

		// check superadmin
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mwSuper.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Admin passed superadmin check: got status %d", rr.Code)
		}

		// check admin
		req = httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr = httptest.NewRecorder()
		mwAdmin.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Admin failed admin check: got status %d", rr.Code)
		}
	}

	// Case 3: Regular member has access to member but not admin/superadmin
	{
		finder := &mockUserFinder{user: &domain.User{ID: uuid.New(), TelegramID: "3"}}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mwAdmin := RoleAuth(finder, teamAdmin, cfg, "admin")(nextHandler)
		mwMember := RoleAuth(finder, teamAdmin, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "3", Username: "member_user"}

		// check admin
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mwAdmin.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Member passed admin check: got status %d", rr.Code)
		}

		// check member
		req = httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr = httptest.NewRecorder()
		mwMember.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Member failed member check: got status %d", rr.Code)
		}
	}

	// Case 4: Non-existent user gets 403
	{
		finder := &mockUserFinder{user: nil} // DB user not found
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mwMember := RoleAuth(finder, teamAdmin, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "4", Username: "some_guest"}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mwMember.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Guest passed member check: got status %d", rr.Code)
		}
	}

	// Case 5: team_admins недоступна (ошибка репозитория) — трактуется как
	// "не admin", не 500.
	{
		finder := &mockUserFinder{}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false, err: context.DeadlineExceeded}
		mwAdmin := RoleAuth(finder, teamAdmin, cfg, "admin")(nextHandler)
		session := &UserSession{TelegramID: "5", Username: "flaky_user"}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mwAdmin.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 on team-admin lookup error, got %d", rr.Code)
		}
	}
}
