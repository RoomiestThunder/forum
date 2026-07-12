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
	ctxUserID      contextKey = "api_user_id"
	ctxUsername    contextKey = "api_username"
	ctxIsAdmin     contextKey = "api_is_admin"
	ctxIsModerator contextKey = "api_is_moderator"
)

// requireAuth extracts and validates the JWT Bearer token, then checks the
// account hasn't been banned since the token was issued - a JWT alone can't
// be revoked, so this is what makes a ban actually take effect immediately
// instead of only at the next login.
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
		user, err := s.store.GetUserByID(claims.UserID)
		if err != nil {
			jsonError(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		if user.Banned {
			jsonError(w, "account suspended", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxUsername, claims.Username)
		ctx = context.WithValue(ctx, ctxIsAdmin, user.IsAdmin)
		ctx = context.WithValue(ctx, ctxIsModerator, user.IsModerator)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin must be chained after requireAuth.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin, _ := r.Context().Value(ctxIsAdmin).(bool)
		if !isAdmin {
			jsonError(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireStaff must be chained after requireAuth. Admins and moderators both
// pass; moderators just can't manage other users' roles (that's admin-only,
// gated separately by requireAdmin on those specific routes).
func (s *Server) requireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin, _ := r.Context().Value(ctxIsAdmin).(bool)
		isMod, _ := r.Context().Value(ctxIsModerator).(bool)
		if !isAdmin && !isMod {
			jsonError(w, "moderator access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
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
