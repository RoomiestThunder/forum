package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) updateComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" || len([]rune(body.Content)) > 500 {
		jsonError(w, "content must be 1-500 chars", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.UpdateComment(id, userID, body.Content); err != nil {
		status := http.StatusInternalServerError
		if isPermissionErr(err) {
			status = http.StatusForbidden
		}
		jsonError(w, err.Error(), status)
		return
	}
	jsonResponse(w, map[string]string{"status": "updated"}, http.StatusOK)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.DeleteComment(id, userID); err != nil {
		status := http.StatusInternalServerError
		if isPermissionErr(err) {
			status = http.StatusForbidden
		}
		jsonError(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
