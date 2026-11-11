package websocket

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"forum/internal/metrics"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// testTokens is the fixed token -> user mapping the test authenticator uses.
var testTokens = map[string]int{"token-1": 1, "token-1b": 1, "token-2": 2}

func testAuth(token string) (int, error) {
	if id, ok := testTokens[token]; ok {
		return id, nil
	}
	return 0, errors.New("bad token")
}

func newTestHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// newHubServer serves h.ServeWS with the test authenticator.
func newHubServer(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeWS(w, r, testAuth)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// connect dials and authenticates, returning once the server says it is ready.
func connect(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	conn := dial(t, srv)
	if err := conn.WriteJSON(map[string]string{"type": "auth", "token": token}); err != nil {
		t.Fatal(err)
	}
	if got := readType(t, conn); got != "ready" {
		t.Fatalf("expected a ready frame after auth, got %q", got)
	}
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn) ([]byte, error) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	return data, err
}

func readType(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	data, err := readFrame(t, conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("frame is not JSON: %q", data)
	}
	return m.Type
}

// expectSilence asserts nothing arrives within a short window.
func expectSilence(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, data, err := conn.ReadMessage(); err == nil {
		t.Fatalf("expected no message, got %q", data)
	}
}

func expectClose(t *testing.T, conn *websocket.Conn, code int) {
	t.Helper()
	_, _, err := conn.ReadMessage()
	if !websocket.IsCloseError(err, code) {
		t.Fatalf("expected close code %d, got %v", code, err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (h *Hub) connCount(userID int) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID])
}

func TestHandshakeThenDelivery(t *testing.T) {
	h := newTestHub()
	conn := connect(t, newHubServer(t, h), "token-1")

	h.Notify(1, "new_comment", map[string]int{"post_id": 7})

	data, err := readFrame(t, conn)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Type    string         `json:"type"`
		UserID  int            `json:"user_id"`
		Payload map[string]int `json:"payload"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "new_comment" || got.UserID != 1 || got.Payload["post_id"] != 7 {
		t.Fatalf("unexpected event: %s", data)
	}
}

func TestBadTokenIsRejected(t *testing.T) {
	h := newTestHub()
	conn := dial(t, newHubServer(t, h))

	if err := conn.WriteJSON(map[string]string{"type": "auth", "token": "nope"}); err != nil {
		t.Fatal(err)
	}
	expectClose(t, conn, websocket.ClosePolicyViolation)
	if n := h.connCount(1) + h.connCount(2); n != 0 {
		t.Fatalf("a rejected client must not be registered, hub has %d", n)
	}
}

func TestFirstFrameMustBeAuth(t *testing.T) {
	for name, frame := range map[string]any{
		"wrong type":    map[string]string{"type": "subscribe", "token": "token-1"},
		"missing token": map[string]string{"type": "auth"},
		"not an object": []int{1, 2, 3},
	} {
		t.Run(name, func(t *testing.T) {
			conn := dial(t, newHubServer(t, newTestHub()))
			if err := conn.WriteJSON(frame); err != nil {
				t.Fatal(err)
			}
			expectClose(t, conn, websocket.ClosePolicyViolation)
		})
	}
}

func TestSilentClientTimesOut(t *testing.T) {
	h := newTestHub()
	h.authTimeout = 100 * time.Millisecond
	conn := dial(t, newHubServer(t, h))

	// Send nothing: the server has to give up on its own.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("a client that never authenticates should be closed, got %v", err)
	}
}

func TestNotifyOnlyReachesTargetUser(t *testing.T) {
	h := newTestHub()
	srv := newHubServer(t, h)
	one := connect(t, srv, "token-1")
	two := connect(t, srv, "token-2")

	h.Notify(1, "ping", nil)

	if got := readType(t, one); got != "ping" {
		t.Fatalf("user 1 should receive the event, got %q", got)
	}
	expectSilence(t, two)
}

func TestEveryConnectionOfAUserReceives(t *testing.T) {
	h := newTestHub()
	srv := newHubServer(t, h)
	a := connect(t, srv, "token-1")
	b := connect(t, srv, "token-1b") // same user, second device

	h.Notify(1, "ping", nil)

	for name, conn := range map[string]*websocket.Conn{"first": a, "second": b} {
		if got := readType(t, conn); got != "ping" {
			t.Errorf("%s connection: got %q, want ping", name, got)
		}
	}
}

func TestDisconnectOnlyAffectsThatUser(t *testing.T) {
	h := newTestHub()
	srv := newHubServer(t, h)
	banned := connect(t, srv, "token-1")
	other := connect(t, srv, "token-2")

	h.Disconnect(1)

	// The server closes with a normal closure; what matters is that it ends.
	if _, _, err := banned.ReadMessage(); err == nil {
		t.Fatal("the disconnected user's connection should be closed")
	}
	h.Notify(2, "still-here", nil)
	if got := readType(t, other); got != "still-here" {
		t.Fatalf("other users must be unaffected, got %q", got)
	}
}

func TestCloseDisconnectsEveryoneAndRefusesNewClients(t *testing.T) {
	h := newTestHub()
	srv := newHubServer(t, h)
	existing := connect(t, srv, "token-1")

	h.Close()

	if _, _, err := existing.ReadMessage(); err == nil {
		t.Fatal("existing connection should be closed by Close")
	}

	late := dial(t, srv)
	if err := late.WriteJSON(map[string]string{"type": "auth", "token": "token-2"}); err != nil {
		t.Fatal(err)
	}
	expectClose(t, late, websocket.CloseGoingAway)
}

func TestClientGoingAwayIsForgotten(t *testing.T) {
	h := newTestHub()
	conn := connect(t, newHubServer(t, h), "token-1")
	waitFor(t, "registration", func() bool { return h.connCount(1) == 1 })

	_ = conn.Close()

	waitFor(t, "cleanup after disconnect", func() bool { return h.connCount(1) == 0 })
}

// A consumer that stops reading must be dropped, and Notify must never block
// on it - previously a full broadcast channel stalled every handler that
// tried to notify anyone.
func TestSlowClientIsDroppedWithoutBlocking(t *testing.T) {
	h := newTestHub()
	slow := &Client{hub: h, send: make(chan []byte, 1), userID: 1}
	if !h.add(slow) {
		t.Fatal("add refused")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			h.Notify(1, "spam", nil)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked on a client that is not reading")
	}

	if n := h.connCount(1); n != 0 {
		t.Fatalf("the slow client should have been dropped, hub still has %d", n)
	}
	<-slow.send // the one event that fit in the buffer before it filled up
	if _, ok := <-slow.send; ok {
		t.Fatal("a dropped client's send channel must be closed so its write loop exits")
	}
}

// The gauge was decremented once per unregister call even when the client was
// already gone, drifting below zero after every slow-client drop.
func TestRemoveIsIdempotentAndKeepsGaugeBalanced(t *testing.T) {
	h := newTestHub()
	before := testutil.ToFloat64(metrics.ActiveWebSockets)

	c := &Client{hub: h, send: make(chan []byte, 1), userID: 9}
	h.add(c)
	if got := testutil.ToFloat64(metrics.ActiveWebSockets); got != before+1 {
		t.Fatalf("gauge after add: got %v, want %v", got, before+1)
	}

	h.remove(c)
	h.remove(c) // second removal must be a no-op, not a double close or a second Dec
	h.Disconnect(9)

	if got := testutil.ToFloat64(metrics.ActiveWebSockets); got != before {
		t.Fatalf("gauge after repeated removal: got %v, want %v", got, before)
	}
}

func TestConcurrentUseIsRaceFree(t *testing.T) {
	h := newTestHub()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(user int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c := &Client{hub: h, send: make(chan []byte, 4), userID: user % 3}
				h.add(c)
				h.Notify(user%3, "x", nil)
				if j%7 == 0 {
					h.Disconnect(user % 3)
				}
				h.remove(c)
			}
		}(i)
	}
	wg.Wait()
	h.Close()
}
