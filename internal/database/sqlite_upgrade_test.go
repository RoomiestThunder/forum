package database_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"forum/internal/database"
)

// legacySQLiteSchema is the database a first-generation build created: the
// tables of migration 001 and none of the columns added since. A developer who
// kept their forum.db across upgrades has exactly this on disk.
const legacySQLiteSchema = `
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT UNIQUE NOT NULL,
	username TEXT NOT NULL,
	password TEXT NOT NULL,
	avatar_url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE categories (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL);
INSERT INTO categories (name) VALUES ('General');
CREATE TABLE posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
	title TEXT,
	content TEXT,
	image_url TEXT NOT NULL DEFAULT '',
	created_at DATETIME
);
CREATE TABLE comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	post_id INTEGER,
	user_id INTEGER,
	content TEXT,
	created_at DATETIME
);
INSERT INTO users (email, username, password) VALUES ('old@example.com', 'olduser', 'hash');
INSERT INTO posts (user_id, title, content, created_at) VALUES (1, 'old post', 'old body', '2020-01-01 00:00:00');
INSERT INTO comments (post_id, user_id, content, created_at) VALUES (1, 1, 'old comment', '2020-01-01 00:00:00');
`

func TestSQLiteUpgradesAnOlderDatabaseInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(legacySQLiteSchema); err != nil {
		t.Fatalf("build legacy database: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.InitSQLite(path)
	if err != nil {
		t.Fatalf("opening an older database must upgrade it, not fail: %v", err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })

	// Existing data is intact and picks up the defaults of the new columns.
	user, err := store.GetUserByUsername("olduser")
	if err != nil {
		t.Fatalf("existing user lost: %v", err)
	}
	if user.IsAdmin || user.IsModerator || user.Banned || user.EmailVerified {
		t.Errorf("new flags must default to false for existing users: %+v", user)
	}
	post, err := store.GetPost(1)
	if err != nil {
		t.Fatalf("existing post lost: %v", err)
	}
	if post.Draft || post.Pinned {
		t.Errorf("existing posts are published and unpinned: %+v", post)
	}
	comments, err := store.GetCommentsByPostID(1)
	if err != nil || len(comments) != 1 {
		t.Fatalf("existing comment lost: %v %v", comments, err)
	}
	if c := comments[0]; c.Content != "old comment" || c.ParentID != nil || c.Depth != 0 || c.Deleted {
		t.Errorf("existing comment should be a live top-level comment: %+v", c)
	}

	// And the upgraded database supports the new features.
	reply, err := store.CreateComment(1, user.ID, &comments[0].ID, "a reply to an old comment")
	if err != nil {
		t.Fatalf("threaded comments on an upgraded database: %v", err)
	}
	if got, _ := store.GetComment(reply); got == nil || got.Depth != 1 {
		t.Errorf("reply on an upgraded database: %+v", got)
	}
}

// Opening the same file again must not try to re-add columns (which would fail
// with "duplicate column name").
func TestSQLiteUpgradeIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum.db")
	for i := 1; i <= 3; i++ {
		db, err := database.InitSQLite(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// usernames was never unique, so an existing database can hold duplicates. The
// unique index can't be built over them, so opening such a database has to
// resolve them first: the oldest account keeps the name.
func TestSQLiteUpgradeRenamesDuplicateUsernames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dupes.db")

	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(legacySQLiteSchema + `
		INSERT INTO users (email, username, password) VALUES ('second@example.com', 'OLDUSER', 'hash');
		INSERT INTO users (email, username, password) VALUES ('third@example.com', 'olduser', 'hash');
		INSERT INTO users (email, username, password) VALUES ('other@example.com', 'someoneelse', 'hash');`)
	if err != nil {
		t.Fatalf("build legacy database: %v", err)
	}
	_ = legacy.Close()

	db, err := database.InitSQLite(path)
	if err != nil {
		t.Fatalf("a database with duplicate usernames must still open: %v", err)
	}
	store := database.NewSQLiteStoreWithoutLogger(db)
	t.Cleanup(func() { _ = store.Close() })

	first, err := store.GetUserByUsername("olduser")
	if err != nil || first.Email != "old@example.com" {
		t.Fatalf("the oldest account must keep the name, got %+v (%v)", first, err)
	}
	for id, want := range map[int]string{2: "OLDUSER_2", 3: "olduser_3", 4: "someoneelse"} {
		u, err := store.GetUserByID(id)
		if err != nil || u.Username != want {
			t.Errorf("user %d: username = %q, want %q (%v)", id, u.Username, want, err)
		}
	}
	if _, err := store.CreateUser("new@example.com", "OLDUSER", "hash"); err == nil {
		t.Error("the name must be taken now that the index exists")
	}
}
