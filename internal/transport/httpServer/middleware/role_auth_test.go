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

func TestRoleAuth(t *testing.T) {
	cfg := config.BotConfig{
		SuperAdmins: []string{"super"},
		Admins:      []string{"admin_user"},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Case 1: SuperAdmin has access to superadmin, admin and member
	{
		finder := &mockUserFinder{}
		mw := RoleAuth(finder, cfg, "superadmin")(nextHandler)
		req := httptest.NewRequest("GET", "/", nil)
		session := &UserSession{TelegramID: "1", Username: "super"}
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("SuperAdmin failed superadmin role check: got status %d", rr.Code)
		}
	}

	// Case 2: Admin does not have access to superadmin, but has access to admin and member
	{
		finder := &mockUserFinder{}
		mwSuper := RoleAuth(finder, cfg, "superadmin")(nextHandler)
		mwAdmin := RoleAuth(finder, cfg, "admin")(nextHandler)
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
		mwAdmin := RoleAuth(finder, cfg, "admin")(nextHandler)
		mwMember := RoleAuth(finder, cfg, "member")(nextHandler)
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
		mwMember := RoleAuth(finder, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "4", Username: "some_guest"}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mwMember.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Guest passed member check: got status %d", rr.Code)
		}
	}
}
