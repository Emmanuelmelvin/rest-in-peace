package codegen_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func parseAndCheckConcurrency(t *testing.T, name, src string) (*ast.Program, string) {
	t.Helper()
	toks, mode, lds := lexer.Lex(name, []byte(src))
	if lds.HasErrors() {
		t.Fatalf("lex errors:\n%v", lds)
	}
	f, pds := parser.Parse(name, toks, mode)
	if pds.HasErrors() {
		t.Fatalf("parse errors:\n%v", pds)
	}
	prog := &ast.Program{Entry: name, Files: []*ast.File{f}}
	if cds := check.Check(prog); cds.HasErrors() {
		t.Fatalf("check errors:\n%v", cds)
	}
	files, gds := codegen.Generate(prog, codegen.Options{})
	if gds.HasErrors() {
		t.Fatalf("codegen errors:\n%v", gds)
	}
	for _, gf := range files {
		if gf.Name != "main.go" {
			return prog, string(gf.Src)
		}
	}
	t.Fatal("no generated module file found")
	return nil, ""
}

func TestConcurrencyCodegen(t *testing.T) {
	t.Run("SPAWN captures arguments before goroutine", func(t *testing.T) {
		src := `
SUB Work(id AS INTEGER, label AS STRING)
  PRINT id, label
END SUB

FOR i = 1 TO 3
  SPAWN Work(i, "job")
NEXT
`
		_, goSrc := parseAndCheckConcurrency(t, "spawn.bas", src)
		for _, want := range []string{
			"var rip_sa",
			"rt.Spawn(c, func(c *rt.Ctx)",
			"b_spawn_work(c, rip_sa",
		} {
			if !strings.Contains(goSrc, want) {
				t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
			}
		}
	})

	t.Run("EVERY emits rt.Every with handler closure", func(t *testing.T) {
		src := `
SUB Cleanup()
  PRINT "clean"
END SUB

EVERY 5000 CALL Cleanup
`
		_, goSrc := parseAndCheckConcurrency(t, "every.bas", src)
		for _, want := range []string{
			"rt.Every(c, int64(5000), func(c *rt.Ctx)",
			"b_every_cleanup(c)",
		} {
			if !strings.Contains(goSrc, want) {
				t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
			}
		}
	})

	t.Run("PARALLEL emits rt.Parallel with per-statement closures", func(t *testing.T) {
		src := `
FUNCTION Double(n AS INTEGER) AS INTEGER
  RETURN n * 2
END FUNCTION

FUNCTION Run() AS INTEGER
  PARALLEL
    a = Double(10)
    b = Double(20)
  END PARALLEL
  RETURN a + b
END FUNCTION
`
		_, goSrc := parseAndCheckConcurrency(t, "parallel.bas", src)
		for _, want := range []string{
			"rt.Parallel(c,",
			"b_parallel_a = b_parallel_double(c, int64(10))",
			"b_parallel_b = b_parallel_double(c, int64(20))",
		} {
			if !strings.Contains(goSrc, want) {
				t.Errorf("generated Go missing %q\n--- generated ---\n%s", want, goSrc)
			}
		}
	})
}

func TestConcurrencyDiagnostics(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name:    "SPAWN non-call",
			src:     "SPAWN 123\n",
			wantErr: "expected a function call after SPAWN",
		},
		{
			name:    "EVERY missing CALL",
			src:     "EVERY 1000 Cleanup\n",
			wantErr: "expected CALL",
		},
		{
			name:    "EVERY undefined handler",
			src:     "EVERY 1000 CALL Nope\n",
			wantErr: "no function named NOPE",
		},
		{
			name: "EVERY handler with required param",
			src: `
SUB NeedsArg(x)
  PRINT x
END SUB
EVERY 1000 CALL NeedsArg
`,
			wantErr: "cannot require parameters",
		},
		{
			name: "unclosed PARALLEL",
			src: `
PARALLEL
  x = 1
`,
			wantErr: "END PARALLEL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toks, mode, _ := lexer.Lex("bad.bas", []byte(tc.src))
			f, pds := parser.Parse("bad.bas", toks, mode)
			all := pds
			if !all.HasErrors() {
				all = append(all, check.Check(&ast.Program{Entry: "bad.bas", Files: []*ast.File{f}})...)
			}
			if !all.HasErrors() {
				t.Fatalf("expected error containing %q, got no errors", tc.wantErr)
			}
			got := fmt.Sprint(all)
			if !strings.Contains(got, tc.wantErr) {
				t.Errorf("diagnostics = %q, want substring %q", got, tc.wantErr)
			}
		})
	}
}

func TestConcurrencyRunPrograms(t *testing.T) {
	t.Run("SPAWN captures loop variable and runs in background", func(t *testing.T) {
		src := `
SHARED DIM total AS INTEGER = 0
SHARED DIM done AS INTEGER = 0

SUB Add(n AS INTEGER)
  total = total + n
  done = done + 1
END SUB

FOR i = 1 TO 4
  SPAWN Add(i)
NEXT

WHILE done < 4
WEND

PRINT total
`
		prog, _ := parseAndCheckConcurrency(t, "spawn_run.bas", src)
		res := runProgram(t, prog)
		if res.exit != 0 {
			t.Fatalf("exit = %d, stderr:\n%s", res.exit, res.stderr)
		}
		if res.stdout != "10\n" {
			t.Errorf("stdout = %q, want %q", res.stdout, "10\n")
		}
	})

	t.Run("EVERY ticks repeatedly", func(t *testing.T) {
		src := `
SHARED DIM ticks AS INTEGER = 0

SUB OnTick()
  ticks = ticks + 1
END SUB

EVERY 15 CALL OnTick

WHILE ticks < 3
WEND

PRINT "OK"
`
		prog, _ := parseAndCheckConcurrency(t, "every_run.bas", src)
		res := runProgram(t, prog)
		if res.exit != 0 {
			t.Fatalf("exit = %d, stderr:\n%s", res.exit, res.stderr)
		}
		if res.stdout != "OK\n" {
			t.Errorf("stdout = %q, want %q", res.stdout, "OK\n")
		}
	})

	t.Run("PARALLEL runs branches and joins", func(t *testing.T) {
		src := `
FUNCTION Calc(x AS INTEGER) AS INTEGER
  RETURN x * x
END FUNCTION

FUNCTION MainWork() AS INTEGER
  PARALLEL
    a = Calc(3)
    b = Calc(4)
    c = Calc(5)
  END PARALLEL
  RETURN a + b + c
END FUNCTION

PRINT MainWork()
`
		prog, _ := parseAndCheckConcurrency(t, "parallel_run.bas", src)
		res := runProgram(t, prog)
		if res.exit != 0 {
			t.Fatalf("exit = %d, stderr:\n%s", res.exit, res.stderr)
		}
		if res.stdout != "50\n" {
			t.Errorf("stdout = %q, want %q", res.stdout, "50\n")
		}
	})

	t.Run("PARALLEL panic caught by TRY/CATCH", func(t *testing.T) {
		src := `
SUB Fail()
  THROW ERROR("parallel boom")
END SUB

SUB Ok()
  x = 1 + 1
END SUB

TRY
  PARALLEL
    Ok()
    Fail()
  END PARALLEL
CATCH e
  PRINT e.message
END TRY
`
		prog, _ := parseAndCheckConcurrency(t, "parallel_try.bas", src)
		res := runProgram(t, prog)
		if res.exit != 0 {
			t.Fatalf("exit = %d, stderr:\n%s", res.exit, res.stderr)
		}
		if res.stdout != "parallel boom\n" {
			t.Errorf("stdout = %q, want %q", res.stdout, "parallel boom\n")
		}
	})
}
