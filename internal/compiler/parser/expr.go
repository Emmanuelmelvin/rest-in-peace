package parser

import (
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Binding powers, lowest → highest (language.md, "Operators"):
// OR < AND < NOT < compare < & < + - < * / \ MOD < ^ < unary - < postfix.
const (
	precLowest = iota
	precOr
	precAnd
	precNot
	precCompare
	precConcat
	precAdd
	precMul
	precPow
	precUnary
	precPostfix
)

// infixPrec is the binding power of each binary operator.
var infixPrec = map[token.Kind]int{
	token.OR:  precOr,
	token.AND: precAnd,
	token.Eq:  precCompare, token.NotEq: precCompare, token.Lt: precCompare,
	token.Gt: precCompare, token.LtEq: precCompare, token.GtEq: precCompare,
	token.Amp:  precConcat,
	token.Plus: precAdd, token.Minus: precAdd,
	token.Star: precMul, token.Slash: precMul, token.Backslash: precMul, token.MOD: precMul,
	token.Caret: precPow,
}

// expr parses a full expression.
func (p *parser) expr() ast.Expr { return p.exprPrec(precLowest) }

// exprPrec parses an expression whose binary operators all bind tighter than min.
func (p *parser) exprPrec(min int) ast.Expr {
	x := p.prefix()
	for {
		op := p.tok
		prec, ok := infixPrec[op.Kind]
		if !ok || prec <= min {
			return x
		}
		p.next()
		next := prec
		if op.Kind == token.Caret {
			next = prec - 1 // ^ is right-associative: 2 ^ 3 ^ 2 = 2 ^ 9
		}
		y := p.operand(next, op)
		x = &ast.BinaryExpr{At: op.Pos, Op: op.Kind, X: x, Y: y}
	}
}

// operand parses the right-hand side of an operator, with a hint if it is missing.
func (p *parser) operand(min int, op token.Token) ast.Expr {
	if p.endOfExpr() {
		p.fail(p.tok.Pos, "expected a value after %s", describe(op))
	}
	return p.exprPrec(min)
}

// endOfExpr reports whether the current token can't start an expression.
func (p *parser) endOfExpr() bool {
	switch p.tok.Kind {
	case token.Newline, token.EOF, token.Colon, token.RParen, token.RBrack, token.RBrace, token.Comma:
		return true
	}
	return false
}

func (p *parser) prefix() ast.Expr {
	t := p.tok
	switch t.Kind {
	case token.Minus:
		p.next()
		return &ast.UnaryExpr{At: t.Pos, Op: token.Minus, X: p.operand(precUnary-1, t)}
	case token.NOT:
		p.next()
		return &ast.UnaryExpr{At: t.Pos, Op: token.NOT, X: p.operand(precNot, t)}
	}
	return p.postfix(p.primary())
}

// postfix parses .name, [index] and (args) after x.
func (p *parser) postfix(x ast.Expr) ast.Expr {
	for {
		switch p.tok.Kind {
		case token.Dot:
			p.next()
			name := p.tok
			if name.Kind != token.Ident && !name.Kind.IsKeyword() {
				p.fail(name.Pos, "expected a member name after .")
			}
			p.next()
			x = &ast.MemberExpr{At: name.Pos, X: x, Name: name.Text}
		case token.LBrack:
			at := p.tok.Pos
			p.next()
			idx := p.expr()
			p.expect(token.RBrack, "expected ] after the index")
			x = &ast.IndexExpr{At: at, X: x, Index: idx}
		case token.LParen:
			p.next()
			args := p.list(token.RParen, "expected ) after the arguments")
			x = &ast.CallExpr{At: x.Pos(), Fn: x, Args: args}
		default:
			return x
		}
	}
}

// list parses comma-separated expressions up to and including the closing
// token. Newlines are allowed after the opening bracket, after commas and
// before the closing bracket, so long literals and calls can span lines.
func (p *parser) list(close token.Kind, hint string) []ast.Expr {
	var items []ast.Expr
	p.skipNewlines()
	for p.tok.Kind != close {
		items = append(items, p.expr())
		if !p.listSep(close, hint) {
			break
		}
	}
	p.expect(close, hint)
	return items
}

// listSep consumes a comma (and any newlines after it) and reports whether
// another item follows. It looks past newlines for the closing bracket.
func (p *parser) listSep(close token.Kind, hint string) bool {
	if p.tok.Kind == token.Comma {
		p.next()
		p.skipNewlines()
		return p.tok.Kind != close
	}
	if p.tok.Kind == token.Newline && p.peekPastNewlines() == close {
		p.skipNewlines()
	}
	if p.tok.Kind != close {
		p.fail(p.tok.Pos, "%s", hint)
	}
	return false
}

func (p *parser) primary() ast.Expr {
	t := p.tok
	switch t.Kind {
	case token.Ident:
		p.next()
		return &ast.Ident{At: t.Pos, Name: strings.ToUpper(t.Text)}
	case token.Number:
		p.next()
		return &ast.NumberLit{At: t.Pos, Text: t.Text, IsInt: !strings.ContainsAny(t.Text, ".eE")}
	case token.String:
		p.next()
		return &ast.StringLit{At: t.Pos, Value: t.Text}
	case token.TRUE, token.FALSE:
		p.next()
		return &ast.BoolLit{At: t.Pos, Value: t.Kind == token.TRUE}
	case token.NIL:
		p.next()
		return &ast.NilLit{At: t.Pos}
	case token.LParen:
		p.next()
		x := p.expr()
		p.expect(token.RParen, "expected ) to close the (")
		return x
	case token.LBrack:
		p.next()
		return &ast.ArrayLit{At: t.Pos, Items: p.list(token.RBrack, "expected , or ] in the array")}
	case token.LBrace:
		return p.mapLit()
	}
	if p.endOfExpr() {
		p.fail(t.Pos, "expected a value here")
	}
	if t.Kind.IsKeyword() {
		p.fail(t.Pos, "%s is a reserved word, so it can't be used as a value or a name", t.Kind)
	}
	p.fail(t.Pos, "expected a value, not %s", describe(t))
	return nil
}

// mapLit parses {"key": value, …}. Keys are strings and keep their case.
func (p *parser) mapLit() ast.Expr {
	m := &ast.MapLit{At: p.tok.Pos}
	p.next()
	p.skipNewlines()
	for p.tok.Kind != token.RBrace {
		k := p.tok
		if k.Kind != token.String {
			p.fail(k.Pos, `map keys are strings in double quotes, like {"name": "Ada"}`)
		}
		p.next()
		p.expect(token.Colon, "expected : after the map key")
		p.skipNewlines()
		m.Keys = append(m.Keys, k.Text)
		m.Values = append(m.Values, p.expr())
		if !p.listSep(token.RBrace, "expected , or } in the map") {
			break
		}
	}
	p.expect(token.RBrace, "expected , or } in the map")
	return m
}

// describe names a token for an error message.
func describe(t token.Token) string {
	switch t.Kind {
	case token.Newline:
		return "the end of the line"
	case token.EOF:
		return "the end of the file"
	case token.Ident:
		return t.Text
	case token.Number, token.LineNumber:
		return t.Text
	case token.String:
		return `"` + t.Text + `"`
	}
	return t.Kind.String()
}
