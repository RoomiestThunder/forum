// Package dbtest builds throwaway database.Store instances for tests, so that
// the same test can run against every store implementation.
//
// SQLite is always available (in memory). PostgreSQL needs a server reachable
// through TEST_DATABASE_URL; each store gets its own schema inside it, so tests
// are isolated from each other and nothing outside that schema is touched.
package dbtest

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"forum/internal/database"
)

// Kind names a store implementation.
type Kind string

const (
	SQLite   Kind = "sqlite"
	Postgres Kind = "postgres"
)

// Kinds lists every implementation, for tests that run against all of them.
func Kinds() []Kind { return []Kind{SQLite, Postgres} }

// StoreEnv selects the store Default returns; DatabaseURLEnv locates PostgreSQL.
const (
	StoreEnv       = "TEST_STORE"
	DatabaseURLEnv = "TEST_DATABASE_URL"
)

var schemaSeq atomic.Int64

// New returns a fresh, empty store of the given kind, closed when the test
// ends. A PostgreSQL store is skipped when no server is configured or
// reachable.
func New(t testing.TB, kind Kind) database.Store {
	t.Helper()
	switch kind {
	case SQLite:
		return newSQLite(t)
	case Postgres:
		store, err := newPostgres(t)
		if err != nil {
			t.Skip(err)
		}
		return store
	default:
		t.Fatalf("dbtest: unknown store kind %q", kind)
		return nil
	}
}

// Default returns the store named by TEST_STORE (SQLite when unset). Asking for
// PostgreSQL explicitly is a statement that it must be exercised, so an
// unreachable server fails the test instead of silently skipping it - a CI
// job meant to cover PostgreSQL must not go green without running anything.
func Default(t testing.TB) database.Store {
	t.Helper()
	switch Kind(os.Getenv(StoreEnv)) {
	case "", SQLite:
		return newSQLite(t)
	case Postgres:
		store, err := newPostgres(t)
		if err != nil {
			t.Fatalf("%s=postgres but PostgreSQL is not usable: %v", StoreEnv, err)
		}
		return store
	default:
		t.Fatalf("%s=%q: want %q or %q", StoreEnv, os.Getenv(StoreEnv), SQLite, Postgres)
		return nil
	}
}

func newSQLite(t testing.TB) database.Store {
	t.Helper()
	db, err := database.InitSQLite(":memory:")
	if err != nil {
		t.Fatalf("init sqlite: %v", err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newPostgres(t testing.TB) (database.Store, error) {
	t.Helper()
	db, err := rawPostgres(t)
	if err != nil {
		return nil, err
	}
	if err := database.RunPostgresSchema(db); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	store := database.NewPostgresStore(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = store.Close() })
	return store, nil
}

// RawPostgres returns a connection to a brand-new, empty schema - no tables at
// all - for tests that apply the schema themselves (migration tests). It is
// skipped when no PostgreSQL is configured or reachable.
func RawPostgres(t testing.TB) *sql.DB {
	t.Helper()
	db, err := rawPostgres(t)
	if err != nil {
		t.Skip(err)
	}
	return db
}

// rawPostgres creates an empty schema and a pool whose every connection has
// it as its search_path.
func rawPostgres(t testing.TB) (*sql.DB, error) {
	t.Helper()
	dsn := os.Getenv(DatabaseURLEnv)
	if dsn == "" {
		return nil, fmt.Errorf("%s not set", DatabaseURLEnv)
	}

	admin, err := database.InitPostgres(dsn)
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL not reachable: %w", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schema := fmt.Sprintf("t_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), schemaSeq.Add(1))
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE") })

	// Pin search_path in the DSN so that every pooled connection, not just the
	// first, resolves unqualified table names to this test's schema.
	db, err := database.InitPostgres(WithSearchPath(t, dsn, schema))
	if err != nil {
		return nil, fmt.Errorf("connect to schema: %w", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, nil
}

// WithSearchPath returns dsn with the connection's search_path set to schema.
func WithSearchPath(t testing.TB, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", DatabaseURLEnv, err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
