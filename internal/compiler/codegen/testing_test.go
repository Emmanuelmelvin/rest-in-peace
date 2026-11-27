package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// TEST, EXPECT and REQUEST (task 4.2, Requirement 12).

func testDecl(line int, name string, body ...ast.Stmt) ast.Stmt {
	return &ast.TestDecl{At: at(line), Name: name, Body: body}
}

func expect(line int, cond ast.Expr, msg ...ast.Expr) ast.Stmt {
	s := &ast.ExpectStmt{At: at(line), Cond: cond}
	if len(msg) > 0 {
		s.Msg = msg[0]
	}
	return s
}

// TestTestPrograms builds and runs programs with TEST blocks: the program
// runs its tests after the top-level code instead of serving, reports each
// one, and exits non-zero if any failed.
func TestTestPrograms(t *testing.T) {
	r := id("r")
	p := structured("app.test.bas",
		&ast.RouteStmt{At: at(1), Method: "GET", Path: "/hello/:name", Handler: "Hello"},
		&ast.ServeStmt{At: at(2)},
		let(3, id("greeting"), str("HELLO")),
		function(4, "Hello", params(param("req", "", nil)), "",
			ret(5, mapl("message", bin(bin(id("greeting"), token.Amp, str(" ")), token.Amp, mem(mem(id("req"), "params"), "name"))))),
		testDecl(7, "PASSES",
			let(8, r, call("REQUEST", str("GET"), str("/hello/ghost"))),
			expect(9, bin(mem(r, "status"), token.Eq, num(200))),
			expect(10, bin(mem(mem(r, "body"), "message"), token.Eq, str("HELLO ghost")), str("wrong greeting")),
			dim(11, "n", "INTEGER", num(2)),
			expect(12, bin(id("n"), token.Gt, num(1))),
			pr(13, str("IN TEST "), id("n")),
		),
		testDecl(15, "COMPARES",
			let(16, r, call("REQUEST", str("GET"), str("/nowhere"))),
			expect(17, bin(mem(r, "status"), token.Eq, num(200))),
			pr(18, str("NEVER")),
		),
		testDecl(20, "MESSAGE",
			expect(21, boolean(false), bin(str("n is "), token.Amp, num(3))),
		),
		testDecl(23, "ERRORS",
			let(24, id("x"), nilv()),
			pr(25, mem(id("x"), "y")),
		),
		testDecl(27, "LOCALS ARE THE TEST'S OWN",
			let(28, id("greeting"), str("BYE")),
			expect(29, bin(call("REQUEST", str("GET"), str("/hello/x")), token.NotEq, nilv())),
			expect(30, bin(mem(mem(call("REQUEST", str("GET"), str("/hello/x")), "body"), "message"), token.Eq, str("HELLO x"))),
		),
		pr(32, str("TOP LEVEL RUNS FIRST")),
	)
	res := runProgram(t, p)
	if res.exit != 1 {
		t.Errorf("exit status = %d, want 1", res.exit)
	}
	wantOut := "TOP LEVEL RUNS FIRST\nIN TEST 2\nPASS  PASSES\nFAIL  COMPARES\nFAIL  MESSAGE\nFAIL  ERRORS\nPASS  LOCALS ARE THE TEST'S OWN\n\n5 TESTS: 2 PASSED, 3 FAILED\n"
	if res.stdout != wantOut {
		t.Errorf("stdout:\n%s\nwant:\n%s", res.stdout, wantOut)
	}
	for _, want := range []string{
		"?EXPECT FAILED ERROR IN app.test.bas:17 — expected 200, got 404\n",
		"?EXPECT FAILED ERROR IN app.test.bas:21 — n is 3\n",
		"?NIL ACCESS ERROR IN app.test.bas:25 — ",
	} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, res.stderr)
		}
	}
	if strings.Contains(res.stderr, "goroutine") || strings.Contains(res.stderr, ".go:") {
		t.Errorf("stderr has a Go stack trace:\n%s", res.stderr)
	}
}

// EXPECT works outside TEST too, such as in a helper SUB, where a failure
// reports the EXPECT's own line.
func TestExpectInHelper(t *testing.T) {
	p := structured("helper.test.bas",
		sub(1, "MustBe", params(param("a", "", nil), param("b", "", nil)), expect(2, bin(id("a"), token.Eq, id("b")))),
		testDecl(4, "USES A HELPER", do(5, call("MustBe", num(1), num(1))), do(6, call("MustBe", str("a"), str("b")))),
	)
	res := runProgram(t, p)
	if res.exit != 1 || !strings.Contains(res.stderr, `?EXPECT FAILED ERROR IN helper.test.bas:2 — expected "b", got "a"`) {
		t.Errorf("exit %d, stderr:\n%s", res.exit, res.stderr)
	}
}

func TestTestDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		prog *ast.Program
		kind diag.Kind
		want string
		line int
	}{
		{"TEST outside a test file", structured("d.bas", testDecl(3, "t")), diag.Syntax, "TEST blocks go in test files named *.test.bas", 3},
		{"TEST in classic mode", classic("d.test.bas", ln(10, 1, testDecl(1, "t"))), diag.Syntax, "TEST only works in structured mode", 1},
		{"TEST in a block", structured("d.test.bas", iff(1, boolean(true), stmts(testDecl(2, "t")))), diag.Syntax, "TEST goes at the top level", 2},
		{"TEST in a FUNCTION", structured("d.test.bas", function(1, "F", nil, "", testDecl(2, "t"))), diag.Syntax, "TEST goes at the top level", 2},
		{"REQUEST with one argument", structured("d.test.bas", testDecl(1, "t", pr(2, call("REQUEST", str("GET"))))), diag.Syntax, "REQUEST takes 2 or 4 arguments, not 1", 2},
		{"REQUEST with five arguments", structured("d.test.bas", pr(4, call("REQUEST", str("GET"), str("/"), nilv(), nilv(), nilv()))), diag.Syntax, "REQUEST takes 2 or 4 arguments, not 5", 4},
		{"EXPECT an undefined variable", structured("d.test.bas", expect(5, bin(id("nope"), token.Eq, num(1)))), diag.Undefined, "no variable named NOPE", 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, ds := codegen.Generate(c.prog, codegen.Options{})
			if files != nil {
				t.Errorf("got %d files, want none when there are errors", len(files))
			}
			for _, d := range ds {
				if d.Kind == c.kind && strings.Contains(d.Message, c.want) {
					if d.Pos.Line != c.line {
						t.Errorf("diagnostic at %s, want line %d", d.Pos, c.line)
					}
					return
				}
			}
			t.Errorf("no %s diagnostic containing %q; got %v", c.kind, c.want, ds)
		})
	}
}

// TestTestCodegen checks the shape of the generated Go: a TEST registers a
// closure with its name and location, and EXPECT a = b uses rt.ExpectEq.
func TestTestCodegen(t *testing.T) {
	p := structured("tests/x.test.bas",
		testDecl(2, `SAYS "HI"`,
			let(3, id("r"), call("REQUEST", str("POST"), str("/a"), mapl("k", num(1)), mapl("x-api-key", str("k")))),
			expect(4, bin(mem(id("r"), "status"), token.Eq, num(201)), str("CREATED")),
			expect(5, id("r")),
		),
	)
	files, ds := codegen.Generate(p, codegen.Options{})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	src := dump(files)
	for _, want := range []string{
		`rt.Test(c, "SAYS \"HI\"", "tests/x.test.bas", 2, func(c *rt.Ctx) {`,
		`rt.DoRequest(c, "POST", "/a", rt.MapOf("k", int64(1)), rt.MapOf("x-api-key", "k"))`,
		`rt.ExpectEq(c, rt.Get(b_x_x2e_test_r, "status"), int64(201), "CREATED")`,
		`rt.Expect(c, rt.Truthy(b_x_x2e_test_r), "")`,
		"//line tests/x.test.bas:4",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source is missing %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "var (\n\tb_x_x2e_test_r") {
		t.Errorf("a TEST's variable became a module variable:\n%s", src)
	}
}
