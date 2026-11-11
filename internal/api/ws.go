package api

import (
	"errors"
	"net/http"
)

var errAccountSuspended = errors.New("account suspended")

// handleWS upgrades the connection to a WebSocket. Authentication happens in
// the first frame (see the websocket package), not in the URL.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		http.Error(w, "websocket not configured", http.StatusServiceUnavailable)
		return
	}
	s.hub.ServeWS(w, r, s.authenticateWS)
}

// authenticateWS applies the same rules as requireAuth: a valid access token
// for an account that still exists and hasn't been banned.
func (s *Server) authenticateWS(token string) (int, error) {
	claims, err := s.jwt.ValidateAccessToken(token)
	if err != nil {
		return 0, err
	}
	user, err := s.store.GetUserByID(claims.UserID)
	if err != nil {
		return 0, err
	}
	if user.Banned {
		return 0, errAccountSuspended
	}
	return user.ID, nil
}
