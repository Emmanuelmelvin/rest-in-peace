package codegen

import (
	"strconv"
	"strings"

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
		g.routeStmt(o, s, s.Path, sc)
	case *ast.GroupStmt:
		g.groupStmt(o, s, sc)
	case *ast.UseStmt:
		g.useStmt(o, s, sc)
	case *ast.RespondStmt:
		g.respondStmt(o, s, sc)
	case *ast.ServeStmt:
		g.serveStmt(o, s, sc)
	}
}

// groupStmt compiles ROUTE GROUP "/prefix" … END GROUP (Requirement 7.7).
func (g *gen) groupStmt(o *out, s *ast.GroupStmt, sc *scope) {
	if sc.fn != nil {
		g.errorf(s.At, diag.Syntax, "ROUTE GROUP goes in top-level code, not inside a FUNCTION or SUB")
		return
	}
	for _, r := range s.Routes {
		g.routeStmt(o, r, joinRoutePath(s.Prefix, r.Path), sc)
	}
}

func joinRoutePath(prefix, path string) string {
	p := strings.TrimRight(prefix, "/")
	if p == "" {
		return path
	}
	if path == "/" {
		return p
	}
	return p + path
}

// middlewareFactories maps each built-in middleware name to its rt factory
// and allowed [min, max] argument counts (Requirement 8.3).
var middlewareFactories = map[string]struct {
	fn       string
	min, max int
}{
	"APIKEY":     {fn: "rt.APIKey", min: 1, max: 1},
	"CORS":       {fn: "rt.CORS", min: 0, max: 1},
	"REQUESTLOG": {fn: "rt.RequestLog", min: 0, max: 0},
	"RATELIMIT":  {fn: "rt.RateLimit", min: 1, max: 1},
	"BEARERJWT":  {fn: "rt.BearerJWT", min: 1, max: 1},
}

// useStmt compiles USE <Fn> [ON "/path/*"] (Requirements 8.1–8.3).
func (g *gen) useStmt(o *out, s *ast.UseStmt, sc *scope) {
	if g.file.Mode == token.Classic {
		g.errorf(s.At, diag.Syntax, "USE middleware only works in structured mode; remove the line numbers to use it")
		return
	}
	if sc.fn != nil {
		g.errorf(s.At, diag.Syntax, "USE goes in top-level code, not inside a FUNCTION or SUB")
		return
	}
	var mw string
	switch fn := s.Fn.(type) {
	case *ast.Ident:
		name := canon(fn.Name)
		if _, isBuilt := middlewareFactories[name]; isBuilt {
			g.errorf(fn.At, diag.Syntax, "%s is a built-in middleware; call it with parentheses, like USE %s()", fn.Name, fn.Name)
			return
		}
		mw = g.middlewareRef(fn.At, fn.Name)
	case *ast.CallExpr:
		id, ok := fn.Fn.(*ast.Ident)
		if !ok {
			g.errorf(fn.Pos(), diag.Syntax, "USE expects a middleware function name or built-in middleware call")
			return
		}
		name := canon(id.Name)
		mf, ok := middlewareFactories[name]
		if !ok {
			g.errorf(id.At, diag.Syntax, "USE %s(…) is not a built-in middleware; pass a FUNCTION/SUB by name without parentheses (USE %s)", id.Name, id.Name)
			return
		}
		if len(fn.Args) < mf.min || len(fn.Args) > mf.max {
			g.errorf(id.At, diag.Syntax, "%s takes %d to %d argument(s), got %d", id.Name, mf.min, mf.max, len(fn.Args))
			return
		}
		args := make([]string, len(fn.Args))
		for i, a := range fn.Args {
			args[i] = g.expr(a, sc).code
		}
		mw = mf.fn + "(" + strings.Join(args, ", ") + ")"
	default:
		g.errorf(s.At, diag.Syntax, "USE expects a middleware function name or built-in middleware call")
		return
	}
	if mw == "" {
		return
	}
	g.dir(o, s.At)
	o.w("c.App().Use(", mw, ", ", strconv.Quote(s.Scope), ")")
}

// middlewareRef returns an rt.Handler expression for a user FUNCTION or SUB
// used in USE <Name>.
func (g *gen) middlewareRef(at token.Pos, name string) string {
	fn := g.funcs[canon(name)]
	if fn == nil {
		g.errorf(at, diag.Undefined, "there is no FUNCTION or SUB named %s for USE", name)
		return ""
	}
	if len(fn.params) != 1 {
		g.errorf(at, diag.Syntax, "middleware %s takes one parameter, the request: FUNCTION %s(req) or SUB %s(req)", fn.decl.Name, fn.decl.Name, fn.decl.Name)
		return ""
	}
	return g.wrapHandler(fn)
}

// routeStmt compiles ROUTE <METHOD> "<path>" CALL Fn (structured) or
// ROUTE <METHOD> "<path>" GOSUB n (classic).
func (g *gen) routeStmt(o *out, s *ast.RouteStmt, fullPath string, sc *scope) {
	method, path := strconv.Quote(s.Method), strconv.Quote(fullPath)
	if g.file.Mode == token.Classic {
		if s.GosubLine == 0 {
			g.errorf(s.At, diag.Syntax, "classic mode has no FUNCTIONs, so use ROUTE %s %q GOSUB <line>", s.Method, fullPath)
			return
		}
		g.cl.routes[s.GosubLine] = true
		g.dir(o, s.At)
		o.w("c.App().RouteGosub(", method, ", ", path, ", ", strconv.Itoa(s.GosubLine), ", ", g.classicName(), ")")
		return
	}
	if s.GosubLine != 0 {
		g.errorf(s.At, diag.Syntax, "ROUTE … GOSUB only works in classic mode; use ROUTE %s %q CALL FunctionName", s.Method, fullPath)
		return
	}
	if sc.fn != nil {
		g.errorf(s.At, diag.Syntax, "ROUTE goes in top-level code, not inside a FUNCTION or SUB")
		return
	}
	h := g.handlerRef(s, fullPath)
	if h == "" {
		return
	}
	g.dir(o, s.At)
	o.w("c.App().Route(", method, ", ", path, ", ", h, ")")
}

// handlerRef returns a Go expression of type rt.Handler for the route's
// FUNCTION or SUB, or "" after reporting why there is none.
func (g *gen) handlerRef(s *ast.RouteStmt, fullPath string) string {
	fn := g.funcs[canon(s.Handler)]
	if fn == nil {
		g.errorf(s.At, diag.Undefined, "there is no FUNCTION or SUB named %s for ROUTE %s %q", s.Handler, s.Method, fullPath)
		return ""
	}
	if len(fn.params) != 1 {
		g.errorf(s.At, diag.Syntax, "%s handles ROUTE %s %q, so it takes one parameter, the request: FUNCTION %s(req)", fn.decl.Name, s.Method, fullPath, fn.decl.Name)
		return ""
	}
	return g.wrapHandler(fn)
}

func (g *gen) wrapHandler(fn *fnInfo) string {
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
