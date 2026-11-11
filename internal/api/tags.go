package api

import (
	"encoding/json"
	"net/http"
)

// searchTags returns tag name suggestions for autocomplete.
func (s *Server) searchTags(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("q")
	tags, err := s.store.SearchTags(prefix, 10)
	if err != nil {
		s.storeError(w, r, err, "load tags")
		return
	}
	jsonResponse(w, tags, http.StatusOK)
}

func (s *Server) toggleBookmark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PostID     int  `json:"post_id"`
		Bookmarked bool `json:"bookmarked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.ToggleBookmark(body.PostID, userID, body.Bookmarked); err != nil {
		s.storeError(w, r, err, "update bookmark")
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
