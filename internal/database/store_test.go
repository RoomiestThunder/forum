package database

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// The tests in this package are written once against the Store interface and
// run against every implementation. SQLite and PostgreSQL are two hand-written
// copies of the same ~90 methods with different SQL dialects, so a behavioural
// difference between them is a bug that only a shared test can catch.
//
// PostgreSQL is used when TEST_DATABASE_URL points at a reachable server (CI
// provides one; locally any throwaway database works) and skipped otherwise.
// Each test gets its own schema inside that database, so tests are isolated
// from each other and nothing outside the throwaway schema is ever touched.

var schemaSeq atomic.Int64

type storeFactory struct {
	name string
	new  func(t *testing.T) Store
}

func storeFactories() []storeFactory {
	return []storeFactory{
		{name: "sqlite", new: newSQLiteTestStore},
		{name: "postgres", new: newPostgresTestStore},
	}
}

// forEachStore runs fn as a subtest against a fresh, empty store of every kind.
func forEachStore(t *testing.T, fn func(t *testing.T, s Store)) {
	t.Helper()
	for _, f := range storeFactories() {
		t.Run(f.name, func(t *testing.T) {
			fn(t, f.new(t))
		})
	}
}

func newSQLiteTestStore(t *testing.T) Store {
	t.Helper()
	db, err := InitSQLite(":memory:")
	if err != nil {
		t.Fatalf("init sqlite: %v", err)
	}
	store := NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newPostgresTestStore(t *testing.T) Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL")
	}

	admin, err := InitPostgres(dsn)
	if err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schema := fmt.Sprintf("t_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), schemaSeq.Add(1))
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE") })

	// Pin search_path in the DSN so that every pooled connection, not just the
	// first, resolves unqualified table names to this test's schema.
	db, err := InitPostgres(withSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}
	if err := RunPostgresSchema(db); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	store := NewPostgresStore(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// mustUser creates a user and returns its id. The first user created in a
// store becomes the admin, which tests that care must account for.
func mustUser(t *testing.T, s Store, username string) int {
	t.Helper()
	id, err := s.CreateUser(username+"@example.com", username, "hash-"+username)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return id
}
