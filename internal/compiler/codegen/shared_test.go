package codegen

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func genSharedSrc(t *testing.T, name, src string) string {
	t.Helper()
	toks, mode, lds := lexer.Lex(name, []byte(src))
	if lds.HasErrors() {
		t.Fatalf("lex errors: %v", lds)
	}
	f, ds := parser.Parse(name, toks, mode)
	if ds.HasErrors() {
		t.Fatalf("parse errors: %v", ds)
	}
	files, gds := Generate(&ast.Program{Entry: name, Files: []*ast.File{f}}, Options{})
	if gds.HasErrors() {
		t.Fatalf("codegen errors: %v", gds)
	}
	for _, gf := range files {
		if gf.Name != "main.go" {
			return string(gf.Src)
		}
	}
	t.Fatal("no module file generated")
	return ""
}

func TestSharedDimCodegenCounter(t *testing.T) {
	src := `
SHARED DIM counter AS INTEGER = 0
ROUTE POST "/inc" CALL Inc
FUNCTION Inc(req)
  counter = counter + 1
  RETURN counter
END FUNCTION
`
	goSrc := genSharedSrc(t, "counter.bas", src)

	for _, want := range []string{
		"b_counter_counter rt.Shared[int64]",
		"b_counter_counter.Store(int64(0))",
		"b_counter_counter.Modify(func(rip_old",
		"return (rip_old",
		"return b_counter_counter.Load()",
	} {
		if !strings.Contains(goSrc, want) {
			t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
		}
	}
	// Handler must NOT shadow counter with a local variable.
	if strings.Contains(goSrc, "var b_counter_counter ") {
		t.Errorf("handler unexpectedly declared local b_counter_counter\n--- generated ---\n%s", goSrc)
	}
	// Modify callback must NOT call Load() on counter (would self-deadlock).
	idx := strings.Index(goSrc, ".Modify(func(")
	if idx < 0 {
		t.Fatal("missing .Modify")
	}
	end := strings.Index(goSrc[idx:], "})")
	if end > 0 && strings.Contains(goSrc[idx:idx+end], "b_counter_counter.Load()") {
		t.Errorf("Modify callback calls Load() on the same Shared variable (deadlock!)\n--- generated ---\n%s", goSrc)
	}
}

func TestSharedDimCodegenMapAndArray(t *testing.T) {
	src := `
SHARED DIM cache AS MAP
SHARED DIM items AS ARRAY
ROUTE POST "/put" CALL Put
FUNCTION Put(req)
  cache["k"] = "v"
  cache.last = "v"
  items[0] = cache["k"]
  RETURN cache.last
END FUNCTION
`
	goSrc := genSharedSrc(t, "collections.bas", src)

	for _, want := range []string{
		"b_collections_cache rt.Shared[rt.Value]",
		"b_collections_items rt.Shared[rt.Value]",
		"b_collections_cache.Store(rt.NewMap())",
		"b_collections_items.Store(rt.ArrayOf())",
		`b_collections_cache.SetIndex("k", "v")`,
		`b_collections_cache.SetMember("last", "v")`,
		`b_collections_items.SetIndex(int64(0), b_collections_cache.GetIndex("k"))`,
		`b_collections_cache.GetMember("last")`,
	} {
		if !strings.Contains(goSrc, want) {
			t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
		}
	}
}

func TestSharedDimCodegenNoDeadlockOnCallOrCrossVar(t *testing.T) {
	src := `
SHARED DIM a AS INTEGER = 1
SHARED DIM b AS INTEGER = 2

FUNCTION NextVal() AS INTEGER
  RETURN a + 1
END FUNCTION

SUB Update()
  a = 42
  a = NextVal()
  a = a + b
END SUB
`
	goSrc := genSharedSrc(t, "nodeadlock.bas", src)

	if strings.Contains(goSrc, ".Modify(") {
		t.Errorf("expected Store (not Modify) for plain assign, call RHS, and cross-shared RHS\n--- generated ---\n%s", goSrc)
	}
	for _, want := range []string{
		"b_nodeadlock_a.Store(int64(42))",
		"b_nodeadlock_a.Store(b_nodeadlock_nextval(c))",
		"b_nodeadlock_a.Store((b_nodeadlock_a.Load() + b_nodeadlock_b.Load()))",
	} {
		if !strings.Contains(goSrc, want) {
			t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
		}
	}
}

func TestSharedDimCodegenLocalShadowing(t *testing.T) {
	src := `
SHARED DIM counter AS INTEGER = 10

FUNCTION ByParam(counter AS INTEGER) AS INTEGER
  counter = counter + 1
  RETURN counter
END FUNCTION

FUNCTION ByDim() AS INTEGER
  DIM counter AS INTEGER = 5
  counter = counter + 1
  RETURN counter
END FUNCTION
`
	goSrc := genSharedSrc(t, "shadow.bas", src)

	// Neither ByParam nor ByDim should call Load/Store/Modify on the parameter/local.
	if strings.Contains(goSrc, ".Modify(") || strings.Contains(goSrc, ".Load()") {
		t.Errorf("local shadowing should not emit Shared Load/Modify calls\n--- generated ---\n%s", goSrc)
	}
}
