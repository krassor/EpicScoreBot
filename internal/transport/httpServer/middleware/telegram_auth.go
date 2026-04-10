package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// TelegramAuth returns a chi middleware that verifies Telegram Login Widget
// authentication. It checks for a valid tg_auth cookie first, then falls
// back to verifying the query-string parameters signed by the bot.
func TelegramAuth(botToken string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check auth cookie first.
			cookie, err := r.Cookie("tg_auth")
			if err == nil && cookie.Value != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Verify query params (initial login redirect).
			if verifyTelegramAuth(r, botToken) {
				http.SetCookie(w, &http.Cookie{
					Name:     "tg_auth",
					Value:    r.URL.Query().Get("hash"),
					Path:     "/",
					MaxAge:   86400, // 24 hours
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
		})
	}
}

// verifyTelegramAuth verifies the data-check-string from Telegram Login Widget.
func verifyTelegramAuth(r *http.Request, botToken string) bool {
	query := r.URL.Query()
	hash := query.Get("hash")
	if hash == "" {
		return false
	}

	// Build data-check-string: sorted "key=value" lines, excluding "hash".
	var parts []string
	for key, values := range query {
		if key == "hash" {
			continue
		}
		for _, val := range values {
			parts = append(parts, key+"="+val)
		}
	}
	sort.Strings(parts)
	dataCheckStr := strings.Join(parts, "\n")

	// secret_key = SHA256(bot_token)
	secretKey := sha256.Sum256([]byte(botToken))

	// HMAC-SHA256(data_check_string, secret_key)
	mac := hmac.New(sha256.New, secretKey[:])
	mac.Write([]byte(dataCheckStr))
	computedHash := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computedHash), []byte(hash))
}

// LoggerMiddleware logs incoming HTTP requests.
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http request",
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("method", r.Method),
			slog.String("url", r.RequestURI),
		)
		next.ServeHTTP(w, r)
	})
}
