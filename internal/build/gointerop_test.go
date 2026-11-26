package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

func TestThirdPartyImports(t *testing.T) {
	files := []codegen.File{
		{Name: "main.go", Src: []byte(`package main
import "ripapp/rt"
func main() { _ = rt.NewMap() }
`)},
		{Name: "m_main.go", Src: []byte(`package main
import (
	"ripapp/rt"
	go_strings "strings"
	go_uuid "github.com/google/uuid"
	go_sub "example.com/foo/bar/sub"
)
var _ = rt.NewMap
var _ = go_strings.ToUpper
var _ = go_uuid.NewString
var _ = go_sub.X
`)},
	}
	got := thirdPartyImports(files)
	want := []string{"example.com/foo/bar/sub", "github.com/google/uuid"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("thirdPartyImports = %v, want %v", got, want)
	}
}

func TestExtraGoModRequiresUUID(t *testing.T) {
	files := []codegen.File{
		{Name: "m_main.go", Src: []byte(`package main
import (
	"ripapp/rt"
	go_uuid "github.com/google/uuid"
)
var _ = rt.NewMap
var _ = go_uuid.NewString
`)},
	}
	reqs := extraGoModRequires(files)
	found := false
	for _, r := range reqs {
		if r.Path == "github.com/google/uuid" && r.Version != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("extraGoModRequires did not resolve github.com/google/uuid; got %+v", reqs)
	}

	mod := string(goModFor(files))
	if !strings.Contains(mod, "github.com/google/uuid") {
		t.Errorf("goModFor missing github.com/google/uuid:\n%s", mod)
	}
}

func TestResolveModulesLongestPrefixAndPriority(t *testing.T) {
	local := []*debug.Module{
		{Path: "example.com/foo", Version: "v1.0.0"},
		{Path: "example.com/foo/v2", Version: "v2.3.4"},
	}
	fallback := []*debug.Module{
		{Path: "example.com/foo/v2", Version: "v2.0.0"},
		{Path: "example.com/other", Version: "v0.9.0"},
	}
	got := resolveModules(
		[]string{"example.com/foo/v2/subpkg", "example.com/other/x", "example.com/unknown"},
		[][]*debug.Module{local, fallback},
	)
	if len(got) != 2 {
		t.Fatalf("got %d modules, want 2: %+v", len(got), got)
	}
	if got[0].Path != "example.com/foo/v2" || got[0].Version != "v2.3.4" {
		t.Errorf("got[0] = %+v, want example.com/foo/v2 v2.3.4", got[0])
	}
	if got[1].Path != "example.com/other" || got[1].Version != "v0.9.0" {
		t.Errorf("got[1] = %+v, want example.com/other v0.9.0", got[1])
	}
}

func TestLocalGoModRequiresFrom(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.24\n\nrequire example.com/custom v1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqs := localGoModRequiresFrom(sub)
	if len(reqs) != 1 || reqs[0].Path != "example.com/custom" || reqs[0].Version != "v1.2.3" {
		t.Fatalf("localGoModRequiresFrom = %+v", reqs)
	}
}

func TestBuildWithThirdPartyGoInterop(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a Go program; skipped with -short")
	}
	if _, err := LookGo(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	files := []codegen.File{
		{Name: "main.go", Src: []byte(`package main
import (
	"ripapp/rt"
	go_uuid "github.com/google/uuid"
)
func main() {
	id := go_uuid.NewString()
	rt.Print(int64(len(id)))
}
`)},
	}
	if err := Prepare(dir, files); err != nil {
		t.Fatal(err)
	}
	bin, err := Build(context.Background(), dir, Options{Output: filepath.Join(dir, "bin", "uuidapp")})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "36" {
		t.Fatalf("got %q, want 36", got)
	}
}
