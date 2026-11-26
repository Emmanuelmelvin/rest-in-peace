package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// TRY … CATCH … FINALLY and THROW (task 2.2, Requirement 6). They work in
// top-level code, so none of these need HTTP. RESPOND passing through CATCH
// and FINALLY is tested in rt/errors_test.go, because RESPOND compiles in
// the HTTP track.

func tryS(line int, body []ast.Stmt, catchVar string, catch []ast.Stmt, finally ...ast.Stmt) ast.Stmt {
	return &ast.TryStmt{At: at(line), Body: body, CatchVar: strings.ToUpper(catchVar), Catch: catch, Finally: finally}
}
func throw(line int, v ast.Expr) ast.Stmt { return &ast.ThrowStmt{At: at(line), Value: v} }

// whereAt prints e.kind@e.line from a CATCH at line.
func whereAt(line int) ast.Stmt {
	return pr(line, mem(id("e"), "kind"), str("@"), mem(id("e"), "line"))
}

func TestTryPrograms(t *testing.T) {
	e := id("e")
	cases := []outCase{
		{name: "THROW ERROR is caught", prog: structured("t.bas",
			tryS(1, stmts(
				pr(2, str("IN")),
				throw(3, call("ERROR", str("boom"))),
				pr(4, str("NEVER")),
			), "e", stmts(
				pr(6, mem(e, "message"), str(" "), mem(e, "kind"), str(" "), mem(e, "file"), str(":"), mem(e, "line")),
				pr(7, bin(mem(e, "status"), token.Eq, nilv())),
			), pr(9, str("FINALLY"))),
			pr(11, str("AFTER")),
		), stdout: "IN\nboom USER t.bas:3\nTRUE\nFINALLY\nAFTER\n"},

		{name: "runtime errors and Go panics are caught with their kind and line", prog: structured("kinds.bas",
			let(1, id("z"), num(0)),
			dim(2, "n", "INTEGER", num(0)),
			let(3, id("nothing"), nilv()),
			tryS(4, stmts(pr(5, bin(num(1), token.Slash, id("z")))), "e", stmts(whereAt(6))),
			tryS(7, stmts(pr(8, bin(num(7), token.Backslash, id("n")))), "e", stmts(whereAt(9))),
			tryS(10, stmts(pr(11, mem(id("nothing"), "x"))), "e", stmts(whereAt(12))),
			tryS(13, stmts(pr(14, idx(arr(num(1)), num(5)))), "e", stmts(whereAt(15))),
			tryS(16, stmts(let(17, id("n"), str("x"))), "e", stmts(whereAt(18))),
			tryS(19, stmts(pr(20, call("Inner"))), "e", stmts(whereAt(21))),
			function(30, "Inner", nil, "", ret(31, mem(id("nothing"), "deep"))),
		), stdout: "DIVISION BY ZERO@5\nDIVISION BY ZERO@8\nNIL ACCESS@11\nBAD SUBSCRIPT@14\nTYPE MISMATCH@17\nNIL ACCESS@31\n"},

		{name: "a bad THROW or HTTPERROR is itself a catchable error", prog: structured("bad.bas",
			let(1, id("s"), str("oops")),
			tryS(2, stmts(throw(3, id("s"))), "e", stmts(pr(5, mem(e, "kind"), str(": "), mem(e, "message")))),
			tryS(6, stmts(throw(7, call("HTTPERROR", num(42), str("x")))), "e", stmts(pr(9, mem(e, "kind"), str(": "), mem(e, "message")))),
		), stdout: "TYPE MISMATCH: THROW needs an error, like THROW ERROR(\"message\") or THROW HTTPERROR(404, \"NOT FOUND\"), got STRING\n" +
			"TYPE MISMATCH: HTTPERROR needs a status code from 100 to 599, like HTTPERROR(404, \"NOT FOUND\"), got INTEGER 42\n"},

		{name: "HTTPERROR exposes status and details", prog: structured("h.bas",
			tryS(1, stmts(throw(2, call("HTTPERROR", num(404), str("INVOICE IS DEAD"), mapl("id", num(7))))), "e", stmts(
				pr(4, mem(e, "status"), str(" "), mem(e, "message"), str(" "), mem(mem(e, "details"), "id"), str(" "), mem(e, "kind")),
			)),
			tryS(5, stmts(throw(6, call("HTTPERROR", num(400), str("BAD")))), "e", stmts(
				pr(8, bin(mem(e, "details"), token.Eq, nilv())),
			)),
		), stdout: "404 INVOICE IS DEAD 7 HTTP\nTRUE\n"},

		{name: "RETURN inside TRY and CATCH", prog: structured("ret.bas",
			pr(1, call("Div", num(10), num(4))),
			pr(2, call("Div", num(1), num(0))),
			pr(3, call("Half", num(8))),
			pr(4, call("Half", neg(num(8)))),
			do(5, call("Early", boolean(true))),
			do(6, call("Early", boolean(false))),
			pr(7, call("FirstBig", arr(num(20), num(5), num(1)))),
			pr(8, call("FirstBig", arr(num(20), num(0)))),
			function(20, "Div", params(param("a", "", nil), param("b", "", nil)), "",
				tryS(21, stmts(ret(22, bin(id("a"), token.Slash, id("b")))), "e", stmts(
					ret(24, bin(str("NO: "), token.Amp, mem(e, "kind"))),
				), pr(26, str("FINALLY "), id("a"))),
				ret(28, str("UNREACHED")),
			),
			function(30, "Half", params(param("n", "INTEGER", nil)), "INTEGER",
				tryS(31, stmts(
					iff(32, bin(id("n"), token.Lt, num(0)), stmts(throw(32, call("ERROR", str("negative"))))),
					ret(33, bin(id("n"), token.Backslash, num(2))),
				), "e", stmts(ret(35, neg(num(1))))),
			),
			sub(40, "Early", params(param("stop", "BOOLEAN", nil)),
				tryS(41, stmts(
					iff(42, id("stop"), stmts(ret(42, nil))),
					pr(43, str("EARLY BODY")),
				), "e", nil, pr(46, str("EARLY FINALLY"))),
				pr(48, str("EARLY AFTER")),
			),
			// RETURN from inside a loop inside a TRY.
			function(50, "FirstBig", params(param("xs", "", nil)), "",
				each(51, "x", id("xs"),
					tryS(52, stmts(
						iff(53, bin(bin(num(10), token.Slash, id("x")), token.Gt, num(1)), stmts(ret(53, id("x")))),
					), "e", stmts(ret(55, str("BAD")))),
				),
				ret(58, str("NONE")),
			),
		), stdout: "FINALLY 10\n2.5\nFINALLY 1\nNO: DIVISION BY ZERO\n4\n-1\nEARLY FINALLY\nEARLY BODY\nEARLY FINALLY\nEARLY AFTER\n5\nBAD\n"},

		{name: "EXIT FOR, WHILE and DO inside TRY and CATCH", prog: structured("exit.bas",
			forS(1, "i", num(1), num(5), nil,
				tryS(2, stmts(
					iff(3, bin(id("i"), token.Eq, num(4)), stmts(exit(3, token.FOR))),
					iff(4, bin(id("i"), token.Eq, num(2)), stmts(throw(4, call("ERROR", str("two"))))),
					pr(5, id("i")),
				), "e", stmts(pr(7, str("CAUGHT "), mem(e, "message")))),
			),
			pr(10, str("I="), id("i")),
			let(11, id("n"), num(0)),
			while(12, boolean(true),
				let(13, id("n"), bin(id("n"), token.Plus, num(1))),
				tryS(14, stmts(
					iff(15, bin(id("n"), token.Eq, num(3)), stmts(throw(15, call("ERROR", str("stop"))))),
				), "e", stmts(exit(17, token.WHILE)), pr(19, str("F"), id("n"))),
			),
			pr(22, str("N="), id("n")),
			doLoop(23, nil,
				tryS(24, stmts(
					forS(25, "j", num(1), num(10), nil,
						iff(26, bin(id("j"), token.Eq, num(2)), stmts(exit(26, token.FOR))),
					),
					pr(28, str("J="), id("j")),
					// Nested TRY: the EXIT DO passes through both closures.
					tryS(29, stmts(exit(30, token.DO)), "e", nil),
					pr(32, str("NEVER")),
				), "e", nil),
			),
			pr(35, str("OUT")),
		), stdout: "1\nCAUGHT two\n3\nI=4\nF1\nF2\nF3\nN=3\nJ=2\nOUT\n"},

		{name: "nested TRY, rethrow and FINALLY order", prog: structured("nest.bas",
			tryS(1, stmts(
				tryS(2, stmts(throw(3, call("HTTPERROR", num(404), str("GONE"), mapl("id", num(7))))), "e", stmts(
					pr(5, str("INNER "), mem(e, "status")),
					throw(6, e),
				), pr(8, str("INNER FINALLY"))),
			), "outer", stmts(
				pr(11, str("OUTER "), mem(id("outer"), "message"), str(" "), mem(id("outer"), "line"), str(" "), mem(mem(id("outer"), "details"), "id")),
			)),
			// A new error inside CATCH replaces the first one.
			tryS(13, stmts(
				tryS(14, stmts(throw(15, call("ERROR", str("first")))), "e", stmts(
					pr(17, bin(num(1), token.Slash, id("zero"))),
				)),
			), "e", stmts(whereAt(20))),
			let(21, id("zero"), num(0)),
			// A changed e is rethrown with its new message and its old line.
			tryS(22, stmts(
				tryS(23, stmts(throw(24, call("ERROR", str("disk")))), "e", stmts(
					let(26, mem(e, "message"), bin(str("saving: "), token.Amp, mem(e, "message"))),
					throw(27, e),
				)),
			), "e", stmts(pr(30, mem(e, "message"), str("@"), mem(e, "line")))),
		), stdout: "INNER 404\nINNER FINALLY\nOUTER GONE 3 7\nTYPE MISMATCH@17\nsaving: disk@24\n"},

		{name: "the CATCH variable is local inside a FUNCTION", prog: structured("local.bas",
			let(1, e, str("GLOBAL")),
			pr(2, call("F")),
			pr(3, e),
			function(4, "F", nil, "",
				tryS(5, stmts(throw(6, call("ERROR", str("x")))), "e", stmts(ret(8, mem(e, "message")))),
			),
		), stdout: "x\nGLOBAL\n"},

		{name: "the call depth is restored after a caught STACK OVERFLOW", prog: structured("deep.bas",
			tryS(1, stmts(pr(2, call("Down", num(1)))), "e", stmts(pr(4, mem(e, "kind")))),
			tryS(5, stmts(pr(6, call("Down", num(1)))), "e", stmts(pr(8, mem(e, "kind")))),
			function(10, "Down", params(param("n", "", nil)), "", ret(11, call("Down", bin(id("n"), token.Plus, num(1))))),
		), stdout: "STACK OVERFLOW\nSTACK OVERFLOW\n"},

		{name: "END inside TRY skips CATCH and runs FINALLY", prog: structured("end.bas",
			tryS(1, stmts(pr(2, str("BEFORE END")), end(3)), "e", stmts(pr(5, str("NEVER"))), pr(7, str("FINALLY"))),
			pr(9, str("NEVER")),
		), stdout: "BEFORE END\nFINALLY\n"},

		{name: "FINALLY can hold a TRY and loops of its own", prog: structured("fin.bas",
			do(1, call("S")),
			sub(2, "S", nil,
				tryS(3, stmts(pr(4, str("BODY"))), "e", nil,
					forS(6, "k", num(1), num(3), nil,
						iff(7, bin(id("k"), token.Eq, num(2)), stmts(exit(7, token.FOR))),
						tryS(8, stmts(throw(9, call("ERROR", str("in finally")))), "e", stmts(pr(11, mem(e, "message"), id("k")))),
					),
				),
			),
		), stdout: "BODY\nin finally1\n"},
	}
	// Each program is a Go build, which is slow, so the cases run as three
	// programs. Their names don't clash; END has to come last.
	byName := map[string]outCase{}
	for _, c := range cases {
		byName[c.name] = c
	}
	merged := []outCase{
		merge("errors", "t.bas", byName,
			"THROW ERROR is caught",
			"runtime errors and Go panics are caught with their kind and line",
			"a bad THROW or HTTPERROR is itself a catchable error",
			"HTTPERROR exposes status and details",
			"the call depth is restored after a caught STACK OVERFLOW"),
		merge("flow", "flow.bas", byName,
			"the CATCH variable is local inside a FUNCTION",
			"RETURN inside TRY and CATCH",
			"EXIT FOR, WHILE and DO inside TRY and CATCH",
			"FINALLY can hold a TRY and loops of its own"),
		merge("nest", "nest.bas", byName,
			"nested TRY, rethrow and FINALLY order",
			"END inside TRY skips CATCH and runs FINALLY"),
	}
	for name := range byName {
		t.Errorf("case %q is in no merged program", name)
	}
	for _, c := range merged {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := runProgram(t, c.prog)
			if r.exit != 0 {
				t.Fatalf("exit %d, stderr:\n%s", r.exit, r.stderr)
			}
			if r.stdout != c.stdout {
				t.Fatalf("stdout:\n%s\nwant:\n%s", r.stdout, c.stdout)
			}
		})
	}
}

// merge joins the named cases, in order, into one program in file, and
// removes them from cases. name is kept short so t.TempDir stays well under
// Linux's 255-byte filename limit.
func merge(name, file string, cases map[string]outCase, names ...string) outCase {
	var body []ast.Stmt
	var stdout string
	for _, n := range names {
		c, ok := cases[n]
		if !ok {
			panic("no case named " + n)
		}
		delete(cases, n)
		body = append(body, c.prog.Files[0].Body...)
		stdout += c.stdout
	}
	return outCase{name: name, prog: structured(file, body...), stdout: stdout}
}

// Uncaught errors still stop the program with ?KIND ERROR IN file:line — hint.
func TestThrowUncaught(t *testing.T) {
	cases := []struct {
		name   string
		prog   *ast.Program
		stdout string
		stderr string
	}{
		// Uncaught THROW at top level and rethrowing at top level are in the
		// goldens errors/throw_uncaught, throw_http_uncaught and try_rethrow_uncaught.
		{
			name: "an error in CATCH escapes after FINALLY",
			prog: structured("u.bas",
				tryS(1, stmts(throw(2, call("ERROR", str("first")))), "e", stmts(throw(4, call("ERROR", str("again")))), pr(6, str("FINALLY"))),
			),
			stdout: "FINALLY\n",
			stderr: "?USER ERROR IN u.bas:4 — again\n",
		},
		{
			name: "an error inside a FUNCTION called from CATCH",
			prog: structured("u.bas",
				tryS(1, stmts(throw(2, call("ERROR", str("x")))), "e", stmts(do(4, call("Fail")))),
				sub(6, "Fail", nil, throw(7, call("ERROR", str("from FAIL")))),
			),
			stderr: "?USER ERROR IN u.bas:7 — from FAIL\n",
		},
		{
			name:   "THROW in classic mode",
			prog:   classic("c.bas", ln(10, 1, pr(1, str("A"))), ln(20, 2, throw(2, call("ERROR", str("CLASSIC BOOM"))))),
			stdout: "A\n",
			stderr: "?USER ERROR IN c.bas:2 — CLASSIC BOOM\n",
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

func TestTryDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		prog *ast.Program
		kind diag.Kind
		want string
		line int
	}{
		{"RETURN in FINALLY", structured("d.bas",
			function(1, "F", nil, "", tryS(2, nil, "e", nil, ret(5, num(1))))),
			diag.Syntax, "RETURN cannot leave a FINALLY block", 5},
		{"EXIT in FINALLY", structured("d.bas",
			forS(1, "i", num(1), num(2), nil, tryS(2, nil, "e", nil, exit(5, token.FOR)))),
			diag.Syntax, "EXIT FOR cannot leave a FINALLY block", 5},
		{"RETURN in a TRY inside FINALLY", structured("d.bas",
			function(1, "F", nil, "", tryS(2, nil, "e", nil, tryS(4, stmts(ret(5, nil)), "e", nil)))),
			diag.Syntax, "RETURN cannot leave a FINALLY block", 5},
		{"typed CATCH variable", structured("d.bas",
			dim(1, "n", "INTEGER", nil), tryS(2, nil, "n", nil)),
			diag.TypeMismatch, "CATCH N stores the error as a MAP, but N is declared AS INTEGER", 2},
		{"CONST CATCH variable", structured("d.bas",
			cnst(1, "K", num(1)), tryS(2, nil, "K", nil)),
			diag.Syntax, "K is a CONST", 2},
		{"THROW a NUMBER", structured("d.bas", throw(3, num(42))),
			diag.TypeMismatch, "THROW needs an error, like THROW ERROR(\"message\") or THROW HTTPERROR(404, \"NOT FOUND\"), not an INTEGER", 3},
		{"EXIT with no loop around the TRY", structured("d.bas", tryS(1, stmts(exit(2, token.WHILE)), "e", nil)),
			diag.Syntax, "EXIT WHILE is not inside a WHILE loop", 2},
		{"FUNCTION inside TRY", structured("d.bas", tryS(1, stmts(function(2, "F", nil, "")), "e", nil)),
			diag.Syntax, "FUNCTION and SUB go at the top level", 2},
		{"TRY in classic mode", classic("d.bas", ln(10, 1, tryS(1, stmts(pr(2, num(1))), "e", nil))),
			diag.Syntax, "TRY … CATCH only works in structured mode; remove the line numbers", 1},
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

// The generated Go follows design §4.6: closures, defer rt.Recover, and no
// rip_k bookkeeping when nothing reads it.
func TestTryGeneratedShape(t *testing.T) {
	files, ds := codegen.Generate(structured("s.bas",
		tryS(1, stmts(throw(2, call("ERROR", str("x")))), "e", stmts(pr(4, mem(id("e"), "message"))), pr(6, str("F"))),
	), codegen.Options{})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	src := string(files[1].Src)
	for _, want := range []string{
		"defer rt.Recover(&rip_e",
		"rt.Throw(rt.NewError(\"x\"))",
		"b_s_e = rip_e",
		"\n//line s.bas:2\n",
		"\n//line s.bas:6\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source is missing %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "rip_k") {
		t.Errorf("a top-level TRY without EXIT needs no rip_k:\n%s", src)
	}
}
