package rt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Tests in BASIC (Requirement 12, task 4.2).
//
// A TEST "name" … END TEST block compiles to a call to Test in its module's
// top-level code, so it only registers the test. Main runs the program's
// top-level code as usual (DBCONNECT, USE, ROUTE, SERVE, …) and then, when
// any test is registered or the build is a rip test build (EnableTestMode),
// runs the tests one by one instead of starting the server. So SERVE never
// binds a port under test, and REQUEST calls the routes in-process.
//
// Each TEST runs on its own Ctx, and an error in one test never stops the
// others. Under rip test (EnableTestMode) every DBCONNECT is redirected to
// the test database: TEST_DATABASE_URL if it is set, otherwise a fresh
// SQLite :memory: database with the project's migrations applied. With the
// in-memory default, each TEST starts from a copy of the database as it was
// when the top-level code finished, so tests cannot see each other's rows.

// TestDatabaseURLEnv names the environment variable that points rip test at
// a real database instead of SQLite :memory:.
const TestDatabaseURLEnv = "TEST_DATABASE_URL"

// DefaultTestDatabaseURL is the database every DBCONNECT uses under rip test
// when TEST_DATABASE_URL is not set.
const DefaultTestDatabaseURL = "sqlite://:memory:"

// Migration is one migrations/*.sql file embedded into a rip test build.
type Migration struct {
	Name string // shown in errors, e.g. "migrations/001_init.sql"
	SQL  string
}

type testCase struct {
	name string
	file string
	line int
	fn   func(c *Ctx)
}

// testState is global because a program has one set of tests, registered by
// top-level code before any of them runs.
var testState struct {
	mu         sync.Mutex
	enabled    bool   // a rip test build
	url        string // where every DBCONNECT goes
	fresh      bool   // the in-memory default: migrate, and give each TEST its own copy
	bypass     bool   // set while the test hooks call DBConnect themselves
	migrations []Migration
	migrateErr bool     // a migration failed; reported once
	dbNames    []string // DBCONNECT names seen, normalized, in order
	tests      []testCase
	snapDir    string
	snapshots  map[string]string // DB name → snapshot file
}

// EnableTestMode is called by the init function of a rip test build, before
// any top-level code runs. migrations are applied to the in-memory test
// database; they are ignored when TEST_DATABASE_URL is set, because that
// database is managed with rip migrate like any other.
func EnableTestMode(migrations ...Migration) {
	testState.mu.Lock()
	defer testState.mu.Unlock()
	testState.enabled = true
	testState.migrations = migrations
	if u := strings.TrimSpace(os.Getenv(TestDatabaseURLEnv)); u != "" {
		testState.url, testState.fresh = u, false
	} else {
		testState.url, testState.fresh = DefaultTestDatabaseURL, true
	}
}

// Test implements TEST "name" … END TEST: it registers fn, which runs after
// all top-level code (Requirement 12.1). file and line are where the TEST is.
func Test(c *Ctx, name, file string, line int, fn func(c *Ctx)) {
	testState.mu.Lock()
	testState.tests = append(testState.tests, testCase{name: name, file: file, line: line, fn: fn})
	testState.mu.Unlock()
}

// testRun reports whether Main should run tests instead of the server.
func testRun() bool {
	testState.mu.Lock()
	defer testState.mu.Unlock()
	return testState.enabled || len(testState.tests) > 0
}

// testDBConnect is DBConnect's test-mode hook. Under rip test it opens the
// test database instead of rawURL, which is never touched (Requirement
// 12.4), and reports true so DBConnect returns. Otherwise it reports false.
func testDBConnect(c *Ctx, rawURL, name string) bool {
	testState.mu.Lock()
	if !testState.enabled || testState.bypass {
		testState.mu.Unlock()
		return false
	}
	norm := normalizeDBName(name)
	seen := false
	for _, n := range testState.dbNames {
		seen = seen || n == norm
	}
	if !seen {
		testState.dbNames = append(testState.dbNames, norm)
	}
	url, fresh := testState.url, testState.fresh
	testState.mu.Unlock()

	connectTestDB(c, url, name)
	if fresh && norm == "" {
		applyTestMigrations(c)
	}
	return true
}

// connectTestDB runs DBConnect without the test hook.
func connectTestDB(c *Ctx, url, name string) {
	testState.mu.Lock()
	testState.bypass = true
	testState.mu.Unlock()
	defer func() {
		testState.mu.Lock()
		testState.bypass = false
		testState.mu.Unlock()
	}()
	DBConnect(c, url, name)
}

// applyTestMigrations runs the embedded migrations on the default test
// database, in name order. A migration that fails is reported once on
// stderr, and the rest are skipped: the tests still run, and the ones that
// need the missing tables fail with their own errors.
func applyTestMigrations(c *Ctx) {
	db := DBFor("")
	if db == nil {
		return
	}
	for _, m := range testState.migrations {
		if _, err := db.Exec(c, m.SQL, nil); err != nil {
			testState.mu.Lock()
			report := !testState.migrateErr
			testState.migrateErr = true
			testState.mu.Unlock()
			if report {
				fmt.Fprintf(os.Stderr, "?DATABASE ERROR IN %s — could not apply it to the SQLite test database (%v); write migrations in SQL that SQLite and Postgres both accept, or set %s to a Postgres database\n",
					m.Name, err, TestDatabaseURLEnv)
			}
			return
		}
	}
}

// snapshotTestDBs saves each in-memory test database as it is after the
// top-level code, so that every TEST can start from a copy of it. If a
// snapshot cannot be made, the tests share the databases instead.
func snapshotTestDBs(c *Ctx) {
	testState.mu.Lock()
	fresh, names := testState.fresh && testState.enabled, append([]string(nil), testState.dbNames...)
	testState.mu.Unlock()
	if !fresh || len(names) == 0 {
		return
	}
	dir, err := os.MkdirTemp("", "rip-test-")
	if err != nil {
		return
	}
	snaps := map[string]string{}
	for i, name := range names {
		db := DBFor(name)
		if db == nil {
			continue
		}
		path := filepath.Join(dir, "snapshot"+strconv.Itoa(i)+".db")
		if _, err := db.Exec(c, "VACUUM INTO '"+strings.ReplaceAll(path, "'", "''")+"'", nil); err != nil {
			_ = os.RemoveAll(dir)
			return
		}
		snaps[name] = path
	}
	testState.mu.Lock()
	testState.snapDir, testState.snapshots = dir, snaps
	testState.mu.Unlock()
}

// freshTestDBs gives test number n its own copy of every snapshot.
func freshTestDBs(c *Ctx, n int) {
	testState.mu.Lock()
	dir, snaps, names := testState.snapDir, testState.snapshots, append([]string(nil), testState.dbNames...)
	testState.mu.Unlock()
	for i, name := range names {
		snap := snaps[name]
		if snap == "" {
			continue
		}
		path := filepath.Join(dir, "test"+strconv.Itoa(n)+"_"+strconv.Itoa(i)+".db")
		if err := copyFile(snap, path); err != nil {
			panic(Errorf(KindDatabase, "could not make a fresh test database: %v", err))
		}
		connectTestDB(c, "sqlite://"+filepath.ToSlash(path), name)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// cleanupTestDBs closes the test databases and removes their files.
func cleanupTestDBs() {
	testState.mu.Lock()
	dir := testState.snapDir
	testState.snapDir, testState.snapshots = "", nil
	testState.mu.Unlock()
	if dir != "" {
		ResetDBs()
		_ = os.RemoveAll(dir)
	}
}

// runTests runs every registered TEST in order and returns the exit status:
// 0 if all passed, 1 otherwise. Each result goes to stdout as PASS or FAIL;
// a failure's error goes to stderr in the classic form, ?KIND ERROR IN
// file:line — message, and never as a Go stack trace (Requirement 12.2).
func runTests(top *Ctx, stdout, stderr io.Writer) int {
	testState.mu.Lock()
	tests := append([]testCase(nil), testState.tests...)
	testState.mu.Unlock()

	snapshotTestDBs(top)
	defer cleanupTestDBs()

	failed := 0
	for i, t := range tests {
		if e := runTest(top, t, i); e != nil {
			failed++
			fmt.Fprintf(stdout, "FAIL  %s\n", t.name)
			fmt.Fprintln(stderr, e.Error())
			continue
		}
		fmt.Fprintf(stdout, "PASS  %s\n", t.name)
	}
	fmt.Fprintf(stdout, "\n%d %s: %d PASSED, %d FAILED\n", len(tests), plural(len(tests), "TEST", "TESTS"), len(tests)-failed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// runTest runs one TEST on its own Ctx and returns its error, if any. END
// inside a TEST ends it successfully.
func runTest(top *Ctx, t testCase, n int) (err *Error) {
	c := NewCtx(context.Background())
	c.app = top.App()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(endSignal); ok {
			return
		}
		e := FromPanic(r)
		if e.File == "" {
			e.File, e.Line = basLocation()
		}
		if e.File == "" {
			e.File, e.Line = t.file, t.line
		}
		err = e
	}()
	freshTestDBs(c, n)
	t.fn(c)
	return nil
}

// Expect implements EXPECT cond [, "message"] (Requirement 12.2).
func Expect(c *Ctx, cond bool, msg string) {
	if cond {
		return
	}
	if msg == "" {
		msg = "the condition was FALSE"
	}
	panic(Errorf(KindExpectFailed, "%s", msg))
}

// ExpectEq implements EXPECT got = want [, "message"]. It compares like =
// does, and without a message it says what both sides were.
func ExpectEq(c *Ctx, got, want Value, msg string) {
	if Eq(got, want) {
		return
	}
	if msg == "" {
		msg = "expected " + showValue(want) + ", got " + showValue(got)
	}
	panic(Errorf(KindExpectFailed, "%s", msg))
}

// showValue writes a value for an EXPECT message: strings in quotes, so
// "1" and 1 look different, and NIL by name.
func showValue(v Value) string {
	switch x := v.(type) {
	case nil:
		return "NIL"
	case string:
		return strconv.Quote(x)
	}
	return ToString(v)
}

// DoRequest implements REQUEST(method, path [, body, headers]) for generated
// code. It calls the routes of the program's own app (Requirement 12.3).
func DoRequest(c *Ctx, method, path string, rest ...Value) Value {
	var body Value
	var headers *Map
	if len(rest) > 0 {
		body = rest[0]
	}
	if len(rest) > 1 && rest[1] != nil {
		h, ok := rest[1].(*Map)
		if !ok {
			panic(Errorf(KindTypeMismatch, `REQUEST's headers must be a MAP, like {"x-api-key": "secret"}; got %s`, TypeName(rest[1])))
		}
		headers = h
	}
	if c == nil {
		c = NewCtx(context.Background())
	}
	return c.App().Request(c, method, path, body, headers)
}

// testRemoteAddr is the client address REQUEST's requests come from.
const testRemoteAddr = "192.0.2.1:1234"

// Request implements REQUEST(method, path [, body, headers]) for rip test. It
// calls the app's routes in-process, with no network, and returns {status,
// headers, body} (Requirement 12.3).
//
//   - A STRING body is sent as it is, so a test can send invalid JSON; any
//     other non-NIL body is sent as JSON. A body is sent as
//     application/json unless headers set Content-Type.
//   - The response's header names are lower case, like req.headers.
//   - A JSON response body is parsed; any other body is a STRING, and an
//     empty body is NIL.
func (a *App) Request(c *Ctx, method, path string, body Value, headers *Map) *Map {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		panic(Errorf(KindTypeMismatch, `REQUEST needs an HTTP method, like REQUEST("GET", "/hello")`))
	}
	if !strings.HasPrefix(path, "/") {
		panic(Errorf(KindTypeMismatch, `REQUEST needs a path that starts with /, like REQUEST("GET", "/hello"); got %q`, path))
	}
	var rd io.Reader = http.NoBody
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			data, err := marshalValue(body)
			if err != nil {
				panic(Errorf(KindTypeMismatch, "REQUEST cannot send this %s body as JSON: %v", TypeName(body), err))
			}
			rd = bytes.NewReader(data)
		}
	}
	ctx := context.Background()
	if c != nil && c.ctx != nil {
		ctx = c.ctx
	}
	r, err := http.NewRequestWithContext(ctx, method, "http://rip.test"+path, rd)
	if err != nil {
		panic(Errorf(KindTypeMismatch, "REQUEST cannot send %s %s: %v", method, path, err))
	}
	r.RemoteAddr = testRemoteAddr
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if headers != nil {
		for _, k := range headers.Keys() {
			v, _ := headers.Get(k)
			r.Header.Set(k, ToString(v))
		}
	}

	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, r)
	res := rec.Result()

	hdrs := NewMap()
	for _, k := range sortedHeaderKeys(res.Header) {
		hdrs.Set(strings.ToLower(k), strings.Join(res.Header[k], ", "))
	}
	return MapOf("status", int64(res.StatusCode), "headers", hdrs, "body", responseValue(res.Header.Get("Content-Type"), rec.Body.Bytes()))
}

func responseValue(contentType string, data []byte) Value {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if strings.Contains(strings.ToLower(contentType), "json") {
		if v, err := decodeJSON(data); err == nil {
			return v
		}
	}
	return string(data)
}
