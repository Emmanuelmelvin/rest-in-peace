package codegen

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// gexpr is a generated Go expression and its static type. Native-typed
// expressions (t != tAny) always have a definite Go type, never an untyped
// constant, so they box to the right dynamic type when stored in an rt.Value.
type gexpr struct {
	code  string
	t     typ
	konst bool // a Go constant expression
}

var bad = gexpr{code: "nil", t: tAny}

// builtin is a built-in function implemented in rt.
type builtin struct {
	fn       string
	min, max int
	t        typ
	ctx      bool  // pass c *rt.Ctx as the first argument
	args     []typ // Go types of the leading parameters; the rest are rt.Value
}

var builtins = map[string]builtin{
	// DECIMAL (task 2.5, rt/decimal.go).
	"DECIMAL": {fn: "rt.DecimalOf", min: 1, max: 1, t: tDec},
	"ROUND":   {fn: "rt.Round", min: 1, max: 2, t: tAny},
	// ENV (task 2.4, rt/env.go).
	"ENV":        {fn: "rt.Env", min: 1, max: 2, t: tAny, args: []typ{tStr}},
	"ENVREQUIRE": {fn: "rt.EnvRequire", min: 1, max: 1, t: tStr, args: []typ{tStr}},
	// Classic-mode request helpers (task 2.1, rt/request.go).
	"PARAM":  {fn: "rt.Param", min: 1, max: 1, t: tAny, ctx: true},
	"QUERY":  {fn: "rt.Query", min: 1, max: 1, t: tAny, ctx: true},
	"HEADER": {fn: "rt.Header", min: 1, max: 1, t: tAny, ctx: true},
	"BODY":   {fn: "rt.Body", min: 0, max: 0, t: tAny, ctx: true},
	// Error values for THROW (task 2.2, rt/errors.go).
	"ERROR":     {fn: "rt.NewError", min: 1, max: 1, t: tAny},
	"HTTPERROR": {fn: "rt.NewHTTPError", min: 2, max: 3, t: tAny},

	// Phase 1 (rt/builtins.go).
	"LEN":   {fn: "rt.Len", min: 1, max: 1, t: tInt},
	"UPPER": {fn: "rt.Upper", min: 1, max: 1, t: tStr},
	"LOWER": {fn: "rt.Lower", min: 1, max: 1, t: tStr},
	"TRIM":  {fn: "rt.Trim", min: 1, max: 1, t: tStr},
	"LEFT":  {fn: "rt.Left", min: 2, max: 2, t: tStr},
	"RIGHT": {fn: "rt.Right", min: 2, max: 2, t: tStr},
	"MID":   {fn: "rt.Mid", min: 2, max: 3, t: tStr},
	"STR":   {fn: "rt.Str", min: 1, max: 1, t: tStr},
	"VAL":   {fn: "rt.Val", min: 1, max: 1, t: tAny},
	"INT":   {fn: "rt.Int", min: 1, max: 1, t: tInt},
	"ABS":   {fn: "rt.Abs", min: 1, max: 1, t: tAny},
}

// laterBuiltins are the language.md built-ins that later tasks add.
var laterBuiltins = map[string]string{
	"SQL": "3.1", "SQLONE": "3.1", "SQLEXEC": "3.1",
	"FETCH":   "3.7",
	"REQUEST": "4.2",
	"NEW":     "3.5",
}

func isBuiltin(name string) bool {
	_, ok := builtins[name]
	_, later := laterBuiltins[name]
	_, mw := middlewareFactories[name]
	return ok || later || mw || name == "NOW"
}

// lookup finds a variable or constant visible in sc.
func (g *gen) lookup(name string, sc *scope) *varInfo {
	if sc.fn != nil {
		if v, ok := sc.fn.locals[name]; ok {
			return v
		}
	}
	return g.globals[name]
}

// expr compiles an expression.
func (g *gen) expr(e ast.Expr, sc *scope) gexpr {
	switch e := e.(type) {
	case *ast.NumberLit:
		return g.number(e)
	case *ast.StringLit:
		return gexpr{strconv.Quote(e.Value), tStr, true}
	case *ast.BoolLit:
		return gexpr{strconv.FormatBool(e.Value), tBool, true}
	case *ast.NilLit:
		return gexpr{"nil", tAny, false}
	case *ast.MapLit:
		if len(e.Keys) != len(e.Values) {
			g.errorf(e.At, kindInternal, "map literal has %d keys and %d values", len(e.Keys), len(e.Values))
			return bad
		}
		parts := make([]string, 0, 2*len(e.Keys))
		for i, k := range e.Keys {
			parts = append(parts, strconv.Quote(k), g.expr(e.Values[i], sc).code)
		}
		return gexpr{"rt.MapOf(" + strings.Join(parts, ", ") + ")", tAny, false}
	case *ast.ArrayLit:
		parts := make([]string, len(e.Items))
		for i, it := range e.Items {
			parts[i] = g.expr(it, sc).code
		}
		return gexpr{"rt.ArrayOf(" + strings.Join(parts, ", ") + ")", tAny, false}
	case *ast.Ident:
		return g.ident(e, sc)
	case *ast.MemberExpr:
		x := g.expr(e.X, sc)
		return gexpr{"rt.Get(" + x.code + ", " + strconv.Quote(e.Name) + ")", tAny, false}
	case *ast.IndexExpr:
		x := g.expr(e.X, sc)
		i := g.expr(e.Index, sc)
		return gexpr{"rt.Index(" + x.code + ", " + i.code + ")", tAny, false}
	case *ast.CallExpr:
		return g.call(e, sc, false)
	case *ast.UnaryExpr:
		return g.unary(e, sc)
	case *ast.BinaryExpr:
		x := g.expr(e.X, sc)
		y := g.expr(e.Y, sc)
		return g.binary(e.At, e.Op, x, y)
	case nil:
		g.errorf(token.Pos{}, kindInternal, "missing expression")
		return bad
	}
	g.errorf(e.Pos(), kindInternal, "codegen does not handle %T yet", e)
	return bad
}

func (g *gen) number(e *ast.NumberLit) gexpr {
	if e.IsInt {
		n, err := strconv.ParseInt(e.Text, 10, 64)
		if err != nil {
			g.errorf(e.At, diag.TypeMismatch, "%s is too big for an INTEGER (the limit is 9223372036854775807)", e.Text)
			return bad
		}
		return gexpr{"int64(" + strconv.FormatInt(n, 10) + ")", tInt, true}
	}
	f, err := strconv.ParseFloat(e.Text, 64)
	if err != nil {
		g.errorf(e.At, diag.TypeMismatch, "%s is not a NUMBER rip can store", e.Text)
		return bad
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	return gexpr{"float64(" + s + ")", tNum, true}
}

func (g *gen) ident(e *ast.Ident, sc *scope) gexpr {
	name := canon(e.Name)
	if v := g.lookup(name, sc); v != nil {
		return gexpr{v.goName, v.t, v.goConst != ""}
	}
	switch {
	case g.funcs[name] != nil:
		g.errorf(e.At, diag.Undefined, "%s is a FUNCTION; call it with %s(…)", e.Name, e.Name)
	case isBuiltin(name):
		g.errorf(e.At, diag.Undefined, "%s is a built-in function; call it with %s(…)", e.Name, e.Name)
	default:
		g.errorf(e.At, diag.Undefined, "there is no variable named %s; give it a value with LET or DIM first", e.Name)
	}
	return bad
}

// call compiles a call. asStmt allows calling a SUB.
func (g *gen) call(e *ast.CallExpr, sc *scope, asStmt bool) gexpr {
	id, ok := e.Fn.(*ast.Ident)
	if !ok {
		g.todo(e.At, "3.5", "calling a method or a function value")
		return bad
	}
	name := canon(id.Name)
	if fn := g.funcs[name]; fn != nil {
		return g.callUser(e, fn, sc, asStmt)
	}
	if b, ok := builtins[name]; ok {
		if len(e.Args) < b.min || len(e.Args) > b.max {
			want := strconv.Itoa(b.min) + " argument"
			if b.max != b.min {
				want = strconv.Itoa(b.min) + " or " + strconv.Itoa(b.max) + " arguments"
			} else if b.min != 1 {
				want += "s"
			}
			g.errorf(e.At, diag.Syntax, "%s takes %s, not %d", name, want, len(e.Args))
			return bad
		}
		var args []string
		if b.ctx {
			args = append(args, "c")
		}
		for i, a := range e.Args {
			want := tAny
			if i < len(b.args) {
				want = b.args[i]
			}
			args = append(args, coerce(g.expr(a, sc), want))
		}
		return gexpr{b.fn + "(" + strings.Join(args, ", ") + ")", b.t, false}
	}
	if _, ok := middlewareFactories[name]; ok {
		g.errorf(e.At, diag.Syntax, "%s is a middleware built-in; register it with USE %s(…)", name, name)
		return bad
	}
	if task, ok := laterBuiltins[name]; ok {
		g.todo(e.At, task, name)
		return bad
	}
	if name == "NOW" {
		g.errorf(e.At, diag.NotImplemented, "NOW is not built yet (no task in .kiro/specs/rip-basic/tasks.md covers DATE values yet)")
		return bad
	}
	if g.file.Mode == token.Classic {
		g.errorf(e.At, diag.Undefined, "there is no built-in function named %s; classic mode has no FUNCTIONs, so use GOSUB", id.Name)
	} else {
		g.errorf(e.At, diag.Undefined, "there is no FUNCTION or SUB named %s", id.Name)
	}
	return bad
}

func (g *gen) callUser(e *ast.CallExpr, fn *fnInfo, sc *scope, asStmt bool) gexpr {
	if fn.isSub && !asStmt {
		g.errorf(e.At, diag.TypeMismatch, "%s is a SUB, so it has no value; make it a FUNCTION to use its result", fn.decl.Name)
		return bad
	}
	if len(e.Args) > len(fn.params) {
		g.errorf(e.At, diag.Syntax, "%s takes at most %d arguments, not %d", fn.decl.Name, len(fn.params), len(e.Args))
		return bad
	}
	args := []string{"c"}
	top := &scope{}
	for i, p := range fn.params {
		var a gexpr
		switch {
		case i < len(e.Args):
			a = g.expr(e.Args[i], sc)
		case fn.decl.Params[i].Default != nil:
			// Defaults are evaluated at the call site, against module-level names.
			a = g.expr(fn.decl.Params[i].Default, top)
		default:
			g.errorf(e.At, diag.Syntax, "%s needs a value for %s (argument %d)", fn.decl.Name, fn.decl.Params[i].Name, i+1)
			return bad
		}
		args = append(args, coerceDecl(a, p.t, p.decl))
	}
	return gexpr{fn.goName + "(" + strings.Join(args, ", ") + ")", fn.result, false}
}

func (g *gen) unary(e *ast.UnaryExpr, sc *scope) gexpr {
	if e.Op == token.Minus {
		if lit, ok := e.X.(*ast.NumberLit); ok && lit.IsInt && lit.Text == "9223372036854775808" {
			return gexpr{"int64(-9223372036854775808)", tInt, true}
		}
	}
	x := g.expr(e.X, sc)
	switch e.Op {
	case token.Minus:
		if x.t.numeric() {
			return gexpr{"(-" + x.code + ")", x.t, x.konst}
		}
		return gexpr{"rt.Neg(" + x.code + ")", tAny, false}
	case token.Plus:
		return x
	case token.NOT:
		return gexpr{"(!" + truthy(x) + ")", tBool, x.konst && x.t == tBool}
	}
	g.errorf(e.At, kindInternal, "unknown unary operator %s", e.Op)
	return bad
}

// binary compiles x op y: plain Go operators when both sides are native, rt
// helpers otherwise (design §4.2).
func (g *gen) binary(p token.Pos, op token.Kind, x, y gexpr) gexpr {
	both := x.konst && y.konst
	num := x.t.numeric() && y.t.numeric()
	switch op {
	case token.AND, token.OR:
		goOp := " && "
		if op == token.OR {
			goOp = " || "
		}
		return gexpr{"(" + truthy(x) + goOp + truthy(y) + ")", tBool, both && x.t == tBool && y.t == tBool}

	case token.Amp:
		if x.t == tStr && y.t == tStr {
			return gexpr{"(" + x.code + " + " + y.code + ")", tStr, both}
		}
		return gexpr{"rt.Concat(" + x.code + ", " + y.code + ")", tStr, false}

	case token.Plus:
		if x.t == tStr || y.t == tStr { // + concatenates when either side is a STRING
			return gexpr{"(" + asText(x) + " + " + asText(y) + ")", tStr, both && x.t == tStr && y.t == tStr}
		}
		if num {
			return nativeArith("+", x, y)
		}
		return gexpr{"rt.Add(" + x.code + ", " + y.code + ")", tAny, false}

	case token.Minus, token.Star:
		goOp, helper := "-", "rt.Sub"
		if op == token.Star {
			goOp, helper = "*", "rt.Mul"
		}
		if num {
			return nativeArith(goOp, x, y)
		}
		return gexpr{helper + "(" + x.code + ", " + y.code + ")", tAny, false}

	case token.Slash:
		if num {
			return gexpr{"rt.DivNum(" + asFloat(x) + ", " + asFloat(y) + ")", tNum, false}
		}
		return gexpr{"rt.Div(" + x.code + ", " + y.code + ")", tAny, false}

	case token.Backslash:
		if x.t == tInt && y.t == tInt {
			return gexpr{"rt.IntDivInt(" + x.code + ", " + y.code + ")", tInt, false}
		}
		return gexpr{"rt.IntDiv(" + x.code + ", " + y.code + ")", tAny, false}

	case token.MOD:
		if x.t == tInt && y.t == tInt {
			return gexpr{"rt.ModInt(" + x.code + ", " + y.code + ")", tInt, false}
		}
		if num {
			return gexpr{"rt.ModNum(" + asFloat(x) + ", " + asFloat(y) + ")", tNum, false}
		}
		return gexpr{"rt.Mod(" + x.code + ", " + y.code + ")", tAny, false}

	case token.Caret:
		if num && !(x.t == tInt && y.t == tInt) {
			return gexpr{"rt.PowNum(" + asFloat(x) + ", " + asFloat(y) + ")", tNum, false}
		}
		return gexpr{"rt.Pow(" + x.code + ", " + y.code + ")", tAny, false}

	case token.Eq, token.NotEq, token.Lt, token.Gt, token.LtEq, token.GtEq:
		goOp := map[token.Kind]string{token.Eq: "==", token.NotEq: "!=", token.Lt: "<", token.Gt: ">", token.LtEq: "<=", token.GtEq: ">="}[op]
		switch {
		case num:
			a, b := x.code, y.code
			if x.t != y.t {
				a, b = asFloat(x), asFloat(y)
			}
			return gexpr{"(" + a + " " + goOp + " " + b + ")", tBool, both}
		case x.t == tStr && y.t == tStr,
			x.t == tBool && y.t == tBool && (op == token.Eq || op == token.NotEq):
			return gexpr{"(" + x.code + " " + goOp + " " + y.code + ")", tBool, both}
		}
		helper := map[token.Kind]string{token.Eq: "rt.Eq", token.NotEq: "rt.Ne", token.Lt: "rt.Lt", token.Gt: "rt.Gt", token.LtEq: "rt.Le", token.GtEq: "rt.Ge"}[op]
		return gexpr{helper + "(" + x.code + ", " + y.code + ")", tBool, false}
	}
	g.errorf(p, kindInternal, "unknown operator %s", op)
	return bad
}

func nativeArith(goOp string, x, y gexpr) gexpr {
	if x.t == tInt && y.t == tInt {
		return gexpr{"(" + x.code + " " + goOp + " " + y.code + ")", tInt, x.konst && y.konst}
	}
	return gexpr{"(" + asFloat(x) + " " + goOp + " " + asFloat(y) + ")", tNum, x.konst && y.konst}
}

func asFloat(x gexpr) string {
	if x.t == tInt {
		return "float64(" + x.code + ")"
	}
	return x.code
}

func asText(x gexpr) string {
	if x.t == tStr {
		return x.code
	}
	return "rt.ToString(" + x.code + ")"
}

// truthy is a condition as a Go bool.
func truthy(x gexpr) string {
	if x.t == tBool {
		return x.code
	}
	return "rt.Truthy(" + x.code + ")"
}

// coerceDecl converts x for a destination with Go type want and declared
// BASIC type decl, enforcing runtime type checks on dynamic boundaries such
// as AS MAP, AS ARRAY, AS DATE and AS NIL (task 2.8, Requirement 3.2).
func coerceDecl(x gexpr, want typ, decl string) string {
	if want != tAny {
		return coerce(x, want)
	}
	switch decl {
	case "MAP":
		return "rt.CheckMap(" + x.code + ")"
	case "ARRAY":
		return "rt.CheckArray(" + x.code + ")"
	case "DATE":
		return "rt.CheckDate(" + x.code + ")"
	case "NIL":
		return "rt.CheckNil(" + x.code + ")"
	}
	return x.code
}

// coerce converts x for a destination of type want. Converting a dynamic
// value to a native type checks it at run time (Requirement 3.2).
func coerce(x gexpr, want typ) string {
	if x.t == want || want == tAny {
		return x.code
	}
	switch want {
	case tNum:
		if x.t == tInt {
			return "float64(" + x.code + ")"
		}
		return "rt.AsNum(" + x.code + ")"
	case tInt:
		return "rt.AsInt(" + x.code + ")"
	case tStr:
		return "rt.AsStr(" + x.code + ")"
	case tBool:
		return "rt.AsBool(" + x.code + ")"
	case tDec:
		return "rt.AsDecimal(" + x.code + ")"
	}
	return x.code
}
