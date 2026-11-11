package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "a-test-secret-that-is-long-enough-1234"

func newTestManager() *Manager {
	return NewManager(testSecret, 15*time.Minute, 24*time.Hour)
}

func validClaims() Claims {
	return Claims{
		UserID:   7,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "7",
		},
	}
}

func signWith(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAccessTokenRoundTrip(t *testing.T) {
	m := newTestManager()

	token, err := m.GenerateAccessToken(7, "alice")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("a freshly issued token must validate: %v", err)
	}

	if claims.UserID != 7 || claims.Username != "alice" || claims.Subject != "7" {
		t.Errorf("claims = %+v", claims)
	}
	if remaining := time.Until(claims.ExpiresAt.Time); remaining < 14*time.Minute || remaining > 15*time.Minute {
		t.Errorf("expiry should be about 15 minutes away, got %v", remaining)
	}
}

func TestExpiredTokenIsRejected(t *testing.T) {
	m := NewManager(testSecret, -time.Minute, time.Hour) // already expired when issued

	token, err := m.GenerateAccessToken(7, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(token); err == nil {
		t.Fatal("an expired token must be rejected")
	}
}

func TestTokenSignedWithAnotherSecretIsRejected(t *testing.T) {
	token, err := NewManager("some-other-secret-entirely-0123456789", time.Hour, time.Hour).GenerateAccessToken(7, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newTestManager().ValidateAccessToken(token); err == nil {
		t.Fatal("a token signed with a different secret must be rejected")
	}
}

// Changing the payload without being able to re-sign it - say, to become
// another user - must invalidate the signature.
func TestTamperedPayloadIsRejected(t *testing.T) {
	m := newTestManager()
	token, _ := m.GenerateAccessToken(7, "alice")

	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["user_id"] = 1 // someone with more privileges
	forged, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(forged)

	if _, err := m.ValidateAccessToken(strings.Join(parts, ".")); err == nil {
		t.Fatal("a token with an altered payload must be rejected")
	}
}

// The classic JWT attack: declare "alg": "none" and drop the signature.
func TestUnsignedTokenIsRejected(t *testing.T) {
	token := signWith(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims())

	if _, err := newTestManager().ValidateAccessToken(token); err == nil {
		t.Fatal(`a token with "alg": "none" must be rejected`)
	}
}

// The server signs with HS256 only. Accepting other HMAC variants - or any
// algorithm the token's own header asks for - widens the attack surface for
// no benefit.
func TestOnlyHS256IsAccepted(t *testing.T) {
	secret := []byte(testSecret)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	for name, token := range map[string]string{
		"HS384": signWith(t, jwt.SigningMethodHS384, secret, validClaims()),
		"HS512": signWith(t, jwt.SigningMethodHS512, secret, validClaims()),
		"RS256": signWith(t, jwt.SigningMethodRS256, rsaKey, validClaims()),
	} {
		if _, err := newTestManager().ValidateAccessToken(token); err == nil {
			t.Errorf("a %s token must be rejected", name)
		}
	}

	if _, err := newTestManager().ValidateAccessToken(signWith(t, jwt.SigningMethodHS256, secret, validClaims())); err != nil {
		t.Errorf("HS256 with the right secret must be accepted: %v", err)
	}
}

// A token that never expires could never be revoked.
func TestTokenWithoutExpiryIsRejected(t *testing.T) {
	claims := validClaims()
	claims.ExpiresAt = nil

	token := signWith(t, jwt.SigningMethodHS256, []byte(testSecret), claims)
	if _, err := newTestManager().ValidateAccessToken(token); err == nil {
		t.Fatal("a token with no expiry must be rejected")
	}
}

func TestMalformedTokensAreRejected(t *testing.T) {
	for _, token := range []string{"", "garbage", "a.b", "a.b.c", "....", "Bearer abc.def.ghi"} {
		if _, err := newTestManager().ValidateAccessToken(token); err == nil {
			t.Errorf("%q must be rejected", token)
		}
	}
}

func TestRefreshTokensAreRandomAndOpaque(t *testing.T) {
	m := newTestManager()
	seen := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		token, err := m.GenerateRefreshToken()
		if err != nil {
			t.Fatal(err)
		}
		// 32 random bytes, base64url without padding.
		if len(token) != 43 {
			t.Fatalf("token %q has length %d, want 43", token, len(token))
		}
		if strings.ContainsAny(token, "+/=") {
			t.Fatalf("token %q is not URL-safe base64 without padding", token)
		}
		if seen[token] {
			t.Fatalf("token %q was issued twice", token)
		}
		seen[token] = true
	}

	// Opaque: it is not a JWT, so it can't be mistaken for (or replayed as) an access token.
	token, _ := m.GenerateRefreshToken()
	if _, err := m.ValidateAccessToken(token); err == nil {
		t.Fatal("a refresh token must never validate as an access token")
	}
}
