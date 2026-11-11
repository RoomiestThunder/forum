package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

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
		s.storeError(w, r, err, "load users")
		return
	}
	total, err := s.store.CountUsers()
	if err != nil {
		s.storeError(w, r, err, "load users")
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
	target, err := s.store.GetUserByID(userID)
	if err != nil {
		s.storeError(w, r, err, "update role")
		return
	}
	if err := s.store.SetUserRole(userID, body.IsAdmin, body.IsModerator); err != nil {
		s.storeError(w, r, err, "update role")
		return
	}
	s.audit(r, auditUserRole, "user", userID, map[string]interface{}{
		"username":      target.Username,
		"was_admin":     target.IsAdmin,
		"was_moderator": target.IsModerator,
		"is_admin":      body.IsAdmin,
		"is_moderator":  body.IsModerator,
	})
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
	target, err := s.store.GetUserByID(userID)
	if err != nil {
		s.storeError(w, r, err, "update user")
		return
	}
	if err := s.store.SetUserBanned(userID, body.Banned); err != nil {
		s.storeError(w, r, err, "update user")
		return
	}
	// A ban must also cut off the live connection, not just future requests.
	if body.Banned && s.hub != nil {
		s.hub.Disconnect(userID)
	}
	action := auditUserUnban
	if body.Banned {
		action = auditUserBan
	}
	s.audit(r, action, "user", userID, map[string]interface{}{"username": target.Username})
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
	post, err := s.store.GetPost(postID)
	if err != nil {
		s.storeError(w, r, err, "update post")
		return
	}
	if err := s.store.SetPostPinned(postID, body.Pinned); err != nil {
		s.storeError(w, r, err, "update post")
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	action := auditPostUnpin
	if body.Pinned {
		action = auditPostPin
	}
	s.audit(r, action, "post", postID, map[string]interface{}{"title": post.Title})
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) adminDeletePost(w http.ResponseWriter, r *http.Request) {
	postID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	// Read the post first: once it is deleted, the log is the only place its
	// title and author survive.
	post, err := s.store.GetPost(postID)
	if err != nil {
		s.storeError(w, r, err, "delete post")
		return
	}
	if err := s.store.AdminDeletePost(postID); err != nil {
		s.storeError(w, r, err, "delete post")
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	s.audit(r, auditPostDelete, "post", postID, map[string]interface{}{
		"title":     post.Title,
		"author":    post.Author,
		"author_id": post.UserID,
		"excerpt":   excerpt(post.Content),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	comment, err := s.store.GetComment(commentID)
	if err != nil {
		s.storeError(w, r, err, "delete comment")
		return
	}
	if err := s.store.AdminDeleteComment(commentID); err != nil {
		s.storeError(w, r, err, "delete comment")
		return
	}
	s.audit(r, auditCommentDelete, "comment", commentID, map[string]interface{}{
		"post_id":   comment.PostID,
		"author":    comment.Author,
		"author_id": comment.UserID,
		"excerpt":   excerpt(comment.Content),
	})
	w.WriteHeader(http.StatusNoContent)
}

const maxReportReasonLength = 500

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
	if n := utf8.RuneCountInString(body.Reason); n == 0 || n > maxReportReasonLength {
		jsonError(w, fmt.Sprintf("reason must be 1-%d characters", maxReportReasonLength), http.StatusBadRequest)
		return
	}

	// A report against something that doesn't exist would sit in the queue
	// forever with nothing for a moderator to look at.
	var lookupErr error
	if body.TargetType == "post" {
		_, lookupErr = s.store.GetPost(body.TargetID)
	} else {
		_, lookupErr = s.store.GetComment(body.TargetID)
	}
	if lookupErr != nil {
		s.storeError(w, r, lookupErr, "file report")
		return
	}

	id, err := s.store.CreateReport(userIDFromCtx(r), body.TargetType, body.TargetID, body.Reason)
	if err != nil {
		s.storeError(w, r, err, "file report")
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
		s.storeError(w, r, err, "load reports")
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
		s.storeError(w, r, err, "resolve report")
		return
	}
	s.audit(r, auditReportResolve, "report", id, nil)
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
