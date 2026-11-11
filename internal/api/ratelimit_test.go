package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// freshConnClient returns a client that opens a new TCP connection for every
// request, so each request arrives from a different ephemeral source port -
// exactly what a client trying to dodge a per-connection limit would do.
func freshConnClient() *http.Client {
	return &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
}

func loginAttempt(t *testing.T, client *http.Client, base string, headers map[string]string) *http.Response {
	t.Helper()
	body := `{"login":"nobody@example.com","password":"wrong-password"}`
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/auth/login", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		trustProxy bool
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{name: "strips the port", remoteAddr: "203.0.113.7:51234", want: "203.0.113.7"},
		{name: "strips the port from IPv6", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "address without a port is used as is", remoteAddr: "203.0.113.7", want: "203.0.113.7"},
		{
			name: "forwarding headers are ignored unless trusted", remoteAddr: "10.0.0.1:80",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.9", "X-Real-IP": "198.51.100.9"},
			want:    "10.0.0.1",
		},
		{
			name: "trusted proxy: last forwarded entry wins", trustProxy: true, remoteAddr: "10.0.0.1:80",
			headers: map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.9"},
			want:    "198.51.100.9",
		},
		{
			name: "trusted proxy: X-Real-IP used when no X-Forwarded-For", trustProxy: true, remoteAddr: "10.0.0.1:80",
			headers: map[string]string{"X-Real-IP": "198.51.100.9"},
			want:    "198.51.100.9",
		},
		{name: "trusted proxy but no headers falls back to the peer", trustProxy: true, remoteAddr: "10.0.0.1:80", want: "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{trustProxy: tt.trustProxy}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := s.clientIP(r); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginIsRateLimitedAcrossConnections(t *testing.T) {
	srv, _ := newTestServerWithOptions(t, Options{BaseURL: "http://localhost"})
	client := freshConnClient()

	for i := 1; i <= limitAuth.limit; i++ {
		if resp := loginAttempt(t, client, srv.URL, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401 (bad credentials), got %d", i, resp.StatusCode)
		}
	}

	resp := loginAttempt(t, client, srv.URL, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("attempt past the limit: want 429, got %d - the limit must hold across new connections", resp.StatusCode)
	}
	retry, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || retry < 1 {
		t.Fatalf("429 must carry a positive Retry-After in seconds, got %q", resp.Header.Get("Retry-After"))
	}
}

func TestForwardedForCannotBypassLimitByDefault(t *testing.T) {
	srv, _ := newTestServerWithOptions(t, Options{BaseURL: "http://localhost"})
	client := freshConnClient()

	for i := 1; i <= limitAuth.limit; i++ {
		loginAttempt(t, client, srv.URL, map[string]string{"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i)})
	}
	resp := loginAttempt(t, client, srv.URL, map[string]string{"X-Forwarded-For": "198.51.100.200"})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("a spoofed X-Forwarded-For must not buy a fresh bucket: want 429, got %d", resp.StatusCode)
	}
}

func TestTrustedProxyLimitsEachForwardedClientSeparately(t *testing.T) {
	srv, _ := newTestServerWithOptions(t, Options{BaseURL: "http://localhost", TrustProxy: true})
	client := freshConnClient()
	noisy := map[string]string{"X-Forwarded-For": "198.51.100.1"}

	for i := 0; i < limitAuth.limit; i++ {
		loginAttempt(t, client, srv.URL, noisy)
	}
	if resp := loginAttempt(t, client, srv.URL, noisy); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("the noisy client should be limited: want 429, got %d", resp.StatusCode)
	}

	quiet := map[string]string{"X-Forwarded-For": "198.51.100.2"}
	if resp := loginAttempt(t, client, srv.URL, quiet); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("another client behind the same proxy must be unaffected: want 401, got %d", resp.StatusCode)
	}
}

func TestPasswordResetRequestsAreRateLimited(t *testing.T) {
	srv := newTestServer(t)

	send := func() int {
		resp := postJSON(t, srv.URL+"/api/v1/auth/forgot-password", "", map[string]string{"email": "someone@example.com"})
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for i := 1; i <= limitEmail.limit; i++ {
		if got := send(); got != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i, got)
		}
	}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("request past the limit: want 429 (mail-bombing guard), got %d", got)
	}
}

func TestContentRateLimitIsPerUser(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")

	for i := 0; i < limitContent.limit; i++ {
		createPost(t, srv.URL, alice, fmt.Sprintf("post %d", i))
	}
	resp := postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "one too many", "content": "x", "category_ids": []int{1},
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("alice past her limit: want 429, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Same IP, different user: bob has his own allowance.
	createPost(t, srv.URL, bob, "bob's post")
}

func TestMessageRateLimit(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	registerUser(t, srv.URL, "bob")

	send := func() int {
		resp := postJSON(t, srv.URL+"/api/v1/messages/bob", alice, map[string]any{"content": "hello"})
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for i := 1; i <= limitMessage.limit; i++ {
		if got := send(); got != http.StatusCreated {
			t.Fatalf("message %d: want 201, got %d", i, got)
		}
	}
	if got := send(); got != http.StatusTooManyRequests {
		t.Fatalf("message past the limit: want 429, got %d", got)
	}
}
