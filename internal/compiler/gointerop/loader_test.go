package gointerop_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/gointerop"
)

func TestLoadStdlibAndThirdParty(t *testing.T) {
	cases := []struct {
		path   string
		name   string
		fn     string
		typ    string
		konst  string
		variad bool
	}{
		{path: "strings", name: "strings", fn: "ToUpper", typ: "Reader"},
		{path: "net/url", name: "url", fn: "Parse", typ: "URL"},
		{path: "math", name: "math", fn: "Sqrt", konst: "Pi"},
		{path: "strconv", name: "strconv", fn: "Atoi"},
		{path: "fmt", name: "fmt", fn: "Sprintf", variad: true},
		{path: "github.com/google/uuid", name: "uuid", fn: "NewString", typ: "UUID"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			pkg, err := gointerop.Load(tc.path, "")
			if err != nil {
				t.Fatalf("Load(%q) failed: %v", tc.path, err)
			}
			if pkg.Name() != tc.name {
				t.Errorf("Name() = %q, want %q", pkg.Name(), tc.name)
			}
			if tc.fn != "" {
				fn, ok := pkg.LookupFunc(tc.fn)
				if !ok {
					t.Fatalf("LookupFunc(%q) not found", tc.fn)
				}
				sig, err := gointerop.AnalyzeFunc(fn)
				if err != nil {
					t.Fatalf("AnalyzeFunc(%s) failed: %v", tc.fn, err)
				}
				if sig.Variadic != tc.variad {
					t.Errorf("%s Variadic = %v, want %v", tc.fn, sig.Variadic, tc.variad)
				}
			}
			if tc.typ != "" {
				tn, ok := pkg.LookupType(tc.typ)
				if !ok {
					t.Fatalf("LookupType(%q) not found", tc.typ)
				}
				if err := gointerop.ValidateNewType(tc.name, tn); err != nil {
					t.Errorf("ValidateNewType(%s) failed: %v", tc.typ, err)
				}
			}
			if tc.konst != "" {
				if _, ok := pkg.LookupConst(tc.konst); !ok {
					t.Errorf("LookupConst(%q) not found", tc.konst)
				}
			}
		})
	}
}

func TestLoadCacheThreadSafe(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]*gointerop.Package, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = gointerop.Load("strings", "")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
		if results[i] == nil {
			t.Fatalf("goroutine %d returned nil", i)
		}
	}
	// Subsequent call must return the cached pointer.
	again, err := gointerop.Load("strings", "")
	if err != nil {
		t.Fatal(err)
	}
	if again != results[0] {
		t.Errorf("expected cached *Package pointer")
	}
}

func TestLoadErrors(t *testing.T) {
	for _, bad := range []string{"", "./local", "/abs", "example.com/definitely/does/not/exist/pkg"} {
		if _, err := gointerop.Load(bad, ""); err == nil {
			t.Errorf("Load(%q) succeeded, want error", bad)
		}
	}
}

func TestSuggestName(t *testing.T) {
	pkg, err := gointerop.Load("github.com/google/uuid", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := pkg.SuggestName("newstring"); got != "NewString" {
		t.Errorf("SuggestName(newstring) = %q, want NewString", got)
	}
	if got := pkg.SuggestName("NewStrng"); got != "NewString" {
		t.Errorf("SuggestName(NewStrng) = %q, want NewString", got)
	}
	if pkg.Lookup("newstring") != nil {
		t.Errorf("Lookup must be case-sensitive")
	}
}

func TestAnalyzeSigFeaturesAndRejections(t *testing.T) {
	// Check stdlib context.Context injection, trailing error, and unsupported signatures.
	httpPkg, err := gointerop.Load("net/http", "")
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := httpPkg.LookupFunc("NewRequestWithContext")
	if !ok {
		t.Fatal("net/http.NewRequestWithContext not found")
	}
	sig, err := gointerop.AnalyzeFunc(fn)
	if err != nil {
		t.Fatalf("AnalyzeFunc(NewRequestWithContext) failed: %v", err)
	}
	if !sig.Params[0].IsContext {
		t.Errorf("param 0 should be recognized as context.Context")
	}
	if sig.MinArgs != 3 || sig.MaxArgs != 3 {
		t.Errorf("MinArgs/MaxArgs = %d/%d, want 3/3 (ctx auto-injected)", sig.MinArgs, sig.MaxArgs)
	}
	if sig.ResultKind != gointerop.ResultValueErr {
		t.Errorf("ResultKind = %v, want ResultValueErr", sig.ResultKind)
	}

	// Rejections in stdlib:
	rejects := []struct {
		pkg  string
		fn   string
		want string
	}{
		{"strings", "Map", "function/callback"},
		{"time", "After", "channel"},
		{"slices", "Contains", "generic"},
		{"strings", "Cut", "returns 3 values"},
	}
	for _, tc := range rejects {
		t.Run(tc.pkg+"."+tc.fn, func(t *testing.T) {
			p, err := gointerop.Load(tc.pkg, "")
			if err != nil {
				t.Fatal(err)
			}
			f, ok := p.LookupFunc(tc.fn)
			if !ok {
				t.Fatalf("%s.%s not found", tc.pkg, tc.fn)
			}
			_, err = gointerop.AnalyzeFunc(f)
			if err == nil {
				t.Fatalf("AnalyzeFunc(%s.%s) succeeded, want error containing %q", tc.pkg, tc.fn, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCustomModuleSignatures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/testmod\n\ngo 1.24\n")
	write("lib.go", `package testmod

import "context"

type User struct {
	Name string `+"`json:\"full_name\"`"+`
}

type unexported struct{ X int }

func VoidFn() {}
func ErrOnly(ctx context.Context) error { return nil }
func TakeChan(ch chan int) {}
func TakeUnexported(u unexported) {}
func TwoValues() (int, string) { return 1, "a" }
func GenericFn[T any](v T) T { return v }
type Iface interface { Do() }
type GenericBox[T any] struct { V T }
`)

	pkg, err := gointerop.Load("example.com/testmod", dir)
	if err != nil {
		t.Fatalf("Load custom module failed: %v", err)
	}

	// Valid functions:
	for _, tc := range []struct {
		name     string
		wantKind gointerop.ResultKind
		minArgs  int
	}{
		{"VoidFn", gointerop.ResultVoid, 0},
		{"ErrOnly", gointerop.ResultError, 0},
	} {
		fn, ok := pkg.LookupFunc(tc.name)
		if !ok {
			t.Fatalf("%s not found", tc.name)
		}
		sig, err := gointerop.AnalyzeFunc(fn)
		if err != nil {
			t.Fatalf("AnalyzeFunc(%s): %v", tc.name, err)
		}
		if sig.ResultKind != tc.wantKind || sig.MinArgs != tc.minArgs {
			t.Errorf("%s: got kind=%v minArgs=%d, want %v/%d", tc.name, sig.ResultKind, sig.MinArgs, tc.wantKind, tc.minArgs)
		}
	}

	// Rejected functions:
	for _, name := range []string{"TakeChan", "TakeUnexported", "TwoValues", "GenericFn"} {
		fn, ok := pkg.LookupFunc(name)
		if !ok {
			t.Fatalf("%s not found", name)
		}
		if _, err := gointerop.AnalyzeFunc(fn); err == nil {
			t.Errorf("AnalyzeFunc(%s) should fail", name)
		}
	}

	// ValidateNewType:
	userT, _ := pkg.LookupType("User")
	if err := gointerop.ValidateNewType("testmod", userT); err != nil {
		t.Errorf("ValidateNewType(User) failed: %v", err)
	}
	for _, badType := range []string{"Iface", "GenericBox"} {
		tn, ok := pkg.LookupType(badType)
		if !ok {
			t.Fatalf("type %s not found", badType)
		}
		if err := gointerop.ValidateNewType("testmod", tn); err == nil {
			t.Errorf("ValidateNewType(%s) should fail", badType)
		}
	}
}
