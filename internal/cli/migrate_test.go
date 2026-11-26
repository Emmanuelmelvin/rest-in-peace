package cli

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// writeFile writes content to dir/name.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeRipJSON writes a minimal rip.json so resolve() can find the project.
func writeRipJSON(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "rip.json", `{"name":"testapp","main":"main.bas"}`)
	writeFile(t, dir, "main.bas", "PRINT \"hello\"\n")
}

// newSQLiteURL builds a sqlite:// URL pointing to a fresh temp file DB.
func newSQLiteURL(t *testing.T) string {
	t.Helper()
	return "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))
}

// runMigrateIn chdir-s into dir, sets DATABASE_URL, calls runMigrate(nil),
// then restores both.
func runMigrateIn(t *testing.T, dir, dbURL string) int {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}

	// Set / unset DATABASE_URL for this test.
	prev, had := os.LookupEnv("DATABASE_URL")
	t.Cleanup(func() {
		if had {
			os.Setenv("DATABASE_URL", prev) //nolint:errcheck
		} else {
			os.Unsetenv("DATABASE_URL") //nolint:errcheck
		}
	})
	if dbURL != "" {
		os.Setenv("DATABASE_URL", dbURL) //nolint:errcheck
	} else {
		os.Unsetenv("DATABASE_URL") //nolint:errcheck
	}

	return runMigrate(nil)
}

// openSQLite opens the SQLite DB at a sqlite:// URL for inspection.
func openSQLite(t *testing.T, dbURL string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(dbURL))
	if err != nil {
		t.Fatalf("open %s: %v", dbURL, err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// appliedNames returns migration names from _rip_migrations in rowid order.
func appliedNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM _rip_migrations ORDER BY rowid`)
	if err != nil {
		t.Fatalf("query _rip_migrations: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

// tableExists reports whether a SQLite table exists.
func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&n)
	if err != nil {
		t.Fatalf("tableExists %s: %v", table, err)
	}
	return n > 0
}

// makeMigrationsDir creates migrations/ under root and writes the given files.
// Pass nil to create no directory (simulate missing migrations/).
func makeMigrationsDir(t *testing.T, root string, files map[string]string) {
	t.Helper()
	if files == nil {
		return
	}
	mdir := filepath.Join(root, "migrations")
	if err := os.Mkdir(mdir, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	for name, content := range files {
		writeFile(t, mdir, name, content)
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestMigrate_AppliesInOrder verifies that multiple SQL files are applied in
// lexicographic filename order regardless of map/fs iteration order.
func TestMigrate_AppliesInOrder(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)
	makeMigrationsDir(t, root, map[string]string{
		// Written in reverse order to confirm sorting.
		"003_orders.sql": "CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL);",
		"001_users.sql":  "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL);",
		"002_items.sql":  "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);",
	})

	dbURL := newSQLiteURL(t)
	if code := runMigrateIn(t, root, dbURL); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}

	db := openSQLite(t, dbURL)
	got := appliedNames(t, db)
	want := []string{"001_users.sql", "002_items.sql", "003_orders.sql"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("applied order: got %v, want %v", got, want)
	}
	for _, table := range []string{"users", "items", "orders"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q was not created", table)
		}
	}
}

// TestMigrate_Idempotent verifies that re-running rip migrate skips already-applied
// migrations and exits 0.
func TestMigrate_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)
	makeMigrationsDir(t, root, map[string]string{
		"001_init.sql": "CREATE TABLE things (id INTEGER PRIMARY KEY);",
	})

	dbURL := newSQLiteURL(t)

	if code := runMigrateIn(t, root, dbURL); code != 0 {
		t.Fatalf("first run exit %d, want 0", code)
	}
	if code := runMigrateIn(t, root, dbURL); code != 0 {
		t.Fatalf("second run exit %d, want 0 (idempotent)", code)
	}

	db := openSQLite(t, dbURL)
	got := appliedNames(t, db)
	if len(got) != 1 || got[0] != "001_init.sql" {
		t.Fatalf("expected [001_init.sql], got %v", got)
	}
}

// TestMigrate_RollbackOnFailure verifies that a migration with invalid SQL rolls
// back and is NOT recorded in _rip_migrations; prior successful migrations are unaffected.
func TestMigrate_RollbackOnFailure(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)
	makeMigrationsDir(t, root, map[string]string{
		"001_good.sql": "CREATE TABLE good (id INTEGER PRIMARY KEY);",
		// Two statements; the second is intentionally broken to exercise rollback.
		"002_bad.sql": "CREATE TABLE almost (id INTEGER PRIMARY KEY);\nTHIS IS NOT VALID SQL;",
	})

	dbURL := newSQLiteURL(t)
	code := runMigrateIn(t, root, dbURL)
	if code == 0 {
		t.Fatal("expected non-zero exit for failing migration, got 0")
	}

	db := openSQLite(t, dbURL)
	got := appliedNames(t, db)
	// 001 succeeded; 002 must NOT be recorded.
	if len(got) != 1 || got[0] != "001_good.sql" {
		t.Fatalf("_rip_migrations = %v, want [001_good.sql]", got)
	}
	// The partial table from 002 must not exist (rolled back).
	if tableExists(t, db, "almost") {
		t.Fatal("table 'almost' exists but migration 002 should have rolled back")
	}
}

// TestMigrate_MissingDatabaseURL verifies the classic error message and non-zero exit.
func TestMigrate_MissingDatabaseURL(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)

	// Capture stderr.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	code := runMigrateIn(t, root, "" /* unset */)

	w.Close()
	os.Stderr = origStderr

	var buf strings.Builder
	tmp := make([]byte, 4096)
	for {
		n, _ := r.Read(tmp)
		if n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}
	r.Close()

	if code == 0 {
		t.Fatal("expected non-zero exit when DATABASE_URL is unset, got 0")
	}
	msg := buf.String()
	if !strings.Contains(msg, "?CONFIG ERROR") {
		t.Errorf("expected ?CONFIG ERROR in stderr, got: %q", msg)
	}
	if !strings.Contains(msg, "DATABASE_URL") {
		t.Errorf("expected DATABASE_URL mentioned in stderr, got: %q", msg)
	}
}

// TestMigrate_MissingMigrationsDir exits 0 when migrations/ does not exist.
func TestMigrate_MissingMigrationsDir(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)
	// No migrations directory created.

	dbURL := newSQLiteURL(t)
	if code := runMigrateIn(t, root, dbURL); code != 0 {
		t.Fatalf("exit %d, want 0 when migrations/ is absent", code)
	}
}

// TestMigrate_EmptyMigrationsDir exits 0 when migrations/ exists but has no .sql files.
func TestMigrate_EmptyMigrationsDir(t *testing.T) {
	root := t.TempDir()
	writeRipJSON(t, root)
	// Create the directory but only put a non-.sql file in it.
	makeMigrationsDir(t, root, map[string]string{
		"README.md": "nothing here",
	})

	dbURL := newSQLiteURL(t)
	if code := runMigrateIn(t, root, dbURL); code != 0 {
		t.Fatalf("exit %d, want 0 for empty migrations dir", code)
	}
}

// ── Unit tests for internal helpers ──────────────────────────────────────────

func TestSplitSQL(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		count int
	}{
		{"single", "CREATE TABLE t (id INTEGER);", 1},
		{"two stmts", "CREATE TABLE a (id INTEGER);\nCREATE TABLE b (id INTEGER);", 2},
		{"comment only", "-- just a comment\n", 0},
		{"stmt after comment", "-- setup\nCREATE TABLE c (x TEXT);", 1},
		{"no trailing semicolon", "CREATE TABLE d (x TEXT)", 1},
		{"empty", "", 0},
		{"blank lines", "\n  \n\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSQL(tc.src)
			if len(got) != tc.count {
				t.Errorf("got %d stmts, want %d: %v", len(got), tc.count, got)
			}
		})
	}
}

func TestSQLiteDSN(t *testing.T) {
	cases := []struct{ url, want string }{
		{"sqlite://:memory:", ":memory:"},
		{"sqlite:///tmp/x.db", "/tmp/x.db"},
		{"sqlite://./rel.db", "./rel.db"},
		{"sqlite://rel.db", "rel.db"},
	}
	for _, tc := range cases {
		got := sqliteDSN(tc.url)
		if got != tc.want {
			t.Errorf("sqliteDSN(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestCollectMigrations_SortedAndFiltered(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"003.sql", "001.sql", "002.sql", "skip.txt", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	got, err := collectMigrations(dir)
	if err != nil {
		t.Fatalf("collectMigrations: %v", err)
	}
	want := []string{"001.sql", "002.sql", "003.sql"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectMigrations_MissingDir(t *testing.T) {
	got, err := collectMigrations(filepath.Join(t.TempDir(), "no-such-dir"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nil/empty, got %v", got)
	}
}
