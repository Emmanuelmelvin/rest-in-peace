// Package check resolves names across files and produces semantic errors and warnings.
package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// Check validates a whole program.
func Check(p *ast.Program) diag.List {
	// TODO(task 1.4): undefined names, with "did you mean …?" hints.
	// TODO(task 1.7): GOTO/GOSUB in structured mode; jumps to missing lines.
	// TODO(task 3.4): IMPORT/EXPORT resolution; using a name that isn't exported.
	// TODO(task 3.6): ?INJECTION RISK WARNING for SQL strings built from req values.
	// TODO(task 4.3): ?SHARED STATE WARNING for module variables assigned in handlers.
	return nil
}
