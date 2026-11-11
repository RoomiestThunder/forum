package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type auditEntryView struct {
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   int            `json:"target_id"`
	Details    map[string]any `json:"details"`
}

func fetchAudit(t *testing.T, base, token string) (entries []auditEntryView, total int) {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/v1/admin/audit", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit log: want 200, got %d", resp.StatusCode)
	}
	body := decode[struct {
		Entries []auditEntryView `json:"entries"`
		Total   int              `json:"total"`
	}](t, resp)
	return body.Entries, body.Total
}

// makeModerator promotes username to moderator using the admin's token.
func makeModerator(t *testing.T, base, adminToken, modToken string) {
	t.Helper()
	id := userIDOf(t, base, modToken)
	got := status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%d/role", base, id), adminToken,
		map[string]bool{"is_admin": false, "is_moderator": true})
	if got != http.StatusOK {
		t.Fatalf("make moderator: want 200, got %d", got)
	}
}

func TestStaffActionsAreAudited(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin") // the first user is the admin
	bob := registerUser(t, srv.URL, "bob")
	bobID := userIDOf(t, srv.URL, bob)

	post := createPost(t, srv.URL, bob, "Questionable post")
	comment := addComment(t, srv.URL, bob, post, 0, "questionable comment")

	postURL := fmt.Sprintf("%s/api/v1/admin/posts/%d", srv.URL, post)
	steps := []struct {
		name         string
		do           func() int
		wantAction   string
		wantTarget   string
		wantTargetID int
		wantDetail   map[string]any
	}{
		{"pin", func() int {
			return status(t, http.MethodPost, postURL+"/pin", admin, map[string]bool{"pinned": true})
		}, "post.pin", "post", post, map[string]any{"title": "Questionable post"}},
		{"unpin", func() int {
			return status(t, http.MethodPost, postURL+"/pin", admin, map[string]bool{"pinned": false})
		}, "post.unpin", "post", post, map[string]any{"title": "Questionable post"}},
		{"ban", func() int {
			return status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, bobID), admin, map[string]bool{"banned": true})
		}, "user.ban", "user", bobID, map[string]any{"username": "bob"}},
		{"unban", func() int {
			return status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%d/ban", srv.URL, bobID), admin, map[string]bool{"banned": false})
		}, "user.unban", "user", bobID, map[string]any{"username": "bob"}},
		{"role", func() int {
			return status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%d/role", srv.URL, bobID), admin,
				map[string]bool{"is_admin": false, "is_moderator": true})
		}, "user.role", "user", bobID, map[string]any{"username": "bob", "was_moderator": false, "is_moderator": true}},
		{"delete comment", func() int {
			return status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/comments/%d", srv.URL, comment), admin, nil)
		}, "comment.delete", "comment", comment, map[string]any{"author": "bob", "excerpt": "questionable comment"}},
		{"delete post", func() int {
			return status(t, http.MethodDelete, postURL, admin, nil)
		}, "post.delete", "post", post, map[string]any{"title": "Questionable post", "author": "bob"}},
	}

	for i, step := range steps {
		if got := step.do(); got != http.StatusOK && got != http.StatusNoContent {
			t.Fatalf("%s: unexpected status %d", step.name, got)
		}
		entries, total := fetchAudit(t, srv.URL, admin)
		if total != i+1 || len(entries) != i+1 {
			t.Fatalf("%s: want %d entries, got total=%d len=%d", step.name, i+1, total, len(entries))
		}
		latest := entries[0] // newest first
		if latest.Action != step.wantAction || latest.TargetType != step.wantTarget || latest.TargetID != step.wantTargetID {
			t.Errorf("%s: entry = %s %s#%d, want %s %s#%d", step.name,
				latest.Action, latest.TargetType, latest.TargetID, step.wantAction, step.wantTarget, step.wantTargetID)
		}
		if latest.Actor != "admin" {
			t.Errorf("%s: actor = %q, want admin", step.name, latest.Actor)
		}
		for k, want := range step.wantDetail {
			if latest.Details[k] != want {
				t.Errorf("%s: details[%q] = %v, want %v (details: %v)", step.name, k, latest.Details[k], want, latest.Details)
			}
		}
	}
}

func TestResolvingAReportIsAudited(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, bob, "Reported")

	resp := postJSON(t, srv.URL+"/api/v1/reports", admin, map[string]any{"target_type": "post", "target_id": post, "reason": "spam"})
	reportID := decode[map[string]int](t, resp)["id"]

	if got := status(t, http.MethodPost, fmt.Sprintf("%s/api/v1/admin/reports/%d/resolve", srv.URL, reportID), admin, nil); got != http.StatusOK {
		t.Fatalf("resolve: want 200, got %d", got)
	}
	entries, _ := fetchAudit(t, srv.URL, admin)
	if len(entries) != 1 || entries[0].Action != "report.resolve" || entries[0].TargetID != reportID {
		t.Fatalf("want a report.resolve entry for report %d, got %+v", reportID, entries)
	}
}

func TestModeratorActionsAreLoggedUnderTheirName(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	mod := registerUser(t, srv.URL, "mod")
	bob := registerUser(t, srv.URL, "bob")
	makeModerator(t, srv.URL, admin, mod)
	post := createPost(t, srv.URL, bob, "Off topic")

	if got := status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/posts/%d", srv.URL, post), mod, nil); got != http.StatusNoContent {
		t.Fatalf("moderator delete: want 204, got %d", got)
	}

	entries, _ := fetchAudit(t, srv.URL, admin)
	if entries[0].Action != "post.delete" || entries[0].Actor != "mod" {
		t.Fatalf("the newest entry should be the moderator's delete, got %+v", entries[0])
	}
}

func TestAuditLogIsAdminOnly(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	mod := registerUser(t, srv.URL, "mod")
	member := registerUser(t, srv.URL, "member")
	makeModerator(t, srv.URL, admin, mod)
	url := srv.URL + "/api/v1/admin/audit"

	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"anonymous": {"", http.StatusUnauthorized},
		"member":    {member, http.StatusForbidden},
		"moderator": {mod, http.StatusForbidden},
		"admin":     {admin, http.StatusOK},
	} {
		if got := status(t, http.MethodGet, url, tc.token, nil); got != tc.want {
			t.Errorf("%s: want %d, got %d", name, tc.want, got)
		}
	}
}

func TestOrdinaryUserActionsAreNotAudited(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, bob, "Mine")
	comment := addComment(t, srv.URL, bob, post, 0, "mine too")

	status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", srv.URL, comment), bob, nil)
	status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/posts/%d", srv.URL, post), bob, nil)

	if _, total := fetchAudit(t, srv.URL, admin); total != 0 {
		t.Fatalf("authors deleting their own content isn't a staff action: %d entries", total)
	}
}

func TestAuditExcerptIsTruncated(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")
	bob := registerUser(t, srv.URL, "bob")
	post := createPost(t, srv.URL, bob, "Long")
	long := strings.Repeat("я", 400)
	comment := addComment(t, srv.URL, bob, post, 0, long)

	status(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/comments/%d", srv.URL, comment), admin, nil)

	entries, _ := fetchAudit(t, srv.URL, admin)
	got, _ := entries[0].Details["excerpt"].(string)
	if want := strings.Repeat("я", excerptLength) + "…"; got != want {
		t.Fatalf("excerpt should be cut to %d characters plus an ellipsis, got %d runes", excerptLength, len([]rune(got)))
	}
}

func TestAdminActionsOnMissingTargetsAre404AndUnlogged(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")

	for name, req := range map[string]struct {
		method, path string
		body         any
	}{
		"delete post":    {http.MethodDelete, "/api/v1/admin/posts/9999", nil},
		"delete comment": {http.MethodDelete, "/api/v1/admin/comments/9999", nil},
		"pin post":       {http.MethodPost, "/api/v1/admin/posts/9999/pin", map[string]bool{"pinned": true}},
		"ban user":       {http.MethodPost, "/api/v1/admin/users/9999/ban", map[string]bool{"banned": true}},
		"set role":       {http.MethodPost, "/api/v1/admin/users/9999/role", map[string]bool{"is_moderator": true}},
	} {
		if got := status(t, req.method, srv.URL+req.path, admin, req.body); got != http.StatusNotFound {
			t.Errorf("%s on a missing target: want 404, got %d", name, got)
		}
	}
	if _, total := fetchAudit(t, srv.URL, admin); total != 0 {
		t.Fatalf("failed actions must not be logged, got %d entries", total)
	}
}

func TestEmptyAuditLogIsAnEmptyList(t *testing.T) {
	srv := newTestServer(t)
	admin := registerUser(t, srv.URL, "admin")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/v1/admin/audit", admin, nil)
	raw := decode[map[string]json.RawMessage](t, resp)
	if string(raw["entries"]) != "[]" {
		t.Fatalf(`entries should serialise as [], got %s`, raw["entries"])
	}
}

func TestReportValidation(t *testing.T) {
	srv := newTestServer(t)
	alice := registerUser(t, srv.URL, "alice")
	post := createPost(t, srv.URL, alice, "Reportable")
	report := func(targetType string, id int, reason string) int {
		return status(t, http.MethodPost, srv.URL+"/api/v1/reports", alice,
			map[string]any{"target_type": targetType, "target_id": id, "reason": reason})
	}

	if got := report("post", post, "spam"); got != http.StatusCreated {
		t.Errorf("a valid report: want 201, got %d", got)
	}
	if got := report("post", 9999, "spam"); got != http.StatusNotFound {
		t.Errorf("reporting a post that doesn't exist: want 404, got %d", got)
	}
	if got := report("comment", 9999, "spam"); got != http.StatusNotFound {
		t.Errorf("reporting a comment that doesn't exist: want 404, got %d", got)
	}
	if got := report("post", post, strings.Repeat("я", maxReportReasonLength)); got != http.StatusCreated {
		t.Errorf("%d Cyrillic characters fit the limit: got %d", maxReportReasonLength, got)
	}
	if got := report("post", post, strings.Repeat("я", maxReportReasonLength+1)); got != http.StatusBadRequest {
		t.Errorf("one character over the limit: want 400, got %d", got)
	}
	if got := report("post", post, ""); got != http.StatusBadRequest {
		t.Errorf("an empty reason: want 400, got %d", got)
	}
}
