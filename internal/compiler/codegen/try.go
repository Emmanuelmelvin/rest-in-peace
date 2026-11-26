package codegen

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Error statements, in both modes: TRY … CATCH e … [FINALLY …] END TRY and
// THROW (Requirement 6; design §4.6). laterStmt in structured.go sends them
// here, and collectDecls sends TRY blocks to collectTry.
//
// Owned by the Phase 2 errors track (docs/phase2-tracks.md).

// errorStmt compiles TRY or THROW. g.file.Mode tells the modes apart.
func (g *gen) errorStmt(o *out, s ast.Stmt, sc *scope) {
	switch s.(type) {
	case *ast.ThrowStmt:
		g.todo(s.Pos(), "2.2", "THROW") // TODO(task 2.2)
	case *ast.TryStmt:
		g.todo(s.Pos(), "2.2", "TRY … CATCH") // TODO(task 2.2)
	}
}

// collectTry reports the names a TRY statement introduces, for collectDecls.
// TODO(task 2.2): the CATCH variable.
func (g *gen) collectTry(s *ast.TryStmt, sc *scope, add func(kind declKind, name string, p token.Pos, s ast.Stmt, sc *scope)) {
	g.collectDecls(s.Body, sc, add)
	g.collectDecls(s.Catch, sc, add)
	g.collectDecls(s.Finally, sc, add)
}
