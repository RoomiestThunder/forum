package database_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"forum/internal/database"
	"forum/internal/database/dbtest"
)

// The PostgreSQL schema exists in two forms that must describe the same
// database: the numbered migrations in migrations/ (what docker-compose and
// real deployments apply) and schema_postgres.sql (what tests and first-run
// setup apply). They are edited by hand in parallel, so without a test the
// first forgotten column or index goes unnoticed until production.

const migrationsDir = "../../migrations"

func migrationFiles(t *testing.T, suffix string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*"+suffix))
	if err != nil || len(files) == 0 {
		t.Fatalf("no %s migrations found in %s: %v", suffix, migrationsDir, err)
	}
	sort.Strings(files) // numeric prefixes make lexical order the application order
	return files
}

func applyAll(t *testing.T, db *sql.DB, files []string) {
	t.Helper()
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("applying %s: %v", filepath.Base(f), err)
		}
	}
}

// describeSchema returns one canonical line per column, constraint and index
// of the connection's schema, sorted, so two schemas can be diffed.
func describeSchema(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var lines []string

	collect := func(query, label string) {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]sql.NullString, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			parts := []string{label}
			for _, v := range vals {
				parts = append(parts, v.String)
			}
			lines = append(lines, strings.Join(parts, " | "))
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}

	// Column position is deliberately not compared: ALTER TABLE ADD COLUMN can
	// only append, while the schema file declares columns where they read best.
	collect(`SELECT table_name, column_name, data_type, is_nullable,
	                COALESCE(column_default, ''), is_generated, COALESCE(generation_expression, '')
	         FROM information_schema.columns WHERE table_schema = current_schema()`, "column")

	collect(`SELECT conrelid::regclass::text, contype::text, pg_get_constraintdef(oid)
	         FROM pg_constraint WHERE connamespace = current_schema()::regnamespace`, "constraint")

	// The index definition embeds the schema name, which differs per test.
	collect(`SELECT tablename, indexname, replace(indexdef, current_schema() || '.', '')
	         FROM pg_indexes WHERE schemaname = current_schema()`, "index")

	sort.Strings(lines)
	return lines
}

func diff(a, b []string) (onlyA, onlyB []string) {
	inB := make(map[string]bool, len(b))
	for _, l := range b {
		inB[l] = true
	}
	inA := make(map[string]bool, len(a))
	for _, l := range a {
		inA[l] = true
		if !inB[l] {
			onlyA = append(onlyA, l)
		}
	}
	for _, l := range b {
		if !inA[l] {
			onlyB = append(onlyB, l)
		}
	}
	return onlyA, onlyB
}

func TestMigrationsProduceTheEmbeddedSchema(t *testing.T) {
	migrated := dbtest.RawPostgres(t)
	applyAll(t, migrated, migrationFiles(t, ".up.sql"))

	embedded := dbtest.RawPostgres(t)
	if err := database.RunPostgresSchema(embedded); err != nil {
		t.Fatalf("apply embedded schema: %v", err)
	}

	onlyMigrations, onlyEmbedded := diff(describeSchema(t, migrated), describeSchema(t, embedded))
	for _, l := range onlyMigrations {
		t.Errorf("in migrations/ but not in schema_postgres.sql:\n    %s", l)
	}
	for _, l := range onlyEmbedded {
		t.Errorf("in schema_postgres.sql but not in migrations/:\n    %s", l)
	}
}

// Applying every up migration and then every down migration in reverse must
// leave nothing behind, otherwise a rollback leaves a database that no
// migration describes.
func TestDownMigrationsUndoTheUpMigrations(t *testing.T) {
	db := dbtest.RawPostgres(t)
	applyAll(t, db, migrationFiles(t, ".up.sql"))

	downs := migrationFiles(t, ".down.sql")
	for i, j := 0, len(downs)-1; i < j; i, j = i+1, j-1 {
		downs[i], downs[j] = downs[j], downs[i]
	}
	applyAll(t, db, downs)

	if left := describeSchema(t, db); len(left) != 0 {
		t.Errorf("rolling everything back should leave an empty schema, but left:\n    %s", strings.Join(left, "\n    "))
	}
}

// Every migration must have a matching down and vice versa.
func TestEveryUpMigrationHasADown(t *testing.T) {
	base := func(files []string, suffix string) map[string]bool {
		m := map[string]bool{}
		for _, f := range files {
			m[strings.TrimSuffix(filepath.Base(f), suffix)] = true
		}
		return m
	}
	ups, downs := base(migrationFiles(t, ".up.sql"), ".up.sql"), base(migrationFiles(t, ".down.sql"), ".down.sql")
	for name := range ups {
		if !downs[name] {
			t.Errorf("migration %s has no .down.sql", name)
		}
	}
	for name := range downs {
		if !ups[name] {
			t.Errorf("%s.down.sql has no matching .up.sql", name)
		}
	}
}

// docker-compose lists the init scripts one by one (a directory mount would
// also run the .down.sql files), so a new migration that isn't added there
// silently never reaches a compose-based database.
func TestEveryMigrationIsMountedInDockerCompose(t *testing.T) {
	compose, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	mounted := map[string]bool{}
	for _, m := range regexp.MustCompile(`migrations/([\w]+\.up\.sql)`).FindAllStringSubmatch(string(compose), -1) {
		mounted[m[1]] = true
	}

	for _, f := range migrationFiles(t, ".up.sql") {
		name := filepath.Base(f)
		if !mounted[name] {
			t.Errorf("%s is not mounted in docker-compose.yml", name)
		}
		delete(mounted, name)
	}
	for name := range mounted {
		t.Errorf("docker-compose.yml mounts %s, which doesn't exist", name)
	}
}

// Migration 013 makes usernames unique, but a database that has been running
// without that constraint may already hold duplicates, which would make
// building the index fail. The migration has to rename them first.
func TestUniqueUsernameMigrationRenamesExistingDuplicates(t *testing.T) {
	db := dbtest.RawPostgres(t)

	var before, after []string
	for _, f := range migrationFiles(t, ".up.sql") {
		if strings.HasPrefix(filepath.Base(f), "013_") {
			after = append(after, f)
		} else {
			before = append(before, f)
		}
	}
	if len(after) != 1 {
		t.Fatalf("expected exactly one 013 migration, found %v", after)
	}
	applyAll(t, db, before)

	if _, err := db.Exec(`
		INSERT INTO users (email, username, password) VALUES
			('a@example.com', 'alice', 'h'),
			('b@example.com', 'ALICE', 'h'),
			('c@example.com', 'Alice', 'h'),
			('d@example.com', 'bob',   'h')`); err != nil {
		t.Fatal(err)
	}

	applyAll(t, db, after)

	rows, err := db.Query("SELECT username FROM users ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	want := []string{"alice", "ALICE_2", "Alice_3", "bob"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("usernames after migrating = %v, want %v", got, want)
	}

	if _, err := db.Exec(`INSERT INTO users (email, username, password) VALUES ('e@example.com', 'aLiCe', 'h')`); err == nil {
		t.Error("the unique index must now reject a case-variant of an existing name")
	}
}
