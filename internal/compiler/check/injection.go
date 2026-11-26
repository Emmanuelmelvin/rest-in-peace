package check

import (
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// checkSQLInjection warns when a SQL, SQLONE or SQLEXEC query string is built
// with + or & from a value that came from req (Requirement 9.4, task 3.6).
//
// Kiro owns this file and injection_test.go (.kiro/steering/structure.md).
// Called from Check in check.go; replace this stub body with the full check.
func checkSQLInjection(p *ast.Program) diag.List {
	var ds diag.List
	for _, f := range p.Files {
		if f == nil {
			continue
		}
		a := &injectionAnalyzer{file: f}
		ds = append(ds, a.analyzeFile()...)
	}
	return ds
}

// injectionAnalyzer walks one file looking for tainted SQL query strings.
type injectionAnalyzer struct {
	file    *ast.File
	classic bool
}

func (a *injectionAnalyzer) analyzeFile() diag.List {
	a.classic = a.file.Mode == token.Classic
	var ds diag.List
	if a.classic {
		// All classic-mode lines share one global scope (variables are global),
		// so we use a single env across all lines to track transitive taint.
		env := newTaintEnv(nil)
		for _, ln := range a.file.Lines {
			ds = append(ds, a.scanStmts(ln.Stmts, env)...)
		}
	} else {
		// Module-level taint env shared across top-level statements.
		modEnv := newTaintEnv(nil)
		for _, s := range a.file.Body {
			switch d := s.(type) {
			case *ast.FuncDecl:
				ds = append(ds, a.analyzeFunc(d)...)
			default:
				ds = append(ds, a.scanStmts([]ast.Stmt{s}, modEnv)...)
			}
		}
	}
	return ds
}

func (a *injectionAnalyzer) analyzeFunc(fn *ast.FuncDecl) diag.List {
	env := newTaintEnv(nil)
	// The conventional first parameter of a handler is the request object.
	// We taint the parameter name itself so member access on it is tainted.
	if len(fn.Params) > 0 {
		env.setTainted(fn.Params[0].Name, true)
	}
	return a.scanStmts(fn.Body, env)
}

// taintEnv tracks which variable names are tainted in the current scope.
type taintEnv struct {
	parent  *taintEnv
	tainted map[string]bool
}

func newTaintEnv(parent *taintEnv) *taintEnv {
	return &taintEnv{parent: parent, tainted: map[string]bool{}}
}

func (e *taintEnv) isTainted(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.tainted[name] {
			return true
		}
	}
	return false
}

// setTainted marks name tainted in this scope level.
func (e *taintEnv) setTainted(name string, taint bool) {
	if taint {
		e.tainted[name] = true
	}
}

// scanStmts walks a statement list, updating env as assignments are seen.
func (a *injectionAnalyzer) scanStmts(stmts []ast.Stmt, env *taintEnv) diag.List {
	var ds diag.List
	for _, s := range stmts {
		ds = append(ds, a.scanStmt(s, env)...)
	}
	return ds
}

func (a *injectionAnalyzer) scanStmt(s ast.Stmt, env *taintEnv) diag.List {
	if s == nil {
		return nil
	}
	var ds diag.List
	switch st := s.(type) {
	case *ast.LetStmt:
		taint := a.isTainted(st.Value, env)
		if id, ok := st.Target.(*ast.Ident); ok {
			env.setTainted(id.Name, taint)
		}
		ds = append(ds, a.checkExprForSQL(st.Value, env)...)

	case *ast.DimStmt:
		if st.Value != nil {
			taint := a.isTainted(st.Value, env)
			env.setTainted(st.Name, taint)
			ds = append(ds, a.checkExprForSQL(st.Value, env)...)
		}

	case *ast.ExprStmt:
		ds = append(ds, a.checkCallForSQL(st.X, env)...)

	case *ast.PrintStmt:
		for _, arg := range st.Args {
			ds = append(ds, a.checkExprForSQL(arg, env)...)
		}

	case *ast.ReturnStmt:
		if st.Value != nil {
			ds = append(ds, a.checkExprForSQL(st.Value, env)...)
		}

	case *ast.RespondStmt:
		ds = append(ds, a.checkExprForSQL(st.Status, env)...)
		ds = append(ds, a.checkExprForSQL(st.Value, env)...)

	case *ast.IfStmt:
		inner := newTaintEnv(env)
		ds = append(ds, a.scanStmts(st.Then, inner)...)
		ds = append(ds, a.scanStmts(st.Else, inner)...)

	case *ast.ForStmt:
		inner := newTaintEnv(env)
		ds = append(ds, a.scanStmts(st.Body, inner)...)

	case *ast.ForEachStmt:
		inner := newTaintEnv(env)
		ds = append(ds, a.scanStmts(st.Body, inner)...)

	case *ast.WhileStmt:
		inner := newTaintEnv(env)
		ds = append(ds, a.scanStmts(st.Body, inner)...)

	case *ast.DoStmt:
		inner := newTaintEnv(env)
		ds = append(ds, a.scanStmts(st.Body, inner)...)

	case *ast.SelectStmt:
		for _, sc := range st.Cases {
			inner := newTaintEnv(env)
			ds = append(ds, a.scanStmts(sc.Body, inner)...)
		}
		ds = append(ds, a.scanStmts(st.Else, newTaintEnv(env))...)

	case *ast.TryStmt:
		ds = append(ds, a.scanStmts(st.Body, newTaintEnv(env))...)
		ds = append(ds, a.scanStmts(st.Catch, newTaintEnv(env))...)
		ds = append(ds, a.scanStmts(st.Finally, newTaintEnv(env))...)

	case *ast.TransactionStmt:
		ds = append(ds, a.scanStmts(st.Body, newTaintEnv(env))...)

	case *ast.FuncDecl:
		// Nested function declarations (classic mode); treat as a fresh scope.
		ds = append(ds, a.analyzeFunc(st)...)

	case *ast.LogStmt:
		ds = append(ds, a.checkExprForSQL(st.Msg, env)...)

	case *ast.ThrowStmt:
		ds = append(ds, a.checkExprForSQL(st.Value, env)...)
	}
	return ds
}

// checkExprForSQL checks for tainted SQL calls anywhere inside expr.
func (a *injectionAnalyzer) checkExprForSQL(e ast.Expr, env *taintEnv) diag.List {
	if e == nil {
		return nil
	}
	var ds diag.List
	switch ex := e.(type) {
	case *ast.CallExpr:
		ds = append(ds, a.checkCallForSQL(ex, env)...)
	case *ast.BinaryExpr:
		ds = append(ds, a.checkExprForSQL(ex.X, env)...)
		ds = append(ds, a.checkExprForSQL(ex.Y, env)...)
	case *ast.UnaryExpr:
		ds = append(ds, a.checkExprForSQL(ex.X, env)...)
	case *ast.ArrayLit:
		for _, item := range ex.Items {
			ds = append(ds, a.checkExprForSQL(item, env)...)
		}
	case *ast.MapLit:
		for _, v := range ex.Values {
			ds = append(ds, a.checkExprForSQL(v, env)...)
		}
	case *ast.IndexExpr:
		ds = append(ds, a.checkExprForSQL(ex.X, env)...)
		ds = append(ds, a.checkExprForSQL(ex.Index, env)...)
	case *ast.MemberExpr:
		ds = append(ds, a.checkExprForSQL(ex.X, env)...)
	}
	return ds
}

// checkCallForSQL checks whether this expression is a tainted SQL call.
// If it is a CallExpr to SQL/SQLONE/SQLEXEC with a tainted query arg, it warns.
// It also recurses into arguments to catch nested SQL calls.
func (a *injectionAnalyzer) checkCallForSQL(e ast.Expr, env *taintEnv) diag.List {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return a.checkExprForSQL(e, env)
	}

	var ds diag.List

	// Check arguments for nested SQL calls first.
	for _, arg := range call.Args {
		ds = append(ds, a.checkExprForSQL(arg, env)...)
	}

	// Is this a SQL/SQLONE/SQLEXEC call?
	id, ok := call.Fn.(*ast.Ident)
	if !ok || !isSQLBuiltin(id.Name) {
		return ds
	}
	if len(call.Args) == 0 {
		return ds
	}

	// Check whether the query argument (first arg) is a concatenation that
	// includes a tainted value. We only warn on + or & operators that are
	// built partly from tainted data — safe parameterized calls are fine.
	queryArg := call.Args[0]
	if a.isConcatWithTaint(queryArg, env) {
		ds = append(ds, diag.Warnf(call.At, diag.InjectionRisk, "use $1 placeholders"))
	}
	return ds
}

// isConcatWithTaint reports whether expr is a + or & concatenation that
// contains at least one tainted sub-expression anywhere in the concat tree.
// It only triggers when there is an actual concat operator (not bare tainted
// identifiers, which are safe as full query strings from constants/vars).
func (a *injectionAnalyzer) isConcatWithTaint(e ast.Expr, env *taintEnv) bool {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op != token.Plus && bin.Op != token.Amp {
		return false
	}
	// The expression is a concatenation. Warn if any leaf in the concat tree
	// is tainted.
	return a.concatTreeHasTaint(e, env)
}

// concatTreeHasTaint recursively walks a + / & tree and returns true if any
// sub-expression is tainted. Non-binary nodes are evaluated directly.
func (a *injectionAnalyzer) concatTreeHasTaint(e ast.Expr, env *taintEnv) bool {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok {
		return a.isTainted(e, env)
	}
	if bin.Op != token.Plus && bin.Op != token.Amp {
		// Not a concat — treat the whole sub-expression as one leaf.
		return a.isTainted(e, env)
	}
	return a.concatTreeHasTaint(bin.X, env) || a.concatTreeHasTaint(bin.Y, env)
}

// isTainted reports whether expr produces a tainted value.
func (a *injectionAnalyzer) isTainted(e ast.Expr, env *taintEnv) bool {
	if e == nil {
		return false
	}
	switch ex := e.(type) {
	case *ast.Ident:
		return env.isTainted(ex.Name)

	case *ast.MemberExpr:
		// req.params, req.query, req.headers, req.body, or any field on req.
		return a.isTainted(ex.X, env)

	case *ast.IndexExpr:
		// req.params["id"] or req["foo"] — index access on a tainted object is tainted.
		return a.isTainted(ex.X, env)

	case *ast.CallExpr:
		return a.isCallTainted(ex, env)

	case *ast.BinaryExpr:
		// Concatenation or arithmetic on tainted value propagates taint.
		return a.isTainted(ex.X, env) || a.isTainted(ex.Y, env)

	case *ast.UnaryExpr:
		return a.isTainted(ex.X, env)

	// Literals are never tainted.
	case *ast.StringLit, *ast.NumberLit, *ast.BoolLit, *ast.NilLit:
		return false

	case *ast.ArrayLit:
		for _, item := range ex.Items {
			if a.isTainted(item, env) {
				return true
			}
		}
		return false

	case *ast.MapLit:
		for _, v := range ex.Values {
			if a.isTainted(v, env) {
				return true
			}
		}
		return false
	}
	return false
}

// isCallTainted reports whether a function call returns a tainted value.
// In classic mode PARAM/QUERY/HEADER/BODY are taint sources.
// String manipulation built-ins propagate taint from their first argument.
func (a *injectionAnalyzer) isCallTainted(call *ast.CallExpr, env *taintEnv) bool {
	id, ok := call.Fn.(*ast.Ident)
	if !ok {
		return false
	}
	name := id.Name

	// Classic-mode HTTP helpers are always tainted.
	if a.classic {
		if isClassicHTTPHelper(name) {
			return true
		}
	}

	// String manipulation built-ins propagate taint from their first argument.
	if isStringBuiltin(name) && len(call.Args) > 0 {
		return a.isTainted(call.Args[0], env)
	}

	return false
}

// isSQLBuiltin reports whether name is SQL, SQLONE or SQLEXEC.
func isSQLBuiltin(name string) bool {
	switch strings.ToUpper(name) {
	case "SQL", "SQLONE", "SQLEXEC":
		return true
	}
	return false
}

// isClassicHTTPHelper reports whether name is PARAM, QUERY, HEADER or BODY.
func isClassicHTTPHelper(name string) bool {
	switch strings.ToUpper(name) {
	case "PARAM", "QUERY", "HEADER", "BODY":
		return true
	}
	return false
}

// isStringBuiltin reports whether name is a string manipulation built-in
// that propagates taint from its first argument.
func isStringBuiltin(name string) bool {
	switch strings.ToUpper(name) {
	case "TRIM", "UPPER", "LOWER", "LEFT", "RIGHT", "MID", "STR":
		return true
	}
	return false
}
