package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	apperrors "forum/internal/errors"
)

type contextKey string

const (
	ctxUserID   contextKey = "api_user_id"
	ctxUsername contextKey = "api_username"
)

// requireAuth extracts and validates the JWT Bearer token.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		ctx = context.WithValue(ctx, ctxUsername, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// optionalAuth attaches the user ID to the context when a valid Bearer
// token is present, but - unlike requireAuth - lets the request through
// either way. Routes that behave differently for logged-in users (e.g. the
// feed's "my posts" filter) without requiring a login use this.
func (s *Server) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			if claims, err := s.jwt.ValidateAccessToken(auth[7:]); err == nil {
				ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
				ctx = context.WithValue(ctx, ctxUsername, claims.Username)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func userIDFromCtx(r *http.Request) int {
	id, _ := r.Context().Value(ctxUserID).(int)
	return id
}

func usernameFromCtx(r *http.Request) string {
	name, _ := r.Context().Value(ctxUsername).(string)
	return name
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
