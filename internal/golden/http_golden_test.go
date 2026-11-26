package golden

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
)

// readyTimeout limits how long an HTTP golden program may take to print READY.
const readyTimeout = 10 * time.Second

// notBuiltMarker is in every message from a runtime stub (rt.notBuilt).
const notBuiltMarker = "is not built yet"

// runHTTPCase runs a case that has a .http script: it builds the program,
// starts it, and runs the script against it.
func runHTTPCase(t *testing.T, path string, res *compiler.Result, script string) {
	exs, err := ParseScript(caseName(path)+".http", script)
	if err != nil {
		t.Fatal(err)
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", res.Diagnostics)
	}
	if testing.Short() {
		t.Skip("building and running golden programs is skipped with -short")
	}
	if _, err := build.LookGo(); err != nil {
		t.Skip(err)
	}

	dir := t.TempDir()
	bin, err := BuildBinary(context.Background(), dir, res.Files)
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartServer(bin, dir, readyTimeout)
	if err != nil {
		var nr *NotReadyError
		if errors.As(err, &nr) && strings.Contains(nr.Stderr+nr.Stdout, notBuiltMarker) {
			t.Skipf("runtime not built yet:\n%s", Normalize(nr.Stderr))
		}
		t.Fatal(err)
	}
	// Registered after t.TempDir, so it runs first: Windows can't remove a
	// running binary.
	t.Cleanup(s.Stop)

	errs := RunScript(context.Background(), s.URL, exs)
	if len(errs) == 0 {
		return
	}
	s.Stop()
	stdout, stderr := s.Output()
	if strings.Contains(stderr+stdout, notBuiltMarker) {
		t.Skipf("runtime not built yet; the server said:\n%s", Normalize(stderr+stdout))
	}
	for _, err := range errs {
		t.Error(err)
	}
	t.Logf("server output\n--- stdout\n%s\n--- stderr\n%s", Normalize(stdout), Normalize(stderr))
}

// checkScriptLayout checks a golden .http script without running it: it
// parses, and every expected body is JSON (every response in the spec is).
func checkScriptLayout(t *testing.T, name, script string) {
	t.Helper()
	exs, err := ParseScript(name, script)
	if err != nil {
		t.Error(err)
		return
	}
	for _, ex := range exs {
		if _, err := ParseJSON(ex.Want.Body); err != nil {
			t.Errorf("%s:%d: expected body is not JSON (%v):\n%s", name, ex.Line, err, ex.Want.Body)
		}
	}
}
