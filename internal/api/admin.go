package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// adminUserDTO strips the password hash out of the wire response - the
// admin panel needs everything else on models.User.
type adminUserDTO struct {
	ID          int    `json:"id"`
	Email       string `json:"email"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	IsAdmin     bool   `json:"is_admin"`
	IsModerator bool   `json:"is_moderator"`
	Banned      bool   `json:"banned"`
	CreatedAt   string `json:"created_at"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 25
	users, err := s.store.ListUsers(pageSize, (page-1)*pageSize)
	if err != nil {
		jsonError(w, "failed to load users", http.StatusInternalServerError)
		return
	}
	total, err := s.store.CountUsers()
	if err != nil {
		jsonError(w, "failed to load users", http.StatusInternalServerError)
		return
	}
	out := make([]adminUserDTO, len(users))
	for i, u := range users {
		out[i] = adminUserDTO{
			ID: u.ID, Email: u.Email, Username: u.Username, AvatarURL: u.AvatarURL,
			IsAdmin: u.IsAdmin, IsModerator: u.IsModerator, Banned: u.Banned,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	jsonResponse(w, map[string]interface{}{"users": out, "total": total, "page": page}, http.StatusOK)
}

func (s *Server) setUserRole(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}
	var body struct {
		IsAdmin     bool `json:"is_admin"`
		IsModerator bool `json:"is_moderator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if userID == userIDFromCtx(r) && !body.IsAdmin {
		jsonError(w, "you can't remove your own admin access", http.StatusBadRequest)
		return
	}
	if err := s.store.SetUserRole(userID, body.IsAdmin, body.IsModerator); err != nil {
		jsonError(w, "failed to update role", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) setUserBanned(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if userID == userIDFromCtx(r) {
		jsonError(w, "you can't ban yourself", http.StatusBadRequest)
		return
	}
	var body struct {
		Banned bool `json:"banned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.store.SetUserBanned(userID, body.Banned); err != nil {
		jsonError(w, "failed to update user", http.StatusInternalServerError)
		return
	}
	// A ban must also cut off the live connection, not just future requests.
	if body.Banned && s.hub != nil {
		s.hub.Disconnect(userID)
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) setPostPinned(w http.ResponseWriter, r *http.Request) {
	postID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	var body struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := s.store.SetPostPinned(postID, body.Pinned); err != nil {
		jsonError(w, "failed to update post", http.StatusInternalServerError)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) adminDeletePost(w http.ResponseWriter, r *http.Request) {
	postID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	if err := s.store.AdminDeletePost(postID); err != nil {
		jsonError(w, "failed to delete post", http.StatusInternalServerError)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	if err := s.store.AdminDeleteComment(commentID); err != nil {
		jsonError(w, "failed to delete comment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createReport lets any authenticated user flag a post or comment.
func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetType string `json:"target_type"`
		TargetID   int    `json:"target_id"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.TargetType != "post" && body.TargetType != "comment" {
		jsonError(w, "target_type must be 'post' or 'comment'", http.StatusBadRequest)
		return
	}
	if body.TargetID <= 0 {
		jsonError(w, "target_id is required", http.StatusBadRequest)
		return
	}
	if len(body.Reason) == 0 || len(body.Reason) > 500 {
		jsonError(w, "reason must be 1-500 chars", http.StatusBadRequest)
		return
	}
	id, err := s.store.CreateReport(userIDFromCtx(r), body.TargetType, body.TargetID, body.Reason)
	if err != nil {
		jsonError(w, "failed to file report", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]int{"id": id}, http.StatusCreated)
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "open"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 25
	reports, err := s.store.ListReports(status, pageSize, (page-1)*pageSize)
	if err != nil {
		jsonError(w, "failed to load reports", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, reports, http.StatusOK)
}

func (s *Server) resolveReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid report id", http.StatusBadRequest)
		return
	}
	if err := s.store.ResolveReport(id); err != nil {
		jsonError(w, "failed to resolve report", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
