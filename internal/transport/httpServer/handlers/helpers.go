package handlers

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/transport/httpServer/middleware"
	"encoding/json"
	"net/http"
	"strings"
)

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeErrorCode writes a JSON error response in the project's standard
// structured format: {"error":{"code":"...","message":"..."}}. Use this for
// errors the frontend needs to branch on programmatically (by code), rather
// than the plain writeError string form.
func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// isSuperAdminSession сообщает, является ли сессия superadmin (роль
// остаётся config-based, BotConfig.SuperAdmins — не связана с team_admins).
func isSuperAdminSession(session *middleware.UserSession, cfg *config.BotConfig) bool {
	if session == nil {
		return false
	}
	for _, sa := range cfg.SuperAdmins {
		if strings.EqualFold(session.Username, sa) {
			return true
		}
	}
	return false
}

// requireSession извлекает и валидирует UserSession из контекста запроса;
// при отсутствии/невалидности сессии пишет 401 и возвращает ok=false.
func requireSession(w http.ResponseWriter, r *http.Request) (*middleware.UserSession, bool) {
	sessionData := r.Context().Value(middleware.UserSessionKey)
	if sessionData == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	session, ok := sessionData.(*middleware.UserSession)
	if !ok || session.TelegramID == "" {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return nil, false
	}
	return session, true
}
