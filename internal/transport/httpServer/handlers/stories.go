package handlers

import (
	"EpicScoreBot/internal/models/domain"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// GetStories returns all stories for a parent epic.
func (h *GanttHandler) GetStories(w http.ResponseWriter, r *http.Request) {
	op := "handlers.GetStories"
	epicIDStr := chi.URLParam(r, "epic_id")
	epicUUID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	stories, err := h.repo.GetStoriesByEpicID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to get stories", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get stories")
		return
	}

	writeJSON(w, http.StatusOK, stories)
}

// CreateStory creates a new story under a parent epic.
func (h *GanttHandler) CreateStory(w http.ResponseWriter, r *http.Request) {
	op := "handlers.CreateStory"
	epicIDStr := chi.URLParam(r, "epic_id")
	epicUUID, err := uuid.Parse(epicIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid epic_id")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// 1. Получаем и проверяем родительский эпик
	parent, err := h.repo.GetEpicByID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to get parent epic", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusNotFound, "parent epic not found")
		return
	}

	if parent.ParentEpicID != nil {
		writeError(w, http.StatusBadRequest, "cannot decompose a story")
		return
	}

	if parent.Status != domain.StatusNew {
		writeError(w, http.StatusBadRequest, "cannot add stories to an epic that is already in progress or scored")
		return
	}

	// 2. Считаем количество существующих сторей для автонумерации
	count, err := h.repo.CountStoriesByEpicID(r.Context(), epicUUID)
	if err != nil {
		h.log.Error("failed to count stories", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to generate story number")
		return
	}

	storyNumber := fmt.Sprintf("%s-S%d", parent.Number, count+1)

	// 3. Создаем сторю (наследует свойства от родителя)
	story, err := h.repo.CreateStory(r.Context(), epicUUID, storyNumber, req.Name, req.Description, parent.TeamID, parent.Year, parent.Quarter, parent.Type, parent.EvaluatingRoleIDs)
	if err != nil {
		h.log.Error("failed to create story", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to create story")
		return
	}

	writeJSON(w, http.StatusCreated, story)
}

// DeleteStory deletes a story (only if parent epic is in NEW status).
func (h *GanttHandler) DeleteStory(w http.ResponseWriter, r *http.Request) {
	op := "handlers.DeleteStory"
	storyIDStr := chi.URLParam(r, "story_id")
	storyUUID, err := uuid.Parse(storyIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid story_id")
		return
	}

	// 1. Получаем сторю
	story, err := h.repo.GetEpicByID(r.Context(), storyUUID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}) // уже удалено
		return
	}

	if story.ParentEpicID == nil {
		writeError(w, http.StatusBadRequest, "cannot delete epic via DeleteStory")
		return
	}

	// 2. Проверяем статус родительского эпика
	parent, err := h.repo.GetEpicByID(r.Context(), *story.ParentEpicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check parent status")
		return
	}

	if parent.Status != domain.StatusNew {
		writeError(w, http.StatusBadRequest, "cannot delete story of an epic that is already in progress or scored")
		return
	}

	// 3. Удаляем
	if err := h.repo.DeleteEpic(r.Context(), storyUUID); err != nil {
		h.log.Error("failed to delete story", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to delete story")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateStory updates an existing story.
func (h *GanttHandler) UpdateStory(w http.ResponseWriter, r *http.Request) {
	op := "handlers.UpdateStory"
	storyIDStr := chi.URLParam(r, "story_id")
	storyUUID, err := uuid.Parse(storyIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid story_id")
		return
	}

	var req struct {
		Number       string  `json:"number"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		ParentEpicID *string `json:"parent_epic_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Number == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "story number and name are required")
		return
	}

	session, ok := requireSession(w, r)
	if !ok {
		return
	}
	isSuper := isSuperAdminSession(session, &h.cfg)

	story, err := h.repo.GetEpicByID(r.Context(), storyUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}

	if !isSuper {
		isAdminOf, err := h.repo.IsTeamAdminOf(r.Context(), session.TelegramID, story.TeamID)
		if err != nil || !isAdminOf {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	if story.ParentEpicID == nil {
		writeError(w, http.StatusBadRequest, "cannot update epic using story endpoint")
		return
	}

	if req.ParentEpicID != nil && *req.ParentEpicID != "" {
		newParentUUID, err := uuid.Parse(*req.ParentEpicID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid parent_epic_id")
			return
		}

		if newParentUUID != *story.ParentEpicID {
			if story.Status != domain.StatusNew {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot change parent epic when story status is %s", story.Status))
				return
			}
			parentEpic, err := h.repo.GetEpicByID(r.Context(), newParentUUID)
			if err != nil {
				writeError(w, http.StatusNotFound, "parent epic not found")
				return
			}
			story.ParentEpicID = &newParentUUID
			story.TeamID = parentEpic.TeamID
			story.Year = parentEpic.Year
			story.Quarter = parentEpic.Quarter
			story.Type = parentEpic.Type
			story.EvaluatingRoleIDs = parentEpic.EvaluatingRoleIDs
		}
	}

	story.Number = req.Number
	story.Name = req.Name
	story.Description = req.Description

	if err := h.repo.UpdateStory(r.Context(), story); err != nil {
		h.log.Error("failed to update story", slog.String("op", op), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updatedStory, err := h.repo.GetEpicByID(r.Context(), storyUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updatedStory)
}
