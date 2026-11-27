package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
)

// testDatabaseURLEnv and defaultTestDatabaseURL match rt.TestDatabaseURLEnv
// and rt.DefaultTestDatabaseURL (the CLI does not import rt).
const (
	testDatabaseURLEnv     = "TEST_DATABASE_URL"
	defaultTestDatabaseURL = "sqlite://:memory:"
)

// runTest implements rip test [file.test.bas …] (Requirements 12 and 13).
//
// It compiles the project's entry file together with tests/*.test.bas (or
// the test files given), builds the program in test mode, and runs it with
// .env.test loaded. The program runs its top-level code without binding a
// port, sends every DBCONNECT to the test database, runs each TEST, prints
// PASS or FAIL for each and a summary, and exits non-zero if any failed.
func runTest(args []string) int {
	p, err := loadProject(".")
	if err != nil {
		return fail(err)
	}
	entry := filepath.Join(p.Root, p.Main)
	if _, err := os.Stat(entry); err != nil {
		return fail(fmt.Errorf("?FILE NOT FOUND ERROR — no %s here; run rip test inside a project", p.Main))
	}
	tests, err := findTests(p.Root, args)
	if err != nil {
		return fail(err)
	}
	if len(tests) == 0 {
		fmt.Println("NO TESTS FOUND — put TEST blocks in tests/*.test.bas")
		return 0
	}

	res, err := compiler.CompileTests(entry, tests, compiler.Options{})
	if err != nil {
		return fail(err)
	}
	for _, d := range res.Diagnostics {
		fmt.Fprintln(os.Stderr, d)
	}
	if res.Diagnostics.HasErrors() {
		return 1
	}

	migrations, err := build.MigrationsFS(p.Root)
	if err != nil {
		return fail(err)
	}
	files := append(res.Files, build.TestModeFile(migrations))
	dir := build.TestBuildDir(p.Root)
	if err := build.Prepare(dir, files); err != nil {
		return fail(err)
	}
	bin, err := build.Build(context.Background(), dir, build.Options{Output: build.TestBinary(p.Root, p.Name)})
	if err != nil {
		return fail(err)
	}
	return execProgram(bin, testEnv(filepath.Join(p.Root, ".env.test")))
}

// findTests returns the test files to run: the .test.bas files in args, or
// else every tests/*.test.bas in the project, in name order.
func findTests(root string, args []string) ([]string, error) {
	if len(args) > 0 {
		var out []string
		for _, a := range args {
			if !strings.HasSuffix(strings.ToLower(a), ".test.bas") {
				return nil, fmt.Errorf("?SYNTAX ERROR — rip test takes test files named *.test.bas, like rip test tests/main.test.bas; got %q", a)
			}
			abs, err := filepath.Abs(a)
			if err != nil {
				return nil, err
			}
			if _, err := os.Stat(abs); err != nil {
				return nil, fmt.Errorf("?FILE NOT FOUND ERROR — there is no test file %s", a)
			}
			out = append(out, abs)
		}
		return out, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "tests"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".test.bas") {
			out = append(out, filepath.Join(root, "tests", e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// testEnv is the extra environment of the test program: .env.test (whose
// keys the real environment overrides, as with .env), and DATABASE_URL set
// to the test database, so that DBCONNECT ENVREQUIRE("DATABASE_URL") works
// and never sees a real database URL. The runtime redirects every DBCONNECT
// to the test database anyway.
func testEnv(dotEnvTest string) []string {
	env := loadDotEnv(dotEnvTest)
	url := strings.TrimSpace(os.Getenv(testDatabaseURLEnv))
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == testDatabaseURLEnv && url == "" {
			url = strings.TrimSpace(v)
		}
	}
	if url == "" {
		url = defaultTestDatabaseURL
	}
	return append(env, "DATABASE_URL="+url)
}
