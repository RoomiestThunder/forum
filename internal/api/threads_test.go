package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "forum/internal/errors"
)

type commentView struct {
	ID       int    `json:"id"`
	UserID   int    `json:"user_id"`
	ParentID *int   `json:"parent_id"`
	Depth    int    `json:"depth"`
	Content  string `json:"content"`
	Author   string `json:"author"`
	Deleted  bool   `json:"deleted"`
}

func postComments(t *testing.T, base string, postID int) []commentView {
	t.Helper()
	resp := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", base, postID), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get post: want 200, got %d", resp.StatusCode)
	}
	return decode[struct {
		Comments []commentView `json:"comments"`
	}](t, resp).Comments
}

// addComment posts a comment (parent 0 = top level) and returns its id.
func addComment(t *testing.T, base, token string, postID, parentID int, text string) int {
	t.Helper()
	payload := map[string]any{"content": text}
	if parentID != 0 {
		payload["parent_id"] = parentID
	}
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/posts/%d", base, postID), token, payload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("comment %q: want 201, got %d", text, resp.StatusCode)
	}
	return decode[map[string]int](t, resp)["id"]
}

func tryComment(t *testing.T, base, token string, postID, parentID int, text string) int {
	t.Helper()
	payload := map[string]any{"content": text}
	if parentID != 0 {
		payload["parent_id"] = parentID
	}
	return status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/posts/%d", base, postID), token, payload)
}

func notificationTypes(t *testing.T, base, token string) []string {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/v1/notifications", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("notifications: want 200, got %d", resp.StatusCode)
	}
	var types []string
	for _, n := range decode[[]map[string]any](t, resp) {
		types = append(types, n["type"].(string))
	}
	return types
}

func TestRepliesFormATree(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, alice, "Discussion")

	root := addComment(t, srv.URL, bob, post, 0, "top level")
	reply := addComment(t, srv.URL, alice, post, root, "a reply")
	addComment(t, srv.URL, bob, post, reply, "a reply to the reply")

	comments := postComments(t, srv.URL, post)
	if len(comments) != 3 {
		t.Fatalf("want 3 comments, got %d", len(comments))
	}
	wantDepth := []int{0, 1, 2}
	for i, c := range comments {
		if c.Depth != wantDepth[i] {
			t.Errorf("comment %d: depth = %d, want %d", i, c.Depth, wantDepth[i])
		}
	}
	if comments[0].ParentID != nil {
		t.Errorf("a top-level comment has no parent, got %d", *comments[0].ParentID)
	}
	if comments[1].ParentID == nil || *comments[1].ParentID != root {
		t.Errorf("reply's parent = %v, want %d", comments[1].ParentID, root)
	}
}

func TestReplyValidation(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	postA := createPost(t, srv.URL, alice, "A")
	postB := createPost(t, srv.URL, alice, "B")
	onA := addComment(t, srv.URL, alice, postA, 0, "on A")

	if got := tryComment(t, srv.URL, bob, postB, onA, "wrong post"); got != http.StatusNotFound {
		t.Errorf("reply whose parent is on another post: want 404, got %d", got)
	}
	if got := tryComment(t, srv.URL, bob, postA, 99999, "orphan"); got != http.StatusNotFound {
		t.Errorf("reply to a comment that doesn't exist: want 404, got %d", got)
	}
	if got := tryComment(t, srv.URL, bob, 99999, 0, "nowhere"); got != http.StatusNotFound {
		t.Errorf("comment on a post that doesn't exist: want 404, got %d", got)
	}
	if got := tryComment(t, srv.URL, bob, postA, 0, "   "); got != http.StatusBadRequest {
		t.Errorf("blank comment: want 400, got %d", got)
	}
}

func TestRepliesAreCappedAtMaxDepth(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	post := createPost(t, srv.URL, alice, "Deep")

	parent := 0
	for level := 0; level <= maxCommentDepth; level++ {
		parent = addComment(t, srv.URL, alice, post, parent, fmt.Sprintf("level %d", level))
	}
	if got := tryComment(t, srv.URL, alice, post, parent, "one level too deep"); got != http.StatusBadRequest {
		t.Fatalf("a reply past the maximum depth: want 400, got %d", got)
	}
}

func TestCannotReplyToADeletedComment(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, alice, "Thread")
	parent := addComment(t, srv.URL, bob, post, 0, "parent")
	addComment(t, srv.URL, alice, post, parent, "keeps the parent as a tombstone")
	if got := status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, parent), bob, nil); got != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", got)
	}

	if got := tryComment(t, srv.URL, alice, post, parent, "replying to nothing"); got != http.StatusBadRequest {
		t.Fatalf("reply to a tombstone: want 400, got %d", got)
	}
}

func TestDeletedCommentWithRepliesBecomesAnAnonymousTombstone(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	carol := registerUser(t, srv.URL, "carol")
	post := createPost(t, srv.URL, alice, "Thread")
	parent := addComment(t, srv.URL, bob, post, 0, "bob says something regrettable")
	reply := addComment(t, srv.URL, carol, post, parent, "carol answers")

	status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, parent), bob, nil)

	comments := postComments(t, srv.URL, post)
	if len(comments) != 2 {
		t.Fatalf("the tombstone and the reply should both remain, got %d comments", len(comments))
	}
	tomb := comments[0]
	if !tomb.Deleted || tomb.Content != "" || tomb.Author != "" || tomb.UserID != 0 {
		t.Errorf("a tombstone must not reveal content or author: %+v", tomb)
	}
	if comments[1].ID != reply || comments[1].Content != "carol answers" || comments[1].Author != "carol" {
		t.Errorf("the reply must be untouched: %+v", comments[1])
	}

	// Once the last reply is deleted the empty tombstone goes too.
	status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, reply), carol, nil)
	if left := postComments(t, srv.URL, post); len(left) != 0 {
		t.Fatalf("nothing should be left, got %+v", left)
	}
}

func TestDeleteCommentErrors(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, alice, "Thread")
	comment := addComment(t, srv.URL, alice, post, 0, "alice's")

	resp := doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, comment), bob, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("deleting someone else's comment: want 403, got %d", resp.StatusCode)
	}
	if msg := decode[map[string]string](t, resp)["error"]; strings.Contains(msg, "[403]") {
		t.Errorf("the error shown to the client must not be the internal AppError string: %q", msg)
	}

	if got := status(t, http.MethodDelete, srv.URL+"/api/v1/comments/99999", alice, nil); got != http.StatusNotFound {
		t.Errorf("deleting a missing comment: want 404, got %d", got)
	}
	if got := status(t, http.MethodDelete, srv.URL+"/api/v1/comments/abc", alice, nil); got != http.StatusBadRequest {
		t.Errorf("a malformed id: want 400, got %d", got)
	}
}

func TestNotificationsForReplies(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice") // owns the post
	bob := registerUser(t, srv.URL, "bob")
	carol := registerUser(t, srv.URL, "carol")
	post := createPost(t, srv.URL, alice, "Thread")

	bobsComment := addComment(t, srv.URL, bob, post, 0, "bob's comment")
	if got := notificationTypes(t, srv.URL, alice); len(got) != 1 || got[0] != "new_comment" {
		t.Fatalf("the post author hears about a top-level comment: %v", got)
	}

	// carol replies to bob: bob is told it is a reply, and alice (the post
	// author, not the parent's author) still hears about activity on her post.
	addComment(t, srv.URL, carol, post, bobsComment, "carol replies")
	if got := notificationTypes(t, srv.URL, bob); len(got) != 1 || got[0] != "new_reply" {
		t.Errorf("the parent's author should get new_reply: %v", got)
	}
	if got := notificationTypes(t, srv.URL, alice); len(got) != 2 {
		t.Errorf("the post author should get a second new_comment: %v", got)
	}
}

// When the parent comment's author is also the post's author they are one
// person, and must get one notification for the reply, not two.
func TestPostAuthorGetsOneNotificationWhenRepliedTo(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, alice, "Thread")
	alicesComment := addComment(t, srv.URL, alice, post, 0, "alice comments on her own post")

	addComment(t, srv.URL, bob, post, alicesComment, "bob replies")

	got := notificationTypes(t, srv.URL, alice)
	if len(got) != 1 || got[0] != "new_reply" {
		t.Fatalf("want exactly one new_reply, got %v", got)
	}
}

func TestNoNotificationForReplyingToYourself(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	post := createPost(t, srv.URL, alice, "Thread")
	root := addComment(t, srv.URL, alice, post, 0, "mine")

	addComment(t, srv.URL, alice, post, root, "also mine")

	if got := notificationTypes(t, srv.URL, alice); len(got) != 0 {
		t.Fatalf("talking to yourself notifies no one: %v", got)
	}
}

// The limit is on characters. Counting bytes would cut Cyrillic text off at
// 250 characters, since each letter takes two bytes.
func TestCommentLengthCountsCharactersNotBytes(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	post := createPost(t, srv.URL, alice, "Thread")

	if got := tryComment(t, srv.URL, alice, post, 0, strings.Repeat("я", maxCommentLength)); got != http.StatusCreated {
		t.Errorf("exactly %d Cyrillic characters should be accepted, got %d", maxCommentLength, got)
	}
	if got := tryComment(t, srv.URL, alice, post, 0, strings.Repeat("я", maxCommentLength+1)); got != http.StatusBadRequest {
		t.Errorf("%d characters should be rejected, got %d", maxCommentLength+1, got)
	}
}

func TestDraftsCannotBeCommentedOnByOthers(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	bob := registerUser(t, srv.URL, "bob")
	resp := postJSON(t, srv.URL+"/api/v1/posts", alice, map[string]any{
		"title": "Unfinished", "content": "wip", "category_ids": []int{1}, "draft": true,
	})
	draft := decode[map[string]int](t, resp)["id"]

	if got := tryComment(t, srv.URL, bob, draft, 0, "found you"); got != http.StatusNotFound {
		t.Errorf("a draft must answer like a missing post: want 404, got %d", got)
	}
	if got := tryComment(t, srv.URL, alice, draft, 0, "note to self"); got != http.StatusCreated {
		t.Errorf("the owner may comment on their own draft: got %d", got)
	}
}

func TestStoreErrorNeverLeaksInternals(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
		forbidden  string
	}{
		{
			name:       "database failure",
			err:        apperrors.DatabaseError(errors.New(`pq: relation "secret_table" does not exist`)),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to do the thing",
			forbidden:  "secret_table",
		},
		{
			name:       "unexpected error type",
			err:        errors.New("dial tcp 10.0.0.5:5432: connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to do the thing",
			forbidden:  "10.0.0.5",
		},
		{
			name:       "not found passes through",
			err:        apperrors.NotFoundError("Comment"),
			wantStatus: http.StatusNotFound,
			wantBody:   "Comment not found",
		},
		{
			name:       "validation details pass through",
			err:        apperrors.ValidationError("parent comment belongs to a different post"),
			wantStatus: http.StatusBadRequest,
			wantBody:   "parent comment belongs to a different post",
		},
		{
			name:       "permission denied",
			err:        apperrors.PermissionError("delete comment"),
			wantStatus: http.StatusForbidden,
			wantBody:   "Permission denied",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.storeError(rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err, "do the thing")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body %q should contain %q", body, tt.wantBody)
			}
			if tt.forbidden != "" && strings.Contains(body, tt.forbidden) {
				t.Errorf("body %q leaks %q", body, tt.forbidden)
			}
		})
	}
}
