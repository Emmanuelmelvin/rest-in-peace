package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// checkSharedState warns when structured-mode code assigns a module-level
// variable inside a FUNCTION or SUB in a server file, unless that variable is
// declared SHARED (Requirement 14.4, task 4.3).
//
// Approach (simplest correct choice): for every structured-mode file that
// contains at least one ROUTE, USE, or ROUTE GROUP statement, report a
// ?SHARED STATE WARNING for every assignment (bare, member, or index) to a
// module-level variable inside any FUNCTION or SUB body — unless the variable
// was declared with SHARED DIM at module level or shadowed by a local
// parameter, DIM, CONST, loop variable, or CATCH variable. This catches both
// direct handlers and helper functions called by handlers without requiring
// call-graph reachability analysis.
func checkSharedState(p *ast.Program) diag.List {
	var ds diag.List
	for _, f := range p.Files {
		if f == nil || f.Mode != token.Structured {
			continue
		}
		ds = append(ds, checkFileSharedState(f)...)
	}
	return ds
}

// checkFileSharedState checks one structured-mode file.
func checkFileSharedState(f *ast.File) diag.List {
	if !stmtsHaveRoutes(f.Body) {
		return nil
	}

	modVars := collectModuleVars(f)
	if len(modVars) == 0 {
		return nil
	}
	sharedVars := collectSharedVars(f)

	var ds diag.List
	for _, s := range f.Body {
		fn, ok := s.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// Locals live for the whole function in BASIC: parameters, DIM, CONST,
		// and FOR / FOR EACH loop variables.
		locals := collectFuncLocals(fn)
		ds = append(ds, scanFuncForSharedWrites(fn.Body, modVars, sharedVars, locals)...)
	}
	return ds
}

// stmtsHaveRoutes reports whether stmts contains any ROUTE, USE, or ROUTE GROUP
// statement.
func stmtsHaveRoutes(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.RouteStmt, *ast.UseStmt, *ast.GroupStmt:
			return true
		case *ast.IfStmt:
			if stmtsHaveRoutes(st.Then) || stmtsHaveRoutes(st.Else) {
				return true
			}
		case *ast.ForStmt:
			if stmtsHaveRoutes(st.Body) {
				return true
			}
		case *ast.ForEachStmt:
			if stmtsHaveRoutes(st.Body) {
				return true
			}
		case *ast.WhileStmt:
			if stmtsHaveRoutes(st.Body) {
				return true
			}
		case *ast.DoStmt:
			if stmtsHaveRoutes(st.Body) {
				return true
			}
		case *ast.SelectStmt:
			for _, c := range st.Cases {
				if stmtsHaveRoutes(c.Body) {
					return true
				}
			}
			if stmtsHaveRoutes(st.Else) {
				return true
			}
		case *ast.TryStmt:
			if stmtsHaveRoutes(st.Body) || stmtsHaveRoutes(st.Catch) || stmtsHaveRoutes(st.Finally) {
				return true
			}
		case *ast.TransactionStmt:
			if stmtsHaveRoutes(st.Body) {
				return true
			}
		}
	}
	return false
}

// collectModuleVars returns the set of canonical names assigned or declared at
// the top level (not inside FUNCTION/SUB). CONST is excluded because assigning
// a CONST is already a compile error.
func collectModuleVars(f *ast.File) map[string]bool {
	vars := map[string]bool{}
	walkModuleStmts(f.Body, vars)
	return vars
}

func walkModuleStmts(stmts []ast.Stmt, vars map[string]bool) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.FuncDecl:
			// Stop at function boundaries — their assignments are locals.

		case *ast.LetStmt:
			if id, ok := st.Target.(*ast.Ident); ok {
				vars[canonUpper(id.Name)] = true
			}

		case *ast.DimStmt:
			vars[canonUpper(st.Name)] = true

		case *ast.ForStmt:
			vars[canonUpper(st.Var)] = true
			walkModuleStmts(st.Body, vars)

		case *ast.ForEachStmt:
			vars[canonUpper(st.Var)] = true
			walkModuleStmts(st.Body, vars)

		case *ast.IfStmt:
			walkModuleStmts(st.Then, vars)
			walkModuleStmts(st.Else, vars)

		case *ast.WhileStmt:
			walkModuleStmts(st.Body, vars)

		case *ast.DoStmt:
			walkModuleStmts(st.Body, vars)

		case *ast.SelectStmt:
			for _, c := range st.Cases {
				walkModuleStmts(c.Body, vars)
			}
			walkModuleStmts(st.Else, vars)

		case *ast.TryStmt:
			walkModuleStmts(st.Body, vars)
			walkModuleStmts(st.Catch, vars)
			walkModuleStmts(st.Finally, vars)

		case *ast.TransactionStmt:
			walkModuleStmts(st.Body, vars)
		}
	}
}

// collectSharedVars returns the set of canonical names declared with SHARED
// DIM at the top level of f.
func collectSharedVars(f *ast.File) map[string]bool {
	shared := map[string]bool{}
	for _, s := range f.Body {
		if d, ok := s.(*ast.DimStmt); ok && d.Shared {
			shared[canonUpper(d.Name)] = true
		}
	}
	return shared
}

// collectFuncLocals collects names explicitly declared local to fn: parameters,
// DIM, CONST, and FOR / FOR EACH loop variables. Bare LET is intentionally not
// included here so assigning a module-level variable still warns.
func collectFuncLocals(fn *ast.FuncDecl) map[string]bool {
	locals := make(map[string]bool, len(fn.Params))
	for _, p := range fn.Params {
		locals[canonUpper(p.Name)] = true
	}
	walkFuncLocals(fn.Body, locals)
	return locals
}

func walkFuncLocals(stmts []ast.Stmt, locals map[string]bool) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.DimStmt:
			locals[canonUpper(st.Name)] = true
		case *ast.ConstDecl:
			locals[canonUpper(st.Name)] = true
		case *ast.ForStmt:
			locals[canonUpper(st.Var)] = true
			walkFuncLocals(st.Body, locals)
		case *ast.ForEachStmt:
			locals[canonUpper(st.Var)] = true
			walkFuncLocals(st.Body, locals)
		case *ast.IfStmt:
			walkFuncLocals(st.Then, locals)
			walkFuncLocals(st.Else, locals)
		case *ast.WhileStmt:
			walkFuncLocals(st.Body, locals)
		case *ast.DoStmt:
			walkFuncLocals(st.Body, locals)
		case *ast.SelectStmt:
			for _, c := range st.Cases {
				walkFuncLocals(c.Body, locals)
			}
			walkFuncLocals(st.Else, locals)
		case *ast.TryStmt:
			walkFuncLocals(st.Body, locals)
			walkFuncLocals(st.Catch, locals)
			walkFuncLocals(st.Finally, locals)
		case *ast.TransactionStmt:
			walkFuncLocals(st.Body, locals)
		}
	}
}

// scanFuncForSharedWrites checks stmts (the body of a FUNCTION or SUB) for
// assignments to module-level variables that are not shared.
func scanFuncForSharedWrites(
	stmts []ast.Stmt,
	modVars, sharedVars, localVars map[string]bool,
) diag.List {
	var ds diag.List
	for _, s := range stmts {
		ds = append(ds, scanStmtForSharedWrites(s, modVars, sharedVars, localVars)...)
	}
	return ds
}

func scanStmtForSharedWrites(
	s ast.Stmt,
	modVars, sharedVars, localVars map[string]bool,
) diag.List {
	if s == nil {
		return nil
	}
	var ds diag.List
	switch st := s.(type) {
	case *ast.LetStmt:
		if id := rootIdent(st.Target); id != nil {
			name := canonUpper(id.Name)
			if modVars[name] && !sharedVars[name] && !localVars[name] {
				ds = append(ds, diag.Warnf(st.At, diag.SharedState,
					"assigning module-level variable %s inside a handler is not safe for concurrent requests — declare it with SHARED DIM %s AS <TYPE> instead",
					id.Name, id.Name,
				))
			}
		}

	case *ast.IfStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Then, modVars, sharedVars, localVars)...)
		ds = append(ds, scanFuncForSharedWrites(st.Else, modVars, sharedVars, localVars)...)

	case *ast.ForStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)

	case *ast.ForEachStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)

	case *ast.WhileStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)

	case *ast.DoStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)

	case *ast.SelectStmt:
		for _, c := range st.Cases {
			ds = append(ds, scanFuncForSharedWrites(c.Body, modVars, sharedVars, localVars)...)
		}
		ds = append(ds, scanFuncForSharedWrites(st.Else, modVars, sharedVars, localVars)...)

	case *ast.TryStmt:
		catchVars := localVars
		if st.CatchVar != "" {
			catchVars = copyMap(localVars)
			catchVars[canonUpper(st.CatchVar)] = true
		}
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)
		ds = append(ds, scanFuncForSharedWrites(st.Catch, modVars, sharedVars, catchVars)...)
		ds = append(ds, scanFuncForSharedWrites(st.Finally, modVars, sharedVars, localVars)...)

	case *ast.TransactionStmt:
		ds = append(ds, scanFuncForSharedWrites(st.Body, modVars, sharedVars, localVars)...)
	}
	return ds
}

// rootIdent returns the base variable identifier of an assignment target
// (x, x.k, x[i], x.a[0].b, ...), or nil if the target is not rooted at one.
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch t := e.(type) {
		case *ast.Ident:
			return t
		case *ast.MemberExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		default:
			return nil
		}
	}
}

func canonUpper(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}

func copyMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
