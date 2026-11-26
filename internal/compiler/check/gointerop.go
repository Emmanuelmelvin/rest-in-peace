package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// checkGoImports validates IMPORT … FROM GO declarations and usage
// (Requirements 5.4–5.7, task 3.5).
//
// Called from Check in check.go.
func checkGoImports(p *ast.Program) diag.List {
	// TODO(task 3.5): load imported Go packages with internal/compiler/gointerop
	// and validate named imports, aliases, and unsupported signatures.
	return nil
}
