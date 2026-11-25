package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Runtime errors escape to rt.Main, which prints ?KIND ERROR IN file:line — hint
// on stderr, using the //line directives for the position, and exits 1.
func TestRuntimeErrors(t *testing.T) {
	cases := []struct {
		name   string
		prog   *ast.Program
		stdout string
		stderr string
	}{
		{
			name: "division by zero inside a function",
			prog: structured("errs.bas",
				pr(1, str("BEFORE")),
				let(2, id("z"), num(0)),
				function(3, "Boom", params(param("d", "", nil)), "",
					ret(4, bin(num(10), token.Backslash, id("d"))),
				),
				pr(6, call("Boom", id("z"))),
				pr(7, str("NEVER")),
			),
			stdout: "BEFORE\n",
			stderr: "?DIVISION BY ZERO ERROR IN errs.bas:4 — the right side of \\ is zero\n",
		},
		{
			name: "member of NIL at top level",
			prog: structured("nil.bas",
				let(1, id("x"), nilv()),
				pr(2, mem(id("x"), "name")),
			),
			stderr: "?NIL ACCESS ERROR IN nil.bas:2 — cannot read \"name\" of NIL — check the value exists before using it\n",
		},
		{
			name: "typed variable gets the wrong type",
			prog: structured("types.bas",
				let(1, id("s"), str("x")),
				dim(2, "n", "INTEGER", nil),
				let(3, id("n"), id("s")),
			),
			stderr: "?TYPE MISMATCH ERROR IN types.bas:3 — expected an INTEGER, got STRING x\n",
		},
		{
			name: "array index out of range",
			prog: structured("subscript.bas",
				let(1, id("a"), arr(num(1))),
				forS(2, "i", num(0), num(5), nil,
					pr(3, idx(id("a"), id("i"))),
				),
			),
			stdout: "1\n",
			stderr: "?BAD SUBSCRIPT ERROR IN subscript.bas:3 — index 1 is outside 0..0 — arrays start at 0\n",
		},
		{
			name: "runaway recursion",
			prog: structured("deep.bas",
				pr(1, call("Down", num(1))),
				function(3, "Down", params(param("n", "", nil)), "",
					ret(4, call("Down", bin(id("n"), token.Plus, num(1)))),
				),
			),
			stderr: "?STACK OVERFLOW ERROR IN deep.bas:3 — more than 10000 nested calls — is a function calling itself forever?\n",
		},
		{
			name: "classic RETURN without GOSUB",
			prog: classic("rwg.bas",
				ln(10, 1, pr(1, str("HI"))),
				ln(20, 2, ret(2, nil)),
			),
			stdout: "HI\n",
			stderr: "?RETURN WITHOUT GOSUB ERROR IN rwg.bas:2 — a RETURN ran with no GOSUB to return to\n",
		},
		{
			name: "classic error inside a GOSUB",
			prog: classic("csub.bas",
				ln(10, 1, gosub(1, 100)),
				ln(20, 2, end(2)),
				ln(100, 3, let(3, id("Y"), bin(num(1), token.Slash, id("ZERO")))),
				ln(110, 4, ret(4, nil)),
			),
			stderr: "?TYPE MISMATCH ERROR IN csub.bas:3 — cannot use / on INTEGER and NIL\n",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := runProgram(t, c.prog)
			if r.exit != 1 {
				t.Errorf("exit %d, want 1", r.exit)
			}
			if r.stdout != c.stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, c.stdout)
			}
			if r.stderr != c.stderr {
				t.Errorf("stderr:\n%q\nwant:\n%q", r.stderr, c.stderr)
			}
		})
	}
}

// Features from later phases, and programs codegen cannot compile, must fail
// with a diagnostic rather than crash or produce broken Go.
func TestDiagnostics(t *testing.T) {
	str1 := str("x")
	cases := []struct {
		name string
		prog *ast.Program
		kind diag.Kind
		want string // substring of the message
		line int
	}{
		{"ROUTE", structured("d.bas", &ast.RouteStmt{At: at(3), Method: "GET", Path: "/", Handler: "H"}), diag.NotImplemented, "task 2.1", 3},
		{"SERVE", structured("d.bas", &ast.ServeStmt{At: at(1)}), diag.NotImplemented, "SERVE is not built yet (task 2.1", 1},
		{"RESPOND", structured("d.bas", &ast.RespondStmt{At: at(1), Status: num(200), Value: str1}), diag.NotImplemented, "task 2.1", 1},
		{"GROUP", structured("d.bas", &ast.GroupStmt{At: at(1), Prefix: "/v1"}), diag.NotImplemented, "task 2.7", 1},
		{"USE", structured("d.bas", &ast.UseStmt{At: at(1), Fn: id("Auth")}), diag.NotImplemented, "task 2.3", 1},
		{"TRY", structured("d.bas", &ast.TryStmt{At: at(1)}), diag.NotImplemented, "task 2.2", 1},
		{"THROW", structured("d.bas", &ast.ThrowStmt{At: at(1), Value: str1}), diag.NotImplemented, "task 2.2", 1},
		{"DBCONNECT", structured("d.bas", &ast.DBConnectStmt{At: at(1), URL: str1}), diag.NotImplemented, "task 3.1", 1},
		{"TRANSACTION", structured("d.bas", &ast.TransactionStmt{At: at(1)}), diag.NotImplemented, "task 3.2", 1},
		{"LOG", structured("d.bas", &ast.LogStmt{At: at(1), Level: str1, Msg: str1}), diag.NotImplemented, "task 2.4", 1},
		{"TEST", structured("d.bas", &ast.TestDecl{At: at(1), Name: "t"}), diag.NotImplemented, "task 4.2", 1},
		{"EXPECT", structured("d.bas", &ast.ExpectStmt{At: at(1), Cond: boolean(true)}), diag.NotImplemented, "task 4.2", 1},
		{"SHARED DIM", structured("d.bas", &ast.DimStmt{At: at(2), Name: "X", Shared: true}), diag.NotImplemented, "task 4.3", 2},
		{"DECIMAL type", structured("d.bas", dim(1, "price", "DECIMAL", nil)), diag.NotImplemented, "task 2.5", 1},
		{"later built-in", structured("d.bas", pr(4, call("ENV", str("PORT")))), diag.NotImplemented, "ENV is not built yet (task 2.4", 4},
		{"ROUND", structured("d.bas", pr(1, call("ROUND", flt("1.5")))), diag.NotImplemented, "task 2.5", 1},
		{"method call", structured("d.bas", do(1, &ast.CallExpr{Fn: mem(id("uuid"), "NewString")})), diag.NotImplemented, "task 3.5", 1},
		{"classic ROUTE GOSUB", classic("d.bas", ln(10, 1, &ast.RouteStmt{At: at(1), Method: "GET", Path: "/", GosubLine: 100}), ln(100, 2, end(2))), diag.NotImplemented, "task 2.1", 1},
		{"GOTO in structured mode", structured("d.bas", pr(1, str("HELLO")), gotoS(2, 10)), diag.Syntax, "GOTO only works in classic mode; add line numbers, or use a loop or a FUNCTION", 2},
		{"GOSUB in structured mode", structured("d.bas", gosub(2, 10)), diag.Syntax, "GOSUB only works in classic mode", 2},
		{"undefined variable", structured("d.bas", pr(5, id("nope"))), diag.Undefined, "no variable named NOPE", 5},
		{"undefined function", structured("d.bas", pr(5, call("Nope"))), diag.Undefined, "no FUNCTION or SUB named NOPE", 5},
		{"SUB used as a value", structured("d.bas", pr(1, call("S")), sub(2, "S", nil)), diag.TypeMismatch, "S is a SUB", 1},
		{"missing argument", structured("d.bas", pr(1, call("F")), function(2, "F", params(param("a", "", nil)), "", ret(3, id("a")))), diag.Syntax, "F needs a value for A", 1},
		{"too many arguments", structured("d.bas", pr(1, call("LEN", str1, str1))), diag.Syntax, "LEN takes 1 argument, not 2", 1},
		{"EXIT outside a loop", structured("d.bas", exit(7, token.FOR)), diag.Syntax, "EXIT FOR is not inside a FOR loop", 7},
		{"RETURN at top level", structured("d.bas", ret(1, nil)), diag.Syntax, "RETURN only works inside a FUNCTION or SUB", 1},
		{"assigning a CONST", structured("d.bas", cnst(1, "K", num(1)), let(2, id("K"), num(2))), diag.Syntax, "K is a CONST", 2},
		{"GOTO to a missing line", classic("d.bas", ln(10, 1, gotoS(1, 99))), diag.Undefined, "there is no line 99", 1},
		{"FUNCTION in classic mode", classic("d.bas", ln(10, 1, function(1, "F", nil, ""))), diag.Syntax, "only work in structured mode", 1},
		{"IMPORT", &ast.Program{Entry: "d.bas", Files: []*ast.File{{Name: "d.bas", Imports: []*ast.Import{{At: at(1), Path: "./x.bas"}}}}}, diag.NotImplemented, "task 3.4", 1},
		{"IMPORT FROM GO", &ast.Program{Entry: "d.bas", Files: []*ast.File{{Name: "d.bas", Imports: []*ast.Import{{At: at(1), Path: "github.com/google/uuid", FromGo: true}}}}}, diag.NotImplemented, "task 3.5", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, ds := codegen.Generate(c.prog, codegen.Options{})
			if files != nil {
				t.Errorf("got %d files, want none when there are errors", len(files))
			}
			for _, d := range ds {
				if d.Kind == c.kind && strings.Contains(d.Message, c.want) {
					if d.Pos.Line != c.line || d.Pos.File != "d.bas" {
						t.Errorf("diagnostic at %s, want d.bas:%d", d.Pos, c.line)
					}
					return
				}
			}
			t.Fatalf("no %s diagnostic containing %q; got:\n%v", c.kind, c.want, ds)
		})
	}
}

// The generated Go follows design §4.3–4.5 and the style rules.
func TestGeneratedSourceShape(t *testing.T) {
	files, ds := codegen.Generate(goldenFunctions(), codegen.Options{})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	if len(files) != 2 || files[0].Name != "main.go" || files[1].Name != "m_functions.go" {
		t.Fatalf("files: %v", names(files))
	}
	main, mod := string(files[0].Src), string(files[1].Src)
	for _, f := range files {
		if !strings.HasPrefix(string(f.Src), codegen.Header+"\n") {
			t.Errorf("%s does not start with the generated-code header", f.Name)
		}
	}
	for _, want := range []string{"rt.Main(func(c *rt.Ctx) {", "rip_functions_run(c)"} {
		if !strings.Contains(main, want) {
			t.Errorf("main.go is missing %q:\n%s", want, main)
		}
	}
	for _, want := range []string{
		`import "ripapp/rt"`,
		"\n//line functions.bas:1\n",
		"\n//line functions.bas:5\n",
		"func b_functions_double(c *rt.Ctx, b_functions_n int64) int64 {",
		"func b_functions_greet(c *rt.Ctx, b_functions_name rt.Value, b_functions_greeting rt.Value) rt.Value {",
		"defer c.Enter()()",
		"return (b_functions_n * int64(2))", // native operator on typed values
		`rt.Concat(rt.Concat(b_functions_greeting, ", "), b_functions_name)`,
		`b_functions_greet(c, "Ada", "HELLO")`, // the default filled in at the call site
	} {
		if !strings.Contains(mod, want) {
			t.Errorf("m_functions.go is missing %q:\n%s", want, mod)
		}
	}
}

func TestGuardOption(t *testing.T) {
	p := structured("g.bas", while(1, boolean(true), exit(2, token.WHILE)))
	files, ds := codegen.Generate(p, codegen.Options{Guard: true})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	if !strings.Contains(string(files[1].Src), "c.Guard()") {
		t.Errorf("no c.Guard() in a guarded loop:\n%s", files[1].Src)
	}
	c := classic("h.bas", ln(10, 1, gotoS(1, 10)))
	files, ds = codegen.Generate(c, codegen.Options{Guard: true})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	if !strings.Contains(string(files[1].Src), "c.Guard()\n\tgoto L10") {
		t.Errorf("no c.Guard() before GOTO:\n%s", files[1].Src)
	}
}

func TestModuleFileNames(t *testing.T) {
	for file, want := range map[string]string{
		"main.bas":         "m_main.go",
		"billing.bas":      "m_billing.go",
		"windows.bas":      "m_windows_bas.go", // would otherwise only build on Windows
		"billing_test.bas": "m_billing_test_bas.go",
		"Hello World.bas":  "m_hello_x20_world.go",
	} {
		files, ds := codegen.Generate(structured(file, pr(1, num(1))), codegen.Options{})
		if ds.HasErrors() {
			t.Fatal(ds)
		}
		if got := files[1].Name; got != want {
			t.Errorf("%s → %s, want %s", file, got, want)
		}
	}
}

func names(files []codegen.File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}
