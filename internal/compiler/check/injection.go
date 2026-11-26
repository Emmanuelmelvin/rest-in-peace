package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// checkSQLInjection warns when a SQL, SQLONE or SQLEXEC query string is built
// with + or & from a value that came from req (Requirement 9.4, task 3.6).
//
// Kiro owns this file and injection_test.go (.kiro/steering/structure.md).
// Called from Check in check.go; replace this stub body with the full check.
func checkSQLInjection(p *ast.Program) diag.List {
	// TODO(task 3.6): emit diag.Warnf(pos, diag.InjectionRisk, "use $1 placeholders")
	// when a SQL/SQLONE/SQLEXEC query expression concatenates (+ or &) a value
	// tainted by req (or PARAM/QUERY/HEADER/BODY in classic mode).
	return nil
}
