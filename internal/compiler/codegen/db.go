package codegen

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"

// dbStmt compiles DBCONNECT (task 3.1) and TRANSACTION (task 3.2), called from
// laterStmt in structured.go (and available in both modes).
func (g *gen) dbStmt(o *out, s ast.Stmt, sc *scope) {
	switch s := s.(type) {
	case *ast.DBConnectStmt:
		// TODO(task 3.1): emit rt.DBConnect(c, url, name).
		g.todo(s.Pos(), "3.1", "DBCONNECT")
	case *ast.TransactionStmt:
		// TODO(task 3.2): emit rt.Tx(c, name, func() { … }), propagating RETURN/EXIT.
		g.todo(s.Pos(), "3.2", "TRANSACTION")
	}
}
