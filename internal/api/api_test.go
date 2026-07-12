package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forum/internal/auth"
	"forum/internal/database"

	"github.com/go-chi/chi/v5"
)

// newTestServer spins up the API against an in-memory SQLite store.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := database.InitSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })

	jwtManager := auth.NewManager("test-secret", 15*time.Minute, 24*time.Hour)
	r := chi.NewRouter()
	NewServer(store, jwtManager, nil, nil, nil).Mount(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, url, token string, payload any) *http.Response {
	t.Helper()
	return doJSON(t, http.MethodPost, url, token, payload)
}

func doJSON(t *testing.T, method, url, token string, payload any) *http.Response {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// registerUser registers a fresh user and returns its access token.
func registerUser(t *testing.T, base, username string) string {
	t.Helper()
	resp := postJSON(t, base+"/api/v1/auth/register", "", map[string]string{
		"email":    username + "@example.com",
		"username": username,
		"password": "password123",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: want 201, got %d", username, resp.StatusCode)
	}
	data := decode[map[string]any](t, resp)
	token, _ := data["access_token"].(string)
	if token == "" {
		t.Fatal("register: empty access token")
	}
	return token
}

func createPost(t *testing.T, base, token, title string) int {
	t.Helper()
	resp := postJSON(t, base+"/api/v1/posts", token, map[string]any{
		"title": title, "content": "some content", "category_ids": []int{1},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post: want 201, got %d", resp.StatusCode)
	}
	data := decode[map[string]int](t, resp)
	return data["id"]
}

func TestMe(t *testing.T) {
	srv := newTestServer(t)
	token := registerUser(t, srv.URL, "alice")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: want 200, got %d", resp.StatusCode)
	}
	me := decode[map[string]any](t, resp)
	if me["username"] != "alice" {
		t.Errorf("me: want username alice, got %v", me["username"])
	}
	if me["email"] != "alice@example.com" {
		t.Errorf("me: want email alice@example.com, got %v", me["email"])
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without token: want 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCommentLifecycle(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	postID := createPost(t, srv.URL, alice, "Discussion")

	// Bob comments on Alice's post.
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), bob, map[string]string{
		"content": "first!",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create comment: want 201, got %d", resp.StatusCode)
	}
	commentID := decode[map[string]int](t, resp)["id"]

	// Bob edits his comment.
	resp = doJSON(t, http.MethodPut, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), bob, map[string]string{
		"content": "edited",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update comment: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Alice must not be able to edit Bob's comment.
	resp = doJSON(t, http.MethodPut, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), alice, map[string]string{
		"content": "hijacked",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("update foreign comment: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// The edit is visible on the post.
	resp = doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), "", nil)
	page := decode[struct {
		Comments []struct {
			Content string `json:"content"`
		} `json:"comments"`
	}](t, resp)
	if len(page.Comments) != 1 || page.Comments[0].Content != "edited" {
		t.Fatalf("expected single edited comment, got %+v", page.Comments)
	}

	// Alice must not be able to delete it either.
	resp = doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), alice, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("delete foreign comment: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Bob deletes his own comment.
	resp = doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), bob, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete comment: want 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCommentValidation(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	postID := createPost(t, srv.URL, alice, "Post")

	resp := postJSON(t, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), alice, map[string]string{
		"content": "ok",
	})
	commentID := decode[map[string]int](t, resp)["id"]

	resp = doJSON(t, http.MethodPut, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), alice, map[string]string{
		"content": "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty comment: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, commentID), alice, map[string]string{
		"content": strings.Repeat("x", 501),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized comment: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPostTitleValidation(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")

	longTitle := strings.Repeat("a", 101)
	resp := postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": longTitle, "content": "body", "category_ids": []int{1},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("101-char title: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	okTitle := strings.Repeat("a", 100)
	resp = postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": okTitle, "content": "body", "category_ids": []int{1},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("100-char title: want 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update path enforces the same rule.
	postID := createPost(t, srv.URL, alice, "Original")
	resp = doJSON(t, http.MethodPut, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), alice, map[string]any{
		"title": longTitle, "content": "body",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("update with 101-char title: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProfile(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	createPost(t, srv.URL, alice, "Post one")
	createPost(t, srv.URL, alice, "Post two")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/users/alice", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("profile: want 200, got %d", resp.StatusCode)
	}
	profile := decode[map[string]any](t, resp)
	if profile["username"] != "alice" {
		t.Errorf("profile username: want alice, got %v", profile["username"])
	}
	if profile["post_count"] != float64(2) {
		t.Errorf("profile post_count: want 2, got %v", profile["post_count"])
	}
	if _, hasEmail := profile["email"]; hasEmail {
		t.Error("profile should not leak email")
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/users/nobody", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown profile: want 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update bio.
	resp = doJSON(t, http.MethodPut, srv.URL+"/api/v1/me/bio", alice, map[string]string{
		"bio": "Hello, I like Go.",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update bio: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/users/alice", "", nil)
	profile = decode[map[string]any](t, resp)
	if profile["bio"] != "Hello, I like Go." {
		t.Errorf("bio: want updated bio, got %v", profile["bio"])
	}

	resp = doJSON(t, http.MethodPut, srv.URL+"/api/v1/me/bio", alice, map[string]string{
		"bio": strings.Repeat("x", 281),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized bio: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestListPostsFilters(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	alicePostID := createPost(t, srv.URL, alice, "Alice's post")
	createPost(t, srv.URL, bob, "Bob's post")

	// myposts must reflect the authenticated caller, not everyone.
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=myposts", alice, nil)
	posts := decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Alice's post" {
		t.Fatalf("myposts for alice: want [Alice's post], got %+v", posts)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=myposts", bob, nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Bob's post" {
		t.Fatalf("myposts for bob: want [Bob's post], got %+v", posts)
	}

	// liked must reflect what the caller liked, not what's popular globally.
	resp = postJSON(t, srv.URL+"/api/v1/like/post", bob, map[string]any{
		"post_id": alicePostID, "is_like": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("like: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=liked", bob, nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Alice's post" {
		t.Fatalf("liked for bob: want [Alice's post], got %+v", posts)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=liked", alice, nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 0 {
		t.Fatalf("liked for alice: want [], got %+v", posts)
	}

	// author filter is public, no auth required.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?author=bob", "", nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Bob's post" {
		t.Fatalf("author=bob: want [Bob's post], got %+v", posts)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?author=nobody", "", nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 0 {
		t.Fatalf("author=nobody: want [], got %+v", posts)
	}
}
