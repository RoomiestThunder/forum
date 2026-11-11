package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "forum/internal/websocket"

	"github.com/gorilla/websocket"
)

func newWSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	hub := ws.NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(hub.Close)
	srv, _ := newTestServerFull(t, Options{BaseURL: "http://localhost"}, hub)
	return srv
}

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func wsAuth(t *testing.T, conn *websocket.Conn, token string) {
	t.Helper()
	if err := conn.WriteJSON(map[string]string{"type": "auth", "token": token}); err != nil {
		t.Fatal(err)
	}
}

func wsReadType(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("not JSON: %q", data)
	}
	return m.Type
}

func wsExpectClosed(t *testing.T, conn *websocket.Conn) error {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("expected the connection to be closed")
	}
	return err
}

func TestWebSocketAuthenticatesViaFirstFrame(t *testing.T) {
	srv := newWSTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")

	// Note the URL carries no token at all.
	conn := dialWS(t, srv)
	wsAuth(t, conn, alice)
	if got := wsReadType(t, conn); got != "ready" {
		t.Fatalf("want ready after auth, got %q", got)
	}

	// A message to alice is pushed over her socket.
	resp := postJSON(t, srv.URL+"/api/v1/messages/alice", bob, map[string]any{"content": "hi alice"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send message: want 201, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if got := wsReadType(t, conn); got != "new_message" {
		t.Fatalf("want new_message push, got %q", got)
	}
}

func TestWebSocketRejectsInvalidToken(t *testing.T) {
	srv := newWSTestServer(t)
	conn := dialWS(t, srv)

	wsAuth(t, conn, "not-a-jwt")

	if err := wsExpectClosed(t, conn); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("want policy-violation close, got %v", err)
	}
}

// The token used to travel in ?token=..., ending up in access logs. It is no
// longer read from the URL, so a socket opened that way must not be treated as
// authenticated.
func TestWebSocketIgnoresTokenInURL(t *testing.T) {
	srv := newWSTestServer(t)
	alice := registerUser(t, srv.URL, "alice")

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws?token=" + alice
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Authenticated sockets get a ready frame immediately; this one must not.
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, data, err := conn.ReadMessage(); err == nil {
		t.Fatalf("a URL token must not authenticate the socket, got frame %q", data)
	}
}

func TestWebSocketRejectsBannedUser(t *testing.T) {
	srv := newWSTestServer(t)
	admin := registerUser(t, srv.URL, "admin") // first user is the admin
	bob := registerUser(t, srv.URL, "bob")

	bobID := userIDOf(t, srv.URL, bob)
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, bobID), admin, map[string]bool{"banned": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ban: want 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// bob's access token is still cryptographically valid; the ban must win.
	conn := dialWS(t, srv)
	wsAuth(t, conn, bob)
	if err := wsExpectClosed(t, conn); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("want policy-violation close for a banned user, got %v", err)
	}
}

func TestBanClosesLiveConnection(t *testing.T) {
	srv := newWSTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	bob := registerUser(t, srv.URL, "bob")

	conn := dialWS(t, srv)
	wsAuth(t, conn, bob)
	if got := wsReadType(t, conn); got != "ready" {
		t.Fatalf("want ready, got %q", got)
	}

	bobID := userIDOf(t, srv.URL, bob)
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, bobID), admin, map[string]bool{"banned": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ban: want 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Banning must cut the live socket, not just reject future requests.
	_ = wsExpectClosed(t, conn)
}

func TestWebSocketUnavailableWithoutHub(t *testing.T) {
	srv := newTestServer(t) // built without a hub
	resp, err := http.Get(srv.URL + "/api/v1/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 without a hub, got %d", resp.StatusCode)
	}
}

// userIDOf resolves a user's id from their own access token; public profiles
// deliberately don't expose it.
func userIDOf(t *testing.T, base, token string) int {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/v1/me", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: want 200, got %d", resp.StatusCode)
	}
	me := decode[map[string]any](t, resp)
	id, ok := me["id"].(float64)
	if !ok {
		t.Fatalf("/me has no numeric id: %v", me)
	}
	return int(id)
}
