package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forum/internal/models"

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
	if body.Content == "" || utf8.RuneCountInString(body.Content) > maxCommentLength {
		jsonError(w, fmt.Sprintf("content must be 1-%d characters", maxCommentLength), http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateComment(id, userIDFromCtx(r), body.Content); err != nil {
		s.storeError(w, r, err, "update comment")
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
	if err := s.store.DeleteComment(id, userIDFromCtx(r)); err != nil {
		s.storeError(w, r, err, "delete comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// redactDeleted strips what a tombstone must not show. The store keeps the
// author's id on the row (it is still a row with an owner), but a deleted
// comment is placeholder text in the UI, so who wrote it isn't revealed.
func redactDeleted(comments []*models.Comment) []*models.Comment {
	for _, c := range comments {
		if c.Deleted {
			c.Author = ""
			c.UserID = 0
			c.Content = ""
		}
	}
	return comments
}
