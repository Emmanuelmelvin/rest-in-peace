package codegen

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"

// sharedDim compiles SHARED DIM x AS T [= v] to a value that is safe for
// concurrent access (Requirement 14.4; design §4.8: an rt.Shared guarded by a
// mutex). Kiro task 4.3 replaces this stub.
func (g *gen) sharedDim(o *out, s *ast.DimStmt, sc *scope) {
	g.todo(s.At, "4.3", "SHARED")
}
