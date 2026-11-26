package codegen

import (
	"strconv"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// HTTP statements, in both modes: ROUTE, ROUTE GROUP, USE, RESPOND and SERVE
// (Requirements 7 and 8; design §4.7 and §4.8). laterStmt in structured.go
// sends them here, and classicScan in classic.go already marks ROUTE … GOSUB
// lines as jump targets.
//
// Owned by the Phase 2 HTTP track (docs/phase2-tracks.md).
//
// The app lives on the Ctx: top-level code registers routes with
// c.App().Route(…) and c.App().RouteGosub(…), SERVE records the port with
// c.App().Serve(…), and rt.Main starts listening once all top-level code has
// run.

// httpStmt compiles one HTTP statement. g.file.Mode tells the modes apart.
func (g *gen) httpStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.RouteStmt:
		g.routeStmt(o, s, sc)
	case *ast.GroupStmt:
		g.todo(s.Pos(), "2.7", "ROUTE GROUP") // TODO(task 2.7)
	case *ast.UseStmt:
		g.todo(s.Pos(), "2.3", "USE") // TODO(task 2.3)
	case *ast.RespondStmt:
		g.respondStmt(o, s, sc)
	case *ast.ServeStmt:
		g.serveStmt(o, s, sc)
	}
}

// routeStmt compiles ROUTE <METHOD> "<path>" CALL Fn (structured) or
// ROUTE <METHOD> "<path>" GOSUB n (classic).
func (g *gen) routeStmt(o *out, s *ast.RouteStmt, sc *scope) {
	method, path := strconv.Quote(s.Method), strconv.Quote(s.Path)
	if g.file.Mode == token.Classic {
		if s.GosubLine == 0 {
			g.errorf(s.At, diag.Syntax, "classic mode has no FUNCTIONs, so use ROUTE %s %q GOSUB <line>", s.Method, s.Path)
			return
		}
		g.cl.routes[s.GosubLine] = true
		g.dir(o, s.At)
		o.w("c.App().RouteGosub(", method, ", ", path, ", ", strconv.Itoa(s.GosubLine), ", ", g.classicName(), ")")
		return
	}
	if s.GosubLine != 0 {
		g.errorf(s.At, diag.Syntax, "ROUTE … GOSUB only works in classic mode; use ROUTE %s %q CALL FunctionName", s.Method, s.Path)
		return
	}
	if sc.fn != nil {
		g.errorf(s.At, diag.Syntax, "ROUTE goes in top-level code, not inside a FUNCTION or SUB")
		return
	}
	h := g.handlerRef(s)
	if h == "" {
		return
	}
	g.dir(o, s.At)
	o.w("c.App().Route(", method, ", ", path, ", ", h, ")")
}

// handlerRef returns a Go expression of type rt.Handler for the route's
// FUNCTION or SUB, or "" after reporting why there is none.
func (g *gen) handlerRef(s *ast.RouteStmt) string {
	fn := g.funcs[canon(s.Handler)]
	if fn == nil {
		g.errorf(s.At, diag.Undefined, "there is no FUNCTION or SUB named %s for ROUTE %s %q", s.Handler, s.Method, s.Path)
		return ""
	}
	if len(fn.params) != 1 {
		g.errorf(s.At, diag.Syntax, "%s handles ROUTE %s %q, so it takes one parameter, the request: FUNCTION %s(req)", fn.decl.Name, s.Method, s.Path, fn.decl.Name)
		return ""
	}
	if !fn.isSub && fn.params[0].t == tAny && fn.result == tAny {
		return fn.goName // already a func(c *rt.Ctx, req rt.Value) rt.Value
	}
	call := fn.goName + "(c, " + coerce(gexpr{"req", tAny, false}, fn.params[0].t) + ")"
	if fn.isSub {
		return "func(c *rt.Ctx, req rt.Value) rt.Value { " + call + "; return nil }"
	}
	return "func(c *rt.Ctx, req rt.Value) rt.Value { return " + call + " }"
}

// respondStmt compiles RESPOND status, value [, headers]. RespondNow panics
// with *rt.Respond, which ends the handler (and, in classic mode, the GOSUB).
func (g *gen) respondStmt(o *out, s *ast.RespondStmt, sc *scope) {
	status := g.statusExpr(s.Status, sc)
	args := "c, " + status + ", " + g.expr(s.Value, sc).code
	if s.Headers != nil {
		args += ", rt.AsMap(" + g.expr(s.Headers, sc).code + ")"
	}
	g.dir(o, s.At)
	o.w("rt.RespondNow(", args, ")")
}

// statusExpr compiles RESPOND's status. A whole-number literal is checked
// here; anything else is checked by rt.Status when it runs.
func (g *gen) statusExpr(e ast.Expr, sc *scope) string {
	if lit, ok := e.(*ast.NumberLit); ok && lit.IsInt {
		n, err := strconv.Atoi(lit.Text)
		if err != nil || n < 100 || n > 599 {
			g.errorf(e.Pos(), diag.TypeMismatch, "RESPOND needs an HTTP status from 100 to 599, like RESPOND 200, value; got %s", lit.Text)
			return "500"
		}
		return strconv.Itoa(n)
	}
	return "rt.Status(" + g.expr(e, sc).code + ")"
}

// serveStmt compiles SERVE [port]. It only records the port; rt.Main starts
// listening after all top-level code has run. In classic mode the top-level
// run stops at SERVE, so the lines after it run only as GOSUB targets.
func (g *gen) serveStmt(o *out, s *ast.ServeStmt, sc *scope) {
	if sc.fn != nil {
		g.errorf(s.At, diag.Syntax, "SERVE goes in top-level code, not inside a FUNCTION or SUB")
		return
	}
	port := "nil"
	if s.Port != nil {
		port = g.expr(s.Port, sc).code
	}
	g.dir(o, s.At)
	o.w("c.App().Serve(", port, ")")
	if g.file.Mode == token.Classic {
		o.w("return")
	}
}
