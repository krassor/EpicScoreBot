package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/notify"

	"github.com/google/uuid"
)

// NotifyEpicReminders рассылает напоминания участникам команды эпика,
// ещё не завершившим оценку (трудоёмкость эпика и/или риски), личными
// сообщениями в Telegram — тот же критерий "непроголосовавший" и тот же
// текст напоминания, что и у команды /epicnotify Telegram-бота (единый
// источник бизнес-логики — internal/notify).
func (h *GanttHandler) NotifyEpicReminders(w http.ResponseWriter, r *http.Request) {
	op := "handlers.NotifyEpicReminders"

	// 1. Проверка сессии инициатора запроса.
	session, ok := requireSession(w, r)
	if !ok {
		return
	}

	// Telegram-бот мог не инициализироваться (см. app/main.go) — в этом
	// случае рассылка недоступна независимо от прав инициатора.
	if h.notifier == nil {
		writeErrorCode(w, http.StatusInternalServerError, "NOTIFIER_UNAVAILABLE",
			"telegram notifications are unavailable")
		return
	}

	// 2. Декодирование тела запроса.
	var req struct {
		EpicID string `json:"epic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return
	}

	epicID, err := uuid.Parse(req.EpicID)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EPIC_ID", "invalid epic_id")
		return
	}

	// 3. Эпик должен существовать — нужен для team-scoped проверки прав и
	// валидации статуса.
	epic, err := h.repo.GetEpicByID(r.Context(), epicID)
	if err != nil || epic == nil {
		writeErrorCode(w, http.StatusNotFound, "EPIC_NOT_FOUND", "epic not found")
		return
	}

	// 4. Team-scoped проверка admin: superadmin — без ограничения,
	// team-admin — только для эпиков своей команды (по образцу
	// AdminSubmitEpicScore/admin_scores.go).
	isSuper := isSuperAdminSession(session, &h.cfg)
	if !isSuper {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, epic.TeamID)
		if err != nil || !isAdminOf {
			writeErrorCode(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}
	}

	// 5. Рассылка напоминаний имеет видимый внешний побочный эффект (реальные
	// сообщения в Telegram), поэтому допустима только для эпика в статусе
	// SCORING — как и в боте (/epicnotify предлагает только такие эпики).
	if epic.Status != domain.StatusScoring {
		writeErrorCode(w, http.StatusBadRequest, "EPIC_NOT_IN_SCORING",
			"epic must be in SCORING status to send reminders")
		return
	}

	// 6. Вычисление списка непроголосовавших и рассылка — единая бизнес-логика
	// с Telegram-ботом (internal/notify), GanttHandler.Repository напрямую
	// удовлетворяет notify.ReminderRepository.
	_, reminders, err := notify.BuildEpicScoringReminders(r.Context(), h.repo, epicID)
	if err != nil {
		h.log.Error("failed to build epic scoring reminders",
			slog.String("op", op), slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to build reminders")
		return
	}

	sentCount, failedTelegramIDs := notify.DeliverReminders(r.Context(), reminders, h.notifier.SendDirectMessage)
	if failedTelegramIDs == nil {
		failedTelegramIDs = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sent_count":          sentCount,
		"failed_telegram_ids": failedTelegramIDs,
	})
}
