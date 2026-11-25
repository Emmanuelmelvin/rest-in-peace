package codegen

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Scoping rules (structured mode):
//   - Every name assigned in top-level code (LET, DIM, CONST, FOR and FOR EACH
//     variables, at any block depth) is a module-level variable: a Go package var.
//   - Inside a FUNCTION or SUB, every name the function assigns (parameters,
//     LET, DIM, CONST, FOR and FOR EACH variables) is local, even when a
//     module-level variable has the same name, as in QBasic
//     (testdata/golden/structured/function_locals.bas). A name the function
//     only reads is the module-level variable or CONST. Module-level maps
//     and arrays can still be changed through x.name = … and x[i] = ….
//   - Locals live for the whole function, as in BASIC, so they are declared
//     at the top of the Go function.

func (g *gen) structuredFile(o *out) {
	g.collectFuncs()
	top := &scope{}
	g.collectDecls(g.file.Body, top, g.addGlobal)
	for _, fn := range g.funcOrder {
		g.collectLocals(fn)
	}

	g.emitGlobals(o)
	for _, fn := range g.funcOrder {
		g.emitFunc(o, fn)
	}
	o.w("func ", g.runName(), "(c *rt.Ctx) {")
	o.w("defer c.Enter()()")
	g.block(o, g.file.Body, top)
	o.w("}")
}

// ---- Collecting names ----

func (g *gen) collectFuncs() {
	for _, s := range g.file.Body {
		d, ok := s.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := canon(d.Name)
		if prev := g.funcs[name]; prev != nil {
			g.errorf(d.At, diag.Syntax, "%s is already defined at line %d", d.Name, prev.decl.At.Line)
			continue
		}
		fn := &fnInfo{decl: d, name: name, goName: g.mangle(name), isSub: d.IsSub, locals: map[string]*varInfo{}}
		if !d.IsSub {
			fn.result, _ = g.typeRef(d.At, d.Result)
		}
		for _, p := range d.Params {
			pn := canon(p.Name)
			if _, dup := fn.locals[pn]; dup {
				g.errorf(d.At, diag.Syntax, "%s has two parameters named %s", d.Name, p.Name)
				continue
			}
			t, decl := g.typeRef(d.At, p.Type)
			v := &varInfo{name: pn, goName: g.mangle(pn), t: t, decl: decl, param: true}
			fn.params = append(fn.params, v)
			fn.locals[pn] = v
		}
		g.funcs[name] = fn
		g.funcOrder = append(g.funcOrder, fn)
	}
	for _, fn := range g.funcOrder {
		for _, p := range fn.params {
			if g.funcs[p.name] != nil {
				g.errorf(fn.decl.At, diag.Syntax, "parameter %s has the same name as a FUNCTION; rename one of them", p.name)
			}
		}
	}
}

// declKind says how a name was introduced.
type declKind int

const (
	declLet declKind = iota
	declDim
	declConst
	declLoop
)

// collectDecls walks stmts (not into FUNCTION bodies) and reports every name
// they introduce, in source order.
func (g *gen) collectDecls(stmts []ast.Stmt, sc *scope, add func(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope)) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *ast.LetStmt:
			if id, ok := s.Target.(*ast.Ident); ok {
				add(declLet, canon(id.Name), s.At, s, sc)
			}
		case *ast.DimStmt:
			add(declDim, canon(s.Name), s.At, s, sc)
		case *ast.ConstDecl:
			add(declConst, canon(s.Name), s.At, s, sc)
		case *ast.ForStmt:
			add(declLoop, canon(s.Var), s.At, s, sc)
			g.collectDecls(s.Body, sc, add)
		case *ast.ForEachStmt:
			add(declLoop, canon(s.Var), s.At, s, sc)
			g.collectDecls(s.Body, sc, add)
		case *ast.IfStmt:
			g.collectDecls(s.Then, sc, add)
			g.collectDecls(s.Else, sc, add)
		case *ast.WhileStmt:
			g.collectDecls(s.Body, sc, add)
		case *ast.DoStmt:
			g.collectDecls(s.Body, sc, add)
		case *ast.SelectStmt:
			for _, c := range s.Cases {
				g.collectDecls(c.Body, sc, add)
			}
			g.collectDecls(s.Else, sc, add)
		}
	}
}

// addGlobal records a module-level name.
func (g *gen) addGlobal(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope) {
	if g.funcs[name] != nil {
		g.errorf(p, diag.Syntax, "%s is already a FUNCTION or SUB; pick another name for the variable", name)
		return
	}
	v := g.globals[name]
	if v == nil {
		v = &varInfo{name: name, goName: g.mangle(name)}
	} else if v.konst || kind == declConst {
		g.errorf(p, diag.Syntax, "%s is a CONST, so it cannot be declared or assigned again", name)
		return
	}
	g.declare(v, kind, s, sc)
	if g.globals[name] == nil {
		g.globals[name] = v
		g.globOrder = append(g.globOrder, v)
	}
}

// declare applies a DIM or CONST to v.
func (g *gen) declare(v *varInfo, kind declKind, s ast.Stmt, sc *scope) {
	switch kind {
	case declDim:
		d := s.(*ast.DimStmt)
		t, decl := g.typeRef(d.At, d.Type)
		if v.decl != "" && (v.decl != decl || v.t != t) {
			g.errorf(d.At, diag.TypeMismatch, "%s was already declared AS %s", d.Name, v.decl)
			return
		}
		v.t, v.decl = t, decl
		if decl == "" {
			v.decl = "ANY"
		}
	case declConst:
		c := s.(*ast.ConstDecl)
		v.konst = true
		if e, ok := g.trial(func() gexpr { return g.expr(c.Value, sc) }); ok && e.konst && e.t != tAny {
			v.t, v.goConst = e.t, e.code
		}
	}
}

// collectLocals finds the locals of a FUNCTION or SUB.
func (g *gen) collectLocals(fn *fnInfo) {
	sc := &scope{fn: fn}
	add := func(v *varInfo) {
		fn.locals[v.name] = v
		fn.localOrder = append(fn.localOrder, v)
	}
	conflict := func(name string, p token.Pos) bool {
		if g.funcs[name] != nil {
			g.errorf(p, diag.Syntax, "%s is already a FUNCTION or SUB; pick another name for the variable", name)
			return true
		}
		return false
	}
	// DIM, CONST and loop variables first, so a LET before a DIM still sees the local.
	g.collectDecls(fn.decl.Body, sc, func(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope) {
		if kind == declLet || conflict(name, p) {
			return
		}
		v := fn.locals[name]
		if v != nil && v.param && kind != declLoop {
			g.errorf(p, diag.Syntax, "%s is already a parameter of %s", name, fn.decl.Name)
			return
		}
		if v != nil && v.konst {
			g.errorf(p, diag.Syntax, "%s is a CONST, so it cannot be declared or assigned again", name)
			return
		}
		if v == nil {
			v = &varInfo{name: name, goName: g.mangle(name)}
			add(v)
		} else if kind == declConst {
			g.errorf(p, diag.Syntax, "%s is already a variable, so it cannot become a CONST", name)
			return
		}
		g.declare(v, kind, s, sc)
	})
	g.collectDecls(fn.decl.Body, sc, func(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope) {
		if kind != declLet || fn.locals[name] != nil || conflict(name, p) {
			return
		}
		if gv := g.globals[name]; gv != nil && gv.konst {
			return // assigning a module-level CONST is reported where it happens
		}
		add(&varInfo{name: name, goName: g.mangle(name)})
	})
}

// ---- Emitting declarations ----

func (g *gen) emitGlobals(o *out) {
	var consts, vars []*varInfo
	for _, v := range g.globOrder {
		if v.goConst != "" {
			consts = append(consts, v)
		} else {
			vars = append(vars, v)
		}
	}
	if len(consts) > 0 {
		o.w("const (")
		for _, v := range consts {
			o.w(v.goName, " ", v.t.goType(), " = ", v.goConst)
		}
		o.w(")")
		o.w("")
	}
	if len(vars) > 0 {
		o.w("var (")
		for _, v := range vars {
			o.w(v.goName, " ", v.t.goType())
		}
		o.w(")")
		o.w("")
	}
}

func (g *gen) emitFunc(o *out, fn *fnInfo) {
	d := fn.decl
	sig := []string{"c *rt.Ctx"}
	for _, p := range fn.params {
		sig = append(sig, p.goName+" "+p.t.goType())
	}
	result := ""
	if !fn.isSub {
		result = " " + fn.result.goType()
	}
	g.dir(o, d.At)
	o.w("func ", fn.goName, "(", strings.Join(sig, ", "), ")", result, " {")
	g.dir(o, d.At)
	o.w("defer c.Enter()()")
	for _, v := range fn.localOrder {
		if v.goConst != "" {
			o.w("const ", v.goName, " ", v.t.goType(), " = ", v.goConst)
		} else {
			o.w("var ", v.goName, " ", v.t.goType())
			o.w("_ = ", v.goName)
		}
	}
	g.block(o, d.Body, &scope{fn: fn})
	if !fn.isSub {
		o.w("return ", fn.result.zero())
	}
	o.w("}")
	o.w("")
}

// ---- Statements ----

func (g *gen) block(o *out, stmts []ast.Stmt, sc *scope) {
	for _, s := range stmts {
		g.stmt(o, s, sc)
	}
}

// nested compiles a block one level deeper.
func (g *gen) nested(o *out, stmts []ast.Stmt, sc *scope) {
	sc.depth++
	g.block(o, stmts, sc)
	sc.depth--
}

func (g *gen) stmt(o *out, s ast.Stmt, sc *scope) {
	if d, ok := s.(*ast.FuncDecl); ok {
		if sc.fn != nil || sc.depth > 0 {
			g.errorf(d.At, diag.Syntax, "FUNCTION and SUB go at the top level of the file, not inside another block")
		}
		return // hoisted to package level
	}
	if g.simpleStmt(o, s, sc) || g.laterStmt(s) {
		return
	}
	g.dir(o, s.Pos())
	switch s := s.(type) {
	case *ast.IfStmt:
		o.w("if ", truthy(g.expr(s.Cond, sc)), " {")
		g.nested(o, s.Then, sc)
		if len(s.Else) > 0 {
			o.w("} else {")
			g.nested(o, s.Else, sc)
		}
		o.w("}")

	case *ast.ForStmt:
		g.forStmt(o, s, sc)

	case *ast.ForEachStmt:
		v := g.loopVar(s.Var, s.At, sc, false)
		id := g.next()
		x := g.expr(s.In, sc)
		lp := &loop{kind: token.FOR, label: "rip_loop" + id}
		var body out
		body.w("for _, rip_it", id, " := range rt.Iter(", x.code, ") {")
		g.guard(&body)
		body.w(v.goName, " = ", coerce(gexpr{"rip_it" + id, tAny, false}, v.t))
		g.loopBody(&body, s.Body, sc, lp)
		body.w("}")
		emitLabeled(o, lp, &body)

	case *ast.WhileStmt:
		lp := &loop{kind: token.WHILE, label: "rip_loop" + g.next()}
		var body out
		body.w("for ", truthy(g.expr(s.Cond, sc)), " {")
		g.guard(&body)
		g.loopBody(&body, s.Body, sc, lp)
		body.w("}")
		emitLabeled(o, lp, &body)

	case *ast.DoStmt:
		lp := &loop{kind: token.DO, label: "rip_loop" + g.next()}
		var body out
		body.w("for {")
		g.guard(&body)
		g.loopBody(&body, s.Body, sc, lp)
		if s.Until != nil {
			g.dir(&body, s.Until.Pos())
			body.w("if ", truthy(g.expr(s.Until, sc)), " {")
			body.w("break")
			body.w("}")
		}
		body.w("}")
		emitLabeled(o, lp, &body)

	case *ast.SelectStmt:
		x := g.expr(s.X, sc)
		sel := gexpr{"rip_sel" + g.next(), x.t, false}
		o.w("{")
		o.w("var ", sel.code, " ", x.t.goType(), " = ", x.code)
		o.w("_ = ", sel.code)
		o.w("switch {")
		for _, c := range s.Cases {
			if len(c.Values) == 0 {
				continue
			}
			conds := make([]string, len(c.Values))
			for i, v := range c.Values {
				conds[i] = g.binary(v.Pos(), token.Eq, sel, g.expr(v, sc)).code
			}
			g.dir(o, c.Values[0].Pos())
			o.w("case ", strings.Join(conds, " || "), ":")
			g.nested(o, c.Body, sc)
		}
		if len(s.Else) > 0 {
			o.w("default:")
			g.nested(o, s.Else, sc)
		}
		o.w("}")
		o.w("}")

	case *ast.ExitStmt:
		if lp := g.findLoop(s, sc); lp != nil {
			o.w("break ", lp.label)
		}

	case *ast.EndStmt:
		o.w("rt.End()")

	case *ast.ReturnStmt:
		g.returnStmt(o, s, sc)

	case *ast.GotoStmt, *ast.GosubStmt, *ast.OnGotoStmt:
		word := "GOTO"
		if _, ok := s.(*ast.GosubStmt); ok {
			word = "GOSUB"
		}
		g.errorf(s.Pos(), diag.Syntax, "%s only works in classic mode; add line numbers, or use a loop or a FUNCTION", word)

	default:
		g.errorf(s.Pos(), kindInternal, "codegen does not handle %T yet", s)
	}
}

func (g *gen) forStmt(o *out, s *ast.ForStmt, sc *scope) {
	v := g.loopVar(s.Var, s.At, sc, true)
	id := g.next()
	from, to := g.expr(s.From, sc), g.expr(s.To, sc)
	step := gexpr{"int64(1)", tInt, true}
	if s.Step != nil {
		step = g.expr(s.Step, sc)
	}
	lp := &loop{kind: token.FOR, label: "rip_loop" + id}
	fromV, toV, stepV := "rip_from"+id, "rip_to"+id, "rip_step"+id
	T := v.t.goType()
	o.w("{")
	o.w("var ", fromV, " ", T, " = ", coerce(from, v.t))
	o.w("var ", toV, " ", T, " = ", coerce(to, v.t))
	o.w("var ", stepV, " ", T, " = ", coerce(step, v.t))
	o.w(v.goName, " = ", fromV)
	var cond, post string
	if v.t.numeric() {
		cond = "(" + stepV + " >= 0 && " + v.goName + " <= " + toV + ") || (" + stepV + " < 0 && " + v.goName + " >= " + toV + ")"
		post = v.goName + " += " + stepV
	} else {
		cond = "rt.ForCond(" + v.goName + ", " + toV + ", " + stepV + ")"
		post = v.goName + " = rt.Add(" + v.goName + ", " + stepV + ")"
	}
	var body out
	body.w("for ", cond, " {")
	g.guard(&body)
	g.loopBody(&body, s.Body, sc, lp)
	g.dir(&body, s.At)
	body.w(post)
	body.w("}")
	emitLabeled(o, lp, &body)
	o.w("}")
}

// loopVar finds the variable a FOR or FOR EACH assigns.
func (g *gen) loopVar(name string, p token.Pos, sc *scope, numeric bool) *varInfo {
	v := g.lookup(canon(name), sc)
	if v == nil {
		g.errorf(p, kindInternal, "loop variable %s was not collected", name)
		return &varInfo{goName: "_", t: tAny}
	}
	if v.konst {
		g.errorf(p, diag.Syntax, "%s is a CONST, so it cannot be a loop variable", name)
	}
	if numeric && (v.t == tStr || v.t == tBool) {
		g.errorf(p, diag.TypeMismatch, "the FOR variable %s is declared AS %s; it must be a NUMBER or INTEGER", name, v.decl)
	}
	return v
}

func (g *gen) loopBody(o *out, body []ast.Stmt, sc *scope, lp *loop) {
	sc.loops = append(sc.loops, lp)
	g.nested(o, body, sc)
	sc.loops = sc.loops[:len(sc.loops)-1]
}

// emitLabeled writes a loop, with its label only if an EXIT uses it (Go
// rejects unused labels).
func emitLabeled(o *out, lp *loop, body *out) {
	if lp.used {
		o.w(lp.label, ":")
	}
	o.appendOut(body)
}

func (g *gen) findLoop(s *ast.ExitStmt, sc *scope) *loop {
	for i := len(sc.loops) - 1; i >= 0; i-- {
		if sc.loops[i].kind == s.Loop {
			sc.loops[i].used = true
			return sc.loops[i]
		}
	}
	g.errorf(s.At, diag.Syntax, "EXIT %s is not inside a %s loop", s.Loop, s.Loop)
	return nil
}

func (g *gen) guard(o *out) {
	if g.opts.Guard {
		o.w("c.Guard()")
	}
}

func (g *gen) returnStmt(o *out, s *ast.ReturnStmt, sc *scope) {
	fn := sc.fn
	switch {
	case fn == nil:
		g.errorf(s.At, diag.Syntax, "RETURN only works inside a FUNCTION or SUB")
	case fn.isSub && s.Value != nil:
		g.errorf(s.At, diag.Syntax, "%s is a SUB, so RETURN cannot give a value; make it a FUNCTION", fn.decl.Name)
	case fn.isSub:
		o.w("return")
	case s.Value == nil:
		o.w("return ", fn.result.zero())
	default:
		o.w("return ", coerce(g.expr(s.Value, sc), fn.result))
	}
}

// simpleStmt compiles the statements that work the same in both modes.
func (g *gen) simpleStmt(o *out, s ast.Stmt, sc *scope) bool {
	g.cur = s.Pos()
	switch s := s.(type) {
	case *ast.LetStmt:
		g.dir(o, s.At)
		g.assign(o, s.At, s.Target, s.Value, sc)
	case *ast.DimStmt:
		if s.Shared {
			g.todo(s.At, "4.3", "SHARED")
			return true
		}
		v := g.lookup(canon(s.Name), sc)
		if v == nil {
			g.errorf(s.At, kindInternal, "DIM %s was not collected", s.Name)
			return true
		}
		g.dir(o, s.At)
		if s.Value != nil {
			o.w(v.goName, " = ", coerce(g.expr(s.Value, sc), v.t))
		} else {
			o.w(v.goName, " = ", zeroFor(v))
		}
	case *ast.ConstDecl:
		v := g.lookup(canon(s.Name), sc)
		if v == nil {
			g.errorf(s.At, kindInternal, "CONST %s was not collected", s.Name)
			return true
		}
		if v.goConst == "" {
			g.dir(o, s.At)
			o.w(v.goName, " = ", coerce(g.expr(s.Value, sc), v.t))
		}
	case *ast.PrintStmt:
		args := make([]string, len(s.Args))
		for i, a := range s.Args {
			args[i] = g.expr(a, sc).code
		}
		g.dir(o, s.At)
		o.w("rt.Print(", strings.Join(args, ", "), ")")
	case *ast.ExprStmt:
		g.dir(o, s.At)
		if call, ok := s.X.(*ast.CallExpr); ok {
			o.w(g.call(call, sc, true).code)
		} else {
			o.w("_ = ", g.expr(s.X, sc).code)
		}
	default:
		return false
	}
	return true
}

// assign compiles target = value.
func (g *gen) assign(o *out, p token.Pos, target, value ast.Expr, sc *scope) {
	switch t := target.(type) {
	case *ast.Ident:
		v := g.lookup(canon(t.Name), sc)
		if v == nil {
			g.errorf(p, kindInternal, "variable %s was not collected", t.Name)
			return
		}
		if v.konst {
			g.errorf(p, diag.Syntax, "%s is a CONST, so it cannot change", t.Name)
			return
		}
		o.w(v.goName, " = ", coerce(g.expr(value, sc), v.t))
	case *ast.MemberExpr:
		x := g.expr(t.X, sc)
		val := g.expr(value, sc)
		o.w("rt.SetMember(", x.code, ", ", strconv.Quote(t.Name), ", ", val.code, ")")
	case *ast.IndexExpr:
		x := g.expr(t.X, sc)
		i := g.expr(t.Index, sc)
		val := g.expr(value, sc)
		o.w("rt.SetIndex(", x.code, ", ", i.code, ", ", val.code, ")")
	default:
		g.errorf(p, diag.Syntax, "the left side of = must be a variable, a member like x.name, or an index like x[i]")
	}
}

// laterStmt reports statements that later tasks build (tasks.md), so they fail
// with a clear diagnostic instead of broken Go.
func (g *gen) laterStmt(s ast.Stmt) bool {
	task, what := "", ""
	switch s.(type) {
	case *ast.ThrowStmt:
		task, what = "2.2", "THROW"
	case *ast.TryStmt:
		task, what = "2.2", "TRY … CATCH"
	case *ast.RouteStmt:
		task, what = "2.1", "ROUTE"
	case *ast.GroupStmt:
		task, what = "2.7", "ROUTE GROUP"
	case *ast.UseStmt:
		task, what = "2.3", "USE"
	case *ast.RespondStmt:
		task, what = "2.1", "RESPOND"
	case *ast.ServeStmt:
		task, what = "2.1", "SERVE"
	case *ast.DBConnectStmt:
		task, what = "3.1", "DBCONNECT"
	case *ast.TransactionStmt:
		task, what = "3.2", "TRANSACTION"
	case *ast.LogStmt:
		task, what = "2.4", "LOG"
	case *ast.TestDecl:
		task, what = "4.2", "TEST"
	case *ast.ExpectStmt:
		task, what = "4.2", "EXPECT"
	default:
		return false
	}
	g.todo(s.Pos(), task, what)
	return true
}
