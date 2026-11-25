// Package lexer turns .bas source into tokens and detects the file's mode.
package lexer

import (
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Lex splits one .bas file into tokens and reports whether it is classic or
// structured mode (Requirement 1.2). file is the name used in diagnostics.
func Lex(file string, src []byte) ([]token.Token, token.Mode, diag.List) {
	// TODO(task 1.1): implement the lexer.
	//   - Track line and column for every token.
	//   - Mode: classic if the first non-blank, non-comment line starts with a number.
	//   - Mixing numbered and unnumbered lines → ?SYNTAX ERROR (Requirement 1.3).
	//   - REM … and ' … are comments; identifiers go through token.Lookup.
	//   - Strings support \" and \\ escapes; numbers: 42, 3.14, 1e6.
	//   - Emit Newline tokens (statements end at a newline), and Colon in classic mode.
	return nil, token.Structured, diag.List{diag.Todo(token.Pos{File: file, Line: 1, Col: 1}, "1.1", "the lexer")}
}
