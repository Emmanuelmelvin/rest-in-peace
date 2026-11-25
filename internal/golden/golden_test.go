package golden

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
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
	if d, ok := res.Diagnostics.NotImplemented(); ok {
		t.Skipf("compiler not built yet: %s", d)
	}
	base := strings.TrimSuffix(path, ".bas")

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
var expectedLine = regexp.MustCompile(`^\?[A-Z][A-Z ]* (ERROR|WARNING) IN ([^:]+):([0-9]+)(` + HintSep + `.+)?$`)

// TestGoldenLayout checks the golden files themselves, so mistakes show up
// before the compiler exists: every case has the right expected files, and
// every expected diagnostic names the case's own .bas file.
func TestGoldenLayout(t *testing.T) {
	cases := goldenCases(t)
	bases := map[string]bool{}
	for _, c := range cases {
		base := strings.TrimSuffix(c, ".bas")
		bases[base] = true
		_, hasErr := readOptional(t, base+".err")
		_, hasOut := readOptional(t, base+".out")
		_, hasStderr := readOptional(t, base+".stderr")
		switch {
		case hasErr && (hasOut || hasStderr):
			t.Errorf("%s: a .err case can't also have .out or .stderr", caseName(c))
		case !hasErr && !hasOut && !hasStderr:
			t.Errorf("%s: needs a .out, .stderr or .err file", caseName(c))
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
				} else if m[2] != filepath.Base(c) {
					t.Errorf("%s%s: %q names %s, want %s", caseName(c), ext, l, m[2], filepath.Base(c))
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
		case ".out", ".err", ".stderr":
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
