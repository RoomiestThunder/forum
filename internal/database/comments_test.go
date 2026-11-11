package database

import (
	stderrors "errors"
	"net/http"
	"testing"

	apperrors "forum/internal/errors"
)

func mustPost(t *testing.T, s Store, userID int, title string) int {
	t.Helper()
	id, err := s.CreatePost(userID, title, "content", []int{1}, "", false)
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	return id
}

func mustComment(t *testing.T, s Store, postID, userID int, parentID *int, text string) int {
	t.Helper()
	id, err := s.CreateComment(postID, userID, parentID, text)
	if err != nil {
		t.Fatalf("create comment %q: %v", text, err)
	}
	return id
}

// statusOf extracts the HTTP status an error maps to, or 0 for a nil/foreign error.
func statusOf(err error) int {
	var appErr *apperrors.AppError
	if stderrors.As(err, &appErr) {
		return appErr.HTTPStatusCode()
	}
	return 0
}

func commentIDs(t *testing.T, s Store, postID int) []int {
	t.Helper()
	comments, err := s.GetCommentsByPostID(postID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestRepliesRecordParentAndDepth(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")

		root := mustComment(t, s, post, alice, nil, "root")
		reply := mustComment(t, s, post, alice, &root, "reply")
		nested := mustComment(t, s, post, alice, &reply, "nested")

		for _, tt := range []struct {
			id, depth int
			parent    *int
		}{{root, 0, nil}, {reply, 1, &root}, {nested, 2, &reply}} {
			c, err := s.GetComment(tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if c.Depth != tt.depth {
				t.Errorf("comment %d: depth = %d, want %d", tt.id, c.Depth, tt.depth)
			}
			switch {
			case tt.parent == nil && c.ParentID != nil:
				t.Errorf("comment %d: top-level comment has parent %d", tt.id, *c.ParentID)
			case tt.parent != nil && (c.ParentID == nil || *c.ParentID != *tt.parent):
				t.Errorf("comment %d: parent = %v, want %d", tt.id, c.ParentID, *tt.parent)
			}
		}
	})
}

func TestReplyParentMustExistAndShareThePost(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		postA, postB := mustPost(t, s, alice, "a"), mustPost(t, s, alice, "b")
		onA := mustComment(t, s, postA, alice, nil, "on a")

		if _, err := s.CreateComment(postB, alice, &onA, "cross-post reply"); statusOf(err) != http.StatusBadRequest {
			t.Errorf("a reply must stay on its parent's post: got status %d (%v)", statusOf(err), err)
		}
		missing := 99999
		if _, err := s.CreateComment(postA, alice, &missing, "orphan"); statusOf(err) != http.StatusNotFound {
			t.Errorf("replying to a missing comment: want 404, got %d (%v)", statusOf(err), err)
		}
	})
}

func TestCommentsListedFlatOldestFirst(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		a := mustComment(t, s, post, alice, nil, "a")
		b := mustComment(t, s, post, alice, &a, "b")
		c := mustComment(t, s, post, alice, nil, "c")

		got := commentIDs(t, s, post)
		want := []int{a, b, c}
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
}

func TestGetCommentNotFound(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		if _, err := s.GetComment(12345); statusOf(err) != http.StatusNotFound {
			t.Fatalf("want 404, got %d (%v)", statusOf(err), err)
		}
	})
}

func TestDeleteLeafRemovesIt(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		c := mustComment(t, s, post, alice, nil, "only")

		if err := s.DeleteComment(c, alice); err != nil {
			t.Fatal(err)
		}
		if ids := commentIDs(t, s, post); len(ids) != 0 {
			t.Fatalf("leaf should be gone entirely, got %v", ids)
		}
	})
}

func TestDeleteWithRepliesLeavesATombstone(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")
		post := mustPost(t, s, alice, "thread")
		parent := mustComment(t, s, post, alice, nil, "secret words")
		reply := mustComment(t, s, post, bob, &parent, "bob's reply")
		if err := s.ToggleCommentLike(parent, bob, true); err != nil {
			t.Fatal(err)
		}

		if err := s.DeleteComment(parent, alice); err != nil {
			t.Fatal(err)
		}

		tomb, err := s.GetComment(parent)
		if err != nil {
			t.Fatalf("the comment must remain as a tombstone: %v", err)
		}
		if !tomb.Deleted || tomb.Content != "" {
			t.Errorf("tombstone should be marked deleted with content cleared, got deleted=%v content=%q", tomb.Deleted, tomb.Content)
		}
		if tomb.Likes != 0 {
			t.Errorf("a deleted comment keeps no likes, got %d", tomb.Likes)
		}
		kept, err := s.GetComment(reply)
		if err != nil || kept.Content != "bob's reply" || kept.ParentID == nil || *kept.ParentID != parent {
			t.Fatalf("the reply must survive untouched under the tombstone: %+v, %v", kept, err)
		}
	})
}

func TestTombstoneIsRemovedWhenItsLastReplyGoes(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		a := mustComment(t, s, post, alice, nil, "a")
		b := mustComment(t, s, post, alice, &a, "b")
		c := mustComment(t, s, post, alice, &b, "c")

		// Delete top-down so that a and b become tombstones, c a live leaf.
		for _, id := range []int{a, b} {
			if err := s.DeleteComment(id, alice); err != nil {
				t.Fatal(err)
			}
		}
		if got := len(commentIDs(t, s, post)); got != 3 {
			t.Fatalf("a and b are tombstones, c is live: want 3 rows, got %d", got)
		}

		// Removing c leaves a chain of childless tombstones, which all go.
		if err := s.DeleteComment(c, alice); err != nil {
			t.Fatal(err)
		}
		if ids := commentIDs(t, s, post); len(ids) != 0 {
			t.Fatalf("orphaned tombstones should be cleaned up, got %v", ids)
		}
	})
}

func TestTombstoneStaysWhileASiblingReplyRemains(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		parent := mustComment(t, s, post, alice, nil, "parent")
		first := mustComment(t, s, post, alice, &parent, "first")
		second := mustComment(t, s, post, alice, &parent, "second")

		if err := s.DeleteComment(parent, alice); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteComment(first, alice); err != nil {
			t.Fatal(err)
		}
		if got := commentIDs(t, s, post); len(got) != 2 {
			t.Fatalf("tombstone must stay while 'second' is under it: %v", got)
		}
		if err := s.DeleteComment(second, alice); err != nil {
			t.Fatal(err)
		}
		if got := commentIDs(t, s, post); len(got) != 0 {
			t.Fatalf("nothing left once the last reply is gone: %v", got)
		}
	})
}

// A delete the caller isn't allowed to make used to wipe the comment's likes
// before the ownership check failed, so anyone could clear anyone's likes.
func TestRejectedDeleteHasNoSideEffects(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")
		post := mustPost(t, s, alice, "thread")
		c := mustComment(t, s, post, alice, nil, "alice's comment")
		if err := s.ToggleCommentLike(c, bob, true); err != nil {
			t.Fatal(err)
		}

		if err := s.DeleteComment(c, bob); statusOf(err) != http.StatusForbidden {
			t.Fatalf("a non-owner delete: want 403, got %d (%v)", statusOf(err), err)
		}

		got, err := s.GetComment(c)
		if err != nil {
			t.Fatalf("comment must still exist: %v", err)
		}
		if got.Likes != 1 {
			t.Fatalf("likes must survive a rejected delete: got %d, want 1", got.Likes)
		}
		if got.Deleted || got.Content != "alice's comment" {
			t.Fatalf("comment must be untouched: %+v", got)
		}
	})
}

func TestDeletingTwiceOrDeletingNothingIsNotFound(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		parent := mustComment(t, s, post, alice, nil, "parent")
		mustComment(t, s, post, alice, &parent, "reply")

		if err := s.DeleteComment(parent, alice); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteComment(parent, alice); statusOf(err) != http.StatusNotFound {
			t.Errorf("deleting a tombstone again: want 404, got %d (%v)", statusOf(err), err)
		}
		if err := s.DeleteComment(99999, alice); statusOf(err) != http.StatusNotFound {
			t.Errorf("deleting a missing comment: want 404, got %d (%v)", statusOf(err), err)
		}
	})
}

func TestAdminDeleteIgnoresOwnership(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		c := mustComment(t, s, post, alice, nil, "spam")

		if err := s.AdminDeleteComment(c); err != nil {
			t.Fatal(err)
		}
		if ids := commentIDs(t, s, post); len(ids) != 0 {
			t.Fatalf("staff delete should remove the comment, got %v", ids)
		}
	})
}

func TestUpdateCommentOwnerOnlyAndNotOnTombstones(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice, bob := mustUser(t, s, "alice"), mustUser(t, s, "bob")
		post := mustPost(t, s, alice, "thread")
		c := mustComment(t, s, post, alice, nil, "before")
		mustComment(t, s, post, bob, &c, "keeps the parent alive")

		if err := s.UpdateComment(c, bob, "hijacked"); statusOf(err) != http.StatusForbidden {
			t.Errorf("editing someone else's comment: want 403, got %d", statusOf(err))
		}
		if err := s.UpdateComment(c, alice, "after"); err != nil {
			t.Fatalf("owner edit: %v", err)
		}
		if got, _ := s.GetComment(c); got.Content != "after" {
			t.Errorf("content = %q, want after", got.Content)
		}

		if err := s.DeleteComment(c, alice); err != nil {
			t.Fatal(err)
		}
		if err := s.UpdateComment(c, alice, "resurrected"); err == nil {
			t.Error("a tombstone must not be editable")
		}
		if got, _ := s.GetComment(c); got.Content != "" {
			t.Errorf("tombstone content must stay empty, got %q", got.Content)
		}
	})
}

// Deleting a post removes a whole thread of self-referencing comments in one
// statement; PostgreSQL has to accept that despite the parent_id foreign key.
func TestDeletingAPostRemovesItsWholeThread(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		alice := mustUser(t, s, "alice")
		post := mustPost(t, s, alice, "thread")
		a := mustComment(t, s, post, alice, nil, "a")
		b := mustComment(t, s, post, alice, &a, "b")
		mustComment(t, s, post, alice, &b, "c")

		if err := s.DeletePost(post, alice); err != nil {
			t.Fatalf("delete post with nested comments: %v", err)
		}
		if _, err := s.GetComment(a); statusOf(err) != http.StatusNotFound {
			t.Fatalf("the thread should be gone with its post, got %v", err)
		}
	})
}
