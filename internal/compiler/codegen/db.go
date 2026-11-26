package codegen

import (
	"strconv"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// dbStmt compiles DBCONNECT (task 3.1) and TRANSACTION (task 3.2), called from
// laterStmt in structured.go (and available in both modes).
func (g *gen) dbStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.DBConnectStmt:
		g.dbConnectStmt(o, s, sc)
	case *ast.TransactionStmt:
		if g.file.Mode == token.Classic {
			g.errorf(s.At, diag.Syntax, "TRANSACTION only works in structured mode; remove the line numbers to use it")
			return
		}
		g.transactionStmt(o, s, sc)
	}
}

func (g *gen) dbConnectStmt(o *out, s *ast.DBConnectStmt, sc *scope) {
	url := coerce(g.expr(s.URL, sc), tStr)
	g.dir(o, s.At)
	o.w("rt.DBConnect(c, ", url, ", ", strconv.Quote(s.Name), ")")
}

// transactionStmt compiles TRANSACTION [ON name] … END TRANSACTION to
// rt.Tx(c, name, func() { … }). It mirrors try.go so RETURN and EXIT inside
// the transaction body commit the transaction cleanly and then propagate out
// of the enclosing function or loop.
func (g *gen) transactionStmt(o *out, s *ast.TransactionStmt, sc *scope) {
	id := g.next()
	k, v := "rip_k"+id, "rip_v"+id

	outer := map[token.Kind]*loop{}
	for i := len(sc.loops) - 1; i >= 0; i-- {
		if outer[sc.loops[i].kind] == nil {
			outer[sc.loops[i].kind] = sc.loops[i]
		}
	}

	body, bodyExits := g.tryBlock(s.Body, sc, outer)

	used := map[token.Kind]bool{}
	for _, lp := range bodyExits {
		if lp.used {
			used[lp.kind] = true
		}
	}
	needK := sc.fn != nil || len(used) > 0
	if !needK {
		k = ""
	}
	hasResult := sc.fn != nil && !sc.fn.isSub

	g.dir(o, s.At)
	if !needK {
		o.w("rt.Tx(c, ", strconv.Quote(s.Name), ", func() {")
		o.raw(withExits(body, bodyExits, ""))
		o.w("})")
		return
	}

	o.w("{")
	o.w(k, " := 1")
	if hasResult {
		o.w("var ", v, " ", sc.fn.result.goType())
	}
	g.dir(o, s.At)
	o.w("rt.Tx(c, ", strconv.Quote(s.Name), ", func() {")
	if hasResult {
		o.w(v, " = func() (rip_r", id, " ", sc.fn.result.goType(), ") {")
		o.raw(withExits(body, bodyExits, k))
		o.w("}()")
	} else {
		o.raw(withExits(body, bodyExits, k))
	}
	o.w("})")

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
	o.w("}")
}
