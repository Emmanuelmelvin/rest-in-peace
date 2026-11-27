package codegen

// shared.go — codegen support for SHARED DIM (Requirement 14.4, task 4.3).
//
// SHARED DIM x AS T [= v] compiles to:
//   - A package-level variable of type rt.Shared[GoT] (instead of GoT).
//   - Top-level initialisation via b_mod_x.Store(initVal) (or zeroFor(v)).
//   - Reads of x become b_mod_x.Load() (or b_mod_x.GetMember / GetIndex for
//     member/index reads on a shared map or array).
//   - Writes of x become b_mod_x.Store(newVal), or b_mod_x.Modify(func(old GoT)
//     GoT { return ... }) when the right-hand side reads x without calling
//     functions or locking another shared variable (atomic read-modify-write,
//     deadlock-free). Member/index writes become b_mod_x.SetMember / SetIndex.
//
// Integration with structured.go and expr.go (one-line hooks only):
//   - structured.go/declare       calls g.markSharedDecl(d, v)
//   - structured.go/collectLocals checks g.isSharedVar(gv)
//   - structured.go/emitGlobals   calls g.globalGoType(v)
//   - structured.go/simpleStmt    calls g.sharedDim(o, s, sc)
//   - structured.go/assign        calls g.sharedAssignHook(o, p, target, value, sc)
//   - expr.go/expr (MemberExpr)   calls g.sharedMemberHook(e, sc)
//   - expr.go/expr (IndexExpr)    calls g.sharedIndexHook(e, sc)
//   - expr.go/ident               calls g.sharedLoadHook(v)

import (
	"strconv"
	"sync"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

type sharedState struct {
	vars         map[*varInfo]bool   // global *varInfo pointers declared SHARED DIM
	activeModify map[*varInfo]string // during Modify(func(oldVar T) T), maps v -> oldVar
}

// sharedGens maps each *gen to its per-generator sharedState.
var sharedGens sync.Map // map[*gen]*sharedState

func (g *gen) getSharedState() *sharedState {
	raw, _ := sharedGens.LoadOrStore(g, &sharedState{
		vars:         map[*varInfo]bool{},
		activeModify: map[*varInfo]string{},
	})
	return raw.(*sharedState)
}

// markSharedDecl records v as a SHARED variable when d.Shared is true.
// Keyed by *varInfo pointer so local parameters or DIMs with the same name
// are never confused with the shared global.
func (g *gen) markSharedDecl(d *ast.DimStmt, v *varInfo) {
	if d.Shared {
		g.getSharedState().vars[v] = true
	}
}

// isSharedVar reports whether v is a SHARED global variable in this generator.
func (g *gen) isSharedVar(v *varInfo) bool {
	if v == nil {
		return false
	}
	raw, ok := sharedGens.Load(g)
	if !ok {
		return false
	}
	return raw.(*sharedState).vars[v]
}

// globalGoType returns "rt.Shared[T]" for SHARED globals and "T" otherwise.
func (g *gen) globalGoType(v *varInfo) string {
	if g.isSharedVar(v) {
		return "rt.Shared[" + v.t.goType() + "]"
	}
	return v.t.goType()
}

// sharedDim compiles SHARED DIM x AS T [= v] in the module's run function.
func (g *gen) sharedDim(o *out, s *ast.DimStmt, sc *scope) {
	v := g.globals[canon(s.Name)]
	if v == nil {
		g.errorf(s.At, kindInternal, "SHARED DIM %s was not collected", s.Name)
		return
	}
	g.markSharedDecl(s, v)
	g.dir(o, s.At)
	if s.Value != nil {
		init := g.expr(s.Value, sc)
		o.w(v.goName, ".Store(", coerceDecl(init, v.t, v.decl), ")")
	} else {
		o.w(v.goName, ".Store(", zeroFor(v), ")")
	}
}

// sharedAssignHook handles assignments targeting a SHARED variable (bare,
// member, or index). Returns true when handled.
func (g *gen) sharedAssignHook(o *out, p token.Pos, target, value ast.Expr, sc *scope) bool {
	switch t := target.(type) {
	case *ast.Ident:
		v := g.lookup(canon(t.Name), sc)
		if !g.isSharedVar(v) {
			return false
		}
		if g.canAtomicModify(value, v, sc) {
			st := g.getSharedState()
			oldVar := "rip_old" + g.next()
			T := v.t.goType()
			st.activeModify[v] = oldVar
			val := g.expr(value, sc)
			delete(st.activeModify, v)
			o.w(v.goName, ".Modify(func(", oldVar, " ", T, ") ", T, " {")
			o.w("return ", coerceDecl(val, v.t, v.decl))
			o.w("})")
			return true
		}
		val := g.expr(value, sc)
		o.w(v.goName, ".Store(", coerceDecl(val, v.t, v.decl), ")")
		return true

	case *ast.MemberExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			v := g.lookup(canon(id.Name), sc)
			if g.isSharedVar(v) {
				val := g.expr(value, sc)
				o.w(v.goName, ".SetMember(", strconv.Quote(t.Name), ", ", val.code, ")")
				return true
			}
		}

	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			v := g.lookup(canon(id.Name), sc)
			if g.isSharedVar(v) {
				idx := g.expr(t.Index, sc)
				val := g.expr(value, sc)
				o.w(v.goName, ".SetIndex(", idx.code, ", ", val.code, ")")
				return true
			}
		}
	}
	return false
}

// sharedLoadHook intercepts reads of a SHARED variable. Inside Modify for v,
// it substitutes the Modify callback's parameter (avoiding self-deadlock).
func (g *gen) sharedLoadHook(v *varInfo) (gexpr, bool) {
	if !g.isSharedVar(v) {
		return gexpr{}, false
	}
	if oldVar, active := g.getSharedState().activeModify[v]; active {
		return gexpr{code: oldVar, t: v.t, konst: false}, true
	}
	return gexpr{code: v.goName + ".Load()", t: v.t, konst: false}, true
}

// sharedMemberHook intercepts direct member reads (x.field) on a SHARED
// variable so the read executes while holding x's mutex.
func (g *gen) sharedMemberHook(e *ast.MemberExpr, sc *scope) (gexpr, bool) {
	id, ok := e.X.(*ast.Ident)
	if !ok {
		return gexpr{}, false
	}
	v := g.lookup(canon(id.Name), sc)
	if !g.isSharedVar(v) {
		return gexpr{}, false
	}
	if _, active := g.getSharedState().activeModify[v]; active {
		return gexpr{}, false
	}
	return gexpr{code: v.goName + ".GetMember(" + strconv.Quote(e.Name) + ")", t: tAny, konst: false}, true
}

// sharedIndexHook intercepts direct index reads (x[i]) on a SHARED variable so
// the read executes while holding x's mutex.
func (g *gen) sharedIndexHook(e *ast.IndexExpr, sc *scope) (gexpr, bool) {
	id, ok := e.X.(*ast.Ident)
	if !ok {
		return gexpr{}, false
	}
	v := g.lookup(canon(id.Name), sc)
	if !g.isSharedVar(v) {
		return gexpr{}, false
	}
	if _, active := g.getSharedState().activeModify[v]; active {
		return gexpr{}, false
	}
	idx := g.expr(e.Index, sc)
	return gexpr{code: v.goName + ".GetIndex(" + idx.code + ")", t: tAny, konst: false}, true
}

// canAtomicModify reports whether `v = expr` can safely compile to
// `v.Modify(func(old T) T { return ... })`. It requires that:
//   - expr references v (otherwise Store is simpler and sufficient),
//   - expr contains no function/method calls (which could re-enter v or lock
//     another shared variable), and
//   - expr does not reference any other SHARED variable (preventing lock-order
//     deadlocks).
func (g *gen) canAtomicModify(e ast.Expr, target *varInfo, sc *scope) bool {
	usesTarget := false
	unsafe := false
	var walk func(ast.Expr)
	walk = func(x ast.Expr) {
		if x == nil || unsafe {
			return
		}
		switch n := x.(type) {
		case *ast.Ident:
			v := g.lookup(canon(n.Name), sc)
			if v == target {
				usesTarget = true
			} else if g.isSharedVar(v) {
				unsafe = true
			}
		case *ast.CallExpr:
			unsafe = true
		case *ast.UnaryExpr:
			walk(n.X)
		case *ast.BinaryExpr:
			walk(n.X)
			walk(n.Y)
		case *ast.MemberExpr:
			walk(n.X)
		case *ast.IndexExpr:
			walk(n.X)
			walk(n.Index)
		case *ast.MapLit:
			for _, v := range n.Values {
				walk(v)
			}
		case *ast.ArrayLit:
			for _, it := range n.Items {
				walk(it)
			}
		}
	}
	walk(e)
	return usesTarget && !unsafe
}
