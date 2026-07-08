package handlers

import (
	"encoding/json"
	"net/http"
)

// AskAI handles requests to ask a question to the AI assistant.
func (h *GanttHandler) AskAI(w http.ResponseWriter, r *http.Request) {
	if h.ai == nil {
		writeError(w, http.StatusServiceUnavailable, "AI service is disabled or not configured")
		return
	}

	var req struct {
		Question string `json:"question"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}

	answer, err := h.ai.Ask(r.Context(), req.Question)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get answer from AI: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"answer": answer,
	})
}
