package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

type contextKey string

const UserSessionKey contextKey = "user_session"

// UserSession holds authenticated Telegram user details.
type UserSession struct {
	TelegramID string `json:"telegram_id"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
}

// createSessionToken generates a signed token: base64(json) + "." + HMAC(base64(json)).
func createSessionToken(session UserSession, secret string) (string, error) {
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	payload := base64.URLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + signature, nil
}

// verifySessionToken decodes a token if the signature is valid.
func verifySessionToken(token string, secret string) (*UserSession, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, false
	}
	payload, signature := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, false
	}

	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, false
	}

	var session UserSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, false
	}

	return &session, true
}

// TelegramAuth returns a chi middleware that verifies Telegram Login Widget
// authentication using a signed session cookie.
func TelegramAuth(botToken string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for signed session cookie first.
			cookie, err := r.Cookie("tg_sys_auth")
			if err == nil && cookie.Value != "" {
				if session, ok := verifySessionToken(cookie.Value, botToken); ok {
					ctx := context.WithValue(r.Context(), UserSessionKey, session)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Verify query params (initial login redirect).
			if verifyTelegramAuth(r, botToken) {
				query := r.URL.Query()
				session := UserSession{
					TelegramID: query.Get("id"),
					Username:   query.Get("username"),
					FirstName:  query.Get("first_name"),
				}
				token, err := createSessionToken(session, botToken)
				if err == nil {
					http.SetCookie(w, &http.Cookie{
						Name:     "tg_sys_auth",
						Value:    token,
						Path:     "/",
						MaxAge:   86400 * 7, // 7 days
						HttpOnly: false,     // Frontend JS needs to read this to know auth state
						SameSite: http.SameSiteLaxMode,
					})
					// Old cookie cleanup
					http.SetCookie(w, &http.Cookie{
						Name:   "tg_auth",
						Value:  "",
						Path:   "/",
						MaxAge: -1,
					})
					ctx := context.WithValue(r.Context(), UserSessionKey, &session)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
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
