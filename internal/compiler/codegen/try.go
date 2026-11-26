package codegen

import (
	"strconv"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Error statements: TRY … CATCH e … [FINALLY …] END TRY and THROW
// (Requirement 6; design §4.6). laterStmt in structured.go sends them here,
// and collectDecls sends TRY blocks to collectTry.
//
// Owned by the Phase 2 errors track (docs/phase2-tracks.md).
//
// A TRY compiles to immediately invoked closures:
//
//	rip_k := 1                              // how the TRY ended, see below
//	rip_v := func() (rip_r T) {             // T: the FUNCTION's result type
//		defer func() (rip_f T) {            // FINALLY: always runs
//			…
//			return
//		}()
//		var rip_e *rt.Error
//		rip_r = func() (rip_a T) {          // the TRY body
//			defer rt.Recover(&rip_e)        // stops errors; RESPOND and END pass
//			…
//			rip_k = 0
//			return
//		}()
//		if rip_e == nil {
//			return
//		}
//		rip_k = 1
//		e = rip_e.Value()                   // CATCH e
//		…
//		rip_k = 0
//		return
//	}()
//	switch rip_k {
//	case 1:
//		return rip_v                         // a RETURN inside the TRY
//	case 2:
//		break rip_loop7                      // an EXIT FOR inside the TRY
//	}
//
// RETURN inside a closure compiles as usual (returnStmt), because the
// closures have the function's own result type; it leaves rip_k at 1, and
// the code after the TRY returns the value from the function. An EXIT can't
// break a loop outside its closure, so inside the closures each enclosing
// loop kind gets a stand-in: a labeled switch that EXIT breaks, followed by
// rip_k = <code for that kind>.

// exitCodes are the rip_k values for EXIT FOR, EXIT WHILE and EXIT DO
// inside a TRY. 0 means the TRY ran to its end, 1 that it ran a RETURN.
var exitCodes = []struct {
	kind token.Kind
	code int
}{{token.FOR, 2}, {token.WHILE, 3}, {token.DO, 4}}

// errorStmt compiles TRY or THROW. g.file.Mode tells the modes apart.
func (g *gen) errorStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.ThrowStmt:
		x := g.expr(s.Value, sc)
		if x.t != tAny {
			g.errorf(s.At, diag.TypeMismatch, `THROW needs an error, like THROW ERROR("message") or THROW HTTPERROR(404, "NOT FOUND"), not %s`, typeWord(x.t))
			return
		}
		g.dir(o, s.At)
		o.w("rt.Throw(", x.code, ")")
	case *ast.TryStmt:
		if g.file.Mode == token.Classic {
			g.errorf(s.At, diag.Syntax, "TRY … CATCH only works in structured mode; remove the line numbers to use it, or let the error stop the program")
			return
		}
		g.tryStmt(o, s, sc)
	}
}

func (g *gen) tryStmt(o *out, s *ast.TryStmt, sc *scope) {
	id := g.next()
	k, v, e := "rip_k"+id, "rip_v"+id, "rip_e"+id

	var catchVar *varInfo
	if s.CatchVar != "" {
		catchVar = g.lookup(canon(s.CatchVar), sc)
		switch {
		case catchVar == nil:
			g.errorf(s.At, kindInternal, "CATCH variable %s was not collected", s.CatchVar)
		case catchVar.konst:
			g.errorf(s.At, diag.Syntax, "%s is a CONST, so CATCH cannot store the error in it; pick another name", s.CatchVar)
			catchVar = nil
		case catchVar.t != tAny || (catchVar.decl != "" && catchVar.decl != "ANY" && catchVar.decl != "MAP"):
			g.errorf(s.At, diag.TypeMismatch, "CATCH %s stores the error as a MAP, but %s is declared AS %s; pick another name", s.CatchVar, s.CatchVar, catchVar.decl)
			catchVar = nil
		}
	}

	// The closures return what the function returns, so RETURN works inside.
	results := func(name string) string { return "" }
	if sc.fn != nil && !sc.fn.isSub {
		T := sc.fn.result.goType()
		results = func(name string) string { return " (" + name + " " + T + ")" }
	}
	outer := map[token.Kind]*loop{}
	for i := len(sc.loops) - 1; i >= 0; i-- {
		if outer[sc.loops[i].kind] == nil {
			outer[sc.loops[i].kind] = sc.loops[i]
		}
	}

	body, bodyExits := g.tryBlock(s.Body, sc, outer)
	catch, catchExits := g.tryBlock(s.Catch, sc, outer)
	var finally out
	if len(s.Finally) > 0 && g.finallyStaysPut(s.Finally) {
		saved := sc.loops
		sc.loops = nil
		g.nested(&finally, s.Finally, sc)
		sc.loops = saved
	}

	used := map[token.Kind]bool{}
	for _, lp := range append(bodyExits, catchExits...) {
		if lp.used {
			used[lp.kind] = true
		}
	}
	needK := sc.fn != nil || len(used) > 0
	if !needK {
		k = ""
	}

	g.dir(o, s.At)
	o.w("{")
	if needK {
		o.w(k, " := 1")
	}
	if r := results("rip_r" + id); r != "" {
		o.w(v, " := func()", r, " {")
	} else {
		o.w("func() {")
	}
	if len(s.Finally) > 0 {
		o.w("defer func()", results("rip_f"+id), " {")
		o.appendOut(&finally)
		g.dir(o, s.At)
		o.w("return")
		o.w("}()")
	}
	g.dir(o, s.At)
	o.w("var ", e, " *rt.Error")
	if r := results("rip_a" + id); r != "" {
		o.w("rip_r", id, " = func()", r, " {")
	} else {
		o.w("func() {")
	}
	o.w("defer rt.Recover(&", e, ")")
	o.raw(withExits(body, bodyExits, k))
	o.w("}()")
	g.dir(o, s.At)
	o.w("if ", e, " == nil {")
	o.w("return")
	o.w("}")
	if needK {
		o.w(k, " = 1")
	}
	if catchVar != nil {
		o.w(catchVar.goName, " = ", e, ".Value()")
	}
	o.raw(withExits(catch, catchExits, k))
	o.w("}()")

	if needK {
		g.dir(o, s.At)
		o.w("switch ", k, " {")
		if sc.fn != nil {
			o.w("case 1:")
			if sc.fn.isSub {
				o.w("return")
			} else {
				o.w("return ", v)
			}
		}
		for _, x := range exitCodes {
			if used[x.kind] {
				lp := outer[x.kind]
				lp.used = true
				o.w("case ", strconv.Itoa(x.code), ":")
				o.w("break ", lp.label)
			}
		}
		o.w("}")
	}
	o.w("}")
}

// tryBlock compiles the TRY body or the CATCH body for its closure. EXIT
// can't break a loop outside the closure, so it breaks a stand-in for each
// kind of loop around the TRY; exits lists them, with used set by EXIT.
func (g *gen) tryBlock(stmts []ast.Stmt, sc *scope, outer map[token.Kind]*loop) (string, []*loop) {
	saved := sc.loops
	var exits []*loop
	for _, x := range exitCodes {
		if outer[x.kind] != nil {
			exits = append(exits, &loop{kind: x.kind, label: "rip_x" + g.next()})
		}
	}
	sc.loops = append([]*loop(nil), exits...)
	var b out
	g.nested(&b, stmts, sc)
	sc.loops = saved
	return b.String(), exits
}

// withExits ends a TRY or CATCH closure: rip_k = 0 when its code runs to the
// end, and the labeled stand-ins that its EXITs break out of. k is "" when
// nothing reads rip_k.
func withExits(code string, exits []*loop, k string) string {
	var b out
	b.raw(code)
	if k != "" {
		b.w(k, " = 0")
	}
	b.w("return")
	s := b.String()
	for _, lp := range exits {
		if !lp.used {
			continue
		}
		var w out
		w.w(lp.label, ":")
		w.w("switch {")
		w.w("default:")
		w.raw(s)
		w.w("}")
		for _, x := range exitCodes {
			if x.kind == lp.kind {
				w.w(k, " = ", strconv.Itoa(x.code))
			}
		}
		w.w("return")
		s = w.String()
	}
	return s
}

// finallyStaysPut reports RETURN and EXIT statements that would leave a
// FINALLY block. FINALLY runs while an error or RESPOND is on its way out,
// so it always runs to its end.
func (g *gen) finallyStaysPut(stmts []ast.Stmt) bool {
	ok := true
	var walk func(stmts []ast.Stmt, loops []token.Kind)
	walk = func(stmts []ast.Stmt, loops []token.Kind) {
		inLoop := func(k token.Kind) []token.Kind { return append(loops[:len(loops):len(loops)], k) }
		for _, s := range stmts {
			switch s := s.(type) {
			case *ast.ReturnStmt:
				g.errorf(s.At, diag.Syntax, "RETURN cannot leave a FINALLY block; set a variable in FINALLY and RETURN after END TRY")
				ok = false
			case *ast.ExitStmt:
				inside := false
				for _, k := range loops {
					inside = inside || k == s.Loop
				}
				if !inside {
					g.errorf(s.At, diag.Syntax, "EXIT %s cannot leave a FINALLY block; FINALLY always runs to its end", s.Loop)
					ok = false
				}
			case *ast.IfStmt:
				walk(s.Then, loops)
				walk(s.Else, loops)
			case *ast.ForStmt:
				walk(s.Body, inLoop(token.FOR))
			case *ast.ForEachStmt:
				walk(s.Body, inLoop(token.FOR))
			case *ast.WhileStmt:
				walk(s.Body, inLoop(token.WHILE))
			case *ast.DoStmt:
				walk(s.Body, inLoop(token.DO))
			case *ast.SelectStmt:
				for _, c := range s.Cases {
					walk(c.Body, loops)
				}
				walk(s.Else, loops)
			case *ast.TryStmt:
				walk(s.Body, loops)
				walk(s.Catch, loops)
				// A nested FINALLY is checked when its own TRY compiles.
			case *ast.TransactionStmt:
				walk(s.Body, loops)
			}
		}
	}
	walk(stmts, nil)
	return ok
}

// typeWord names a native type in a diagnostic, with its article.
func typeWord(t typ) string {
	switch t {
	case tInt:
		return "an INTEGER"
	case tNum:
		return "a NUMBER"
	case tStr:
		return "a STRING"
	case tBool:
		return "a BOOLEAN"
	case tDec:
		return "a DECIMAL"
	}
	return "a value"
}

// collectTry reports the names a TRY statement introduces, for collectDecls:
// the CATCH variable is assigned like a LET, so it is a local inside a
// FUNCTION and a module-level variable in top-level code.
func (g *gen) collectTry(s *ast.TryStmt, sc *scope, add func(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope)) {
	g.collectDecls(s.Body, sc, add)
	if s.CatchVar != "" {
		add(declLet, canon(s.CatchVar), s.At, s, sc)
	}
	g.collectDecls(s.Catch, sc, add)
	g.collectDecls(s.Finally, sc, add)
}
