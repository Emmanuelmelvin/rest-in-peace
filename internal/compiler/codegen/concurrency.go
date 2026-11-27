package codegen

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// concurrencyStmt compiles SPAWN, EVERY, and PARALLEL ... END PARALLEL (task 7.4).
func (g *gen) concurrencyStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.SpawnStmt:
		g.spawnStmt(o, s, sc)
	case *ast.EveryStmt:
		g.everyStmt(o, s, sc)
	case *ast.ParallelStmt:
		g.parallelStmt(o, s, sc)
	}
}

// spawnStmt compiles SPAWN Fn(args...). Arguments are evaluated and captured
// into temporary variables before launching the background goroutine so loop
// variables and mutable expressions are not raced.
func (g *gen) spawnStmt(o *out, s *ast.SpawnStmt, sc *scope) {
	call := s.Call
	if call == nil {
		return
	}

	if id, ok := call.Fn.(*ast.Ident); ok {
		name := canon(id.Name)
		if fn := g.funcs[name]; fn != nil {
			g.spawnUserCall(o, s, fn, sc)
			return
		}
		if b, ok := builtins[name]; ok {
			g.spawnBuiltinCall(o, s, name, b, sc)
			return
		}
	}

	code := g.call(call, sc, true).code
	g.dir(o, s.At)
	o.w("rt.Spawn(c, func(c *rt.Ctx) {")
	g.dir(o, call.At)
	o.w(code)
	o.w("})")
}

func (g *gen) spawnUserCall(o *out, s *ast.SpawnStmt, fn *fnInfo, sc *scope) {
	call := s.Call
	if len(call.Args) > len(fn.params) {
		g.errorf(call.At, diag.Syntax, "%s takes at most %d arguments, not %d", fn.decl.Name, len(fn.params), len(call.Args))
		return
	}

	if len(fn.params) == 0 {
		g.dir(o, s.At)
		o.w("rt.Spawn(c, func(c *rt.Ctx) {")
		g.dir(o, call.At)
		o.w(fn.goName, "(c)")
		o.w("})")
		return
	}

	id := g.next()
	top := &scope{}
	var decls []string
	callArgs := []string{"c"}
	for i, p := range fn.params {
		var a gexpr
		switch {
		case i < len(call.Args):
			a = g.expr(call.Args[i], sc)
		case fn.decl.Params[i].Default != nil:
			a = g.expr(fn.decl.Params[i].Default, top)
		default:
			g.errorf(call.At, diag.Syntax, "%s needs a value for %s (argument %d)", fn.decl.Name, fn.decl.Params[i].Name, i+1)
			return
		}
		tmp := "rip_sa" + id + "_" + strconv.Itoa(i)
		decls = append(decls, "var "+tmp+" "+p.t.goType()+" = "+coerceDecl(a, p.t, p.decl))
		callArgs = append(callArgs, tmp)
	}

	o.w("{")
	for _, d := range decls {
		g.dir(o, s.At)
		o.w(d)
	}
	g.dir(o, s.At)
	o.w("rt.Spawn(c, func(c *rt.Ctx) {")
	g.dir(o, call.At)
	o.w(fn.goName, "(", strings.Join(callArgs, ", "), ")")
	o.w("})")
	o.w("}")
}

func (g *gen) spawnBuiltinCall(o *out, s *ast.SpawnStmt, name string, b builtin, sc *scope) {
	call := s.Call
	if len(call.Args) < b.min || (b.max >= 0 && len(call.Args) > b.max) {
		// Delegate to g.call to emit the standard arity diagnostic.
		_ = g.call(call, sc, true)
		return
	}

	id := g.next()
	var decls []string
	var callArgs []string
	if b.ctx {
		callArgs = append(callArgs, "c")
	}
	for i, a := range call.Args {
		want := tAny
		if i < len(b.args) {
			want = b.args[i]
		}
		tmp := "rip_sa" + id + "_" + strconv.Itoa(i)
		decls = append(decls, "var "+tmp+" "+want.goType()+" = "+coerce(g.expr(a, sc), want))
		callArgs = append(callArgs, tmp)
	}

	if len(decls) > 0 {
		o.w("{")
		for _, d := range decls {
			g.dir(o, s.At)
			o.w(d)
		}
	}
	g.dir(o, s.At)
	o.w("rt.Spawn(c, func(c *rt.Ctx) {")
	g.dir(o, call.At)
	o.w(b.fn, "(", strings.Join(callArgs, ", "), ")")
	o.w("})")
	if len(decls) > 0 {
		o.w("}")
	}
}

// everyStmt compiles EVERY <interval> CALL <Handler>.
func (g *gen) everyStmt(o *out, s *ast.EveryStmt, sc *scope) {
	if g.file.Mode == token.Classic {
		g.errorf(s.At, diag.Syntax, "EVERY only works in structured mode; remove the line numbers to use it")
		return
	}
	fn := g.funcs[canon(s.Handler)]
	if fn == nil {
		g.errorf(s.HandlerAt, diag.Undefined, "there is no FUNCTION or SUB named %s for EVERY", s.Handler)
		return
	}

	args := []string{"c"}
	top := &scope{}
	for i, p := range fn.params {
		if fn.decl.Params[i].Default == nil {
			g.errorf(s.HandlerAt, diag.Syntax, "%s is called by EVERY, so it cannot require parameters: SUB %s() or FUNCTION %s()", fn.decl.Name, fn.decl.Name, fn.decl.Name)
			return
		}
		a := g.expr(fn.decl.Params[i].Default, top)
		args = append(args, coerceDecl(a, p.t, p.decl))
	}

	interval := g.expr(s.Interval, sc).code
	g.dir(o, s.At)
	o.w("rt.Every(c, ", interval, ", func(c *rt.Ctx) {")
	g.dir(o, s.HandlerAt)
	o.w(fn.goName, "(", strings.Join(args, ", "), ")")
	o.w("})")
}

// parallelStmt compiles PARALLEL ... END PARALLEL to rt.Parallel(c, func() { ... }, ...).
func (g *gen) parallelStmt(o *out, s *ast.ParallelStmt, sc *scope) {
	if len(s.Body) == 0 {
		return
	}
	g.dir(o, s.At)
	o.w("rt.Parallel(c,")
	for _, st := range s.Body {
		o.w("func() {")
		sc.depth++
		g.stmt(o, st, sc)
		sc.depth--
		o.w("},")
	}
	o.w(")")
}
