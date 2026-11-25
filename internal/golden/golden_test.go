// Package golden runs every program in testdata/golden.
//
// Each case is <name>.bas, plus either:
//   - <name>.out: the expected stdout when the program runs, or
//   - <name>.err: the expected compiler diagnostics, one per line.
package golden

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
)

const root = "../../testdata/golden"

func TestGolden(t *testing.T) {
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
	for _, c := range cases {
		rel, _ := filepath.Rel(root, c)
		t.Run(strings.TrimSuffix(filepath.ToSlash(rel), ".bas"), func(t *testing.T) { runCase(t, c) })
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

	if want, err := os.ReadFile(base + ".err"); err == nil {
		var got []string
		for _, d := range res.Diagnostics {
			got = append(got, d.String())
		}
		if g, w := strings.Join(got, "\n"), normalize(want); g != w {
			t.Errorf("diagnostics differ\n--- got\n%s\n--- want\n%s", g, w)
		}
		return
	}

	want, err := os.ReadFile(base + ".out")
	if err != nil {
		t.Fatalf("%s needs a .out or .err file next to it", path)
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", res.Diagnostics)
	}
	_ = want
	// TODO(task 1.9): build with internal/build, run the binary, and compare its
	// stdout with normalize(want).
	t.Skip("running golden programs lands with task 1.9")
}

func normalize(b []byte) string {
	return strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
}
