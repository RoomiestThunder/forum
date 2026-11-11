package database_test

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"forum/internal/database"
)

func TestRefreshTokenCanBeConsumedOnce(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		alice := mustUser(t, s, "alice")
		if err := s.CreateRefreshToken(alice, "tok-1", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}

		rt, err := s.ConsumeRefreshToken("tok-1")
		if err != nil {
			t.Fatalf("first use: %v", err)
		}
		if rt.UserID != alice || rt.Token != "tok-1" {
			t.Errorf("consumed %+v", rt)
		}

		if _, err := s.ConsumeRefreshToken("tok-1"); statusOf(err) != http.StatusNotFound {
			t.Fatalf("second use of the same token: want 404, got %d (%v)", statusOf(err), err)
		}
	})
}

func TestUnknownAndExpiredRefreshTokensAreNotConsumable(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		alice := mustUser(t, s, "alice")
		if err := s.CreateRefreshToken(alice, "expired", time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}

		for _, token := range []string{"expired", "never-issued", ""} {
			if _, err := s.ConsumeRefreshToken(token); statusOf(err) != http.StatusNotFound {
				t.Errorf("token %q: want 404, got %d (%v)", token, statusOf(err), err)
			}
		}
	})
}

// Reading a token and deleting it as two steps lets concurrent requests all
// read it before any deletes it, and each be issued a fresh pair. Of any number
// of simultaneous attempts exactly one may win.
//
// The window between the read and the delete is small, so a single round can
// pass against a broken implementation by luck. Many rounds, each with a fresh
// token contested by many goroutines released together, make that vanishingly
// unlikely.
func TestConcurrentConsumeSucceedsExactlyOnce(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		alice := mustUser(t, s, "alice")
		const rounds, attempts = 40, 16

		for round := 0; round < rounds; round++ {
			token := fmt.Sprintf("contested-%d", round)
			if err := s.CreateRefreshToken(alice, token, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}

			var wins atomic.Int32
			var ready, done sync.WaitGroup
			start := make(chan struct{})
			ready.Add(attempts)
			done.Add(attempts)
			for i := 0; i < attempts; i++ {
				go func() {
					defer done.Done()
					ready.Done()
					<-start
					if _, err := s.ConsumeRefreshToken(token); err == nil {
						wins.Add(1)
					}
				}()
			}
			ready.Wait() // every goroutine is parked at the gate
			close(start)
			done.Wait()

			if got := wins.Load(); got != 1 {
				t.Fatalf("round %d: %d of %d concurrent attempts consumed the token; exactly 1 must", round, got, attempts)
			}
		}
	})
}

func TestDeleteRefreshTokensByUserOnlyAffectsThatUser(t *testing.T) {
	forEachStore(t, func(t *testing.T, s database.Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")
		for token, owner := range map[string]int{"a1": alice, "a2": alice, "b1": bob} {
			if err := s.CreateRefreshToken(owner, token, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
		}

		if err := s.DeleteRefreshTokensByUser(alice); err != nil {
			t.Fatal(err)
		}

		for _, token := range []string{"a1", "a2"} {
			if _, err := s.ConsumeRefreshToken(token); err == nil {
				t.Errorf("alice's token %s should have been revoked", token)
			}
		}
		if _, err := s.ConsumeRefreshToken("b1"); err != nil {
			t.Errorf("bob's token must survive: %v", err)
		}
	})
}
