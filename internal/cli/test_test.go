package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runTestIn chdir-s into dir, captures stdout and stderr, runs `rip test`
// with args, and returns the exit code and captured outputs.
func runTestIn(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() { _ = os.Chdir(orig) }()

	capDir := t.TempDir()
	outF, err := os.Create(filepath.Join(capDir, "stdout"))
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	errF, err := os.Create(filepath.Join(capDir, "stderr"))
	if err != nil {
		outF.Close()
		t.Fatalf("create stderr file: %v", err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outF, errF
	code = Run(append([]string{"test"}, args...))
	os.Stdout, os.Stderr = origOut, origErr

	_ = outF.Close()
	_ = errF.Close()

	outBytes, err := os.ReadFile(outF.Name())
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	errBytes, err := os.ReadFile(errF.Name())
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return code, string(outBytes), string(errBytes)
}

func writeProjectFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestTestEndToEndPass exercises rip test on a full project: main.bas with
// DBCONNECT, migrations, SERVE, routes, .env.test, and tests/*.test.bas using
// REQUEST(...) and fresh SQLite :memory: databases per TEST.
func TestTestEndToEndPass(t *testing.T) {
	t.Setenv("TEST_DATABASE_URL", "")
	// Even if DATABASE_URL points to an unreachable host, rip test must override it.
	t.Setenv("DATABASE_URL", "postgres://prod.invalid:5432/app")
	t.Setenv("RIP_ENV", "")

	dir := t.TempDir()
	writeProjectFile(t, dir, "rip.json", `{"name":"demo","main":"main.bas"}`)
	writeProjectFile(t, dir, ".env.test", "APP_SECRET=from-env-test\n")
	writeProjectFile(t, dir, "migrations/001_init.sql", `
CREATE TABLE items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);
`)
	writeProjectFile(t, dir, "main.bas", `
DBCONNECT ENVREQUIRE("DATABASE_URL")

ROUTE GET "/secret" CALL GetSecret
ROUTE GET "/items" CALL ListItems
ROUTE POST "/items" CALL CreateItem
SERVE 18999

FUNCTION GetSecret(req)
	RETURN {"secret": ENV("APP_SECRET", "missing")}
END FUNCTION

FUNCTION ListItems(req)
	LET rows = SQL("SELECT name FROM items ORDER BY id")
	RETURN {"count": LEN(rows), "items": rows}
END FUNCTION

FUNCTION CreateItem(req)
	SQLEXEC("INSERT INTO items (name) VALUES ($1)", req.body.name)
	RESPOND 201, {"created": req.body.name}
END FUNCTION
`)
	writeProjectFile(t, dir, "tests/app.test.bas", `
TEST "LOADS DOT ENV TEST"
	LET r = REQUEST("GET", "/secret")
	EXPECT r.status = 200
	EXPECT r.body.secret = "from-env-test"
END TEST

TEST "CREATES AND READS ITEMS"
	LET c = REQUEST("POST", "/items", {"name": "ghost"})
	EXPECT c.status = 201
	EXPECT c.body.created = "ghost"

	LET list = REQUEST("GET", "/items")
	EXPECT list.status = 200
	EXPECT list.body.count = 1
	EXPECT list.body.items[0].name = "ghost"
END TEST

TEST "STARTS WITH A FRESH DATABASE"
	LET list = REQUEST("GET", "/items")
	EXPECT list.status = 200
	EXPECT list.body.count = 0
END TEST
`)

	code, stdout, stderr := runTestIn(t, dir)
	if code != 0 {
		t.Fatalf("rip test exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr:\n%s", stderr)
	}
	for _, want := range []string{
		"PASS  LOADS DOT ENV TEST",
		"PASS  CREATES AND READS ITEMS",
		"PASS  STARTS WITH A FRESH DATABASE",
		"3 TESTS: 3 PASSED, 0 FAILED",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// TestTestEndToEndFail verifies that a failing EXPECT prints the BASIC file
// and line to stderr, still runs the remaining tests, and exits non-zero.
func TestTestEndToEndFail(t *testing.T) {
	t.Setenv("TEST_DATABASE_URL", "")
	dir := t.TempDir()
	writeProjectFile(t, dir, "rip.json", `{"name":"faildemo","main":"main.bas"}`)
	writeProjectFile(t, dir, "main.bas", `
ROUTE GET "/ping" CALL Ping
SERVE

FUNCTION Ping(req)
	RETURN {"ok": TRUE}
END FUNCTION
`)
	writeProjectFile(t, dir, "tests/fail.test.bas", `TEST "PASSES"
	LET r = REQUEST("GET", "/ping")
	EXPECT r.status = 200
END TEST

TEST "FAILS WITH LOCATION"
	LET r = REQUEST("GET", "/ping")
	EXPECT r.status = 404, "expected 404 for test"
END TEST
`)

	code, stdout, stderr := runTestIn(t, dir)
	if code == 0 {
		t.Fatalf("rip test exited 0, want non-zero\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	for _, want := range []string{
		"PASS  PASSES",
		"FAIL  FAILS WITH LOCATION",
		"2 TESTS: 1 PASSED, 1 FAILED",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	wantErr := "?EXPECT FAILED ERROR IN tests/fail.test.bas:8 — expected 404 for test"
	if !strings.Contains(stderr, wantErr) {
		t.Errorf("stderr missing %q:\n%s", wantErr, stderr)
	}
}

// TestTestDatabaseURLOverride verifies that TEST_DATABASE_URL overrides the
// default SQLite :memory: database.
func TestTestDatabaseURLOverride(t *testing.T) {
	dbFile := filepath.ToSlash(filepath.Join(t.TempDir(), "override.db"))
	t.Setenv("TEST_DATABASE_URL", "sqlite://"+dbFile)

	dir := t.TempDir()
	writeProjectFile(t, dir, "rip.json", `{"name":"overridedemo","main":"main.bas"}`)
	writeProjectFile(t, dir, "main.bas", `
DBCONNECT "postgres://prod.invalid:5432/app"
SQLEXEC("CREATE TABLE IF NOT EXISTS notes (text TEXT)")
ROUTE POST "/notes" CALL AddNote
ROUTE GET "/notes" CALL CountNotes
SERVE

FUNCTION AddNote(req)
	SQLEXEC("INSERT INTO notes (text) VALUES ($1)", req.body.text)
	RETURN {"ok": TRUE}
END FUNCTION

FUNCTION CountNotes(req)
	LET row = SQLONE("SELECT COUNT(*) AS n FROM notes")
	RETURN {"n": row.n}
END FUNCTION
`)
	writeProjectFile(t, dir, "tests/db.test.bas", `
TEST "WRITES TO TEST_DATABASE_URL"
	EXPECT REQUEST("POST", "/notes", {"text": "persisted"}).status = 200
END TEST

TEST "SEES ROW ON SHARED TEST_DATABASE_URL"
	LET r = REQUEST("GET", "/notes")
	EXPECT r.status = 200
	EXPECT r.body.n = 1
END TEST
`)

	code, stdout, stderr := runTestIn(t, dir, "tests/db.test.bas")
	if code != 0 {
		t.Fatalf("rip test exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.FromSlash(dbFile)); err != nil {
		t.Errorf("expected TEST_DATABASE_URL file %s to exist: %v", dbFile, err)
	}
}

func TestTestNoTestsAndInvalidArgs(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "rip.json", `{"name":"empty","main":"main.bas"}`)
	writeProjectFile(t, dir, "main.bas", "PRINT \"HI\"\n")

	code, stdout, stderr := runTestIn(t, dir)
	if code != 0 {
		t.Fatalf("empty project exited %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "NO TESTS FOUND") {
		t.Errorf("stdout = %q, want NO TESTS FOUND", stdout)
	}

	code, _, stderr = runTestIn(t, dir, "main.bas")
	if code == 0 || !strings.Contains(stderr, "*.test.bas") {
		t.Errorf("bad arg: code=%d stderr=%q", code, stderr)
	}

	code, _, stderr = runTestIn(t, dir, "tests/missing.test.bas")
	if code == 0 || !strings.Contains(stderr, "?FILE NOT FOUND ERROR") {
		t.Errorf("missing file: code=%d stderr=%q", code, stderr)
	}
}

func TestTestEnv(t *testing.T) {
	t.Setenv("RIP_ENV", "")
	t.Setenv("TEST_DATABASE_URL", "")
	_ = os.Unsetenv("TEST_DATABASE_URL")
	t.Setenv("FROM_REAL_ENV", "real")

	dotEnv := filepath.Join(t.TempDir(), ".env.test")
	if err := os.WriteFile(dotEnv, []byte("FOO=bar\nFROM_REAL_ENV=ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testEnv(dotEnv)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "FOO=bar") {
		t.Errorf("missing FOO=bar in %v", got)
	}
	if strings.Contains(joined, "FROM_REAL_ENV=ignored") {
		t.Errorf("real env should override .env.test: %v", got)
	}
	if !strings.Contains(joined, "DATABASE_URL=sqlite://:memory:") {
		t.Errorf("missing default DATABASE_URL in %v", got)
	}

	// TEST_DATABASE_URL in .env.test sets DATABASE_URL when not in real env.
	dotEnv2 := filepath.Join(t.TempDir(), ".env.test")
	if err := os.WriteFile(dotEnv2, []byte("TEST_DATABASE_URL=sqlite:///tmp/a.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got2 := testEnv(dotEnv2)
	if !strings.Contains(strings.Join(got2, "\n"), "DATABASE_URL=sqlite:///tmp/a.db") {
		t.Errorf("expected DATABASE_URL from .env.test TEST_DATABASE_URL, got %v", got2)
	}
}
