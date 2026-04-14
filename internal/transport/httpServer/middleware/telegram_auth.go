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
	"net/url"
	"sort"
	"strconv"
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

// CreateSessionToken generates a signed token: base64(json) + "." + HMAC(base64(json)).
func CreateSessionToken(session UserSession, secret string) (string, error) {
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

			// Since auth is done at the callback endpoint now,
			// any request reaching this point without a valid cookie is unauthorized.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
		})
	}
}

// VerifyTelegramAuth verifies the data-check-string from Telegram Login Widget.
func VerifyTelegramAuth(r *http.Request, botToken string) bool {
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

// VerifyTelegramWebAppData verifies the initData from Telegram Mini App.
func VerifyTelegramWebAppData(initData, botToken string) (*UserSession, bool) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return nil, false
	}
	hash := vals.Get("hash")
	if hash == "" {
		return nil, false
	}

	var parts []string
	for key, values := range vals {
		if key == "hash" {
			continue
		}
		parts = append(parts, key+"="+values[0])
	}
	sort.Strings(parts)
	dataCheckStr := strings.Join(parts, "\n")

	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))
	secretKey := mac.Sum(nil)

	mac2 := hmac.New(sha256.New, secretKey)
	mac2.Write([]byte(dataCheckStr))
	computedHash := hex.EncodeToString(mac2.Sum(nil))

	if !hmac.Equal([]byte(computedHash), []byte(hash)) {
		return nil, false
	}

	userStr := vals.Get("user")
	if userStr == "" {
		return nil, false
	}

	var tpUser struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
	}
	if err := json.Unmarshal([]byte(userStr), &tpUser); err != nil {
		return nil, false
	}

	return &UserSession{
		TelegramID: strconv.FormatInt(tpUser.ID, 10),
		Username:   tpUser.Username,
		FirstName:  tpUser.FirstName,
	}, true
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
