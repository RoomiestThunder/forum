package api

import (
	"encoding/json"
	"net/http"
	"strings"

	apperrors "forum/internal/errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for password hashes. It is a variable only so
// the test suite can lower it: at the default cost every registration costs
// ~100ms (several times that under the race detector), which dominates the
// run time of tests that create users.
var bcryptCost = bcrypt.DefaultCost

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"` // email or username
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" || req.Username == "" || req.Password == "" {
		jsonError(w, "email, username and password are required", http.StatusBadRequest)
		return
	}
	if msg := validateRegistration(req); msg != "" {
		jsonError(w, msg, http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		s.internalError(w, r, err, "create account")
		return
	}

	// A taken email or username comes back from the store as a 409; any other
	// failure is a real error and must not be reported as "already taken".
	userID, err := s.store.CreateUser(req.Email, req.Username, string(hash))
	if err != nil {
		s.storeError(w, r, err, "create account")
		return
	}
	s.sendVerificationEmail(userID, req.Email)

	tokens, err := s.issueTokens(userID, req.Username)
	if err != nil {
		s.internalError(w, r, err, "issue tokens")
		return
	}

	jsonResponse(w, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		UserID:       userID,
		Username:     req.Username,
	}, http.StatusCreated)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		jsonError(w, "login and password are required", http.StatusBadRequest)
		return
	}

	// Try email first, then username.
	user, err := s.store.GetUserByEmail(req.Login)
	if err != nil {
		user, err = s.store.GetUserByUsername(req.Login)
	}
	if err != nil {
		// Do the same amount of work as for a wrong password, so the response
		// time doesn't reveal that the account doesn't exist.
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte(req.Password))
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if user.Banned {
		jsonError(w, "account suspended", http.StatusForbidden)
		return
	}

	tokens, err := s.issueTokens(user.ID, user.Username)
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		UserID:       user.ID,
		Username:     user.Username,
	}, http.StatusOK)
}

func (s *Server) refreshToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		jsonError(w, "refresh_token required", http.StatusBadRequest)
		return
	}

	rt, err := s.store.GetRefreshToken(body.RefreshToken)
	if err != nil {
		jsonError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	user, err := s.store.GetUserByID(rt.UserID)
	if err != nil {
		jsonError(w, "user not found", http.StatusUnauthorized)
		return
	}
	if user.Banned {
		jsonError(w, "account suspended", http.StatusForbidden)
		return
	}

	// Rotate: delete old token first; if delete fails the old token remains valid,
	// so we must abort rather than issuing a second valid token.
	if err := s.store.DeleteRefreshToken(body.RefreshToken); err != nil {
		jsonError(w, "token rotation failed", http.StatusInternalServerError)
		return
	}
	tokens, err := s.issueTokens(user.ID, user.Username)
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, authResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		UserID:       user.ID,
		Username:     user.Username,
	}, http.StatusOK)
}

// me returns the authenticated user's profile.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.store.GetUserByID(userIDFromCtx(r))
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, user, http.StatusOK)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	_ = s.store.DeleteRefreshTokensByUser(userID)
	jsonResponse(w, map[string]string{"message": "logged out"}, http.StatusOK)
}

// issueTokens creates an access + refresh token pair and persists the refresh token.
func (s *Server) issueTokens(userID int, username string) (*struct{ AccessToken, RefreshToken string }, error) {
	access, err := s.jwt.GenerateAccessToken(userID, username)
	if err != nil {
		return nil, err
	}
	refresh, err := s.jwt.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateRefreshToken(userID, refresh); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &struct{ AccessToken, RefreshToken string }{access, refresh}, nil
}
