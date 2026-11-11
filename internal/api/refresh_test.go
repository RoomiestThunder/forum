package api

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type tokenPair struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

func registerPair(t *testing.T, base, username string) tokenPair {
	t.Helper()
	resp := postJSON(t, base+"/api/v1/auth/register", "", map[string]string{
		"email": username + "@example.com", "username": username, "password": "password123",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", resp.StatusCode)
	}
	return decode[tokenPair](t, resp)
}

func refresh(t *testing.T, base, token string) (int, tokenPair) {
	t.Helper()
	resp := postJSON(t, base+"/api/v1/auth/refresh", "", map[string]string{"refresh_token": token})
	code := resp.StatusCode
	if code != http.StatusOK {
		_ = resp.Body.Close()
		return code, tokenPair{}
	}
	return code, decode[tokenPair](t, resp)
}

func TestRefreshRotatesTheTokenAndOldOneStopsWorking(t *testing.T) {
	srv := newTestServer(t)
	first := registerPair(t, srv.URL, "alice")

	code, second := refresh(t, srv.URL, first.Refresh)
	if code != http.StatusOK || second.Refresh == "" || second.Refresh == first.Refresh {
		t.Fatalf("refresh: status %d, new token %q (old %q)", code, second.Refresh, first.Refresh)
	}
	if got := status(t, http.MethodGet, srv.URL+"/api/v1/me", second.Access, nil); got != http.StatusOK {
		t.Errorf("the new access token should work: got %d", got)
	}

	if code, _ := refresh(t, srv.URL, first.Refresh); code != http.StatusUnauthorized {
		t.Fatalf("replaying the used refresh token: want 401, got %d", code)
	}
	if code, _ := refresh(t, srv.URL, second.Refresh); code != http.StatusOK {
		t.Fatalf("the newest refresh token must still work: got %d", code)
	}
}

// Two requests presenting the same token at the same moment - a stolen token
// raced against its owner, or a client retrying - must not both be issued a
// new pair. Several rounds, each released from a common gate, because a single
// round can slip past a non-atomic implementation by luck.
func TestConcurrentRefreshesOfOneTokenIssueOnePair(t *testing.T) {
	srv := newTestServer(t)
	const rounds, attempts = 5, 8 // 40 refreshes, within the per-IP limit of 60/min

	pair := registerPair(t, srv.URL, "alice")
	for round := 0; round < rounds; round++ {
		var ok atomic.Int32
		var next atomic.Value // the winner's new token, for the next round
		var ready, done sync.WaitGroup
		start := make(chan struct{})
		ready.Add(attempts)
		done.Add(attempts)
		for i := 0; i < attempts; i++ {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				if code, fresh := refresh(t, srv.URL, pair.Refresh); code == http.StatusOK {
					ok.Add(1)
					next.Store(fresh)
				}
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()

		if got := ok.Load(); got != 1 {
			t.Fatalf("round %d: %d of %d concurrent refreshes succeeded; exactly 1 must", round, got, attempts)
		}
		pair = next.Load().(tokenPair)
	}
}

func TestRefreshTokenLifetimeComesFromTheConfiguredDuration(t *testing.T) {
	srv, store := newTestServerWithStore(t) // the test manager's refresh lifetime is 24h
	pair := registerPair(t, srv.URL, "alice")

	rt, err := store.ConsumeRefreshToken(pair.Refresh)
	if err != nil {
		t.Fatal(err)
	}
	// The store used to fix this at 7 days, ignoring JWT_REFRESH_DURATION_DAYS.
	if remaining := time.Until(rt.ExpiresAt); remaining < 23*time.Hour || remaining > 24*time.Hour+time.Minute {
		t.Fatalf("the token should expire in ~24h, got %v", remaining)
	}
}

func TestBannedUserCannotRefresh(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	bob := registerPair(t, srv.URL, "bob")

	bobID := userIDOf(t, srv.URL, bob.Access)
	status(t, http.MethodPost, srv.URL+"/api/v1/admin/users/"+strconv.Itoa(bobID)+"/ban", admin, map[string]bool{"banned": true})

	if code, _ := refresh(t, srv.URL, bob.Refresh); code != http.StatusForbidden {
		t.Fatalf("a banned user's refresh: want 403, got %d", code)
	}
}

func TestLogoutRevokesRefreshTokens(t *testing.T) {
	srv := newTestServer(t)
	pair := registerPair(t, srv.URL, "alice")

	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/logout", pair.Access, nil); got != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", got)
	}
	if code, _ := refresh(t, srv.URL, pair.Refresh); code != http.StatusUnauthorized {
		t.Fatalf("after logout the refresh token must be dead: got %d", code)
	}
}

func TestRefreshWithGarbageIsUnauthorized(t *testing.T) {
	srv := newTestServer(t)
	for _, token := range []string{"not-a-token", "x"} {
		if code, _ := refresh(t, srv.URL, token); code != http.StatusUnauthorized {
			t.Errorf("token %q: want 401, got %d", token, code)
		}
	}
	if got := status(t, http.MethodPost, srv.URL+"/api/v1/auth/refresh", "", map[string]string{}); got != http.StatusBadRequest {
		t.Errorf("a missing token: want 400, got %d", got)
	}
}
