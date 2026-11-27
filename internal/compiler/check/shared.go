package check

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// checkSharedState reports ?SHARED STATE WARNING for module-level variables
// assigned inside handlers (structured mode) unless they are declared SHARED
// (Requirement 14.4). Kiro task 4.3 replaces this stub.
func checkSharedState(p *ast.Program) diag.List {
	return nil
}
