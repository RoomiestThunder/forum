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
	srv, _ := newTestServerWithStore(t)
	return srv
}

// newTestServerWithStore also hands back the raw store, for tests that need
// to simulate out-of-band state a real request can't produce directly (e.g.
// "the verification email arrived and contained this token").
func newTestServerWithStore(t *testing.T) (*httptest.Server, database.Store) {
	t.Helper()
	db, err := database.InitSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })

	jwtManager := auth.NewManager("test-secret", 15*time.Minute, 24*time.Hour)
	r := chi.NewRouter()
	NewServer(store, jwtManager, nil, nil, nil, nil, Options{BaseURL: "http://localhost"}).Mount(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, store
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

func TestTags(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")

	resp := postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "Go tips", "content": "body", "category_ids": []int{1},
		"tags": []string{"Go", "backend", "go"}, // dupes (case-insensitive) should collapse
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post with tags: want 201, got %d", resp.StatusCode)
	}
	postID := decode[map[string]int](t, resp)["id"]

	resp = doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), "", nil)
	page := decode[struct {
		Post struct {
			Tags []map[string]any `json:"tags"`
		} `json:"post"`
	}](t, resp)
	if len(page.Post.Tags) != 2 {
		t.Fatalf("expected 2 distinct tags, got %+v", page.Post.Tags)
	}

	// Tag autocomplete.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/tags?q=go", "", nil)
	tags := decode[[]map[string]any](t, resp)
	if len(tags) != 1 || tags[0]["name"] != "go" {
		t.Fatalf("tag search 'go': want [go], got %+v", tags)
	}

	// Filtering the feed by tag.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?tag=backend", "", nil)
	posts := decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Go tips" {
		t.Fatalf("posts by tag=backend: want [Go tips], got %+v", posts)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?tag=nonexistent", "", nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 0 {
		t.Fatalf("posts by unknown tag: want [], got %+v", posts)
	}

	// Too many tags is rejected.
	resp = postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "Too many tags", "content": "body", "category_ids": []int{1},
		"tags": []string{"a", "b", "c", "d", "e", "f"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("6 tags: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBookmarks(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	postID := createPost(t, srv.URL, alice, "Bookmark me")

	// Bookmarking requires auth.
	resp := postJSON(t, srv.URL+"/api/v1/bookmarks", "", map[string]any{"post_id": postID, "bookmarked": true})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous bookmark: want 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/api/v1/bookmarks", bob, map[string]any{"post_id": postID, "bookmarked": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bookmark: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// is_bookmarked reflects the caller, not the post author.
	resp = doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), bob, nil)
	page := decode[map[string]any](t, resp)
	if page["is_bookmarked"] != true {
		t.Fatalf("bob's view: want is_bookmarked=true, got %v", page["is_bookmarked"])
	}

	resp = doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), alice, nil)
	page = decode[map[string]any](t, resp)
	if page["is_bookmarked"] != false {
		t.Fatalf("alice's view: want is_bookmarked=false, got %v", page["is_bookmarked"])
	}

	// Saved filter.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=saved", bob, nil)
	posts := decode[[]map[string]any](t, resp)
	if len(posts) != 1 || posts[0]["title"] != "Bookmark me" {
		t.Fatalf("saved for bob: want [Bookmark me], got %+v", posts)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=saved", alice, nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 0 {
		t.Fatalf("saved for alice: want [], got %+v", posts)
	}

	// Toggling off removes it.
	resp = postJSON(t, srv.URL+"/api/v1/bookmarks", bob, map[string]any{"post_id": postID, "bookmarked": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unbookmark: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts?filter=saved", bob, nil)
	posts = decode[[]map[string]any](t, resp)
	if len(posts) != 0 {
		t.Fatalf("saved for bob after unbookmark: want [], got %+v", posts)
	}
}

func TestNotifications(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	postID := createPost(t, srv.URL, alice, "Notify me")

	// No notifications yet.
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications/unread-count", alice, nil)
	count := decode[map[string]int](t, resp)
	if count["count"] != 0 {
		t.Fatalf("initial unread count: want 0, got %d", count["count"])
	}

	// Bob comments on Alice's post -> Alice gets notified.
	resp = postJSON(t, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), bob, map[string]string{"content": "nice post"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create comment: want 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Bob likes Alice's post -> Alice gets notified again.
	resp = postJSON(t, srv.URL+"/api/v1/like/post", bob, map[string]any{"post_id": postID, "is_like": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("like post: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Alice liking her own post must not notify herself.
	resp = postJSON(t, srv.URL+"/api/v1/like/post", alice, map[string]any{"post_id": postID, "is_like": true})
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications/unread-count", alice, nil)
	count = decode[map[string]int](t, resp)
	if count["count"] != 2 {
		t.Fatalf("unread count after comment+like: want 2, got %d", count["count"])
	}

	// Bob has no notifications of his own.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications/unread-count", bob, nil)
	count = decode[map[string]int](t, resp)
	if count["count"] != 0 {
		t.Fatalf("bob's unread count: want 0, got %d", count["count"])
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications", alice, nil)
	notifs := decode[[]map[string]any](t, resp)
	if len(notifs) != 2 {
		t.Fatalf("notification list: want 2, got %+v", notifs)
	}
	types := map[string]bool{}
	for _, n := range notifs {
		types[n["type"].(string)] = true
		if n["read"] != false {
			t.Errorf("expected unread notification, got %+v", n)
		}
		payload, ok := n["payload"].(map[string]any)
		if !ok || payload["actor"] != "bob" {
			t.Errorf("expected actor=bob in payload, got %+v", n["payload"])
		}
	}
	if !types["new_comment"] || !types["new_like"] {
		t.Fatalf("expected both new_comment and new_like, got %+v", types)
	}

	// Mark one read.
	firstID := int(notifs[0]["id"].(float64))
	resp = postJSON(t, fmt.Sprintf("%s/api/v1/notifications/%d/read", srv.URL, firstID), alice, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mark read: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications/unread-count", alice, nil)
	count = decode[map[string]int](t, resp)
	if count["count"] != 1 {
		t.Fatalf("unread count after marking one read: want 1, got %d", count["count"])
	}

	// Mark all read.
	resp = postJSON(t, srv.URL+"/api/v1/notifications/read-all", alice, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mark all read: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications/unread-count", alice, nil)
	count = decode[map[string]int](t, resp)
	if count["count"] != 0 {
		t.Fatalf("unread count after marking all read: want 0, got %d", count["count"])
	}

	// Notifications endpoints require auth.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/notifications", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous notifications: want 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSearch(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "Understanding Go channels", "content": "Channels are a core concurrency primitive.",
		"category_ids": []int{1},
	}).Body.Close()
	postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "React hooks", "content": "useEffect and useState.", "category_ids": []int{1},
	}).Body.Close()

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/search?q=channels", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: want 200, got %d", resp.StatusCode)
	}
	result := decode[struct {
		Posts []map[string]any `json:"posts"`
		Total int              `json:"total"`
	}](t, resp)
	if result.Total != 1 || len(result.Posts) != 1 || result.Posts[0]["title"] != "Understanding Go channels" {
		t.Fatalf("search 'channels': want 1 match, got %+v", result)
	}
	// Regression: queryPosts scans likes/dislikes/comment_count columns that
	// a naive SELECT (without the aggregate JOINs) won't have, which used to
	// blow up as a 500 instead of returning results.
	if result.Posts[0]["likes"] != float64(0) || result.Posts[0]["comment_count"] != float64(0) {
		t.Errorf("search result missing aggregate columns: %+v", result.Posts[0])
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/search?q=nonexistentterm", "", nil)
	result = decode[struct {
		Posts []map[string]any `json:"posts"`
		Total int              `json:"total"`
	}](t, resp)
	if result.Total != 0 || len(result.Posts) != 0 {
		t.Fatalf("search with no matches: want empty, got %+v", result)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/search", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("search without q: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFirstUserIsAdmin(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice") // first user ever on this store
	bob := registerUser(t, srv.URL, "bob")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", alice, nil)
	me := decode[map[string]any](t, resp)
	if me["is_admin"] != true {
		t.Fatalf("first registered user should be admin, got %+v", me)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", bob, nil)
	me = decode[map[string]any](t, resp)
	if me["is_admin"] != false {
		t.Fatalf("second registered user should not be admin, got %+v", me)
	}
}

func TestAdminAccessControl(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin") // first user -> admin
	mod := registerUser(t, srv.URL, "mod")
	plain := registerUser(t, srv.URL, "plain")

	// Plain user can't reach any /admin route.
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/users", plain, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("plain user /admin/users: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/reports", plain, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("plain user /admin/reports: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin promotes "mod" to moderator.
	var modUserID int
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/users", admin, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin list users: want 200, got %d", resp.StatusCode)
	}
	listing := decode[map[string]any](t, resp)
	for _, u := range listing["users"].([]any) {
		um := u.(map[string]any)
		if um["username"] == "mod" {
			modUserID = int(um["id"].(float64))
		}
	}
	if modUserID == 0 {
		t.Fatalf("could not find mod user in listing: %+v", listing)
	}

	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/role", srv.URL, modUserID), admin, map[string]any{
		"is_admin": false, "is_moderator": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promote to moderator: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Moderator can reach staff routes (reports) but not admin-only ones (users).
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/reports", mod, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("moderator /admin/reports: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/users", mod, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("moderator /admin/users: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin can't strip their own admin flag.
	var adminUserID int
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", admin, nil)
	me := decode[map[string]any](t, resp)
	adminUserID = int(me["id"].(float64))

	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/role", srv.URL, adminUserID), admin, map[string]any{
		"is_admin": false, "is_moderator": false,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self-demote: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin can't ban themselves.
	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, adminUserID), admin, map[string]any{"banned": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self-ban: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBanTakesEffectImmediately(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	targetToken := registerUser(t, srv.URL, "troll")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", targetToken, nil)
	me := decode[map[string]any](t, resp)
	targetID := int(me["id"].(float64))

	// The troll's existing access token still works right up until the ban.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", targetToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-ban /me: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, targetID), admin, map[string]any{"banned": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ban: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Same still-unexpired access token must now be rejected - requireAuth
	// re-checks the ban flag on every request rather than trusting the JWT.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", targetToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("post-ban /me with old token: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Login is also blocked from here on.
	resp = postJSON(t, srv.URL+"/api/v1/auth/login", "", map[string]string{"login": "troll", "password": "password123"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login while banned: want 403, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestReportsAndModeration(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	author := registerUser(t, srv.URL, "author")
	reporter := registerUser(t, srv.URL, "reporter")
	postID := createPost(t, srv.URL, author, "Spam post")

	resp := postJSON(t, srv.URL+"/api/v1/reports", reporter, map[string]any{
		"target_type": "post", "target_id": postID, "reason": "This is spam.",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create report: want 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/reports", admin, nil)
	reports := decode[[]map[string]any](t, resp)
	if len(reports) != 1 || reports[0]["target_id"] != float64(postID) || reports[0]["status"] != "open" {
		t.Fatalf("open reports: want 1 open report for post, got %+v", reports)
	}
	reportID := int(reports[0]["id"].(float64))

	// Admin pins the post, then deletes it as a moderation action.
	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/posts/%d/pin", srv.URL, postID), admin, map[string]any{"pinned": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin post: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, postID), "", nil)
	page := decode[map[string]any](t, resp)
	if page["post"].(map[string]any)["pinned"] != true {
		t.Fatalf("expected pinned post, got %+v", page)
	}

	resp = doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/posts/%d", srv.URL, postID), admin, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin delete post: want 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, fmt.Sprintf("%s/api/v1/admin/reports/%d/resolve", srv.URL, reportID), admin, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve report: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/reports", admin, nil)
	reports = decode[[]map[string]any](t, resp)
	if len(reports) != 0 {
		t.Fatalf("open reports after resolve: want 0, got %+v", reports)
	}
}

func TestPinnedPostsSortFirst(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	createPost(t, srv.URL, admin, "Older post")
	newestID := createPost(t, srv.URL, admin, "Newest post")

	// Pin the older post; it should still outrank the newer, unpinned one.
	olderID := newestID - 1
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/admin/posts/%d/pin", srv.URL, olderID), admin, map[string]any{"pinned": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/posts", "", nil)
	posts := decode[[]map[string]any](t, resp)
	if len(posts) < 2 || posts[0]["id"] != float64(olderID) {
		t.Fatalf("expected pinned post first, got %+v", posts)
	}
}

func TestAuthProviders(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/auth/providers", "", nil)
	providers := decode[map[string]bool](t, resp)
	if providers["github"] {
		t.Fatalf("github should be disabled without client credentials, got %+v", providers)
	}
}

func TestGithubLoginUnconfigured(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/auth/github", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("github login without config: want 503, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEmailVerification(t *testing.T) {
	srv, store := newTestServerWithStore(t)
	token := registerUser(t, srv.URL, "alice")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", token, nil)
	me := decode[map[string]any](t, resp)
	if me["email_verified"] != false {
		t.Fatalf("new user should start unverified, got %+v", me)
	}
	userID := int(me["id"].(float64))

	// Simulate the token that would have been emailed.
	verifyToken := "test-verify-token"
	if err := store.CreateEmailVerificationToken(userID, verifyToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	resp = postJSON(t, srv.URL+"/api/v1/auth/verify-email", "", map[string]string{"token": "wrong-token"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("verify with wrong token: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/api/v1/auth/verify-email", "", map[string]string{"token": verifyToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/v1/me", token, nil)
	me = decode[map[string]any](t, resp)
	if me["email_verified"] != true {
		t.Fatalf("expected verified after using token, got %+v", me)
	}

	// The token is single-use.
	resp = postJSON(t, srv.URL+"/api/v1/auth/verify-email", "", map[string]string{"token": verifyToken})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reuse of verify token: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPasswordReset(t *testing.T) {
	srv, store := newTestServerWithStore(t)
	registerUser(t, srv.URL, "alice")

	// forgot-password always returns 200, whether or not the email exists.
	resp := postJSON(t, srv.URL+"/api/v1/auth/forgot-password", "", map[string]string{"email": "alice@example.com"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password known email: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postJSON(t, srv.URL+"/api/v1/auth/forgot-password", "", map[string]string{"email": "nobody@example.com"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password unknown email: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	alice, err := store.GetUserByEmail("alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	resetToken := "test-reset-token"
	if err := store.CreatePasswordResetToken(alice.ID, resetToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Too short a password is rejected before the token is even consumed.
	resp = postJSON(t, srv.URL+"/api/v1/auth/reset-password", "", map[string]string{
		"token": resetToken, "new_password": "short",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset with short password: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/api/v1/auth/reset-password", "", map[string]string{
		"token": resetToken, "new_password": "newpassword456",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset password: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Old password no longer works, new one does.
	resp = postJSON(t, srv.URL+"/api/v1/auth/login", "", map[string]string{"login": "alice", "password": "password123"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login with old password: want 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/api/v1/auth/login", "", map[string]string{"login": "alice", "password": "newpassword456"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login with new password: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// The reset token is single-use.
	resp = postJSON(t, srv.URL+"/api/v1/auth/reset-password", "", map[string]string{
		"token": resetToken, "new_password": "anotherpassword",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reuse of reset token: want 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestOAuthUserCreationAndLinking(t *testing.T) {
	_, store := newTestServerWithStore(t)

	// First login for a given GitHub account creates a local user.
	u1, err := store.FindOrCreateOAuthUser("github", "12345", "octocat@example.com", "octocat", "https://example.com/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if u1.Username != "octocat" || !u1.EmailVerified {
		t.Fatalf("expected new oauth user 'octocat' pre-verified, got %+v", u1)
	}

	// Logging in again with the same provider id returns the same user, not a duplicate.
	u2, err := store.FindOrCreateOAuthUser("github", "12345", "octocat@example.com", "octocat", "https://example.com/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("expected same user on repeat login, got %d vs %d", u1.ID, u2.ID)
	}

	// A different GitHub account whose username collides gets de-duplicated.
	u3, err := store.FindOrCreateOAuthUser("github", "99999", "someoneelse@example.com", "octocat", "")
	if err != nil {
		t.Fatal(err)
	}
	if u3.Username == "octocat" {
		t.Fatalf("expected a de-duplicated username, got %q", u3.Username)
	}

	// A GitHub account whose email matches an existing local account links
	// to it instead of creating a duplicate.
	existingID, err := store.CreateUser("linked@example.com", "localuser", "irrelevant-hash")
	if err != nil {
		t.Fatal(err)
	}
	u4, err := store.FindOrCreateOAuthUser("github", "55555", "linked@example.com", "localuser-github", "")
	if err != nil {
		t.Fatal(err)
	}
	if u4.ID != existingID {
		t.Fatalf("expected oauth login to link to existing account %d, got %d", existingID, u4.ID)
	}
}
