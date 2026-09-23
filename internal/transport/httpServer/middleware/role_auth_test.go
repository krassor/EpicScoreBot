package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

type mockUserFinder struct {
	user *domain.User

	// lastKey фиксирует последний переданный ключ поиска — используется,
	// чтобы проверить, что RoleAuth ищет по session.DirectoryKey()
	// (нормализованный Username), а не по session.TelegramID (числовой
	// Telegram ID), design.md Решение 2 заявки fix-webapp-user-identity.
	lastKey string
}

func (m *mockUserFinder) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	m.lastKey = telegramID
	return m.user, nil
}

// mockTeamAdminChecker — конфигурируемая заглушка TeamAdminChecker для
// тестов team-scoped роли admin.
type mockTeamAdminChecker struct {
	isAdmin bool
	err     error

	// lastKey — см. mockUserFinder.lastKey.
	lastKey string
}

func (m *mockTeamAdminChecker) IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error) {
	m.lastKey = telegramID
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

	// Case 6: резолвинг в справочнике идёт по session.DirectoryKey()
	// (нормализованный Username), а не по session.TelegramID (числовой
	// Telegram ID) — design.md Решение 2 заявки fix-webapp-user-identity.
	// Username с ведущим "@" и в верхнем регистре должен дойти до
	// UserFinder/TeamAdminChecker уже нормализованным.
	{
		finder := &mockUserFinder{user: &domain.User{ID: uuid.New(), TelegramID: "6"}}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mw := RoleAuth(finder, teamAdmin, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "6", Username: "@Ivan_Petrov"}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if teamAdmin.lastKey != "ivan_petrov" {
			t.Errorf("expected TeamAdminChecker to receive DirectoryKey() %q, got %q", "ivan_petrov", teamAdmin.lastKey)
		}
		if finder.lastKey != "ivan_petrov" {
			t.Errorf("expected UserFinder to receive DirectoryKey() %q, got %q", "ivan_petrov", finder.lastKey)
		}
	}

	// Case 7: пустой DirectoryKey() (username не задан) отказывает раньше
	// любого обращения к справочнику и не путается с «не найден в
	// справочнике» — разный текст отказа (design.md Решение 4). Формат
	// тела ({"error":"..."}) при этом не меняется (Решение 4 / секция 3.2
	// заявки fix-webapp-user-identity).
	{
		finder := &mockUserFinder{user: &domain.User{ID: uuid.New()}}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mw := RoleAuth(finder, teamAdmin, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "7", Username: ""}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for empty username, got %d", rr.Code)
		}
		var body map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["error"] == "" {
			t.Error("expected non-empty error message")
		}
		if teamAdmin.lastKey != "" || finder.lastKey != "" {
			t.Error("expected no directory lookup for a session without @username")
		}
	}

	// Case 8: непустой DirectoryKey() без совпадения в справочнике даёт
	// другое сообщение, чем Case 7 — «не зарегистрирован», не «нет
	// @username» (design.md Решение 4).
	{
		finder := &mockUserFinder{user: nil}
		teamAdmin := &mockTeamAdminChecker{isAdmin: false}
		mw := RoleAuth(finder, teamAdmin, cfg, "member")(nextHandler)
		session := &UserSession{TelegramID: "8", Username: "unknown_guest"}

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserSessionKey, session))
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for unregistered user, got %d", rr.Code)
		}
		var unregisteredBody map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&unregisteredBody); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		// Case 7 body — для сравнения, что тексты различны.
		finder7 := &mockUserFinder{user: &domain.User{ID: uuid.New()}}
		teamAdmin7 := &mockTeamAdminChecker{isAdmin: false}
		mw7 := RoleAuth(finder7, teamAdmin7, cfg, "member")(nextHandler)
		session7 := &UserSession{TelegramID: "7", Username: ""}
		req7 := httptest.NewRequest("GET", "/", nil)
		req7 = req7.WithContext(context.WithValue(req7.Context(), UserSessionKey, session7))
		rr7 := httptest.NewRecorder()
		mw7.ServeHTTP(rr7, req7)
		var noUsernameBody map[string]string
		if err := json.NewDecoder(rr7.Body).Decode(&noUsernameBody); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		if unregisteredBody["error"] == noUsernameBody["error"] {
			t.Error("expected different messages for 'no username' vs 'not registered'")
		}
	}
}
