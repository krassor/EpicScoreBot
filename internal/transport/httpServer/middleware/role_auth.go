package middleware

import (
	"context"
	"net/http"
	"strings"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

// UserFinder defines the interface to find a user by their Telegram ID.
//
// Несмотря на имя параметра (сохранено ради обратной совместимости
// сигнатуры Repository.FindUserByTelegramID), вызывающая сторона передаёт
// сюда UserSession.DirectoryKey() — нормализованный ключ справочника
// (username), а не UserSession.TelegramID (числовой Telegram ID) —
// design.md Решение 2 заявки fix-webapp-user-identity.
type UserFinder interface {
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
}

// TeamAdminChecker сообщает, является ли пользователь (по DirectoryKey()
// HTTP-сессии — нормализованному username, design.md Решение 2 заявки
// fix-webapp-user-identity) team-admin хотя бы одной команды. Роль "admin"
// в RoleAuth теперь team-scoped (таблица team_admins в БД) вместо
// глобального списка BotConfig.Admins — см. design.md изменения
// add-team-admin.
type TeamAdminChecker interface {
	IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error)
}

// TeamAdminScoper предоставляет точечные team-scoped проверки роли admin —
// используется не в RoleAuth (грубый гейт на уровне группы роутов), а в
// хендлерах, где конкретный team_id ресурса известен только после разбора
// тела запроса/URL-параметров.
type TeamAdminScoper interface {
	IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error)
	AdminTeamIDs(ctx context.Context, telegramID string) ([]uuid.UUID, error)
}

// RoleAuth creates a middleware that checks if the authenticated user has the required role.
// requiredRole can be "member", "admin", or "superadmin". Роль "admin"
// определяется через teamAdminChecker (team_admins в БД, team-scoped) —
// это грубый гейт "admin хотя бы одной команды"; точечная проверка
// конкретной команды выполняется дополнительно на уровне хендлера через
// TeamAdminScoper. Роль "superadmin" остаётся config-based (cfg.SuperAdmins).
func RoleAuth(finder UserFinder, teamAdminChecker TeamAdminChecker, cfg config.BotConfig, requiredRole string) func(http.Handler) http.Handler {
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
			} else if session.DirectoryKey() == "" {
				// Пустой DirectoryKey() означает, что у пользователя не
				// задан @username в Telegram — опознать его в справочнике
				// невозможно в принципе, и это отличная от «не найден в
				// справочнике» причина отказа: сообщаем именно её, тем же по
				// смыслу текстом, что уже выдаёт бот (design.md Решение 4
				// заявки fix-webapp-user-identity). Проверяется до любого
				// обращения к справочнику — по пустому ключу там всё равно
				// нечего искать.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"У вас не задан @username в Telegram. Установите его в настройках профиля."}`))
				return
			} else {
				// 2. Check team-admin role (team_admins в БД, team-scoped):
				// admin хотя бы одной команды. Ключ опознания в справочнике —
				// DirectoryKey() (нормализованный Username), а не TelegramID
				// (числовой Telegram ID) — design.md, Решение 2 заявки
				// fix-webapp-user-identity.
				isAdmin, _ := teamAdminChecker.IsTeamAdminOfAny(r.Context(), session.DirectoryKey())

				if isAdmin {
					role = "admin"
				} else {
					// 3. Check regular member. Отсутствие совпадения в
					// справочнике при непустом DirectoryKey() — отдельная
					// причина отказа от «нет @username» (design.md Решение
					// 4): пользователь просто не зарегистрирован.
					user, err := finder.FindUserByTelegramID(r.Context(), session.DirectoryKey())
					if err != nil || user == nil {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte(`{"error":"Вы не зарегистрированы в системе. Обратитесь к администратору."}`))
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
