package database

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	apperrors "forum/internal/errors"
)

// maxUsernameLength matches the rule enforced on registration, so an account
// created through a provider is indistinguishable in shape from a registered one.
const maxUsernameLength = 20

// oauthPlaceholderPassword is stored in place of a password hash for accounts
// that sign in through a provider. It is not a valid bcrypt hash, so no
// password can ever match it and password login is impossible until the user
// sets one through the reset flow.
func oauthPlaceholderPassword(provider, providerUserID string) string {
	return "oauth:" + provider + ":" + providerUserID
}

// oauthNoreplyEmail stands in for an email the provider didn't give us (or
// didn't verify). It is unique per identity, can't collide with a real
// address, and so can never cause a link to an existing account.
func oauthNoreplyEmail(provider, providerUserID string) string {
	return fmt.Sprintf("%s-%s@users.noreply.%s", provider, providerUserID, provider)
}

// oauthBaseUsername turns a provider's login into something that satisfies the
// username rules (3-20 letters, digits and underscores): GitHub allows hyphens
// and up to 39 characters, which registration does not.
func oauthBaseUsername(preferred string) string {
	var b strings.Builder
	for _, r := range preferred {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if strings.Trim(name, "_") == "" {
		// Nothing usable survived (a login in another script, say): a name made of
		// underscores would say nothing about who the account belongs to.
		name = "user"
	}
	if len(name) < 3 {
		name = "user_" + name
	}
	if len(name) > maxUsernameLength {
		name = name[:maxUsernameLength]
	}
	return name
}

// usernameCandidate is base for attempt 0 and base+"1", base+"2", ... after,
// shortened so that the whole name still fits the length limit.
func usernameCandidate(base string, attempt int) string {
	if attempt == 0 {
		return base
	}
	suffix := fmt.Sprintf("%d", attempt)
	if len(base)+len(suffix) > maxUsernameLength {
		base = base[:maxUsernameLength-len(suffix)]
	}
	return base + suffix
}

func isConflict(err error) bool {
	var appErr *apperrors.AppError
	return errors.As(err, &appErr) && appErr.HTTPStatusCode() == http.StatusConflict
}

// maxUsernameAttempts bounds the search for a free username.
const maxUsernameAttempts = 20
