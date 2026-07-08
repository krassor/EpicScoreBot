package middleware

import (
	"context"
	"net/http"
	"strings"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
)

// UserFinder defines the interface to find a user by their Telegram ID.
type UserFinder interface {
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
}

// RoleAuth creates a middleware that checks if the authenticated user has the required role.
// requiredRole can be "member", "admin", or "superadmin".
func RoleAuth(finder UserFinder, cfg config.BotConfig, requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionData := r.Context().Value(UserSessionKey)
			if sessionData == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			session, ok := sessionData.(*UserSession)
			if !ok || session.TelegramID == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid session"}`))
				return
			}

			// Determine user's role:
			var role string

			// 1. Check SuperAdmin config (by username)
			isSuperAdmin := false
			for _, sa := range cfg.SuperAdmins {
				if strings.EqualFold(session.Username, sa) {
					isSuperAdmin = true
					break
				}
			}

			if isSuperAdmin {
				role = "superadmin"
			} else {
				// 2. Check Admin config (by username)
				isAdmin := false
				for _, ad := range cfg.Admins {
					if strings.EqualFold(session.Username, ad) {
						isAdmin = true
						break
					}
				}

				if isAdmin {
					role = "admin"
				} else {
					// 3. Check regular member
					user, err := finder.FindUserByTelegramID(r.Context(), session.TelegramID)
					if err != nil || user == nil {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte(`{"error":"access denied"}`))
						return
					}
					role = "member"
				}
			}

			// Validate if the role meets the required role hierarchy:
			// superadmin has access to everything.
			// admin has access to admin and member actions.
			// member has access to member actions.
			allowed := false
			switch requiredRole {
			case "superadmin":
				allowed = (role == "superadmin")
			case "admin":
				allowed = (role == "superadmin" || role == "admin")
			case "member":
				allowed = (role == "superadmin" || role == "admin" || role == "member")
			}

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
