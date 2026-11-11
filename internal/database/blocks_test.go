package database

import (
	"reflect"
	"testing"

	"forum/internal/models"
)

func blockedNames(t *testing.T, s Store, blocker int) []string {
	t.Helper()
	users, err := s.ListBlockedUsers(blocker)
	if err != nil {
		t.Fatalf("list blocked: %v", err)
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Username)
	}
	return names
}

func TestBlockRoundTrip(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")

		if blocked, err := s.IsBlocked(alice, bob); err != nil || blocked {
			t.Fatalf("nobody is blocked initially: blocked=%v err=%v", blocked, err)
		}

		if err := s.BlockUser(alice, bob); err != nil {
			t.Fatal(err)
		}
		if blocked, _ := s.IsBlocked(alice, bob); !blocked {
			t.Fatal("alice should have blocked bob")
		}
		// Directional: bob has not blocked alice.
		if blocked, _ := s.IsBlocked(bob, alice); blocked {
			t.Fatal("a block must not apply in the reverse direction")
		}

		users, err := s.ListBlockedUsers(alice)
		if err != nil {
			t.Fatal(err)
		}
		want := []*models.UserSummary{{ID: bob, Username: "bob", AvatarURL: ""}}
		if !reflect.DeepEqual(users, want) {
			t.Fatalf("blocked list = %+v, want %+v", users, want)
		}

		if err := s.UnblockUser(alice, bob); err != nil {
			t.Fatal(err)
		}
		if blocked, _ := s.IsBlocked(alice, bob); blocked {
			t.Fatal("bob should be unblocked")
		}
		if names := blockedNames(t, s, alice); len(names) != 0 {
			t.Fatalf("blocked list should be empty after unblocking, got %v", names)
		}
	})
}

func TestBlockAndUnblockAreIdempotent(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")

		for i := 0; i < 2; i++ {
			if err := s.BlockUser(alice, bob); err != nil {
				t.Fatalf("block #%d: %v", i+1, err)
			}
		}
		if names := blockedNames(t, s, alice); len(names) != 1 {
			t.Fatalf("blocking twice must not duplicate the entry, got %v", names)
		}

		for i := 0; i < 2; i++ {
			if err := s.UnblockUser(alice, bob); err != nil {
				t.Fatalf("unblock #%d: %v", i+1, err)
			}
		}
	})
}

func TestCannotBlockYourself(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		if err := s.BlockUser(alice, alice); err == nil {
			t.Fatal("blocking yourself must be rejected by the database")
		}
	})
}

func TestBlockedListIsPerUserAndNewestFirst(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob, carol, dave := mustUser(t, s, "alice"), mustUser(t, s, "bob"), mustUser(t, s, "carol"), mustUser(t, s, "dave")

		for _, target := range []int{bob, carol} {
			if err := s.BlockUser(alice, target); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.BlockUser(dave, bob); err != nil {
			t.Fatal(err)
		}

		if got, want := blockedNames(t, s, alice), []string{"carol", "bob"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("alice's list = %v, want %v (most recent first)", got, want)
		}
		if got, want := blockedNames(t, s, dave), []string{"bob"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("dave's list = %v, want %v (must not include alice's blocks)", got, want)
		}
		if got := blockedNames(t, s, bob); len(got) != 0 {
			t.Fatalf("being blocked doesn't put anyone on your own list, got %v", got)
		}
	})
}
