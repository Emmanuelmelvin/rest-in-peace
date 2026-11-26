package golden

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/rt"
)

const root = "../../testdata/golden"

// runTimeout limits how long one golden program may run (not build).
const runTimeout = 10 * time.Second

func TestGolden(t *testing.T) {
	for _, c := range goldenCases(t) {
		t.Run(caseName(c), func(t *testing.T) {
			t.Parallel()
			runCase(t, c)
		})
	}
}

func runCase(t *testing.T, path string) {
	res, err := compiler.Compile(path, compiler.Options{})
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(path, ".bas")
	script, isHTTP := readOptional(t, base+".http")
	if isHTTP {
		// HTTP goldens are written ahead of the server, so make sure that the
		// only thing wrong with them is what isn't built yet.
		for _, d := range res.Diagnostics {
			if d.Severity == diag.Error && d.Kind != diag.NotImplemented {
				t.Errorf("compile error in an HTTP golden: %s", d)
			}
		}
	}
	if d, ok := res.Diagnostics.NotImplemented(); ok {
		t.Skipf("compiler not built yet: %s", d)
	}

	if isHTTP {
		runHTTPCase(t, path, res, script)
		return
	}

	if want, ok := readOptional(t, base+".err"); ok {
		var got []string
		for _, d := range res.Diagnostics {
			got = append(got, d.String())
		}
		if err := MatchLines(strings.Join(got, "\n"), want); err != nil {
			t.Errorf("diagnostics: %v", err)
		}
		return
	}

	wantOut, hasOut := readOptional(t, base+".out")
	wantErr, hasErr := readOptional(t, base+".stderr")
	if !hasOut && !hasErr {
		t.Fatalf("%s needs a .out, .stderr or .err file next to it", path)
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

	o, err := BuildAndRun(context.Background(), t.TempDir(), res.Files, runTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if hasErr {
		if o.ExitCode == 0 {
			t.Errorf("exited with status 0; a case with a .stderr file must fail")
		}
		if err := MatchLines(o.Stderr, wantErr); err != nil {
			t.Errorf("stderr: %v", err)
		}
	} else if o.ExitCode != 0 {
		t.Errorf("exited with status %d; stderr:\n%s", o.ExitCode, o.Stderr)
	}
	if got, want := Normalize(o.Stdout), Normalize(wantOut); got != want {
		t.Errorf("stdout differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}

// expectedLine is the form of every line in .err and .stderr files.
var expectedLine = regexp.MustCompile(`^\?([A-Z][A-Z ]*) (ERROR|WARNING) IN ([^:]+):([0-9]+)(` + HintSep + `.+)?$`)

// validCompileKinds lists every diagnostic kind in internal/compiler/diag and
// whether it is an ERROR or WARNING.
var validCompileKinds = map[string]string{
	string(diag.Syntax):        "ERROR",
	string(diag.TypeMismatch):  "ERROR",
	string(diag.Undefined):     "ERROR",
	string(diag.InjectionRisk): "WARNING",
	string(diag.SharedState):   "WARNING",
}

// validRuntimeKinds lists every runtime error kind in rt/errors.go.
var validRuntimeKinds = map[string]bool{
	string(rt.KindTypeMismatch):       true,
	string(rt.KindNilAccess):          true,
	string(rt.KindBadSubscript):       true,
	string(rt.KindDivisionByZero):     true,
	string(rt.KindStackOverflow):      true,
	string(rt.KindReturnWithoutGosub): true,
	string(rt.KindUndefined):          true,
	string(rt.KindUser):               true,
	string(rt.KindHTTP):               true,
	string(rt.KindGo):                 true,
	string(rt.KindEternalLoop):        true,
	string(rt.KindExpectFailed):       true,
	string(rt.KindInternal):           true,
}

// TestGoldenLayout checks the golden files themselves, so mistakes show up
// before the compiler exists: every case has the right expected files, and
// every expected diagnostic names the case's own .bas file, a valid kind, and
// an in-range line number.
func TestGoldenLayout(t *testing.T) {
	cases := goldenCases(t)
	bases := map[string]bool{}
	for _, c := range cases {
		base := strings.TrimSuffix(c, ".bas")
		bases[base] = true
		srcBytes, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		srcLines := splitLines(string(srcBytes))
		if len(srcLines) == 0 {
			t.Errorf("%s: .bas file is empty", caseName(c))
		}
		_, hasErr := readOptional(t, base+".err")
		_, hasOut := readOptional(t, base+".out")
		_, hasStderr := readOptional(t, base+".stderr")
		script, hasHTTP := readOptional(t, base+".http")
		switch {
		case hasHTTP && (hasErr || hasOut || hasStderr):
			t.Errorf("%s: an .http case can't also have .err, .out or .stderr", caseName(c))
		case hasErr && (hasOut || hasStderr):
			t.Errorf("%s: a .err case can't also have .out or .stderr", caseName(c))
		case !hasErr && !hasOut && !hasStderr && !hasHTTP:
			t.Errorf("%s: needs a .out, .stderr, .err or .http file", caseName(c))
		}
		if hasHTTP {
			checkScriptLayout(t, caseName(c)+".http", script)
		}
		for _, ext := range []string{".err", ".stderr"} {
			want, ok := readOptional(t, base+ext)
			if !ok {
				continue
			}
			lines := splitLines(want)
			if len(lines) == 0 {
				t.Errorf("%s%s is empty", caseName(c), ext)
			}
			for _, l := range lines {
				m := expectedLine.FindStringSubmatch(l)
				if m == nil {
					t.Errorf("%s%s: %q is not ?KIND ERROR IN file:line [— hint]", caseName(c), ext, l)
					continue
				}
				kind, severity, file, lineStr := m[1], m[2], m[3], m[4]
				if file != filepath.Base(c) {
					t.Errorf("%s%s: %q names %s, want %s", caseName(c), ext, l, file, filepath.Base(c))
				}
				if ext == ".err" {
					if wantSev, ok := validCompileKinds[kind]; !ok {
						t.Errorf("%s%s: unknown compile diagnostic kind %q", caseName(c), ext, kind)
					} else if severity != wantSev {
						t.Errorf("%s%s: kind %q should be %s, not %s", caseName(c), ext, kind, wantSev, severity)
					}
				} else {
					if severity != "ERROR" || !validRuntimeKinds[kind] {
						t.Errorf("%s%s: unknown runtime error %q %s", caseName(c), ext, kind, severity)
					}
				}
				line, err := strconv.Atoi(lineStr)
				if err != nil || line < 1 || line > len(srcLines)+1 {
					t.Errorf("%s%s: line %s is out of range for %d-line .bas file", caseName(c), ext, lineStr, len(srcLines))
				}
			}
		}
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(p)
		switch ext {
		case ".bas":
		case ".out", ".err", ".stderr", ".http":
			if !bases[strings.TrimSuffix(p, ext)] {
				t.Errorf("%s has no .bas file next to it", p)
			}
		default:
			t.Errorf("%s: unexpected file in testdata/golden", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func goldenCases(t *testing.T) []string {
	t.Helper()
	var cases []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".bas") {
			cases = append(cases, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no golden cases found in " + root)
	}
	return cases
}

func caseName(path string) string {
	rel, _ := filepath.Rel(root, path)
	return strings.TrimSuffix(filepath.ToSlash(rel), ".bas")
}

// readOptional reads a file that may not exist.
func readOptional(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}
