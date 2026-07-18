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
		jsonError(w, "failed to load profile", http.StatusInternalServerError)
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
		jsonError(w, "search failed", http.StatusInternalServerError)
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
		jsonError(w, "failed to update bio", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "updated"}, http.StatusOK)
}

// uploadAvatar uploads a new avatar image and attaches it to the
// authenticated user.
func (s *Server) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	if s.storage == nil {
		jsonError(w, "file storage not configured", http.StatusServiceUnavailable)
		return
	}

	const hardLimit = 6 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, hardLimit)
	if err := r.ParseMultipartForm(5 * 1024 * 1024); err != nil {
		jsonError(w, "file too large or invalid form data", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file field required", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	userID := userIDFromCtx(r)

	_, _, fileURL, _, err := s.storage.Upload(r.Context(), file, header.Filename, -1)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateUserAvatar(userID, fileURL); err != nil {
		jsonError(w, "failed to save avatar", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"avatar_url": fileURL}, http.StatusOK)
}
