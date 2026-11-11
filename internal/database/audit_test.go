package database

import (
	"testing"
)

func TestAuditEntryRoundTrip(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		admin := mustUser(t, s, "admin")

		details := `{"title":"Spam post","author":"mallory"}`
		if err := s.CreateAuditEntry(admin, "post.delete", "post", 42, details); err != nil {
			t.Fatal(err)
		}

		entries, err := s.ListAuditEntries(10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("want 1 entry, got %d", len(entries))
		}
		e := entries[0]
		if e.ActorID != admin || e.Actor != "admin" {
			t.Errorf("actor = %d/%q, want %d/admin", e.ActorID, e.Actor, admin)
		}
		if e.Action != "post.delete" || e.TargetType != "post" || e.TargetID != 42 {
			t.Errorf("unexpected entry: %+v", e)
		}
		if string(e.Details) != details {
			t.Errorf("details = %s, want %s", e.Details, details)
		}
		if e.CreatedAt.IsZero() {
			t.Error("created_at must be set")
		}
	})
}

func TestAuditEntriesAreNewestFirstAndPaginated(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		admin := mustUser(t, s, "admin")
		for i := 1; i <= 5; i++ {
			if err := s.CreateAuditEntry(admin, "post.pin", "post", i, "{}"); err != nil {
				t.Fatal(err)
			}
		}

		if n, err := s.CountAuditEntries(); err != nil || n != 5 {
			t.Fatalf("count = %d, %v; want 5", n, err)
		}

		targets := func(limit, offset int) []int {
			t.Helper()
			entries, err := s.ListAuditEntries(limit, offset)
			if err != nil {
				t.Fatal(err)
			}
			var ids []int
			for _, e := range entries {
				ids = append(ids, e.TargetID)
			}
			return ids
		}
		if got := targets(2, 0); len(got) != 2 || got[0] != 5 || got[1] != 4 {
			t.Errorf("first page = %v, want [5 4] (newest first)", got)
		}
		if got := targets(2, 2); len(got) != 2 || got[0] != 3 || got[1] != 2 {
			t.Errorf("second page = %v, want [3 2]", got)
		}
		if got := targets(2, 4); len(got) != 1 || got[0] != 1 {
			t.Errorf("last page = %v, want [1]", got)
		}
		if got := targets(2, 10); len(got) != 0 {
			t.Errorf("past the end = %v, want empty", got)
		}
	})
}

func TestEmptyAuditLog(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		entries, err := s.ListAuditEntries(10, 0)
		if err != nil || len(entries) != 0 {
			t.Fatalf("an empty log lists nothing: %v, %v", entries, err)
		}
		if n, err := s.CountAuditEntries(); err != nil || n != 0 {
			t.Fatalf("count = %d, %v", n, err)
		}
	})
}
