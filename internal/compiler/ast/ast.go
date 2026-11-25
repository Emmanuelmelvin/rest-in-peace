// Package ast defines the syntax tree of RIP BASIC.
package ast

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"

// Node is any syntax tree node.
type Node interface{ Pos() token.Pos }

// Expr is an expression node.
type Expr interface {
	Node
	exprNode()
}

// Stmt is a statement or declaration node.
type Stmt interface {
	Node
	stmtNode()
}

// Program is every .bas file compiled into one binary.
type Program struct {
	Entry string // entry file name, e.g. "main.bas"
	Files []*File
}

// File is one parsed .bas file.
type File struct {
	Name    string
	Mode    token.Mode
	Imports []*Import
	Body    []Stmt  // structured mode: top-level statements and declarations, in order
	Lines   []*Line // classic mode: lines sorted by number, duplicates replaced
}

// Line is one numbered classic-mode line.
type Line struct {
	At     token.Pos
	Number int
	Stmts  []Stmt
}

// Import is IMPORT "./x.bas", IMPORT { A, B } FROM "…", or IMPORT x FROM GO "…".
type Import struct {
	At     token.Pos
	Path   string
	Names  []string // named imports; nil means everything (.bas) or the package (Go)
	Alias  string   // IMPORT uuid FROM GO "…"
	FromGo bool
}

// TypeRef is a declared type: NUMBER, INTEGER, STRING, BOOLEAN, MAP, ARRAY,
// DECIMAL, DATE, NIL, or "" for ANY.
type TypeRef struct{ Name string }

// Param is one function parameter.
type Param struct {
	Name    string
	Type    TypeRef
	Default Expr
}

// SelectCase is one CASE clause of SELECT CASE.
type SelectCase struct {
	Values []Expr
	Body   []Stmt
}

// ---- Expressions ----

type (
	Ident struct {
		At   token.Pos
		Name string
	} // Name is canonical (upper case)
	NumberLit struct {
		At    token.Pos
		Text  string
		IsInt bool
	}
	StringLit struct {
		At    token.Pos
		Value string
	}
	BoolLit struct {
		At    token.Pos
		Value bool
	}
	NilLit struct{ At token.Pos }
	MapLit struct {
		At     token.Pos
		Keys   []string
		Values []Expr
	}
	ArrayLit struct {
		At    token.Pos
		Items []Expr
	}
	MemberExpr struct {
		At   token.Pos
		X    Expr
		Name string
	} // Name keeps its case (Requirement 1.5)
	IndexExpr struct {
		At       token.Pos
		X, Index Expr
	}
	CallExpr struct {
		At   token.Pos
		Fn   Expr
		Args []Expr
	}
	UnaryExpr struct {
		At token.Pos
		Op token.Kind
		X  Expr
	}
	BinaryExpr struct {
		At   token.Pos
		Op   token.Kind
		X, Y Expr
	}
)

// ---- Statements and declarations ----

type (
	FuncDecl struct {
		At     token.Pos
		Name   string
		Params []Param
		Result TypeRef
		IsSub  bool
		Export bool
		Body   []Stmt
	}
	ConstDecl struct {
		At     token.Pos
		Name   string
		Value  Expr
		Export bool
	}
	DimStmt struct {
		At     token.Pos
		Name   string
		Type   TypeRef
		Value  Expr
		Shared bool
	}
	LetStmt struct {
		At            token.Pos
		Target, Value Expr
	}
	PrintStmt struct {
		At   token.Pos
		Args []Expr
	}
	ExprStmt struct {
		At token.Pos
		X  Expr
	}
	IfStmt struct {
		At         token.Pos
		Cond       Expr
		Then, Else []Stmt
	} // ELSEIF is a nested IfStmt in Else
	ForStmt struct {
		At             token.Pos
		Var            string
		From, To, Step Expr
		Body           []Stmt
	}
	ForEachStmt struct {
		At   token.Pos
		Var  string
		In   Expr
		Body []Stmt
	}
	WhileStmt struct {
		At   token.Pos
		Cond Expr
		Body []Stmt
	}
	DoStmt struct {
		At    token.Pos
		Body  []Stmt
		Until Expr
	}
	SelectStmt struct {
		At    token.Pos
		X     Expr
		Cases []SelectCase
		Else  []Stmt
	}
	ExitStmt struct {
		At   token.Pos
		Loop token.Kind
	} // FOR, WHILE or DO
	EndStmt    struct{ At token.Pos }
	ReturnStmt struct {
		At    token.Pos
		Value Expr
	}
	ThrowStmt struct {
		At    token.Pos
		Value Expr
	}
	TryStmt struct {
		At             token.Pos
		Body           []Stmt
		CatchVar       string
		Catch, Finally []Stmt
	}
	RouteStmt struct {
		At                    token.Pos
		Method, Path, Handler string
		GosubLine             int
	}
	GroupStmt struct {
		At     token.Pos
		Prefix string
		Routes []*RouteStmt
	}
	UseStmt struct {
		At    token.Pos
		Fn    Expr
		Scope string
	}
	RespondStmt struct {
		At                     token.Pos
		Status, Value, Headers Expr
	}
	ServeStmt struct {
		At   token.Pos
		Port Expr
	}
	DBConnectStmt struct {
		At   token.Pos
		URL  Expr
		Name string
	}
	TransactionStmt struct {
		At   token.Pos
		Name string
		Body []Stmt
	}
	LogStmt struct {
		At                 token.Pos
		Level, Msg, Fields Expr
	}
	TestDecl struct {
		At   token.Pos
		Name string
		Body []Stmt
	}
	ExpectStmt struct {
		At        token.Pos
		Cond, Msg Expr
	}
	GotoStmt struct {
		At   token.Pos
		Line int
	}
	GosubStmt struct {
		At   token.Pos
		Line int
	}
	OnGotoStmt struct {
		At    token.Pos
		X     Expr
		Lines []int
	}
)

func (n *Ident) Pos() token.Pos      { return n.At }
func (n *NumberLit) Pos() token.Pos  { return n.At }
func (n *StringLit) Pos() token.Pos  { return n.At }
func (n *BoolLit) Pos() token.Pos    { return n.At }
func (n *NilLit) Pos() token.Pos     { return n.At }
func (n *MapLit) Pos() token.Pos     { return n.At }
func (n *ArrayLit) Pos() token.Pos   { return n.At }
func (n *MemberExpr) Pos() token.Pos { return n.At }
func (n *IndexExpr) Pos() token.Pos  { return n.At }
func (n *CallExpr) Pos() token.Pos   { return n.At }
func (n *UnaryExpr) Pos() token.Pos  { return n.At }
func (n *BinaryExpr) Pos() token.Pos { return n.At }

func (*Ident) exprNode()      {}
func (*NumberLit) exprNode()  {}
func (*StringLit) exprNode()  {}
func (*BoolLit) exprNode()    {}
func (*NilLit) exprNode()     {}
func (*MapLit) exprNode()     {}
func (*ArrayLit) exprNode()   {}
func (*MemberExpr) exprNode() {}
func (*IndexExpr) exprNode()  {}
func (*CallExpr) exprNode()   {}
func (*UnaryExpr) exprNode()  {}
func (*BinaryExpr) exprNode() {}

func (n *FuncDecl) Pos() token.Pos        { return n.At }
func (n *ConstDecl) Pos() token.Pos       { return n.At }
func (n *DimStmt) Pos() token.Pos         { return n.At }
func (n *LetStmt) Pos() token.Pos         { return n.At }
func (n *PrintStmt) Pos() token.Pos       { return n.At }
func (n *ExprStmt) Pos() token.Pos        { return n.At }
func (n *IfStmt) Pos() token.Pos          { return n.At }
func (n *ForStmt) Pos() token.Pos         { return n.At }
func (n *ForEachStmt) Pos() token.Pos     { return n.At }
func (n *WhileStmt) Pos() token.Pos       { return n.At }
func (n *DoStmt) Pos() token.Pos          { return n.At }
func (n *SelectStmt) Pos() token.Pos      { return n.At }
func (n *ExitStmt) Pos() token.Pos        { return n.At }
func (n *EndStmt) Pos() token.Pos         { return n.At }
func (n *ReturnStmt) Pos() token.Pos      { return n.At }
func (n *ThrowStmt) Pos() token.Pos       { return n.At }
func (n *TryStmt) Pos() token.Pos         { return n.At }
func (n *RouteStmt) Pos() token.Pos       { return n.At }
func (n *GroupStmt) Pos() token.Pos       { return n.At }
func (n *UseStmt) Pos() token.Pos         { return n.At }
func (n *RespondStmt) Pos() token.Pos     { return n.At }
func (n *ServeStmt) Pos() token.Pos       { return n.At }
func (n *DBConnectStmt) Pos() token.Pos   { return n.At }
func (n *TransactionStmt) Pos() token.Pos { return n.At }
func (n *LogStmt) Pos() token.Pos         { return n.At }
func (n *TestDecl) Pos() token.Pos        { return n.At }
func (n *ExpectStmt) Pos() token.Pos      { return n.At }
func (n *GotoStmt) Pos() token.Pos        { return n.At }
func (n *GosubStmt) Pos() token.Pos       { return n.At }
func (n *OnGotoStmt) Pos() token.Pos      { return n.At }

func (*FuncDecl) stmtNode()        {}
func (*ConstDecl) stmtNode()       {}
func (*DimStmt) stmtNode()         {}
func (*LetStmt) stmtNode()         {}
func (*PrintStmt) stmtNode()       {}
func (*ExprStmt) stmtNode()        {}
func (*IfStmt) stmtNode()          {}
func (*ForStmt) stmtNode()         {}
func (*ForEachStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()       {}
func (*DoStmt) stmtNode()          {}
func (*SelectStmt) stmtNode()      {}
func (*ExitStmt) stmtNode()        {}
func (*EndStmt) stmtNode()         {}
func (*ReturnStmt) stmtNode()      {}
func (*ThrowStmt) stmtNode()       {}
func (*TryStmt) stmtNode()         {}
func (*RouteStmt) stmtNode()       {}
func (*GroupStmt) stmtNode()       {}
func (*UseStmt) stmtNode()         {}
func (*RespondStmt) stmtNode()     {}
func (*ServeStmt) stmtNode()       {}
func (*DBConnectStmt) stmtNode()   {}
func (*TransactionStmt) stmtNode() {}
func (*LogStmt) stmtNode()         {}
func (*TestDecl) stmtNode()        {}
func (*ExpectStmt) stmtNode()      {}
func (*GotoStmt) stmtNode()        {}
func (*GosubStmt) stmtNode()       {}
func (*OnGotoStmt) stmtNode()      {}
