package database

import (
	"strings"
	"testing"
)

func TestOAuthBaseUsername(t *testing.T) {
	for in, want := range map[string]string{
		"octocat":               "octocat",
		"Octo_Cat9":             "Octo_Cat9",
		"octo-cat":              "octo_cat",
		"a.b c":                 "a_b_c",
		"ab":                    "user_ab",
		"":                      "user",
		"---":                   "user",
		"имя":                   "user",
		strings.Repeat("x", 39): strings.Repeat("x", 20),
	} {
		if got := oauthBaseUsername(in); got != want {
			t.Errorf("oauthBaseUsername(%q) = %q, want %q", in, got, want)
		}
		if got := oauthBaseUsername(in); len(got) < 3 || len(got) > maxUsernameLength {
			t.Errorf("oauthBaseUsername(%q) = %q breaks the 3-20 length rule", in, got)
		}
	}
}

func TestUsernameCandidateAlwaysFits(t *testing.T) {
	base := strings.Repeat("x", 20)
	seen := map[string]bool{}
	for attempt := 0; attempt < 120; attempt++ {
		c := usernameCandidate(base, attempt)
		if len(c) > maxUsernameLength {
			t.Fatalf("attempt %d: %q is %d characters", attempt, c, len(c))
		}
		if seen[c] {
			t.Fatalf("attempt %d repeats %q", attempt, c)
		}
		seen[c] = true
	}
	if usernameCandidate("alice", 0) != "alice" || usernameCandidate("alice", 3) != "alice3" {
		t.Error("short bases should just get the number appended")
	}
}
