package codegen

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Classic mode (design §4.7) compiles to one Go function with a label per
// jumped-to line and native goto:
//
//   - Every variable is a package-level global, so no goto ever jumps over a
//     declaration. The few temporaries (FOR limits, SELECT values) are declared
//     at the top of the function, before the first label.
//   - Block statements (IF, FOR, WHILE, DO, SELECT) are flattened into gotos
//     too, so every label, including the return point after each GOSUB, sits
//     at the top level of the function. Go can jump out of blocks but not into
//     them.
//   - GOSUB pushes a return ID with c.GosubPush and jumps; RETURN pops it with
//     c.GosubPop and switches to the matching return label.
//   - END returns from the function, which ends the top-level run.

type classicState struct {
	lines     map[int]bool // line numbers that exist
	targets   map[int]bool // line numbers that something jumps to
	hasReturn bool
	gosubIDs  []int
	temps     []string // declarations of rip_t temporaries
	used      map[string]bool
}

// returnMarker stands for a RETURN's switch until every GOSUB is known.
const returnMarker = "\x00RETURN\x00"

func (g *gen) classicName() string { return "rip_" + g.mod + "_classic" }

func (g *gen) classicFile(o *out) {
	g.cl = &classicState{lines: map[int]bool{}, targets: map[int]bool{}, used: map[string]bool{}}
	top := &scope{}
	for _, l := range g.file.Lines {
		g.cl.lines[l.Number] = true
	}
	for _, l := range g.file.Lines {
		g.collectDecls(l.Stmts, top, g.addGlobal)
	}
	for _, l := range g.file.Lines {
		g.classicScan(l.Stmts)
	}

	var body out
	for _, l := range g.file.Lines {
		if g.cl.targets[l.Number] {
			body.w(lineLabel(l.Number), ":")
		}
		g.cblock(&body, l.Stmts, top)
	}
	body.w("return")

	g.emitGlobals(o)
	o.w("func ", g.runName(), "(c *rt.Ctx) {")
	o.w("defer c.Enter()()")
	o.w(g.classicName(), "(c, 0)")
	o.w("}")
	o.w("")
	o.w("// ", g.classicName(), " runs the program from its first line (entry 0).")
	o.w("// TODO(task 2.1): other entries are ROUTE … GOSUB targets.")
	o.w("func ", g.classicName(), "(c *rt.Ctx, entry int) {")
	o.w("defer c.Enter()()")
	for _, t := range g.cl.temps {
		o.w(t)
	}
	o.raw(strings.ReplaceAll(body.String(), returnMarker, g.returnSwitch()))
	o.w("}")
}

func lineLabel(n int) string { return "L" + strconv.Itoa(n) }

// classicScan finds every variable name (classic variables need no LET
// before they are read), every jump target and every RETURN.
func (g *gen) classicScan(stmts []ast.Stmt) {
	ident := func(e ast.Expr) {
		walkIdents(e, func(id *ast.Ident) {
			name := canon(id.Name)
			if g.globals[name] == nil {
				v := &varInfo{name: name, goName: g.mangle(name)}
				g.globals[name] = v
				g.globOrder = append(g.globOrder, v)
			}
		})
	}
	target := func(p token.Pos, n int) {
		if !g.cl.lines[n] {
			g.errorf(p, diag.Undefined, "there is no line %d to jump to", n)
			return
		}
		g.cl.targets[n] = true
	}
	for _, s := range stmts {
		switch s := s.(type) {
		case *ast.LetStmt:
			ident(s.Target)
			ident(s.Value)
		case *ast.DimStmt:
			ident(s.Value)
		case *ast.ConstDecl:
			ident(s.Value)
		case *ast.PrintStmt:
			for _, a := range s.Args {
				ident(a)
			}
		case *ast.ExprStmt:
			ident(s.X)
		case *ast.IfStmt:
			ident(s.Cond)
			g.classicScan(s.Then)
			g.classicScan(s.Else)
		case *ast.ForStmt:
			ident(s.From)
			ident(s.To)
			ident(s.Step)
			g.classicScan(s.Body)
		case *ast.ForEachStmt:
			ident(s.In)
			g.classicScan(s.Body)
		case *ast.WhileStmt:
			ident(s.Cond)
			g.classicScan(s.Body)
		case *ast.DoStmt:
			g.classicScan(s.Body)
			ident(s.Until)
		case *ast.SelectStmt:
			ident(s.X)
			for _, c := range s.Cases {
				for _, v := range c.Values {
					ident(v)
				}
				g.classicScan(c.Body)
			}
			g.classicScan(s.Else)
		case *ast.ReturnStmt:
			g.cl.hasReturn = true
		case *ast.GotoStmt:
			target(s.At, s.Line)
		case *ast.GosubStmt:
			target(s.At, s.Line)
		case *ast.OnGotoStmt:
			ident(s.X)
			for _, n := range s.Lines {
				target(s.At, n)
			}
		}
	}
}

// walkIdents calls f for every identifier read or written in e, except the
// names of called functions.
func walkIdents(e ast.Expr, f func(*ast.Ident)) {
	switch e := e.(type) {
	case *ast.Ident:
		f(e)
	case *ast.MapLit:
		for _, v := range e.Values {
			walkIdents(v, f)
		}
	case *ast.ArrayLit:
		for _, v := range e.Items {
			walkIdents(v, f)
		}
	case *ast.MemberExpr:
		walkIdents(e.X, f)
	case *ast.IndexExpr:
		walkIdents(e.X, f)
		walkIdents(e.Index, f)
	case *ast.CallExpr:
		if _, ok := e.Fn.(*ast.Ident); !ok {
			walkIdents(e.Fn, f)
		}
		for _, a := range e.Args {
			walkIdents(a, f)
		}
	case *ast.UnaryExpr:
		walkIdents(e.X, f)
	case *ast.BinaryExpr:
		walkIdents(e.X, f)
		walkIdents(e.Y, f)
	}
}

func (g *gen) returnSwitch() string {
	if len(g.cl.gosubIDs) == 0 {
		return "c.GosubPop()" // no GOSUB anywhere: always RETURN WITHOUT GOSUB
	}
	var o out
	o.w("switch c.GosubPop() {")
	for _, id := range g.cl.gosubIDs {
		o.w("case ", strconv.Itoa(id), ":")
		o.w("goto rip_g", strconv.Itoa(id))
	}
	o.raw("}")
	return o.String()
}

// temp declares a function-level temporary of Go type T.
func (g *gen) temp(T string) string {
	name := "rip_t" + g.next()
	g.cl.temps = append(g.cl.temps, "var "+name+" "+T)
	return name
}

func (g *gen) label(prefix string) string { return "rip_" + prefix + g.next() }

func (g *gen) gotoLabel(o *out, l string) {
	g.cl.used[l] = true
	o.w("goto ", l)
}

// labelIfUsed places a label that only forward jumps may use.
func (g *gen) labelIfUsed(o *out, l string) {
	if g.cl.used[l] {
		o.w(l, ":")
	}
}

func (g *gen) cblock(o *out, stmts []ast.Stmt, sc *scope) {
	for _, s := range stmts {
		g.cstmt(o, s, sc)
	}
}

// ifNot writes: if !(cond) { goto l }.
func (g *gen) ifNot(o *out, cond string, l string) {
	o.w("if !(", cond, ") {")
	g.gotoLabel(o, l)
	o.w("}")
}

func (g *gen) cstmt(o *out, s ast.Stmt, sc *scope) {
	if g.simpleStmt(o, s, sc) || g.laterStmt(s) {
		return
	}
	if d, ok := s.(*ast.FuncDecl); ok {
		g.errorf(d.At, diag.Syntax, "FUNCTION and SUB only work in structured mode; in classic mode use GOSUB")
		return
	}
	g.dir(o, s.Pos())
	switch s := s.(type) {
	case *ast.GotoStmt:
		g.guard(o)
		o.w("goto ", lineLabel(s.Line))

	case *ast.GosubStmt:
		id := len(g.cl.gosubIDs) + 1
		g.cl.gosubIDs = append(g.cl.gosubIDs, id)
		o.w("c.GosubPush(", strconv.Itoa(id), ")")
		g.guard(o)
		o.w("goto ", lineLabel(s.Line))
		if g.cl.hasReturn {
			o.w("rip_g", strconv.Itoa(id), ":")
		}

	case *ast.ReturnStmt:
		if s.Value != nil {
			g.errorf(s.At, diag.Syntax, "in classic mode RETURN goes back to the line after the GOSUB, so it takes no value")
			return
		}
		o.w(returnMarker)

	case *ast.OnGotoStmt:
		x := g.expr(s.X, sc)
		o.w("switch rt.OnIndex(", x.code, ") {")
		for i, n := range s.Lines {
			o.w("case ", strconv.Itoa(i+1), ":")
			g.guard(o)
			o.w("goto ", lineLabel(n))
		}
		o.w("}")

	case *ast.EndStmt:
		o.w("return")

	case *ast.IfStmt:
		cond := truthy(g.expr(s.Cond, sc))
		if len(s.Else) == 0 && len(s.Then) == 1 {
			if gt, ok := s.Then[0].(*ast.GotoStmt); ok && !g.opts.Guard { // IF c THEN GOTO n
				o.w("if ", cond, " {")
				g.dir(o, gt.At)
				o.w("goto ", lineLabel(gt.Line))
				o.w("}")
				return
			}
		}
		end := g.label("endif")
		if len(s.Else) == 0 {
			g.ifNot(o, cond, end)
			g.cblock(o, s.Then, sc)
		} else {
			els := g.label("else")
			g.ifNot(o, cond, els)
			g.cblock(o, s.Then, sc)
			g.gotoLabel(o, end)
			o.w(els, ":")
			g.cblock(o, s.Else, sc)
		}
		g.labelIfUsed(o, end)

	case *ast.ForStmt:
		v := g.loopVar(s.Var, s.At, sc, true)
		from, to := g.expr(s.From, sc), g.expr(s.To, sc)
		step := gexpr{"int64(1)", tInt, true}
		if s.Step != nil {
			step = g.expr(s.Step, sc)
		}
		tf, tt, ts := g.temp("rt.Value"), g.temp("rt.Value"), g.temp("rt.Value")
		g.dir(o, s.At)
		o.w(tf, " = ", from.code)
		g.dir(o, s.At)
		o.w(tt, " = ", to.code)
		g.dir(o, s.At)
		o.w(ts, " = ", step.code)
		g.dir(o, s.At)
		o.w(v.goName, " = ", coerce(gexpr{tf, tAny, false}, v.t))
		top, end := g.label("for"), g.label("next")
		o.w(top, ":")
		g.dir(o, s.At)
		g.ifNot(o, "rt.ForCond("+v.goName+", "+tt+", "+ts+")", end)
		g.cloop(o, s.Body, sc, token.FOR, end)
		g.dir(o, s.At)
		o.w(v.goName, " = ", coerce(gexpr{"rt.Add(" + v.goName + ", " + ts + ")", tAny, false}, v.t))
		g.guard(o)
		o.w("goto ", top)
		g.labelIfUsed(o, end)

	case *ast.ForEachStmt:
		v := g.loopVar(s.Var, s.At, sc, false)
		x := g.expr(s.In, sc)
		items, i := g.temp("[]rt.Value"), g.temp("int")
		g.dir(o, s.At)
		o.w(items, " = rt.Iter(", x.code, ")")
		o.w(i, " = 0")
		top, end := g.label("each"), g.label("next")
		o.w(top, ":")
		o.w("if ", i, " >= len(", items, ") {")
		g.gotoLabel(o, end)
		o.w("}")
		g.dir(o, s.At)
		o.w(v.goName, " = ", coerce(gexpr{items + "[" + i + "]", tAny, false}, v.t))
		o.w(i, "++")
		g.cloop(o, s.Body, sc, token.FOR, end)
		g.guard(o)
		o.w("goto ", top)
		g.labelIfUsed(o, end)

	case *ast.WhileStmt:
		top, end := g.label("while"), g.label("wend")
		o.w(top, ":")
		g.dir(o, s.At)
		g.ifNot(o, truthy(g.expr(s.Cond, sc)), end)
		g.cloop(o, s.Body, sc, token.WHILE, end)
		g.guard(o)
		o.w("goto ", top)
		g.labelIfUsed(o, end)

	case *ast.DoStmt:
		top, end := g.label("do"), g.label("loop")
		o.w(top, ":")
		g.cloop(o, s.Body, sc, token.DO, end)
		g.guard(o)
		if s.Until != nil {
			g.dir(o, s.Until.Pos())
			o.w("if !(", truthy(g.expr(s.Until, sc)), ") {")
			o.w("goto ", top)
			o.w("}")
		} else {
			o.w("goto ", top)
		}
		g.labelIfUsed(o, end)

	case *ast.SelectStmt:
		x := g.expr(s.X, sc)
		sel := gexpr{g.temp(x.t.goType()), x.t, false}
		g.dir(o, s.At)
		o.w(sel.code, " = ", x.code)
		o.w("_ = ", sel.code)
		end := g.label("endselect")
		for _, c := range s.Cases {
			if len(c.Values) == 0 {
				continue
			}
			conds := make([]string, len(c.Values))
			for i, v := range c.Values {
				conds[i] = g.binary(v.Pos(), token.Eq, sel, g.expr(v, sc)).code
			}
			nextCase := g.label("case")
			g.dir(o, c.Values[0].Pos())
			g.ifNot(o, strings.Join(conds, " || "), nextCase)
			g.cblock(o, c.Body, sc)
			g.gotoLabel(o, end)
			o.w(nextCase, ":")
		}
		g.cblock(o, s.Else, sc)
		g.labelIfUsed(o, end)

	case *ast.ExitStmt:
		if lp := g.findLoop(s, sc); lp != nil {
			g.gotoLabel(o, lp.label)
		}

	default:
		g.errorf(s.Pos(), kindInternal, "codegen does not handle %T in classic mode yet", s)
	}
}

func (g *gen) cloop(o *out, body []ast.Stmt, sc *scope, kind token.Kind, end string) {
	sc.loops = append(sc.loops, &loop{kind: kind, label: end})
	g.cblock(o, body, sc)
	sc.loops = sc.loops[:len(sc.loops)-1]
}
