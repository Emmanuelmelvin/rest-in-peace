// Package token defines the lexical tokens of RIP BASIC.
package token

import (
	"fmt"
	"strings"
)

// Pos is a position in a .bas file. Line and Col are 1-based.
type Pos struct {
	File string
	Line int
	Col  int
}

func (p Pos) String() string { return fmt.Sprintf("%s:%d", p.File, p.Line) }

// Mode is how a file is parsed (Requirement 1.2).
type Mode int

const (
	Structured Mode = iota
	Classic
)

func (m Mode) String() string {
	if m == Classic {
		return "classic"
	}
	return "structured"
}

// Kind is the kind of a token.
type Kind int

// Token is one lexical token. Text is the source text; for String it is the unquoted value.
type Token struct {
	Kind Kind
	Text string
	Pos  Pos
}

const (
	Illegal Kind = iota
	EOF
	Newline
	Colon // statement separator in classic mode

	Ident
	LineNumber // the leading number of a classic-mode line
	Number
	String

	Plus      // +
	Minus     // -
	Star      // *
	Slash     // /
	Backslash // \  integer division
	Caret     // ^
	Amp       // &
	Eq        // =  assignment or comparison; the parser decides
	NotEq     // <>
	Lt        // <
	Gt        // >
	LtEq      // <=
	GtEq      // >=
	LParen    // (
	RParen    // )
	LBrack    // [
	RBrack    // ]
	LBrace    // {
	RBrace    // }
	Comma     // ,
	Dot       // .
	Semicolon // ;  PRINT separator

	keywordStart
	AND
	AS
	CALL
	CASE
	CATCH
	CONST
	DBCONNECT
	DIM
	DO
	EACH
	ELSE
	ELSEIF
	END
	EXIT
	EXPECT
	EXPORT
	FALSE
	FINALLY
	FOR
	FROM
	FUNCTION
	GO
	GOSUB
	GOTO
	GROUP
	IF
	IMPORT
	IN
	LET
	LOG
	LOOP
	MOD
	NEXT
	NIL
	NOT
	ON
	OR
	PRINT
	RESPOND
	RETURN
	ROUTE
	SELECT
	SERVE
	SHARED
	STEP
	SUB
	TEST
	THEN
	THROW
	TO
	TRANSACTION
	TRUE
	TRY
	UNTIL
	USE
	WEND
	WHILE
	keywordEnd
)

var names = map[Kind]string{
	Illegal: "ILLEGAL", EOF: "END OF FILE", Newline: "NEWLINE", Colon: ":",
	Ident: "NAME", LineNumber: "LINE NUMBER", Number: "NUMBER", String: "STRING",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Backslash: `\`, Caret: "^", Amp: "&",
	Eq: "=", NotEq: "<>", Lt: "<", Gt: ">", LtEq: "<=", GtEq: ">=",
	LParen: "(", RParen: ")", LBrack: "[", RBrack: "]", LBrace: "{", RBrace: "}",
	Comma: ",", Dot: ".", Semicolon: ";",

	AND: "AND", AS: "AS", CALL: "CALL", CASE: "CASE", CATCH: "CATCH", CONST: "CONST",
	DBCONNECT: "DBCONNECT", DIM: "DIM", DO: "DO", EACH: "EACH", ELSE: "ELSE",
	ELSEIF: "ELSEIF", END: "END", EXIT: "EXIT", EXPECT: "EXPECT", EXPORT: "EXPORT",
	FALSE: "FALSE", FINALLY: "FINALLY", FOR: "FOR", FROM: "FROM", FUNCTION: "FUNCTION",
	GO: "GO", GOSUB: "GOSUB", GOTO: "GOTO", GROUP: "GROUP", IF: "IF", IMPORT: "IMPORT",
	IN: "IN", LET: "LET", LOG: "LOG", LOOP: "LOOP", MOD: "MOD", NEXT: "NEXT", NIL: "NIL",
	NOT: "NOT", ON: "ON", OR: "OR", PRINT: "PRINT", RESPOND: "RESPOND", RETURN: "RETURN",
	ROUTE: "ROUTE", SELECT: "SELECT", SERVE: "SERVE", SHARED: "SHARED", STEP: "STEP",
	SUB: "SUB", TEST: "TEST", THEN: "THEN", THROW: "THROW", TO: "TO",
	TRANSACTION: "TRANSACTION", TRUE: "TRUE", TRY: "TRY", UNTIL: "UNTIL", USE: "USE",
	WEND: "WEND", WHILE: "WHILE",
}

var keywords = func() map[string]Kind {
	m := make(map[string]Kind)
	for k := keywordStart + 1; k < keywordEnd; k++ {
		m[names[k]] = k
	}
	return m
}()

func (k Kind) String() string {
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// IsKeyword reports whether k is a reserved word.
func (k Kind) IsKeyword() bool { return k > keywordStart && k < keywordEnd }

// Lookup returns the keyword kind for an identifier, case-insensitively
// (Requirement 1.4), or Ident if it is not a keyword.
func Lookup(ident string) Kind {
	if k, ok := keywords[strings.ToUpper(ident)]; ok {
		return k
	}
	return Ident
}
