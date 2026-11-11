package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"forum/internal/models"
)

// profile returns the public profile for a username.
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	user, err := s.store.GetUserByUsername(username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	postCount, err := s.store.CountPostsByUser(user.ID)
	if err != nil {
		s.storeError(w, r, err, "load profile")
		return
	}
	jsonResponse(w, models.PublicProfile{
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
		Bio:       user.Bio,
		CreatedAt: user.CreatedAt,
		PostCount: postCount,
	}, http.StatusOK)
}

// searchUsers powers @mention autocomplete - a short prefix match on
// username, capped well below anything worth paginating.
func (s *Server) searchUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		jsonResponse(w, []models.UserSummary{}, http.StatusOK)
		return
	}
	users, err := s.store.SearchUsers(q, 6)
	if err != nil {
		s.storeError(w, r, err, "search users")
		return
	}
	jsonResponse(w, users, http.StatusOK)
}

// updateBio updates the authenticated user's bio.
func (s *Server) updateBio(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Bio string `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Bio = strings.TrimSpace(body.Bio)
	if len([]rune(body.Bio)) > 280 {
		jsonError(w, "bio must be 280 characters or less", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.UpdateUserBio(userID, body.Bio); err != nil {
		s.storeError(w, r, err, "update bio")
		return
	}
	jsonResponse(w, map[string]string{"status": "updated"}, http.StatusOK)
}

// uploadAvatar uploads a new avatar image and attaches it to the
// authenticated user.
func (s *Server) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	f, ok := s.receiveUpload(w, r)
	if !ok {
		return
	}
	if err := s.store.UpdateUserAvatar(userIDFromCtx(r), f.url); err != nil {
		s.discardUpload(r, f)
		s.storeError(w, r, err, "save avatar")
		return
	}
	jsonResponse(w, map[string]string{"avatar_url": f.url}, http.StatusOK)
}
