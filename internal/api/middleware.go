package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	apperrors "forum/internal/errors"
)

type contextKey string

const ctxUserID contextKey = "api_user_id"

// requireAuth extracts and validates the JWT Bearer token.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			jsonError(w, "authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := auth[7:]
		claims, err := s.jwt.ValidateAccessToken(tokenStr)
		if err != nil {
			jsonError(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		next(w, r.WithContext(ctx))
	}
}

func userIDFromCtx(r *http.Request) int {
	id, _ := r.Context().Value(ctxUserID).(int)
	return id
}

func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	jsonResponse(w, map[string]string{"error": message}, status)
}

// isPermissionErr returns true for 403-class AppErrors (access denied).
func isPermissionErr(err error) bool {
	if ae, ok := err.(*apperrors.AppError); ok {
		return ae.HTTPStatusCode() == http.StatusForbidden
	}
	return false
}
