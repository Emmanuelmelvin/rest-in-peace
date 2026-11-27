package codegen

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Tests in BASIC (task 4.2, Requirement 12): TEST, EXPECT and REQUEST.
//
// A TEST block compiles to a registration in its module's top-level code:
//
//	//line tests/main.test.bas:1
//	rt.Test(c, "SAYS HELLO", "tests/main.test.bas", 1, func(c *rt.Ctx) {
//		defer c.Enter()()
//		var b_main_x2e_test_r rt.Value
//		…
//	})
//
// The body is compiled like a SUB's: every name it assigns is local to the
// TEST, and names it only reads are the module's. rt.Main runs the
// registered tests after all top-level code, instead of starting the server.

func init() {
	// REQUEST joins the built-in table here, so the frozen expr.go keeps its
	// table; at merge time this entry can move into builtins in expr.go.
	builtins["REQUEST"] = builtin{fn: "rt.DoRequest", min: 2, max: 4, t: tAny, ctx: true, args: []typ{tStr, tStr}}
	delete(laterBuiltins, "REQUEST")
}

// testStmt compiles TEST and EXPECT.
func (g *gen) testStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.TestDecl:
		g.testDecl(o, s, sc)
	case *ast.ExpectStmt:
		g.expectStmt(o, s, sc)
	}
}

// IsTestFile reports whether a .bas file may hold TEST blocks
// (Requirement 12.1): its name ends in .test.bas.
func IsTestFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".test.bas")
}

func (g *gen) testDecl(o *out, s *ast.TestDecl, sc *scope) {
	switch {
	case g.file.Mode == token.Classic:
		g.errorf(s.At, diag.Syntax, "TEST only works in structured mode; remove the line numbers from %s", g.file.Name)
		return
	case !IsTestFile(g.file.Name):
		g.errorf(s.At, diag.Syntax, "TEST blocks go in test files named *.test.bas, like tests/main.test.bas; move this TEST out of %s", g.file.Name)
		return
	case sc.fn != nil || sc.depth > 0:
		g.errorf(s.At, diag.Syntax, "TEST goes at the top level of the file, not inside another block")
		return
	}
	decl := &ast.FuncDecl{At: s.At, Name: "TEST " + strconv.Quote(s.Name), IsSub: true, Body: s.Body}
	fn := &fnInfo{decl: decl, name: decl.Name, isSub: true, locals: map[string]*varInfo{}}
	g.collectLocals(fn)

	p := g.pos(s.At)
	g.dir(o, s.At)
	o.w("rt.Test(c, ", strconv.Quote(s.Name), ", ", strconv.Quote(p.File), ", ", strconv.Itoa(p.Line), ", func(c *rt.Ctx) {")
	g.dir(o, s.At)
	o.w("defer c.Enter()()")
	for _, v := range fn.localOrder {
		if v.goConst != "" {
			o.w("const ", v.goName, " ", v.t.goType(), " = ", v.goConst)
		} else {
			o.w("var ", v.goName, " ", v.t.goType())
			o.w("_ = ", v.goName)
		}
	}
	g.block(o, s.Body, &scope{fn: fn})
	o.w("})")
}

// expectStmt compiles EXPECT cond [, msg]. EXPECT a = b compiles to
// rt.ExpectEq, so that a failure without a message can say what both sides
// were; anything else compiles to rt.Expect.
func (g *gen) expectStmt(o *out, s *ast.ExpectStmt, sc *scope) {
	msg := `""`
	if s.Msg != nil {
		m := g.expr(s.Msg, sc)
		if m.t == tStr {
			msg = m.code
		} else {
			msg = "rt.ToString(" + m.code + ")"
		}
	}
	if b, ok := s.Cond.(*ast.BinaryExpr); ok && b.Op == token.Eq {
		x, y := g.expr(b.X, sc), g.expr(b.Y, sc)
		g.binary(b.At, b.Op, x, y) // only for any diagnostics = itself would report
		g.dir(o, s.At)
		o.w("rt.ExpectEq(c, ", x.code, ", ", y.code, ", ", msg, ")")
		return
	}
	cond := truthy(g.expr(s.Cond, sc))
	g.dir(o, s.At)
	o.w("rt.Expect(c, ", cond, ", ", msg, ")")
}
