package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// checkModules validates IMPORT and EXPORT across .bas files in p
// (Requirements 5.1–5.3, task 3.4).
//
// Called from Check in check.go.
func checkModules(p *ast.Program) diag.List {
	// TODO(task 3.4): verify imported files exist in p.Files, resolve named and
	// whole-file imports, and reject uses of unexported names across files.
	return nil
}
