package codegen

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"

// HTTP statements, in both modes: ROUTE, ROUTE GROUP, USE, RESPOND and SERVE
// (Requirements 7 and 8; design §4.7 and §4.8). laterStmt in structured.go
// sends them here, and classicScan in classic.go already marks ROUTE … GOSUB
// lines as jump targets.
//
// Owned by the Phase 2 HTTP track (docs/phase2-tracks.md).

// httpStmt compiles one HTTP statement. g.file.Mode tells the modes apart.
func (g *gen) httpStmt(o *out, s ast.Stmt, sc *scope) {
	switch s.(type) {
	case *ast.RouteStmt:
		g.todo(s.Pos(), "2.1", "ROUTE") // TODO(task 2.1)
	case *ast.GroupStmt:
		g.todo(s.Pos(), "2.7", "ROUTE GROUP") // TODO(task 2.7)
	case *ast.UseStmt:
		g.todo(s.Pos(), "2.3", "USE") // TODO(task 2.3)
	case *ast.RespondStmt:
		g.todo(s.Pos(), "2.1", "RESPOND") // TODO(task 2.1)
	case *ast.ServeStmt:
		g.todo(s.Pos(), "2.1", "SERVE") // TODO(task 2.1)
	}
}
