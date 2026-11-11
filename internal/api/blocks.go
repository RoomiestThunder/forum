package api

import (
	"net/http"

	"forum/internal/models"

	"github.com/go-chi/chi/v5"
)

// Blocking is deliberately narrow: it stops the blocked user from sending you
// direct messages and from generating notifications for you (replies, mentions,
// likes). It does not hide their posts, which are public.

func (s *Server) listBlocks(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListBlockedUsers(userIDFromCtx(r))
	if err != nil {
		jsonError(w, "failed to load blocked users", http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []*models.UserSummary{} // serialise as [], not null
	}
	jsonResponse(w, users, http.StatusOK)
}

// blockTarget resolves {username} to a user id, rejecting unknown users and
// the caller themselves.
func (s *Server) blockTarget(w http.ResponseWriter, r *http.Request) (id int, ok bool) {
	target, err := s.store.GetUserByUsername(chi.URLParam(r, "username"))
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return 0, false
	}
	if target.ID == userIDFromCtx(r) {
		jsonError(w, "you can't block yourself", http.StatusBadRequest)
		return 0, false
	}
	return target.ID, true
}

// blockUser is PUT rather than POST: it is idempotent, so repeating it is
// harmless.
func (s *Server) blockUser(w http.ResponseWriter, r *http.Request) {
	targetID, ok := s.blockTarget(w, r)
	if !ok {
		return
	}
	if err := s.store.BlockUser(userIDFromCtx(r), targetID); err != nil {
		jsonError(w, "failed to block user", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) unblockUser(w http.ResponseWriter, r *http.Request) {
	targetID, ok := s.blockTarget(w, r)
	if !ok {
		return
	}
	if err := s.store.UnblockUser(userIDFromCtx(r), targetID); err != nil {
		jsonError(w, "failed to unblock user", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
