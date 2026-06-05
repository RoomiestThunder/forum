package api

import (
	"net/http"
	"strings"
)

// handleWS upgrades the connection to WebSocket.
// Expects Authorization: Bearer <token> header or ?token= query param.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		http.Error(w, "websocket not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract token from header or query param
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = auth[7:]
		}
	}
	if tokenStr == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	claims, err := s.jwt.ValidateAccessToken(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	s.hub.ServeWS(w, r, claims.UserID)
}
