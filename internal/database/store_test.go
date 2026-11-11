package database_test

import (
	"testing"

	"forum/internal/database"
	"forum/internal/database/dbtest"
)

// The tests in this package are written once against the Store interface and
// run against every implementation. SQLite and PostgreSQL are two hand-written
// copies of the same ~90 methods in different SQL dialects, so a behavioural
// difference between them is a bug that only a shared test can catch.
//
// PostgreSQL is used when TEST_DATABASE_URL points at a reachable server (CI
// provides one) and skipped otherwise.

// forEachStore runs fn as a subtest against a fresh, empty store of every kind.
func forEachStore(t *testing.T, fn func(t *testing.T, s database.Store)) {
	t.Helper()
	for _, kind := range dbtest.Kinds() {
		t.Run(string(kind), func(t *testing.T) {
			fn(t, dbtest.New(t, kind))
		})
	}
}

// mustUser creates a user and returns its id. The first user created in a
// store becomes the admin, which tests that care must account for.
func mustUser(t *testing.T, s database.Store, username string) int {
	t.Helper()
	id, err := s.CreateUser(username+"@example.com", username, "hash-"+username)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return id
}
