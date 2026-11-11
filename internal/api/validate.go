package api

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	// maxPasswordBytes is bcrypt's input limit. Anything longer is either
	// silently truncated or rejected, depending on the library version, so it
	// is refused up front with a clear message instead of surfacing as a 500.
	maxPasswordBytes = 72
	// maxEmailLength is the longest address SMTP allows (RFC 5321).
	maxEmailLength = 254
)

// usernamePattern is deliberately the same alphabet as an @mention, so every
// username can be mentioned, and is safe to put in a URL path unescaped.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

// validateEmail returns a message for the client, or "" if the address is acceptable.
func validateEmail(email string) string {
	const invalid = "enter a valid email address"
	if email == "" || len(email) > maxEmailLength || strings.ContainsAny(email, "\r\n") {
		return invalid
	}
	// ParseAddress also accepts "Name <addr>" and similar; only a bare
	// address is wanted here.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return invalid
	}
	return ""
}

func validateUsername(username string) string {
	if !usernamePattern.MatchString(username) {
		return "username must be 3-20 characters: letters, digits and underscores"
	}
	return ""
}

func validatePassword(password string) string {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return fmt.Sprintf("password must be at least %d characters", minPasswordLength)
	}
	if len(password) > maxPasswordBytes {
		return fmt.Sprintf("password must be at most %d bytes", maxPasswordBytes)
	}
	return ""
}

// validateRegistration returns the first problem with a registration, or "".
func validateRegistration(req registerRequest) string {
	for _, msg := range []string{
		validateEmail(req.Email),
		validateUsername(req.Username),
		validatePassword(req.Password),
	} {
		if msg != "" {
			return msg
		}
	}
	return ""
}

var (
	dummyHashMu sync.Mutex
	dummyHashes = map[int][]byte{}
)

// dummyPasswordHash is a valid bcrypt hash at the current cost, compared
// against when a login names no existing account. Skipping the comparison
// would make "no such account" answer in microseconds and "wrong password"
// in ~100ms, which lets anyone probe which emails and usernames are
// registered just by timing the response.
func dummyPasswordHash() []byte {
	dummyHashMu.Lock()
	defer dummyHashMu.Unlock()
	if h, ok := dummyHashes[bcryptCost]; ok {
		return h
	}
	h, err := bcrypt.GenerateFromPassword([]byte("placeholder, never matches a real account"), bcryptCost)
	if err != nil {
		panic(errors.New("api: cannot generate placeholder password hash: " + err.Error()))
	}
	dummyHashes[bcryptCost] = h
	return h
}
