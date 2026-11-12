package database_test

import (
	"net/http"
	"strings"
	"testing"

	"forum/internal/database"
)

func TestUsernamesAreUniqueIgnoringCase(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		mustUser(t, s, "alice")

		for _, name := range []string{"alice", "ALICE", "Alice"} {
			_, err := s.CreateUser("other-"+name+"@example.com", name, "hash")
			if statusOf(err) != http.StatusConflict {
				t.Errorf("registering %q again: want 409, got %d (%v)", name, statusOf(err), err)
			}
		}
		if _, err := s.GetUserByUsername("alice"); err != nil {
			t.Fatalf("the original account must be unaffected: %v", err)
		}
	})
}

func TestEmailsAreUnique(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		mustUser(t, s, "alice")
		_, err := s.CreateUser("alice@example.com", "someone_else", "hash")
		if statusOf(err) != http.StatusConflict {
			t.Fatalf("a duplicate email: want 409, got %d (%v)", statusOf(err), err)
		}
	})
}

func TestFirstUserBecomesAdminAndOnlyTheFirst(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		first, second := mustUser(t, s, "first"), mustUser(t, s, "second")

		if u, _ := s.GetUserByID(first); !u.IsAdmin {
			t.Error("the first account on a fresh install should be the admin")
		}
		if u, _ := s.GetUserByID(second); u.IsAdmin {
			t.Error("later accounts must not be admins")
		}
	})
}

// Emails are compared ignoring case by login, password reset and OAuth linking,
// so a case-variant of an existing address would be a second account for the
// same mailbox.
func TestEmailsAreUniqueIgnoringCase(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		mustUser(t, s, "alice") // alice@example.com

		for _, email := range []string{"ALICE@example.com", "Alice@Example.COM"} {
			_, err := s.CreateUser(email, "someone_"+strings.ToLower(email[:3]), "hash")
			if statusOf(err) != http.StatusConflict {
				t.Errorf("%q: want 409, got %d (%v)", email, statusOf(err), err)
			}
		}
	})
}
