// Package parser builds the syntax tree from tokens: a hand-written
// recursive-descent parser for statements, and a Pratt parser for expressions.
package parser

import (
	"github.com/rip-basic/rip/internal/compiler/ast"
	"github.com/rip-basic/rip/internal/compiler/diag"
	"github.com/rip-basic/rip/internal/compiler/token"
)

// Parse parses the tokens of one file.
func Parse(file string, toks []token.Token, mode token.Mode) (*ast.File, diag.List) {
	// TODO(task 1.2): Pratt expressions. Precedence, lowest → highest:
	//   OR < AND < NOT < compare < & < + - < * / \ MOD < ^ < unary < postfix (. [] ())
	// TODO(task 1.3): structured statements (IF/ELSEIF, FOR/FOR EACH, WHILE, DO, SELECT CASE, EXIT).
	// TODO(task 1.4): FUNCTION/SUB, CONST, DIM AS TYPE.
	// TODO(task 1.7): classic mode: build ast.File.Lines, with duplicate numbers replacing earlier lines.
	// Errors must read like: ?SYNTAX ERROR IN x.bas:3 — expected THEN after IF condition.
	f := &ast.File{Name: file, Mode: mode}
	return f, diag.List{diag.Todo(token.Pos{File: file, Line: 1, Col: 1}, "1.2", "the parser")}
}
