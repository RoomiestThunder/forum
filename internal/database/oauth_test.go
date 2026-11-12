package database_test

import (
	"strings"
	"testing"
	"time"

	"forum/internal/database"
)

const provider = "github"

func signInWithGitHub(t *testing.T, s database.Store, id, email, login string) int {
	t.Helper()
	u, err := s.FindOrCreateOAuthUser(provider, id, email, login, "")
	if err != nil {
		t.Fatalf("sign in %s: %v", login, err)
	}
	return u.ID
}

func TestOAuthSignUpCreatesAVerifiedPasswordlessAccount(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		u, err := s.FindOrCreateOAuthUser(provider, "101", "octocat@example.com", "octocat", "https://example.com/a.png")
		if err != nil {
			t.Fatal(err)
		}

		if u.Username != "octocat" || u.Email != "octocat@example.com" || u.AvatarURL != "https://example.com/a.png" {
			t.Errorf("unexpected account: %+v", u)
		}
		if !u.EmailVerified {
			t.Error("the provider vouched for the address, so it is verified")
		}
		if !strings.HasPrefix(u.Password, "oauth:") {
			t.Errorf("an OAuth-only account must hold an unmatchable placeholder, not a hash: %q", u.Password)
		}
	})
}

func TestOAuthRepeatSignInReturnsTheSameAccount(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		first := signInWithGitHub(t, s, "101", "octocat@example.com", "octocat")
		// Even if the user later changes their GitHub email or login.
		second := signInWithGitHub(t, s, "101", "new-address@example.com", "renamed")

		if first != second {
			t.Fatalf("the same identity must map to the same account: %d vs %d", first, second)
		}
	})
}

func TestOAuthUsernamesFitTheRegistrationRules(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		for i, tt := range []struct{ login, want string }{
			{"octocat", "octocat"},
			{"octo-cat", "octo_cat"}, // GitHub allows hyphens, registration doesn't
			{"a", "user_a"},          // too short
			{"x-" + strings.Repeat("y", 37), "x_" + strings.Repeat("y", 18)}, // GitHub logins run to 39
			{"имя", "user"}, // nothing usable survives
		} {
			id := signInWithGitHub(t, s, string(rune('a'+i))+"-id", "", tt.login)
			u, _ := s.GetUserByID(id)
			if u.Username != tt.want {
				t.Errorf("login %q became %q, want %q", tt.login, u.Username, tt.want)
			}
		}
	})
}

func TestOAuthUsernameCollisionsGetASuffixThatStillFits(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		long := strings.Repeat("z", 25) // sanitised to 20 characters

		names := map[string]bool{}
		for _, id := range []string{"1", "2", "3"} {
			u, _ := s.GetUserByID(signInWithGitHub(t, s, id, "", long))
			if len(u.Username) > 20 {
				t.Errorf("%q is longer than 20 characters", u.Username)
			}
			if names[strings.ToLower(u.Username)] {
				t.Errorf("%q was handed out twice", u.Username)
			}
			names[strings.ToLower(u.Username)] = true
		}
		if len(names) != 3 {
			t.Fatalf("want three distinct usernames, got %v", names)
		}

		// A registered user already has the name: the provider account yields to it.
		if _, err := s.CreateUser("reg@example.com", "taken_name", "hash"); err != nil {
			t.Fatal(err)
		}
		u, _ := s.GetUserByID(signInWithGitHub(t, s, "9", "", "Taken_Name"))
		if strings.EqualFold(u.Username, "taken_name") {
			t.Errorf("must not reuse an existing username, got %q", u.Username)
		}
	})
}

func TestOAuthLinksToAVerifiedAccountWithoutDisturbingIt(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		bob := mustUser(t, s, "bob")
		if err := s.SetEmailVerified(bob); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateRefreshToken(bob, "bobs-session", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}

		got := signInWithGitHub(t, s, "55", "bob@example.com", "bob-on-github")

		if got != bob {
			t.Fatalf("should link to bob (%d), got %d", bob, got)
		}
		u, _ := s.GetUserByID(bob)
		if u.Password != "hash-bob" {
			t.Errorf("a verified account keeps its password, got %q", u.Password)
		}
		if _, err := s.ConsumeRefreshToken("bobs-session"); err != nil {
			t.Errorf("a verified account keeps its sessions: %v", err)
		}
		// Linked: signing in again lands on the same account.
		if again := signInWithGitHub(t, s, "55", "", "bob-on-github"); again != bob {
			t.Errorf("the identity should now resolve to bob, got %d", again)
		}
	})
}

// Registration doesn't verify email, so an account can exist for an address
// that its creator doesn't own, with a password the creator knows. When the real
// owner then signs in through the provider, linking must not leave that password
// (or any session or token the squatter holds) working.
func TestOAuthLinkingTakesOverAnUnverifiedAccount(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		squatted := mustUser(t, s, "victim") // registered by someone else, never verified
		future := time.Now().Add(time.Hour)
		if err := s.CreateRefreshToken(squatted, "attackers-session", future); err != nil {
			t.Fatal(err)
		}
		if err := s.CreatePasswordResetToken(squatted, "attackers-reset", future); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateEmailVerificationToken(squatted, "attackers-verify", future); err != nil {
			t.Fatal(err)
		}

		owner := signInWithGitHub(t, s, "77", "victim@example.com", "the-real-victim")

		if owner != squatted {
			t.Fatalf("the owner lands on the existing account (%d), got %d", squatted, owner)
		}
		u, _ := s.GetUserByID(squatted)
		if !strings.HasPrefix(u.Password, "oauth:") {
			t.Errorf("the squatter's password must be gone, got %q", u.Password)
		}
		if !u.EmailVerified {
			t.Error("the provider proved ownership, so the email is verified now")
		}
		if _, err := s.ConsumeRefreshToken("attackers-session"); err == nil {
			t.Error("the squatter's refresh token must be revoked")
		}
		if _, err := s.ConsumePasswordResetToken("attackers-reset"); err == nil {
			t.Error("an outstanding password-reset token must be revoked")
		}
		if _, err := s.ConsumeEmailVerificationToken("attackers-verify"); err == nil {
			t.Error("an outstanding verification token must be revoked")
		}
	})
}

func TestOAuthEmailMatchIgnoresCase(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		id, err := s.CreateUser("Bob@Example.com", "bobby", "hash")
		if err != nil {
			t.Fatal(err)
		}
		if got := signInWithGitHub(t, s, "55", "bob@example.com", "bobby-gh"); got != id {
			t.Fatalf("addresses differing only in case are the same mailbox: want %d, got %d", id, got)
		}
	})
}

// Without a verified address there is nothing to link on, and two such users
// must not be mistaken for one another.
func TestOAuthWithoutAnEmailNeverLinks(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		a := signInWithGitHub(t, s, "1", "", "alice-gh")
		b := signInWithGitHub(t, s, "2", "", "bob-gh")
		if a == b {
			t.Fatal("two identities without an email must be two accounts")
		}
		u, _ := s.GetUserByID(a)
		if !strings.HasSuffix(u.Email, "@users.noreply.github") {
			t.Errorf("expected a placeholder address, got %q", u.Email)
		}
	})
}
