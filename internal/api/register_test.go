package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func tryRegister(t *testing.T, base, email, username, password string) (int, string) {
	t.Helper()
	resp := postJSON(t, base+"/api/v1/auth/register", "", map[string]string{
		"email": email, "username": username, "password": password,
	})
	body := decode[map[string]any](t, resp)
	msg, _ := body["error"].(string)
	return resp.StatusCode, msg
}

// Two accounts with the same name would let one impersonate the other: profile
// pages, direct messages, mentions and blocks all resolve a user by name.
func TestRegisteringATakenUsernameIsRefusedIgnoringCase(t *testing.T) {
	srv := newTestServer(t)
	registerUser(t, srv.URL, "alice")

	for _, name := range []string{"alice", "ALICE", "Alice"} {
		got, _ := tryRegister(t, srv.URL, name+"-2@example.com", name, "password123")
		if got != http.StatusConflict {
			t.Errorf("registering %q again: want 409, got %d", name, got)
		}
	}
}

func TestRegisteringATakenEmailIsRefused(t *testing.T) {
	srv := newTestServer(t)
	registerUser(t, srv.URL, "alice")

	if got, _ := tryRegister(t, srv.URL, "alice@example.com", "someone_else", "password123"); got != http.StatusConflict {
		t.Fatalf("a duplicate email: want 409, got %d", got)
	}
}

func TestRegistrationValidation(t *testing.T) {
	tests := []struct {
		name                      string
		email, username, password string
		wantStatus                int
		wantInMessage             string
	}{
		{"valid", "ok@example.com", "valid_user1", "password123", http.StatusCreated, ""},

		{"email without @", "not-an-email", "user_a", "password123", http.StatusBadRequest, "email"},
		{"email with a display name", "Eve <eve@example.com>", "user_b", "password123", http.StatusBadRequest, "email"},
		{"email with a line break", "eve@example.com\r\nBcc: x@y.z", "user_c", "password123", http.StatusBadRequest, "email"},
		{"email too long", strings.Repeat("a", 250) + "@example.com", "user_d", "password123", http.StatusBadRequest, "email"},

		{"username too short", "a1@example.com", "ab", "password123", http.StatusBadRequest, "username"},
		{"username too long", "a2@example.com", strings.Repeat("a", 21), "password123", http.StatusBadRequest, "username"},
		{"username with a space", "a3@example.com", "two words", "password123", http.StatusBadRequest, "username"},
		{"username with a slash", "a4@example.com", "a/b/c", "password123", http.StatusBadRequest, "username"},
		{"username that is an @mention", "a5@example.com", "@someone", "password123", http.StatusBadRequest, "username"},
		{"non-latin username", "a6@example.com", "алиса", "password123", http.StatusBadRequest, "username"},

		{"password too short", "p1@example.com", "user_e", "short", http.StatusBadRequest, "password"},
		{"password exactly at the minimum", "p2@example.com", "user_f", "12345678", http.StatusCreated, ""},
		{"password beyond bcrypt's limit", "p3@example.com", "user_g", strings.Repeat("x", 73), http.StatusBadRequest, "password"},
		{"password exactly at bcrypt's limit", "p4@example.com", "user_h", strings.Repeat("x", 72), http.StatusCreated, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A server per case: registration is rate limited per IP, and this
			// table has more cases than the limit allows.
			srv := newTestServer(t)
			got, msg := tryRegister(t, srv.URL, tt.email, tt.username, tt.password)
			if got != tt.wantStatus {
				t.Fatalf("status = %d (%q), want %d", got, msg, tt.wantStatus)
			}
			if tt.wantInMessage != "" && !strings.Contains(strings.ToLower(msg), tt.wantInMessage) {
				t.Errorf("message %q should mention %q", msg, tt.wantInMessage)
			}
		})
	}
}

// Previously a password longer than bcrypt's 72-byte limit made bcrypt fail and
// the server answer 500.
func TestOverlongPasswordIsAClientErrorNotAServerError(t *testing.T) {
	srv := newTestServer(t)
	got, _ := tryRegister(t, srv.URL, "long@example.com", "longpass", strings.Repeat("x", 200))
	if got >= 500 {
		t.Fatalf("an overlong password must be rejected as a client error, got %d", got)
	}
}

// resetToken stores a password-reset token for alice, as if the email had been
// received, and returns it.
func resetToken(t *testing.T, st interface {
	CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error
}, userID int) string {
	t.Helper()
	token := "reset-token-for-test"
	if err := st.CreatePasswordResetToken(userID, token, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return token
}

func TestPasswordResetUsesTheSamePolicyAsRegistration(t *testing.T) {
	srv, store := newTestServerWithStore(t)
	alice := registerUser(t, srv.URL, "alice")
	token := resetToken(t, store, userIDOf(t, srv.URL, alice))
	reset := func(password string) int {
		return status(t, http.MethodPost, srv.URL+"/api/v1/auth/reset-password", "",
			map[string]string{"token": token, "new_password": password})
	}

	if got := reset("short"); got != http.StatusBadRequest {
		t.Errorf("a too-short password: want 400, got %d", got)
	}
	if got := reset(strings.Repeat("x", maxPasswordBytes+1)); got != http.StatusBadRequest {
		t.Errorf("a password beyond bcrypt's limit: want 400, got %d", got)
	}
	// Rejecting a password must not have used up the single-use token.
	if got := reset(strings.Repeat("p", 64)); got != http.StatusOK {
		t.Fatalf("a 64-character password (typical of a password manager) must be accepted: got %d", got)
	}
	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "",
		map[string]string{"login": "alice", "password": strings.Repeat("p", 64)}); got != http.StatusOK {
		t.Errorf("logging in with the new password: want 200, got %d", got)
	}
}

// A login for an account that doesn't exist must cost about as much as one with
// a wrong password; otherwise response time reveals which accounts exist.
func TestLoginForUnknownAccountTakesAsLongAsAWrongPassword(t *testing.T) {
	old := bcryptCost
	bcryptCost = 9 // slow enough that skipping the comparison is unmistakable
	t.Cleanup(func() { bcryptCost = old })

	srv := newTestServer(t)
	registerUser(t, srv.URL, "alice")

	slowest := func(login string) (fastest time.Duration) {
		fastest = time.Hour
		for i := 0; i < 3; i++ {
			start := time.Now()
			status(t, http.MethodPost, srv.URL+"/api/v1/auth/login", "", map[string]string{"login": login, "password": "wrong-password"})
			if d := time.Since(start); d < fastest {
				fastest = d
			}
		}
		return fastest
	}
	known, unknown := slowest("alice"), slowest("nobody")

	// Taking the best of several runs filters out scheduling noise; a skipped
	// comparison would be 50-100x faster, so a factor of two is a safe margin.
	if unknown < known/2 {
		t.Fatalf("login for an unknown account (%v) is much faster than for a wrong password (%v): it leaks which accounts exist", unknown, known)
	}
}

func TestDummyPasswordHashIsAValidBcryptHash(t *testing.T) {
	if _, err := bcrypt.Cost(dummyPasswordHash()); err != nil {
		t.Fatalf("the placeholder must be a well-formed bcrypt hash, or comparing against it returns instantly: %v", err)
	}
	if bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte("anything")) == nil {
		t.Fatal("the placeholder must never match a password")
	}
}
