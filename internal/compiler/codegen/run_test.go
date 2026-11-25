package codegen_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// End-to-end tests: hand-built AST → codegen.Generate → build.Prepare and
// build.Build → run the binary → compare stdout (and stderr for errors).

type result struct {
	stdout, stderr string
	exit           int
}

// runProgram compiles and runs p. On a build failure it reports the generated source.
func runProgram(t *testing.T, p *ast.Program) result {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a Go program; skipped with -short")
	}
	if _, err := build.LookGo(); err != nil {
		t.Skip(err)
	}
	files, ds := codegen.Generate(p, codegen.Options{})
	if ds.HasErrors() {
		t.Fatalf("Generate reported errors:\n%v", ds)
	}
	dir := t.TempDir()
	if err := build.Prepare(dir, files); err != nil {
		t.Fatal(err)
	}
	bin, err := build.Build(context.Background(), dir, build.Options{Output: filepath.Join(dir, "bin", "prog")})
	if err != nil {
		t.Fatalf("%v\n\ngenerated source:\n%s", err, dump(files))
	}
	cmd := exec.Command(bin)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	r := result{stdout: normalize(stdout.String()), stderr: normalize(stderr.String())}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		r.exit = exitErr.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	if os.Getenv("RIP_DUMP") != "" {
		t.Logf("generated source:\n%s", dump(files))
	}
	return r
}

func dump(files []codegen.File) string {
	var b strings.Builder
	for _, f := range files {
		b.WriteString("==== " + f.Name + "\n")
		b.Write(f.Src)
	}
	return b.String()
}

func normalize(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// readGolden returns a file from testdata/golden.
func readGolden(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "golden", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return normalize(string(b))
}

type outCase struct {
	name   string
	prog   *ast.Program
	stdout string
	golden string // if set, the expected stdout is this .out file instead
}

func TestPrograms(t *testing.T) {
	cases := []outCase{
		{name: "golden structured/print_basics", golden: "structured/print_basics.out", prog: goldenPrintBasics()},
		{name: "golden structured/functions", golden: "structured/functions.out", prog: goldenFunctions()},
		{name: "golden classic/count", golden: "classic/count.out", prog: goldenCount()},
		{name: "golden classic/gosub", golden: "classic/gosub.out", prog: goldenGosub()},
		literals(),
		arithmetic(),
		nativeArithmetic(),
		comparisonsAndLogic(),
		membersAndIndexes(),
		ifElseIf(),
		forLoops(),
		forEach(),
		whileDoExit(),
		selectCase(),
		constants(),
		functionsAndSubs(),
		scoping(),
		endStatement(),
		builtinsCase(),
		classicJumps(),
		classicNested(),
		classicBlocks(),
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			want := c.stdout
			if c.golden != "" {
				want = readGolden(t, c.golden)
			}
			r := runProgram(t, c.prog)
			if r.exit != 0 {
				t.Fatalf("exit %d, stderr:\n%s", r.exit, r.stderr)
			}
			if r.stdout != want {
				t.Fatalf("stdout:\n%s\nwant:\n%s", r.stdout, want)
			}
		})
	}
}

// ---- Golden equivalents ----

// testdata/golden/structured/print_basics.bas
func goldenPrintBasics() *ast.Program {
	return structured("print_basics.bas",
		pr(1, str("READY.")),
		let(2, id("x"), bin(num(2), token.Plus, num(3))),
		pr(3, id("x")),
		pr(4, bin(str("TOTAL: "), token.Amp, id("x"))),
	)
}

// testdata/golden/structured/functions.bas
func goldenFunctions() *ast.Program {
	return structured("functions.bas",
		pr(1, call("Double", num(21))),
		pr(2, call("Greet", str("Ada"))),
		function(4, "Double", params(param("n", "INTEGER", nil)), "INTEGER",
			ret(5, bin(id("n"), token.Star, num(2))),
		),
		function(8, "Greet", params(param("name", "", nil), param("greeting", "", str("HELLO"))), "",
			ret(9, bin(bin(id("greeting"), token.Amp, str(", ")), token.Amp, id("name"))),
		),
	)
}

// testdata/golden/classic/count.bas
func goldenCount() *ast.Program {
	return classic("count.bas",
		ln(10, 1, let(1, id("I"), num(1))),
		ln(20, 2, pr(2, id("I"))),
		ln(30, 3, let(3, id("I"), bin(id("I"), token.Plus, num(1)))),
		ln(40, 4, iff(4, bin(id("I"), token.LtEq, num(3)), stmts(gotoS(4, 20)))),
		ln(50, 5, end(5)),
	)
}

// testdata/golden/classic/gosub.bas
func goldenGosub() *ast.Program {
	return classic("gosub.bas",
		ln(10, 1, gosub(1, 100)),
		ln(20, 2, gosub(2, 100)),
		ln(30, 3, pr(3, str("DONE"))),
		ln(40, 4, end(4)),
		ln(100, 5, pr(5, str("IN THE SUBROUTINE"))),
		ln(110, 6, ret(6, nil)),
	)
}

// ---- Structured constructs ----

func literals() outCase {
	return outCase{name: "literals", prog: structured("literals.bas",
		pr(1, num(42)),
		pr(2, flt("3.5")),
		pr(3, flt("1e6")),
		pr(4, str("hi")),
		pr(5, boolean(true)),
		pr(6, boolean(false)),
		pr(7, nilv()),
		pr(8, mapl("b", num(1), "a", arr(num(1), str("x"), boolean(true), nilv()))),
		pr(9, arr()),
		pr(10, mapl()),
		pr(11, str("A"), num(1), str("B")),
	), stdout: "42\n3.5\n1e+06\nhi\nTRUE\nFALSE\n\n" + `{"b":1,"a":[1,"x",true,null]}` + "\n[]\n{}\nA1B\n"}
}

func arithmetic() outCase {
	a, b := id("a"), id("b")
	return outCase{name: "dynamic arithmetic", prog: structured("arith.bas",
		let(1, a, num(7)),
		let(2, b, num(2)),
		pr(3, bin(a, token.Plus, b)),
		pr(4, bin(a, token.Minus, b)),
		pr(5, bin(a, token.Star, b)),
		pr(6, bin(a, token.Slash, b)),
		pr(7, bin(a, token.Backslash, b)),
		pr(8, bin(a, token.MOD, b)),
		pr(9, bin(a, token.Caret, b)),
		pr(10, neg(a)),
		pr(11, bin(neg(a), token.Backslash, b)),
		pr(12, bin(neg(a), token.MOD, b)),
		pr(13, bin(num(2), token.Caret, flt("0.5"))),
		pr(14, bin(str("A"), token.Plus, a)),
		pr(15, bin(a, token.Plus, flt("2.5"))),
		pr(16, bin(flt("7.5"), token.MOD, b)),
		pr(17, bin(flt("7.9"), token.Backslash, b)),
		pr(18, bin(num(2), token.Caret, num(62))),
		pr(19, bin(num(2), token.Caret, num(64))),
		pr(20, bin(a, token.Plus, bin(b, token.Star, num(3)))), // precedence comes from the tree
	), stdout: "9\n5\n14\n3.5\n3\n1\n49\n-7\n-3\n-1\n1.4142135623730951\nA7\n9.5\n1.5\n3\n4611686018427387904\n1.8446744073709552e+19\n13\n"}
}

func nativeArithmetic() outCase {
	i, j, x, s := id("i"), id("j"), id("x"), id("s")
	return outCase{name: "native typed arithmetic", prog: structured("native.bas",
		dim(1, "i", "INTEGER", num(7)),
		dim(2, "j", "INTEGER", num(2)),
		dim(3, "x", "NUMBER", flt("1.5")),
		dim(4, "s", "STRING", str("N=")),
		dim(5, "ok", "BOOLEAN", nil),
		pr(6, bin(i, token.Plus, j)),
		pr(7, bin(i, token.Backslash, j)),
		pr(8, bin(i, token.MOD, j)),
		pr(9, bin(i, token.Slash, j)),
		pr(10, bin(i, token.Caret, j)),
		pr(11, neg(i)),
		pr(12, bin(x, token.Star, i)),
		pr(13, bin(s, token.Plus, i)),
		pr(14, bin(s, token.Amp, x)),
		pr(15, id("ok")),
		let(16, i, bin(i, token.Star, num(3))),
		pr(17, i),
		let(18, x, i), // INTEGER into NUMBER
		pr(19, bin(x, token.Slash, num(2))),
		let(20, i, flt("4.0")), // a whole NUMBER fits an INTEGER
		pr(21, i),
		dim(22, "dyn", "", num(5)),
		let(23, i, id("dyn")), // dynamic into INTEGER, checked at run time
		pr(24, bin(i, token.Plus, num(1))),
		dim(25, "n", "", nil),
		pr(26, bin(str("["), token.Amp, bin(id("n"), token.Amp, str("]")))),
		dim(27, "m", "MAP", nil),
		dim(28, "a", "ARRAY", nil),
		pr(29, id("m"), id("a")),
		pr(30, bin(x, token.MOD, flt("4"))),
		pr(31, bin(x, token.Caret, num(2))),
	), stdout: "9\n3\n1\n3.5\n49\n-7\n10.5\nN=7\nN=1.5\nFALSE\n21\n10.5\n4\n6\n[]\n{}[]\n1\n441\n"}
}

func comparisonsAndLogic() outCase {
	boom := function(30, "Boom", nil, "",
		pr(31, str("BOOM")),
		ret(32, boolean(true)),
	)
	return outCase{name: "comparisons and logic", prog: structured("logic.bas",
		pr(1, bin(num(1), token.Lt, num(2))),
		pr(2, bin(num(2), token.LtEq, num(2))),
		pr(3, bin(num(3), token.Gt, num(4))),
		pr(4, bin(num(3), token.GtEq, num(3))),
		pr(5, bin(num(1), token.Eq, flt("1.0"))),
		pr(6, bin(str("a"), token.Lt, str("b"))),
		pr(7, bin(str("x"), token.NotEq, str("y"))),
		pr(8, not(boolean(true))),
		pr(9, bin(boolean(true), token.AND, boolean(false))),
		pr(10, bin(boolean(true), token.OR, boolean(false))),
		let(11, id("v"), num(5)),
		let(12, id("w"), str("5")),
		pr(13, bin(id("v"), token.Eq, id("w"))),
		pr(14, bin(id("v"), token.NotEq, nilv())),
		pr(15, bin(nilv(), token.Eq, nilv())),
		pr(16, bin(id("v"), token.Gt, flt("4.5"))),
		pr(17, bin(id("v"), token.LtEq, num(4))),
		pr(18, bin(id("v"), token.GtEq, num(5))),
		pr(19, bin(boolean(false), token.AND, call("Boom"))),
		pr(20, bin(boolean(true), token.OR, call("Boom"))),
		pr(21, bin(boolean(true), token.AND, call("Boom"))),
		pr(22, not(id("v"))),
		pr(23, bin(id("v"), token.AND, str(""))),
		boom,
	), stdout: "TRUE\nTRUE\nFALSE\nTRUE\nTRUE\nTRUE\nTRUE\nFALSE\nFALSE\nTRUE\nFALSE\nTRUE\nTRUE\nTRUE\nFALSE\nTRUE\nFALSE\nTRUE\nBOOM\nTRUE\nFALSE\nFALSE\n"}
}

func membersAndIndexes() outCase {
	m := id("m")
	return outCase{name: "members and indexes", prog: structured("members.bas",
		let(1, m, mapl("name", str("Ada"), "tags", arr(str("x"), str("y")))),
		pr(2, mem(m, "name")),
		pr(3, idx(mem(m, "tags"), num(1))),
		let(4, mem(m, "age"), num(36)),
		let(5, idx(m, str("Key")), num(1)),
		let(6, idx(mem(m, "tags"), num(0)), str("z")),
		pr(7, m),
		pr(8, bin(str("["), token.Amp, bin(mem(m, "missing"), token.Amp, str("]")))),
		pr(9, idx(m, str("name"))),
		let(10, id("rows"), arr(mapl("id", num(1)), mapl("id", num(2)))),
		let(11, mem(idx(id("rows"), num(1)), "id"), num(20)),
		pr(12, id("rows")),
		pr(13, mem(m, "NAME")), // member names are case-sensitive
	), stdout: "Ada\ny\n" + `{"name":"Ada","tags":["z","y"],"age":36,"Key":1}` + "\n[]\nAda\n" + `[{"id":1},{"id":20}]` + "\n\n"}
}

func ifElseIf() outCase {
	n := id("n")
	grade := function(10, "Grade", params(param("n", "", nil)), "STRING",
		iff(11, bin(n, token.GtEq, num(90)), stmts(ret(12, str("A"))),
			iff(13, bin(n, token.GtEq, num(80)), stmts(ret(14, str("B"))),
				ret(16, str("C")),
			),
		),
	)
	return outCase{name: "IF ELSEIF ELSE", prog: structured("ifs.bas",
		pr(1, call("Grade", num(95))),
		pr(2, call("Grade", num(85))),
		pr(3, call("Grade", num(10))),
		iff(4, bin(num(1), token.Eq, num(1)), stmts(pr(5, str("YES")))),
		iff(6, nilv(), stmts(pr(7, str("NIL IS TRUE"))), pr(8, str("NIL IS FALSE"))),
		iff(9, num(3), stmts(pr(9, str("3 IS TRUE")))),
		grade,
	), stdout: "A\nB\nC\nYES\nNIL IS FALSE\n3 IS TRUE\n"}
}

func forLoops() outCase {
	i, k := id("i"), id("k")
	return outCase{name: "FOR with STEP", prog: structured("for.bas",
		forS(1, "i", num(1), num(3), nil, pr(2, i)),
		forS(3, "i", num(10), num(1), neg(num(3)), pr(4, i)),
		forS(5, "i", num(0), num(1), flt("0.5"), pr(6, i)),
		forS(7, "i", num(3), num(1), nil, pr(8, str("NEVER"))),
		dim(9, "k", "INTEGER", nil),
		forS(10, "k", num(5), num(1), neg(num(2)), pr(11, k)),
		pr(12, bin(str("AFTER "), token.Amp, k)),
		dim(13, "f", "NUMBER", nil),
		forS(14, "f", num(1), num(2), flt("0.5"), pr(15, id("f"))),
		let(16, id("total"), num(0)),
		forS(17, "i", num(1), num(100), nil, let(18, id("total"), bin(id("total"), token.Plus, i))),
		pr(19, id("total")),
		pr(20, i),
	), stdout: "1\n2\n3\n10\n7\n4\n1\n0\n0.5\n1\n5\n3\n1\nAFTER -1\n1\n1.5\n2\n5050\n101\n"}
}

func forEach() outCase {
	return outCase{name: "FOR EACH", prog: structured("each.bas",
		each(1, "x", arr(num(10), str("b"), boolean(true)), pr(2, id("x"))),
		let(3, id("m"), mapl("z", num(1), "a", num(2))),
		each(4, "k", id("m"), pr(5, bin(id("k"), token.Amp, bin(str("="), token.Amp, idx(id("m"), id("k")))))),
		dim(6, "n", "INTEGER", nil),
		let(7, id("sum"), num(0)),
		each(8, "n", arr(num(1), num(2), num(3)), let(9, id("sum"), bin(id("sum"), token.Plus, id("n")))),
		pr(10, id("sum")),
		each(11, "x", arr(), pr(12, str("NEVER"))),
	), stdout: "10\nb\nTRUE\nz=1\na=2\n6\n"}
}

func whileDoExit() outCase {
	i := id("i")
	return outCase{name: "WHILE DO and EXIT", prog: structured("loops.bas",
		let(1, i, num(0)),
		while(2, bin(i, token.Lt, num(3)), let(3, i, bin(i, token.Plus, num(1))), pr(4, i)),
		doLoop(5, bin(i, token.LtEq, num(0)), let(6, i, bin(i, token.Minus, num(1))), pr(7, i)),
		doLoop(8, nil,
			let(9, i, bin(i, token.Plus, num(10))),
			iff(10, bin(i, token.Gt, num(25)), stmts(exit(11, token.DO))),
		),
		pr(12, i),
		forS(13, "j", num(1), num(10), nil,
			while(14, boolean(true), exit(15, token.WHILE)),
			sel(16, id("j"), nil, cas(exprs(num(3)), exit(17, token.FOR))),
			pr(18, id("j")),
		),
		pr(19, bin(str("J="), token.Amp, id("j"))),
		doLoop(20, boolean(true), pr(21, str("ONCE"))),
	), stdout: "1\n2\n3\n2\n1\n0\n30\n1\n2\nJ=3\nONCE\n"}
}

func selectCase() outCase {
	desc := function(10, "Describe", params(param("x", "", nil)), "",
		sel(11, id("x"), stmts(ret(17, str("OTHER"))),
			cas(exprs(num(1), num(2)), ret(13, str("SMALL"))),
			cas(exprs(str("hi")), ret(15, str("GREETING"))),
			cas(exprs(boolean(true)), ret(16, str("YES"))),
		),
	)
	return outCase{name: "SELECT CASE", prog: structured("select.bas",
		pr(1, call("Describe", num(1))),
		pr(2, call("Describe", flt("2.0"))),
		pr(3, call("Describe", str("hi"))),
		pr(4, call("Describe", num(9))),
		pr(5, call("Describe", boolean(true))),
		dim(6, "n", "INTEGER", num(4)),
		sel(7, id("n"), stmts(pr(8, str("NOT FOUR"))), cas(exprs(num(4)), pr(9, str("FOUR")))),
		sel(20, str("x"), stmts(pr(21, str("ONLY ELSE")))),
		desc,
	), stdout: "SMALL\nSMALL\nGREETING\nOTHER\nYES\nFOUR\nONLY ELSE\n"}
}

func constants() outCase {
	return outCase{name: "CONST", prog: structured("consts.bas",
		pr(1, call("Taxed", num(100))),
		cnst(2, "RATE", flt("0.25")),
		cnst(3, "DOUBLERATE", bin(id("RATE"), token.Star, num(2))),
		cnst(4, "GREETING", bin(str("HI"), token.Amp, str("!"))),
		cnst(5, "LIMIT", num(3)),
		pr(6, id("GREETING")),
		pr(7, id("DOUBLERATE")),
		forS(8, "i", num(1), id("LIMIT"), nil, pr(9, id("i"))),
		cnst(13, "COLORS", arr(str("red"), id("GREETING"))), // not a Go constant: set when it runs
		pr(14, id("COLORS")),
		function(10, "Taxed", params(param("amount", "NUMBER", nil)), "NUMBER",
			cnst(11, "EXTRA", num(1)),
			ret(12, bin(bin(id("amount"), token.Star, bin(num(1), token.Plus, id("RATE"))), token.Plus, id("EXTRA"))),
		),
	), stdout: "126\nHI!\n0.5\n1\n2\n3\n" + `["red","HI!"]` + "\n"}
}

func functionsAndSubs() outCase {
	n := id("n")
	return outCase{name: "FUNCTION and SUB", prog: structured("funcs.bas",
		pr(1, call("Fact", num(20))),
		pr(2, call("Fib", num(15))),
		do(3, call("Shout", str("hey"))),
		do(4, call("Shout", str("quiet"), boolean(false))),
		pr(5, call("Add", num(1))),
		pr(6, call("Add", num(1), num(2))),
		pr(7, call("Add", num(1), num(2), num(3))),
		let(8, id("v"), num(21)),
		pr(9, call("Twice", id("v"))),
		pr(10, call("Twice", flt("4.0"))),
		pr(11, call("Half", num(3))),
		pr(12, call("Nothing")),
		do(13, call("Fact", num(3))),
		function(20, "Fact", params(param("n", "INTEGER", nil)), "INTEGER",
			iff(21, bin(n, token.LtEq, num(1)), stmts(ret(22, num(1)))),
			ret(23, bin(n, token.Star, call("Fact", bin(n, token.Minus, num(1))))),
		),
		function(25, "Fib", params(param("n", "", nil)), "",
			iff(26, bin(n, token.Lt, num(2)), stmts(ret(27, n))),
			ret(28, bin(call("Fib", bin(n, token.Minus, num(1))), token.Plus, call("Fib", bin(n, token.Minus, num(2))))),
		),
		sub(30, "Shout", params(param("msg", "STRING", nil), param("loud", "BOOLEAN", boolean(true))),
			iff(31, not(id("loud")), stmts(pr(32, id("msg")), ret(33, nil))),
			pr(34, call("UPPER", id("msg"))),
		),
		function(36, "Add", params(param("a", "", nil), param("b", "", num(10)), param("c", "", num(100))), "",
			ret(37, bin(bin(id("a"), token.Plus, id("b")), token.Plus, id("c"))),
		),
		function(39, "Twice", params(param("x", "INTEGER", nil)), "INTEGER", ret(40, bin(id("x"), token.Star, num(2)))),
		function(41, "Half", params(param("x", "NUMBER", nil)), "NUMBER", ret(42, bin(id("x"), token.Slash, num(2)))),
		function(43, "Nothing", nil, "", pr(44, str("IN NOTHING"))),
	), stdout: "2432902008176640000\n610\nHEY\nquiet\n111\n103\n6\n42\n8\n1.5\nIN NOTHING\n\n"}
}

func scoping() outCase {
	return outCase{name: "module variables and locals", prog: structured("scope.bas",
		let(1, id("count"), num(0)),
		do(2, call("Bump")),
		do(3, call("Bump")),
		pr(4, id("count")),
		forS(5, "i", num(1), num(2), nil, pr(6, call("Inner", id("i")))),
		pr(7, bin(str("I="), token.Amp, id("i"))),
		sub(10, "Bump", nil,
			let(11, id("count"), bin(id("count"), token.Plus, num(1))),
			let(12, id("scratch"), num(99)),
		),
		function(14, "Inner", params(param("n", "", nil)), "",
			let(15, id("acc"), num(0)),
			forS(16, "i", num(1), id("n"), nil, let(17, id("acc"), bin(id("acc"), token.Plus, num(10)))),
			ret(18, id("acc")),
		),
	), stdout: "2\n10\n20\nI=3\n"}
}

func endStatement() outCase {
	return outCase{name: "END", prog: structured("end.bas",
		pr(1, str("A")),
		do(2, call("Stop")),
		pr(3, str("NEVER")),
		sub(4, "Stop", nil, pr(5, str("B")), end(6), pr(7, str("NEVER"))),
	), stdout: "A\nB\n"}
}

func builtinsCase() outCase {
	return outCase{name: "built-in functions", prog: structured("builtins.bas",
		pr(1, call("LEN", str("héllo"))),
		pr(2, call("LEN", arr(num(1), num(2)))),
		pr(3, call("LEN", nilv())),
		pr(4, call("LEN", mapl("a", num(1)))),
		pr(5, call("UPPER", str("abc")), call("LOWER", str("DEF"))),
		pr(6, bin(str("["), token.Amp, bin(call("TRIM", str("  x  ")), token.Amp, str("]")))),
		pr(7, call("LEFT", str("HELLO"), num(2))),
		pr(8, call("RIGHT", str("HELLO"), num(3))),
		pr(9, call("MID", str("HELLO"), num(2), num(3))),
		pr(10, call("MID", str("HELLO"), num(3))),
		pr(11, call("LEFT", str("HI"), num(10))),
		pr(12, bin(call("STR", num(42)), token.Plus, str("!"))),
		pr(13, bin(call("VAL", str("12abc")), token.Plus, num(1))),
		pr(14, call("VAL", str(" 3.5 "))),
		pr(15, call("VAL", str("x"))),
		pr(16, call("INT", flt("-2.5"))),
		pr(17, call("INT", num(7))),
		pr(18, call("ABS", neg(num(3)))),
		pr(19, call("ABS", flt("-2.5"))),
		pr(20, bin(call("LEN", str("abc")), token.Star, num(2))),
		pr(21, &ast.CallExpr{Fn: &ast.Ident{Name: "lower"}, Args: exprs(str("MiXeD"))}), // names are case-insensitive
	), stdout: "5\n2\n0\n1\nABCdef\n[x]\nHE\nLLO\nELL\nLLO\nHI\n42!\n13\n3.5\n0\n-3\n7\n3\n2.5\n6\nmixed\n"}
}

// ---- Classic mode ----

func classicJumps() outCase {
	x := id("X")
	return outCase{name: "classic ON GOTO", prog: classic("ongoto.bas",
		ln(10, 1, let(1, x, num(2))),
		ln(20, 2, onGoto(2, x, 100, 200, 300)),
		ln(30, 3, pr(3, str("FELL THROUGH"))),
		ln(40, 4, end(4)),
		ln(100, 5, pr(5, str("ONE")), end(5)),
		ln(200, 6, pr(6, str("TWO")), let(6, x, num(5)), gotoS(6, 20)),
		ln(300, 7, pr(7, str("THREE"))),
	), stdout: "TWO\nFELL THROUGH\n"}
}

func classicNested() outCase {
	i := id("I")
	return outCase{name: "classic GOSUB inside blocks", prog: classic("nested.bas",
		ln(10, 1, forS(1, "I", num(1), num(3), nil,
			iff(1, bin(i, token.Eq, num(2)), stmts(gosub(1, 100)), pr(1, i)),
		)),
		ln(20, 2, gosub(2, 200)),
		ln(30, 3, pr(3, bin(str("UNSET=["), token.Amp, bin(id("NEVERSET"), token.Amp, str("]"))))),
		ln(40, 4, end(4)),
		ln(100, 5, pr(5, str("TWO")), ret(5, nil)),
		ln(200, 6, pr(6, str("OUTER")), gosub(6, 300), pr(6, str("BACK"))),
		ln(210, 7, ret(7, nil)),
		ln(300, 8, pr(8, str("INNER")), ret(8, nil)),
	), stdout: "1\nTWO\n3\nOUTER\nINNER\nBACK\nUNSET=[]\n"}
}

func classicBlocks() outCase {
	n, i, w := id("N"), id("I"), id("W")
	return outCase{name: "classic blocks", prog: classic("blocks.bas",
		ln(10, 1, let(1, n, num(0)), doLoop(1, bin(n, token.GtEq, num(3)), let(1, n, bin(n, token.Plus, num(1))))),
		ln(20, 2, pr(2, n)),
		ln(30, 3, forS(3, "I", num(1), num(10), nil, iff(3, bin(i, token.Eq, num(4)), stmts(exit(3, token.FOR))))),
		ln(40, 4, pr(4, i)),
		ln(50, 5, sel(5, i, stmts(pr(5, str("OTHER"))),
			cas(exprs(num(1), num(2)), pr(5, str("LOW"))),
			cas(exprs(num(4)), pr(5, str("FOUR"))),
		)),
		ln(60, 6, let(6, w, num(0)), while(6, bin(w, token.Lt, num(2)), let(6, w, bin(w, token.Plus, num(1)))), pr(6, w)),
		ln(70, 7, each(7, "X", arr(num(10), num(20)), pr(7, id("X")))),
		ln(80, 8, dim(8, "T", "INTEGER", num(5)), pr(8, bin(id("T"), token.Star, num(2)))),
		ln(90, 9, cnst(9, "K", str("CONST")), pr(9, id("K"))),
		ln(100, 10, forS(10, "I", num(3), num(1), neg(num(1)), pr(10, i))),
		ln(110, 11, doLoop(11, nil, let(11, n, bin(n, token.Plus, num(1))), iff(11, bin(n, token.Gt, num(5)), stmts(exit(11, token.DO))))),
		ln(120, 12, pr(12, n), iff(12, boolean(false), stmts(pr(12, str("NO"))), pr(12, str("ELSE")))),
	), stdout: "3\n4\nFOUR\n2\n10\n20\n10\nCONST\n3\n2\n1\n6\nELSE\n"}
}
