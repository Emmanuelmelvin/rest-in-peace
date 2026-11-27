package rt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// resetTests clears the global test state after a test that changed it.
func resetTests(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		cleanupTestDBs()
		ResetDBs()
		testState.mu.Lock()
		testState.enabled, testState.fresh, testState.bypass, testState.migrateErr = false, false, false, false
		testState.url, testState.migrations, testState.dbNames, testState.tests = "", nil, nil, nil
		testState.mu.Unlock()
	})
}

// expectPanic runs f and returns the *Error it raised.
func expectPanic(t *testing.T, f func()) (e *Error) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("did not panic")
		}
		var ok bool
		if e, ok = r.(*Error); !ok {
			t.Fatalf("panicked with %T %v, want *Error", r, r)
		}
	}()
	f()
	return nil
}

func TestExpect(t *testing.T) {
	c := NewCtx(context.Background())
	Expect(c, true, "unused")
	ExpectEq(c, int64(2), float64(2), "")
	ExpectEq(c, nil, nil, "")

	cases := []struct {
		name string
		f    func()
		want string
	}{
		{"false", func() { Expect(c, false, "") }, "the condition was FALSE"},
		{"false with message", func() { Expect(c, false, "TOTAL IS WRONG") }, "TOTAL IS WRONG"},
		{"numbers", func() { ExpectEq(c, int64(404), int64(401), "") }, "expected 401, got 404"},
		{"strings are quoted", func() { ExpectEq(c, "1", int64(1), "") }, `expected 1, got "1"`},
		{"NIL by name", func() { ExpectEq(c, nil, "x", "") }, `expected "x", got NIL`},
		{"maps as JSON", func() { ExpectEq(c, MapOf("a", int64(1)), nil, "") }, `expected NIL, got {"a":1}`},
		{"message wins", func() { ExpectEq(c, int64(1), int64(2), "NOPE") }, "NOPE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := expectPanic(t, tc.f)
			if e.Kind != KindExpectFailed || e.Message != tc.want {
				t.Errorf("got %s %q, want EXPECT FAILED %q", e.Kind, e.Message, tc.want)
			}
		})
	}
}

// requestApp is an app with a few routes for REQUEST.
func requestApp() *App {
	a := NewApp("main.bas")
	a.Route("GET", "/hello/:name", func(c *Ctx, req Value) Value {
		return MapOf("message", "HELLO "+ToString(Get(Get(req, "params"), "name")), "q", Get(Get(req, "query"), "x"))
	})
	a.Route("POST", "/echo", func(c *Ctx, req Value) Value {
		RespondNow(c, 201, MapOf("body", Get(req, "body"), "key", Get(Get(req, "headers"), "x-api-key"), "ip", c.Req.RemoteAddr),
			MapOf("X-Made-By", "rip"))
		return nil
	})
	a.Route("GET", "/text", func(c *Ctx, req Value) Value {
		RespondNow(c, 200, "PLAIN", MapOf("Content-Type", "text/plain"))
		return nil
	})
	a.Route("DELETE", "/gone", func(c *Ctx, req Value) Value {
		RespondNow(c, 204, nil)
		return nil
	})
	return a
}

func TestRequest(t *testing.T) {
	a := requestApp()
	c := NewCtx(context.Background())
	c.app = a

	r := AsMap(DoRequest(c, "get", "/hello/ghost?x=1"))
	if got := ToString(r); got != `{"status":200,"headers":{"content-type":"application/json; charset=utf-8"},"body":{"message":"HELLO ghost","q":"1"}}` {
		t.Errorf("GET = %s", got)
	}

	r = AsMap(DoRequest(c, "POST", "/echo", MapOf("items", ArrayOf(int64(1))), MapOf("x-api-key", "test-key")))
	if got := ToString(Get(r, "status")); got != "201" {
		t.Errorf("POST status = %s", got)
	}
	if got := ToString(Get(Get(r, "headers"), "x-made-by")); got != "rip" {
		t.Errorf("x-made-by header = %q", got)
	}
	if got := ToString(Get(r, "body")); got != `{"body":{"items":[1]},"key":"test-key","ip":"192.0.2.1:1234"}` {
		t.Errorf("POST body = %s", got)
	}

	// A STRING body is sent as it is, so tests can send invalid JSON.
	r = AsMap(DoRequest(c, "POST", "/echo", "{not json"))
	if got := ToString(Get(r, "status")) + " " + ToString(Get(Get(r, "body"), "error")); got != "400 BODY IS NOT VALID JSON" {
		t.Errorf("bad JSON = %s", got)
	}

	r = AsMap(DoRequest(c, "GET", "/text"))
	if b, ok := Get(r, "body").(string); !ok || b != "PLAIN" {
		t.Errorf("text body = %#v", Get(r, "body"))
	}
	r = AsMap(DoRequest(c, "DELETE", "/gone"))
	if Get(r, "body") != nil || ToString(Get(r, "status")) != "204" {
		t.Errorf("204 = %s", ToString(r))
	}
	r = AsMap(DoRequest(c, "GET", "/nowhere"))
	if got := ToString(Get(r, "status")) + " " + ToString(Get(Get(r, "body"), "error")); got != "404 ROUTE NOT FOUND IN THIS REALM" {
		t.Errorf("404 = %s", got)
	}
	r = AsMap(DoRequest(c, "GET", "/_health"))
	if got := ToString(Get(r, "body")); got != `{"status":"ALIVE"}` {
		t.Errorf("health = %s", got)
	}
}

func TestRequestErrors(t *testing.T) {
	c := NewCtx(context.Background())
	c.app = requestApp()
	for _, tc := range []struct {
		name string
		f    func()
		want string
	}{
		{"no slash", func() { DoRequest(c, "GET", "hello") }, "starts with /"},
		{"no method", func() { DoRequest(c, " ", "/hello") }, "needs an HTTP method"},
		{"headers not a MAP", func() { DoRequest(c, "GET", "/hello/x", nil, "x-api-key") }, "headers must be a MAP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := expectPanic(t, tc.f)
			if e.Kind != KindTypeMismatch || !strings.Contains(e.Message, tc.want) {
				t.Errorf("got %s %q, want TYPE MISMATCH containing %q", e.Kind, e.Message, tc.want)
			}
		})
	}
}

func TestRunTests(t *testing.T) {
	resetTests(t)
	top := NewCtx(context.Background())
	top.App().Route("GET", "/ping", func(c *Ctx, req Value) Value { return "PONG" })

	Test(top, "PASSES", "t.test.bas", 1, func(c *Ctx) {
		r := DoRequest(c, "GET", "/ping")
		ExpectEq(c, Get(r, "body"), "PONG", "")
	})
	Test(top, "FAILS", "t.test.bas", 5, func(c *Ctx) { Expect(c, false, "BROKEN") })
	Test(top, "ENDS", "t.test.bas", 9, func(c *Ctx) { End() })
	Test(top, "PANICS", "t.test.bas", 12, func(c *Ctx) {
		var m map[string]int
		m["x"] = 1
	})
	Test(top, "STILL RUNS", "t.test.bas", 15, func(c *Ctx) {})
	if !testRun() {
		t.Fatal("testRun() = false with tests registered")
	}

	var stdout, stderr bytes.Buffer
	if code := runTests(top, &stdout, &stderr); code != 1 {
		t.Errorf("exit status = %d, want 1", code)
	}
	wantOut := "PASS  PASSES\nFAIL  FAILS\nPASS  ENDS\nFAIL  PANICS\nPASS  STILL RUNS\n\n5 TESTS: 3 PASSED, 2 FAILED\n"
	if stdout.String() != wantOut {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout.String(), wantOut)
	}
	// Without //line frames the error falls back to the TEST's own location.
	wantErr := "?EXPECT FAILED ERROR IN t.test.bas:5 — BROKEN\n?NIL ACCESS ERROR IN t.test.bas:12 — a value is NIL — check it exists before using it\n"
	if stderr.String() != wantErr {
		t.Errorf("stderr:\n%s\nwant:\n%s", stderr.String(), wantErr)
	}
}

func TestRunTestsAllPass(t *testing.T) {
	resetTests(t)
	top := NewCtx(context.Background())
	Test(top, "ONE", "t.test.bas", 1, func(c *Ctx) {})
	var stdout, stderr bytes.Buffer
	if code := runTests(top, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Errorf("exit status = %d, stderr %q", code, stderr.String())
	}
	if want := "PASS  ONE\n\n1 TEST: 1 PASSED, 0 FAILED\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestTestDatabase checks rip test's database handling: DBCONNECT never
// opens its own URL, the migrations are applied, and each TEST starts from
// the database as the top-level code left it.
func TestTestDatabase(t *testing.T) {
	resetTests(t)
	t.Setenv(TestDatabaseURLEnv, "")
	EnableTestMode(
		Migration{Name: "migrations/001.sql", SQL: "CREATE TABLE notes (id INTEGER PRIMARY KEY, text TEXT NOT NULL);\nCREATE TABLE tags (name TEXT);"},
	)
	if !testRun() {
		t.Fatal("testRun() = false in test mode")
	}
	top := NewCtx(context.Background())
	// A real program's URL: never contacted under test.
	DBConnect(top, "postgres://prod.invalid:5432/app", "")
	DBConnect(top, "postgres://prod.invalid:5432/audit", "audit")
	SQLExec(top, "INSERT INTO notes (text) VALUES ($1)", "SEEDED")

	count := func(c *Ctx) int64 { return AsInt(Get(SQLOne(c, "SELECT COUNT(*) AS n FROM notes"), "n")) }
	Test(top, "WRITES", "t.test.bas", 1, func(c *Ctx) {
		SQLExec(c, "INSERT INTO notes (text) VALUES ($1)", "MINE")
		ExpectEq(c, count(c), int64(2), "")
	})
	Test(top, "IS FRESH", "t.test.bas", 2, func(c *Ctx) {
		ExpectEq(c, count(c), int64(1), "")
		ExpectEq(c, SQLExec(c, "INSERT INTO tags (name) VALUES ($1)", "x"), int64(1), "")
	})
	var stdout, stderr bytes.Buffer
	if code := runTests(top, &stdout, &stderr); code != 0 {
		t.Fatalf("exit status %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	testState.mu.Lock()
	names := strings.Join(testState.dbNames, ",")
	testState.mu.Unlock()
	if names != ",AUDIT" {
		t.Errorf("dbNames = %q, want \",AUDIT\"", names)
	}
}

// TestTestDatabaseURL checks that TEST_DATABASE_URL replaces every DBCONNECT
// and is used as it is: no migrations, one database for the whole run.
func TestTestDatabaseURL(t *testing.T) {
	dir := t.TempDir() // before resetTests, whose cleanup must close the database first
	resetTests(t)
	url := "sqlite://" + strings.ReplaceAll(dir, `\`, "/") + "/t.db"
	t.Setenv(TestDatabaseURLEnv, url)
	EnableTestMode(Migration{Name: "migrations/001.sql", SQL: "CREATE TABLE never (id INTEGER)"})
	top := NewCtx(context.Background())
	DBConnect(top, "postgres://prod.invalid:5432/app", "")
	SQLExec(top, "CREATE TABLE notes (text TEXT)")
	Test(top, "ONE", "t.test.bas", 1, func(c *Ctx) { SQLExec(c, "INSERT INTO notes (text) VALUES ($1)", "A") })
	Test(top, "TWO", "t.test.bas", 2, func(c *Ctx) {
		ExpectEq(c, Get(SQLOne(c, "SELECT COUNT(*) AS n FROM notes"), "n"), int64(1), "")
		ExpectEq(c, len(SQL(c, "SELECT name FROM sqlite_master WHERE name = 'never'").Items), 0, "")
	})
	var stdout, stderr bytes.Buffer
	if code := runTests(top, &stdout, &stderr); code != 0 {
		t.Fatalf("exit status %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// A migration that SQLite rejects is reported, and the tests still run.
func TestTestMigrationError(t *testing.T) {
	resetTests(t)
	t.Setenv(TestDatabaseURLEnv, "")
	EnableTestMode(Migration{Name: "migrations/001.sql", SQL: "CREATE TABLE x (at TIMESTAMPTZ DEFAULT now())"})
	top := NewCtx(context.Background())
	DBConnect(top, "postgres://prod.invalid:5432/app", "")
	testState.mu.Lock()
	reported := testState.migrateErr
	testState.mu.Unlock()
	if !reported {
		t.Error("the failed migration was not reported")
	}
}

// Outside test mode, DBCONNECT is untouched.
func TestNoTestModeDBConnect(t *testing.T) {
	resetTests(t)
	c := NewCtx(context.Background())
	e := expectPanic(t, func() { DBConnect(c, "mysql://nope", "") })
	if e.Kind != KindDatabase || !strings.Contains(e.Message, "unsupported database URL") {
		t.Errorf("got %s %q", e.Kind, e.Message)
	}
	if testRun() {
		t.Error("testRun() = true with no tests and no test mode")
	}
}
