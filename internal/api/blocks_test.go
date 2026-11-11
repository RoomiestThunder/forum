package api

import (
	"fmt"
	"net/http"
	"testing"
)

func blockedUsernames(t *testing.T, base, token string) []string {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/v1/blocks", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list blocks: want 200, got %d", resp.StatusCode)
	}
	users := decode[[]map[string]any](t, resp)
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u["username"].(string))
	}
	return names
}

func unreadNotifications(t *testing.T, base, token string) int {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/v1/notifications/unread-count", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unread count: want 200, got %d", resp.StatusCode)
	}
	return decode[map[string]int](t, resp)["count"]
}

func status(t *testing.T, method, url, token string, payload any) int {
	t.Helper()
	resp := doJSON(t, method, url, token, payload)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestBlocksRequireAuth(t *testing.T) {
	srv := newTestServer(t)
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/blocks"},
		{http.MethodPut, "/api/v1/blocks/bob"},
		{http.MethodDelete, "/api/v1/blocks/bob"},
	} {
		if got := status(t, tt.method, srv.URL+tt.path, "", nil); got != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: want 401, got %d", tt.method, tt.path, got)
		}
	}
}

func TestBlockLifecycle(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	registerUser(t, srv.URL, "bob")

	if names := blockedUsernames(t, srv.URL, alice); len(names) != 0 {
		t.Fatalf("no one is blocked initially, got %v", names)
	}

	for i := 0; i < 2; i++ { // PUT is idempotent
		if got := status(t, http.MethodPut, srv.URL+"/api/v1/blocks/bob", alice, nil); got != http.StatusOK {
			t.Fatalf("block #%d: want 200, got %d", i+1, got)
		}
	}
	if names := blockedUsernames(t, srv.URL, alice); len(names) != 1 || names[0] != "bob" {
		t.Fatalf("blocked list = %v, want [bob]", names)
	}

	for i := 0; i < 2; i++ {
		if got := status(t, http.MethodDelete, srv.URL+"/api/v1/blocks/bob", alice, nil); got != http.StatusOK {
			t.Fatalf("unblock #%d: want 200, got %d", i+1, got)
		}
	}
	if names := blockedUsernames(t, srv.URL, alice); len(names) != 0 {
		t.Fatalf("list should be empty after unblocking, got %v", names)
	}
}

func TestBlockValidation(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")

	if got := status(t, http.MethodPut, srv.URL+"/api/v1/blocks/alice", alice, nil); got != http.StatusBadRequest {
		t.Errorf("blocking yourself: want 400, got %d", got)
	}
	if got := status(t, http.MethodPut, srv.URL+"/api/v1/blocks/nobody", alice, nil); got != http.StatusNotFound {
		t.Errorf("blocking an unknown user: want 404, got %d", got)
	}
	if got := status(t, http.MethodDelete, srv.URL+"/api/v1/blocks/nobody", alice, nil); got != http.StatusNotFound {
		t.Errorf("unblocking an unknown user: want 404, got %d", got)
	}
}

func TestBlockingStopsDirectMessagesBothWays(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	send := func(token, to string) int {
		return status(t, http.MethodPost, srv.URL+"/api/v1/messages/"+to, token, map[string]string{"content": "hello"})
	}

	if got := send(bob, "alice"); got != http.StatusCreated {
		t.Fatalf("before blocking, bob can message alice: got %d", got)
	}
	status(t, http.MethodPut, srv.URL+"/api/v1/blocks/bob", alice, nil)

	if got := send(bob, "alice"); got != http.StatusForbidden {
		t.Errorf("the blocked user messaging the blocker: want 403, got %d", got)
	}
	if got := send(alice, "bob"); got != http.StatusForbidden {
		t.Errorf("the blocker messaging someone they blocked: want 403, got %d", got)
	}

	// Existing history is not erased.
	if got := status(t, http.MethodGet, srv.URL+"/api/v1/messages/bob", alice, nil); got != http.StatusOK {
		t.Errorf("history stays readable: want 200, got %d", got)
	}

	status(t, http.MethodDelete, srv.URL+"/api/v1/blocks/bob", alice, nil)
	if got := send(bob, "alice"); got != http.StatusCreated {
		t.Errorf("after unblocking, bob can message alice again: got %d", got)
	}
}

// What a blocked user sees must not reveal that they are blocked, while the
// blocker gets an actionable message.
func TestBlockedSenderCannotTellTheyAreBlocked(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	status(t, http.MethodPut, srv.URL+"/api/v1/blocks/bob", alice, nil)

	resp := postJSON(t, srv.URL+"/api/v1/messages/alice", bob, map[string]string{"content": "hi"})
	msg := decode[map[string]string](t, resp)["error"]
	if msg != "you can't message this user" {
		t.Errorf("blocked sender should get the generic refusal, got %q", msg)
	}

	resp = postJSON(t, srv.URL+"/api/v1/messages/bob", alice, map[string]string{"content": "hi"})
	msg = decode[map[string]string](t, resp)["error"]
	if msg == "you can't message this user" || msg == "" {
		t.Errorf("the blocker should be told to unblock, got %q", msg)
	}
}

func TestBlockSuppressesNotificationsFromBlockedUser(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	carol := registerUser(t, srv.URL, "carol")
	postID := createPost(t, srv.URL, alice, "Alice's post")
	status(t, http.MethodPut, srv.URL+"/api/v1/blocks/bob", alice, nil)

	comment := func(token, text string) {
		t.Helper()
		if got := status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), token, map[string]string{"content": text}); got != http.StatusCreated {
			t.Fatalf("comment: want 201, got %d", got)
		}
	}

	// bob's comment, @mention and like must all be silent for alice.
	comment(bob, "nice post @alice")
	status(t, http.MethodPost, srv.URL+"/api/v1/like/post", bob, map[string]any{"post_id": postID, "is_like": true})
	if got := unreadNotifications(t, srv.URL, alice); got != 0 {
		t.Fatalf("alice blocked bob, so bob must not notify her: got %d notifications", got)
	}

	// Blocking is targeted: carol still reaches alice.
	comment(carol, "hello")
	if got := unreadNotifications(t, srv.URL, alice); got != 1 {
		t.Fatalf("carol isn't blocked: want 1 notification, got %d", got)
	}
}
