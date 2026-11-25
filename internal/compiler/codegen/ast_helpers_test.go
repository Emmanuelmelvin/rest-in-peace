package codegen_test

import (
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Hand-built ASTs. The parser is built on another track, so these helpers
// stand in for it. Statement helpers take the source line first.

func at(line int) token.Pos { return token.Pos{Line: line, Col: 1} }

func structured(file string, body ...ast.Stmt) *ast.Program {
	f := &ast.File{Name: file, Mode: token.Structured, Body: body}
	for _, s := range body {
		setFile(s, file)
	}
	return &ast.Program{Entry: file, Files: []*ast.File{f}}
}

func classic(file string, lines ...*ast.Line) *ast.Program {
	return &ast.Program{Entry: file, Files: []*ast.File{{Name: file, Mode: token.Classic, Lines: lines}}}
}

// setFile is cosmetic: it gives top-level statements a file name like the parser would.
func setFile(s ast.Stmt, file string) {
	if f, ok := s.(*ast.FuncDecl); ok {
		f.At.File = file
	}
}

// ln is one classic line: number, physical source line, statements.
func ln(number, line int, stmts ...ast.Stmt) *ast.Line {
	return &ast.Line{At: at(line), Number: number, Stmts: stmts}
}

// ---- Expressions ----

func num(i int64) ast.Expr { return &ast.NumberLit{Text: strconv.FormatInt(i, 10), IsInt: true} }
func flt(text string) ast.Expr {
	return &ast.NumberLit{Text: text}
}
func str(s string) ast.Expr                { return &ast.StringLit{Value: s} }
func boolean(b bool) ast.Expr              { return &ast.BoolLit{Value: b} }
func nilv() ast.Expr                       { return &ast.NilLit{} }
func id(name string) ast.Expr              { return &ast.Ident{Name: strings.ToUpper(name)} }
func mem(x ast.Expr, name string) ast.Expr { return &ast.MemberExpr{X: x, Name: name} }
func idx(x, i ast.Expr) ast.Expr           { return &ast.IndexExpr{X: x, Index: i} }
func neg(x ast.Expr) ast.Expr              { return &ast.UnaryExpr{Op: token.Minus, X: x} }
func not(x ast.Expr) ast.Expr              { return &ast.UnaryExpr{Op: token.NOT, X: x} }
func arr(items ...ast.Expr) ast.Expr       { return &ast.ArrayLit{Items: items} }

func bin(x ast.Expr, op token.Kind, y ast.Expr) ast.Expr {
	return &ast.BinaryExpr{Op: op, X: x, Y: y}
}

func call(name string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fn: id(name), Args: args}
}

// mapl builds {"k1": v1, "k2": v2, …} from alternating keys and values.
func mapl(kv ...any) ast.Expr {
	m := &ast.MapLit{}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Keys = append(m.Keys, kv[i].(string))
		m.Values = append(m.Values, kv[i+1].(ast.Expr))
	}
	return m
}

// ---- Statements ----

func pr(line int, args ...ast.Expr) ast.Stmt { return &ast.PrintStmt{At: at(line), Args: args} }
func let(line int, target, value ast.Expr) ast.Stmt {
	return &ast.LetStmt{At: at(line), Target: target, Value: value}
}
func dim(line int, name, typ string, value ast.Expr) ast.Stmt {
	return &ast.DimStmt{At: at(line), Name: strings.ToUpper(name), Type: ast.TypeRef{Name: typ}, Value: value}
}
func cnst(line int, name string, value ast.Expr) ast.Stmt {
	return &ast.ConstDecl{At: at(line), Name: strings.ToUpper(name), Value: value}
}
func do(line int, x ast.Expr) ast.Stmt { return &ast.ExprStmt{At: at(line), X: x} }
func iff(line int, cond ast.Expr, then []ast.Stmt, els ...ast.Stmt) ast.Stmt {
	return &ast.IfStmt{At: at(line), Cond: cond, Then: then, Else: els}
}
func forS(line int, v string, from, to, step ast.Expr, body ...ast.Stmt) ast.Stmt {
	return &ast.ForStmt{At: at(line), Var: strings.ToUpper(v), From: from, To: to, Step: step, Body: body}
}
func each(line int, v string, in ast.Expr, body ...ast.Stmt) ast.Stmt {
	return &ast.ForEachStmt{At: at(line), Var: strings.ToUpper(v), In: in, Body: body}
}
func while(line int, cond ast.Expr, body ...ast.Stmt) ast.Stmt {
	return &ast.WhileStmt{At: at(line), Cond: cond, Body: body}
}
func doLoop(line int, until ast.Expr, body ...ast.Stmt) ast.Stmt {
	return &ast.DoStmt{At: at(line), Body: body, Until: until}
}
func sel(line int, x ast.Expr, els []ast.Stmt, cases ...ast.SelectCase) ast.Stmt {
	return &ast.SelectStmt{At: at(line), X: x, Cases: cases, Else: els}
}
func cas(values []ast.Expr, body ...ast.Stmt) ast.SelectCase {
	return ast.SelectCase{Values: values, Body: body}
}
func exprs(es ...ast.Expr) []ast.Expr      { return es }
func stmts(ss ...ast.Stmt) []ast.Stmt      { return ss }
func exit(line int, k token.Kind) ast.Stmt { return &ast.ExitStmt{At: at(line), Loop: k} }
func end(line int) ast.Stmt                { return &ast.EndStmt{At: at(line)} }
func ret(line int, v ast.Expr) ast.Stmt    { return &ast.ReturnStmt{At: at(line), Value: v} }
func gotoS(line, target int) ast.Stmt      { return &ast.GotoStmt{At: at(line), Line: target} }
func gosub(line, target int) ast.Stmt      { return &ast.GosubStmt{At: at(line), Line: target} }
func onGoto(line int, x ast.Expr, targets ...int) ast.Stmt {
	return &ast.OnGotoStmt{At: at(line), X: x, Lines: targets}
}

func param(name, typ string, def ast.Expr) ast.Param {
	return ast.Param{Name: strings.ToUpper(name), Type: ast.TypeRef{Name: typ}, Default: def}
}
func params(ps ...ast.Param) []ast.Param { return ps }

func function(line int, name string, ps []ast.Param, result string, body ...ast.Stmt) ast.Stmt {
	return &ast.FuncDecl{At: at(line), Name: strings.ToUpper(name), Params: ps, Result: ast.TypeRef{Name: result}, Body: body}
}
func sub(line int, name string, ps []ast.Param, body ...ast.Stmt) ast.Stmt {
	return &ast.FuncDecl{At: at(line), Name: strings.ToUpper(name), Params: ps, IsSub: true, Body: body}
}
