package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	emailVerificationTTL = 24 * time.Hour
	passwordResetTTL     = 1 * time.Hour
)

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// sendVerificationEmail issues a fresh token and emails (or logs) a
// verification link. Best-effort: a failure here shouldn't fail signup.
func (s *Server) sendVerificationEmail(userID int, email string) {
	if s.mailer == nil {
		return
	}
	token, err := randomToken()
	if err != nil {
		return
	}
	if err := s.store.CreateEmailVerificationToken(userID, token, time.Now().Add(emailVerificationTTL)); err != nil {
		return
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", s.baseURL, token)
	_ = s.mailer.Send(email, "Verify your email",
		fmt.Sprintf("Confirm your email address to finish setting up your account:\n\n%s\n\nThis link expires in 24 hours.", link))
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		jsonError(w, "token required", http.StatusBadRequest)
		return
	}
	userID, err := s.store.ConsumeEmailVerificationToken(body.Token)
	if err != nil {
		jsonError(w, "invalid or expired token", http.StatusBadRequest)
		return
	}
	if err := s.store.SetEmailVerified(userID); err != nil {
		jsonError(w, "failed to verify email", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "verified"}, http.StatusOK)
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	user, err := s.store.GetUserByID(userIDFromCtx(r))
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	if user.EmailVerified {
		jsonError(w, "email already verified", http.StatusBadRequest)
		return
	}
	s.sendVerificationEmail(user.ID, user.Email)
	jsonResponse(w, map[string]string{"status": "sent"}, http.StatusOK)
}

// forgotPassword always returns 200 regardless of whether the email exists,
// so the endpoint can't be used to enumerate registered accounts.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(body.Email)

	if user, err := s.store.GetUserByEmail(email); err == nil && s.mailer != nil {
		if token, err := randomToken(); err == nil {
			if err := s.store.CreatePasswordResetToken(user.ID, token, time.Now().Add(passwordResetTTL)); err == nil {
				link := fmt.Sprintf("%s/reset-password?token=%s", s.baseURL, token)
				_ = s.mailer.Send(user.Email, "Reset your password",
					fmt.Sprintf("Someone (hopefully you) requested a password reset:\n\n%s\n\nThis link expires in 1 hour. If you didn't request this, ignore this email.", link))
			}
		}
	}

	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(body.NewPassword) < 6 || len(body.NewPassword) > 32 {
		jsonError(w, "password must be 6-32 characters", http.StatusBadRequest)
		return
	}
	userID, err := s.store.ConsumePasswordResetToken(body.Token)
	if err != nil {
		jsonError(w, "invalid or expired token", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdateUserPassword(userID, string(hash)); err != nil {
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}
	// A password reset is a good time to invalidate every existing session -
	// if the reset was prompted by a compromised account, this logs the
	// attacker out along with everyone else.
	_ = s.store.DeleteRefreshTokensByUser(userID)

	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
