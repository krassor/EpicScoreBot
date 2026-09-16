package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/transport/httpServer/middleware"

	"log/slog"

	tgbot "github.com/go-telegram/bot"
)

// exportImageTestRepo — репозиторий-заглушка для тестов ExportGanttImage,
// отдающая ровно ту часть Repository, которая нужна хендлеру: пользователя
// по telegram_id сессии (для разрешения chat_id получателя).
type exportImageTestRepo struct {
	Repository

	user    *domain.User
	userErr error
}

func (r *exportImageTestRepo) FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error) {
	return r.user, r.userErr
}

// exportImageTestSender — заглушка DocumentSender, фиксирующая параметры
// каждого вызова (в т.ч. адресата) и умеющая эмулировать ошибку отправки.
type exportImageTestSender struct {
	calls []exportImageSendCall
	err   error
}

type exportImageSendCall struct {
	chatID   int64
	filename string
	data     []byte
	caption  string
}

func (s *exportImageTestSender) SendDocumentToChat(ctx context.Context, chatID int64, filename string, data []byte, caption string) error {
	s.calls = append(s.calls, exportImageSendCall{chatID: chatID, filename: filename, data: data, caption: caption})
	return s.err
}

// buildExportImageRequest собирает multipart/form-data запрос
// POST /api/gantt/export/image — file/format/filename, как отправляет
// фронтенд (frontend §6.1). Пустые format/filename не пишутся вовсе, чтобы
// проверять поведение на отсутствующих полях тоже.
func buildExportImageRequest(t *testing.T, format, filename string, fileContent []byte, session *middleware.UserSession) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if format != "" {
		if err := w.WriteField("format", format); err != nil {
			t.Fatalf("failed to write format field: %v", err)
		}
	}
	if filename != "" {
		if err := w.WriteField("filename", filename); err != nil {
			t.Fatalf("failed to write filename field: %v", err)
		}
	}
	if fileContent != nil {
		part, err := w.CreateFormFile("file", "upload")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err := part.Write(fileContent); err != nil {
			t.Fatalf("failed to write file content: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/gantt/export/image", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if session != nil {
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
	}
	return req
}

func validPNGContent() []byte {
	return append(append([]byte{}, pngSignature...), []byte("...fake-png-payload...")...)
}

func validSVGContent() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
}

func TestExportGanttImage(t *testing.T) {
	cfg := config.BotConfig{}
	session := &middleware.UserSession{TelegramID: "42", Username: "member_user"}

	newHandler := func(repo *exportImageTestRepo, sender *exportImageTestSender) *GanttHandler {
		h := NewGanttHandler(slog.Default(), &mockGanttService{}, repo, &mockScoringService{}, &mockAIClient{}, cfg, &mockNotifier{})
		if sender != nil {
			h = h.WithDocumentSender(sender)
		}
		return h
	}

	t.Run("notifier_unavailable_returns_500", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		// DocumentSender не устанавливается — h.docSender остаётся nil,
		// как и notifier в app/main.go, если бот не поднялся.
		handler := newHandler(repo, nil)

		req := buildExportImageRequest(t, "png", "chart.png", validPNGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "NOTIFIER_UNAVAILABLE")
	})

	t.Run("invalid_format_returns_400", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "jpg", "chart.jpg", validPNGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "INVALID_FORMAT")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt for invalid format")
		}
	})

	t.Run("content_mismatch_returns_400_invalid_file", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		// format=png, но содержимое — не PNG (нет сигнатуры).
		req := buildExportImageRequest(t, "png", "chart.png", []byte("this is not a png file"), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "INVALID_FILE")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt for mismatched content")
		}
	})

	t.Run("not_multipart_returns_400_invalid_request_body", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := httptest.NewRequest("POST", "/api/gantt/export/image", bytes.NewBufferString(`{"format":"png"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserSessionKey, session))
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "INVALID_REQUEST_BODY")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt for non-multipart body")
		}
	})

	t.Run("missing_file_returns_400_invalid_file", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		// Поле file не передаётся вовсе — только format и filename.
		req := buildExportImageRequest(t, "png", "chart.png", nil, session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "INVALID_FILE")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt without file")
		}
	})

	t.Run("body_too_large_returns_413", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		oversized := bytes.Repeat([]byte{0}, maxExportImageBytes+1024)
		req := buildExportImageRequest(t, "png", "chart.png", oversized, session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "FILE_TOO_LARGE")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt for oversized body")
		}
	})

	t.Run("user_not_found_returns_409", func(t *testing.T) {
		repo := &exportImageTestRepo{userErr: fmt.Errorf("Repository.FindUserByTelegramID: %w", sql.ErrNoRows)}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "svg", "chart.svg", validSVGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "BOT_CHAT_NOT_STARTED")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt for unknown user")
		}
	})

	// Сбой БД — не повод советовать пользователю «открыть чат с ботом».
	t.Run("repository_error_returns_500_not_chat_hint", func(t *testing.T) {
		repo := &exportImageTestRepo{userErr: errors.New("connection refused")}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "svg", "chart.svg", validSVGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "internal_error")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt on repository error")
		}
	})

	t.Run("chat_not_started_zero_chat_id_returns_409", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 0}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "svg", "chart.svg", validSVGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "BOT_CHAT_NOT_STARTED")
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt when chat_id is zero")
		}
	})

	t.Run("telegram_forbidden_returns_409_bot_chat_not_started", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{err: fmt.Errorf("bot.SendDocumentToChat: %w", tgbot.ErrorForbidden)}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "png", "chart.png", validPNGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "BOT_CHAT_NOT_STARTED")
	})

	t.Run("other_send_failure_returns_502", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{err: errors.New("network timeout")}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "png", "chart.png", validPNGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		assertErrorCode(t, rr, "TELEGRAM_SEND_FAILED")
	})

	t.Run("success_returns_200_sent", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 777}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		content := validPNGContent()
		req := buildExportImageRequest(t, "png", "gantt-team-2026-01-01_12-00.png", content, session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "sent" {
			t.Errorf("expected status=sent, got %q", resp.Status)
		}
		if len(sender.calls) != 1 {
			t.Fatalf("expected exactly one send call, got %d", len(sender.calls))
		}
		call := sender.calls[0]
		if call.chatID != 777 {
			t.Errorf("expected chatID=777, got %d", call.chatID)
		}
		if call.filename != "gantt-team-2026-01-01_12-00.png" {
			t.Errorf("unexpected filename: %q", call.filename)
		}
		if !bytes.Equal(call.data, content) {
			t.Error("sent data does not match uploaded content")
		}
	})

	// Адресат берётся ИСКЛЮЧИТЕЛЬНО из сессии (session.TelegramID → chat_id
	// в БД), а не из тела запроса — тело не содержит и не может содержать
	// получателя вовсе (design.md Решение 7), но отдельно проверяем, что
	// смена сессии при неизменном теле запроса меняет адресата.
	t.Run("recipient_comes_from_session_not_request_body", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 999}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		otherSession := &middleware.UserSession{TelegramID: "other", Username: "someone_else"}
		req := buildExportImageRequest(t, "png", "chart.png", validPNGContent(), otherSession)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(sender.calls) != 1 || sender.calls[0].chatID != 999 {
			t.Fatalf("expected recipient resolved from session's user (chat_id=999), got %+v", sender.calls)
		}
	})

	t.Run("unauthenticated_returns_401", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "png", "chart.png", validPNGContent(), nil)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(sender.calls) != 0 {
			t.Error("expected no send attempt without a session")
		}
	})

	t.Run("empty_filename_falls_back_to_default_name", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		req := buildExportImageRequest(t, "svg", "", validSVGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(sender.calls) != 1 || sender.calls[0].filename != "gantt-chart.svg" {
			t.Fatalf("expected fallback filename gantt-chart.svg, got %+v", sender.calls)
		}
	})

	t.Run("unsafe_filename_characters_are_stripped", func(t *testing.T) {
		repo := &exportImageTestRepo{user: &domain.User{ChatID: 111}}
		sender := &exportImageTestSender{}
		handler := newHandler(repo, sender)

		// Имя с разделителями пути и кавычками — попытка выйти за пределы
		// безопасного имени файла.
		req := buildExportImageRequest(t, "png", `../../etc/passwd".png`, validPNGContent(), session)
		rr := httptest.NewRecorder()
		handler.ExportGanttImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		if len(sender.calls) != 1 {
			t.Fatalf("expected exactly one send call, got %d", len(sender.calls))
		}
		got := sender.calls[0].filename
		if bytes.ContainsAny([]byte(got), `/\":`) {
			t.Errorf("expected unsafe characters stripped from filename, got %q", got)
		}
	})
}

func assertErrorCode(t *testing.T, rr *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v. Body: %s", err, rr.Body.String())
	}
	if resp.Error.Code != wantCode {
		t.Errorf("expected error code %q, got %q (message: %q)", wantCode, resp.Error.Code, resp.Error.Message)
	}
}
