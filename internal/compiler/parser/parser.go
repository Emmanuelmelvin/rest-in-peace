// Package parser builds the syntax tree from tokens: a hand-written
// recursive-descent parser for statements, and a Pratt parser for expressions.
//
// The parser reports as many errors as it can. A syntax error abandons the
// rest of its statement's line (an internal bailout panic, recovered per
// statement, never escapes Parse), and each source line gets at most one
// error, so one mistake doesn't bury the real problem in noise.
package parser

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Parse parses the tokens of one file. toks must come from lexer.Lex.
//
// In classic mode, File.Lines is sorted by line number, a duplicate number
// replaces the earlier line (Requirement 1.6), and `:` separates statements.
// A block (FOR … NEXT, IF … END IF, …) may span numbered lines; its inner
// lines become part of the block.
func Parse(file string, toks []token.Token, mode token.Mode) (*ast.File, diag.List) {
	f := &ast.File{Name: file, Mode: mode}
	if mode == token.Classic {
		toks = sortLines(toks)
	}
	if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
		toks = append(toks, token.Token{Kind: token.EOF, Pos: token.Pos{File: file, Line: 1, Col: 1}})
	}
	p := &parser{file: file, mode: mode, toks: toks, f: f, reported: map[int]bool{}}
	p.tok = toks[0]
	p.stmts(0, func(s ast.Stmt) {
		switch {
		case mode == token.Structured:
			f.Body = append(f.Body, s)
		case p.line != nil:
			p.line.Stmts = append(p.line.Stmts, s)
		}
	})
	return f, p.diags
}

// sortLines reorders classic-mode tokens line by line into line-number
// order. A later line with the same number replaces the earlier one.
func sortLines(toks []token.Token) []token.Token {
	type seg struct {
		n    int
		toks []token.Token
	}
	var segs []seg
	var eof []token.Token
	for i, t := range toks {
		switch {
		case t.Kind == token.EOF:
			eof = toks[i:]
		case t.Kind == token.LineNumber:
			n, _ := strconv.Atoi(t.Text)
			segs = append(segs, seg{n: n})
			fallthrough
		case len(segs) > 0:
			segs[len(segs)-1].toks = append(segs[len(segs)-1].toks, t)
		}
		if eof != nil {
			break
		}
	}
	last := map[int]int{}
	for i, s := range segs {
		last[s.n] = i
	}
	var keep []seg
	for i, s := range segs {
		if last[s.n] == i {
			keep = append(keep, s)
		}
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].n < keep[j].n })
	var out []token.Token
	for _, s := range keep {
		out = append(out, s.toks...)
	}
	return append(out, eof...)
}

type parser struct {
	file string
	mode token.Mode
	toks []token.Token
	i    int
	tok  token.Token

	f        *ast.File
	line     *ast.Line // classic mode: the current top-level line
	blocks   []block   // the blocks that are open, innermost last
	inline   int       // > 0 while parsing the statements of a single-line IF
	diags    diag.List
	reported map[int]bool // lines that already have an error
}

// block is an open block statement, such as a FOR waiting for its NEXT.
type block struct {
	kind token.Kind // FOR, WHILE, DO, IF, SELECT, TRY, FUNCTION, SUB, TRANSACTION, TEST, GROUP
	at   token.Pos
}

// bailout abandons the statement being parsed. It is recovered in stmtSafe.
type bailout struct{}

// ---- tokens ----

func (p *parser) next() {
	if p.i < len(p.toks)-1 {
		p.i++
	}
	p.tok = p.toks[p.i]
}

func (p *parser) peek() token.Token {
	if p.i+1 < len(p.toks) {
		return p.toks[p.i+1]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) peekPastNewlines() token.Kind {
	for j := p.i; j < len(p.toks); j++ {
		if p.toks[j].Kind != token.Newline {
			return p.toks[j].Kind
		}
	}
	return token.EOF
}

func (p *parser) skipNewlines() {
	for p.tok.Kind == token.Newline {
		p.next()
	}
}

// skipLine skips to the end of the current line.
func (p *parser) skipLine() {
	for p.tok.Kind != token.Newline && p.tok.Kind != token.EOF {
		p.next()
	}
}

func (p *parser) expect(k token.Kind, hint string) token.Token {
	t := p.tok
	if t.Kind != k {
		p.fail(t.Pos, "%s", hint)
	}
	p.next()
	return t
}

// endOfStmt reports whether the current token ends a statement.
func (p *parser) endOfStmt() bool {
	switch p.tok.Kind {
	case token.Newline, token.EOF:
		return true
	case token.Colon:
		return p.mode == token.Classic
	}
	return false
}

// stmtEnd requires the end of a statement.
func (p *parser) stmtEnd() {
	if p.endOfStmt() {
		return
	}
	t := p.tok
	switch {
	case t.Kind == token.Colon:
		p.fail(t.Pos, ": separates statements only in classic mode; put each statement on its own line")
	case t.Kind == token.ELSE && p.inline > 0:
		p.fail(t.Pos, "a single-line IF has no ELSE; use IF … THEN, ELSE and END IF on separate lines")
	}
	p.fail(t.Pos, "expected the end of the statement, not %s", describe(t))
}

// ---- errors ----

// errorAt records a syntax error, at most one per line.
func (p *parser) errorAt(pos token.Pos, format string, args ...any) {
	p.report(diag.Errorf(pos, diag.Syntax, format, args...))
}

func (p *parser) report(d diag.Diagnostic) {
	if p.reported[d.Pos.Line] {
		return
	}
	p.reported[d.Pos.Line] = true
	p.diags = append(p.diags, d)
}

// fail records a syntax error and abandons the current statement.
func (p *parser) fail(pos token.Pos, format string, args ...any) {
	p.errorAt(pos, format, args...)
	panic(bailout{})
}

// recoverLine recovers a bailout and skips the rest of the line. Use it as
// `defer p.recoverLine(&failed)`.
func (p *parser) recoverLine(failed *bool) {
	r := recover()
	if r == nil {
		return
	}
	if _, ok := r.(bailout); !ok {
		panic(r)
	}
	p.skipLine()
	if failed != nil {
		*failed = true
	}
}

// ---- statement lists and blocks ----

// closer is a token that ends (part of) a block, such as NEXT or END IF.
type closer struct {
	owner token.Kind // the block it belongs to
	name  string     // how it is written, e.g. "END IF"
}

// closerHere reports whether the current token starts a block closer.
func (p *parser) closerHere() (closer, bool) {
	switch p.tok.Kind {
	case token.NEXT:
		return closer{token.FOR, "NEXT"}, true
	case token.WEND:
		return closer{token.WHILE, "WEND"}, true
	case token.LOOP:
		return closer{token.DO, "LOOP"}, true
	case token.ELSE, token.ELSEIF:
		return closer{token.IF, p.tok.Kind.String()}, true
	case token.CASE:
		return closer{token.SELECT, "CASE"}, true
	case token.CATCH, token.FINALLY:
		return closer{token.TRY, p.tok.Kind.String()}, true
	case token.END:
		switch k := p.peek().Kind; k {
		case token.IF, token.FUNCTION, token.SUB, token.SELECT, token.TRY, token.GROUP, token.TRANSACTION, token.TEST:
			return closer{k, "END " + k.String()}, true
		}
	}
	return closer{}, false
}

// opener names the statement that opens a block of the given kind.
func opener(k token.Kind) string {
	switch k {
	case token.SELECT:
		return "SELECT CASE"
	case token.GROUP:
		return "ROUTE GROUP"
	}
	return k.String()
}

func (p *parser) isOpen(k token.Kind) bool {
	for _, b := range p.blocks {
		if b.kind == k {
			return true
		}
	}
	return false
}

// stmts parses statements until a closer of the block owner (it returns
// true, at the closer), a closer of an enclosing block or the end of the file
// (it returns false). A closer that belongs to no open block is an error.
func (p *parser) stmts(owner token.Kind, add func(ast.Stmt)) bool {
	for {
		if !p.skipSeps() {
			return false
		}
		if c, ok := p.closerHere(); ok {
			if owner != 0 && c.owner == owner {
				return true
			}
			if p.isOpen(c.owner) {
				return false
			}
			p.errorAt(p.tok.Pos, "%s without %s; there is no open %s for it to close", c.name, opener(c.owner), opener(c.owner))
			p.skipLine()
			continue
		}
		if s := p.stmtSafe(); s != nil {
			add(s)
		}
	}
}

// skipSeps skips blank lines, classic-mode colons and line numbers. It
// returns false at the end of the file, or at the end of the line inside a
// single-line IF.
func (p *parser) skipSeps() bool {
	for {
		switch p.tok.Kind {
		case token.EOF:
			return false
		case token.Newline:
			if p.inline > 0 {
				return false
			}
			p.next()
		case token.Colon:
			if p.mode != token.Classic {
				return true // stmtSafe reports it
			}
			p.next()
		case token.LineNumber:
			p.lineStart()
		default:
			return true
		}
	}
}

// lineStart handles the LineNumber at the start of a classic line.
func (p *parser) lineStart() {
	t := p.tok
	p.next()
	n, err := strconv.Atoi(t.Text)
	if err != nil {
		p.errorAt(t.Pos, "line number %s is too big", t.Text)
	}
	if len(p.blocks) == 0 && p.inline == 0 {
		p.line = &ast.Line{At: t.Pos, Number: n}
		p.f.Lines = append(p.f.Lines, p.line)
	}
}

// block parses the body of a block that opened at `at`.
func (p *parser) block(kind token.Kind, at token.Pos) ([]ast.Stmt, bool) {
	p.blocks = append(p.blocks, block{kind, at})
	defer func() { p.blocks = p.blocks[:len(p.blocks)-1] }()
	var body []ast.Stmt
	closed := p.stmts(kind, func(s ast.Stmt) { body = append(body, s) })
	return body, closed
}

// unclosed reports a block that never reached its closer.
func (p *parser) unclosed(at token.Pos, what, closer string) {
	if p.inline > 0 {
		p.errorAt(at, "%s inside a single-line IF must reach its %s on the same line", what, closer)
		return
	}
	p.errorAt(at, "this %s never reaches its %s; add %s to close it", what, closer, closer)
}

// header parses a block's first line with f, then requires the end of the
// statement. A mistake in the header is reported and the rest of the line
// skipped, but the block's body is still parsed, so its closer still matches.
func (p *parser) header(f func()) {
	defer p.recoverLine(nil)
	f()
	p.stmtEnd()
}

// closeWith2 consumes a two-word closer such as END IF.
func (p *parser) closeWith2() {
	p.next()
	p.next()
}

// stmtSafe parses one statement. On a syntax error it returns nil and skips
// the rest of the line.
func (p *parser) stmtSafe() (s ast.Stmt) {
	inline := p.inline
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
			p.inline = inline
			p.skipLine()
			s = nil
		}
	}()
	s = p.stmt()
	if c, ok := p.closerHere(); ok && p.isOpen(c.owner) {
		return s
	}
	p.stmtEnd()
	return s
}

// topLevel reports a declaration that must not be inside a block.
func (p *parser) topLevel(t token.Token) {
	if len(p.blocks) > 0 || p.inline > 0 {
		where := "a single-line IF"
		if len(p.blocks) > 0 {
			where = opener(p.blocks[len(p.blocks)-1].kind)
		}
		p.errorAt(t.Pos, "%s belongs at the top level of the file, not inside %s", t.Kind, where)
	}
}

// ---- statements ----

func (p *parser) stmt() ast.Stmt {
	t := p.tok
	switch t.Kind {
	case token.LET:
		p.next()
		return p.assign(t.Pos, true)
	case token.Ident:
		return p.assign(t.Pos, false)
	case token.PRINT:
		return p.print()
	case token.DIM:
		return p.dim(t.Pos, false)
	case token.SHARED:
		p.topLevel(t)
		p.next()
		if p.tok.Kind != token.DIM {
			p.fail(p.tok.Pos, "expected DIM after SHARED, like SHARED DIM hits AS INTEGER")
		}
		return p.dim(t.Pos, true)
	case token.CONST:
		return p.constDecl(t.Pos, false)
	case token.EXPORT:
		p.topLevel(t)
		p.next()
		switch p.tok.Kind {
		case token.FUNCTION, token.SUB:
			return p.funcDecl(t.Pos, true)
		case token.CONST:
			return p.constDecl(t.Pos, true)
		}
		p.fail(p.tok.Pos, "EXPORT goes before FUNCTION, SUB or CONST")
	case token.FUNCTION, token.SUB:
		p.topLevel(t)
		return p.funcDecl(t.Pos, false)
	case token.IMPORT:
		p.topLevel(t)
		p.importDecl()
		return nil
	case token.IF:
		return p.ifStmt()
	case token.FOR:
		return p.forStmt()
	case token.WHILE:
		return p.whileStmt()
	case token.DO:
		return p.doStmt()
	case token.SELECT:
		return p.selectStmt()
	case token.TRY:
		return p.tryStmt()
	case token.TRANSACTION:
		return p.transaction()
	case token.TEST:
		p.topLevel(t)
		return p.testDecl()
	case token.EXIT:
		p.next()
		switch k := p.tok.Kind; k {
		case token.FOR, token.WHILE, token.DO:
			p.next()
			return &ast.ExitStmt{At: t.Pos, Loop: k}
		}
		p.fail(p.tok.Pos, "expected FOR, WHILE or DO after EXIT")
	case token.END:
		p.next()
		switch k := p.tok.Kind; k {
		case token.FOR:
			p.fail(t.Pos, "a FOR loop ends with NEXT, not END FOR")
		case token.WHILE:
			p.fail(t.Pos, "a WHILE loop ends with WEND, not END WHILE")
		case token.DO:
			p.fail(t.Pos, "a DO loop ends with LOOP, not END DO")
		}
		return &ast.EndStmt{At: t.Pos}
	case token.RETURN:
		p.next()
		s := &ast.ReturnStmt{At: t.Pos}
		if !p.endOfStmt() {
			s.Value = p.expr()
		}
		return s
	case token.THROW:
		p.next()
		return &ast.ThrowStmt{At: t.Pos, Value: p.operand(precLowest, t)}
	case token.ROUTE:
		return p.route()
	case token.USE:
		p.next()
		s := &ast.UseStmt{At: t.Pos, Fn: p.operand(precLowest, t)}
		if p.tok.Kind == token.ON {
			p.next()
			s.Scope = p.expect(token.String, `expected a path in quotes after ON, like USE APIKEY("API_KEY") ON "/v1/*"`).Text
		}
		return s
	case token.RESPOND:
		p.next()
		s := &ast.RespondStmt{At: t.Pos, Status: p.operand(precLowest, t)}
		p.expect(token.Comma, "RESPOND needs a status and a value, like RESPOND 200, {\"ok\": TRUE}")
		s.Value = p.operand(precLowest, t)
		if p.tok.Kind == token.Comma {
			p.next()
			s.Headers = p.operand(precLowest, t)
		}
		return s
	case token.SERVE:
		p.next()
		s := &ast.ServeStmt{At: t.Pos}
		if !p.endOfStmt() {
			s.Port = p.expr()
		}
		return s
	case token.DBCONNECT:
		p.next()
		s := &ast.DBConnectStmt{At: t.Pos, URL: p.operand(precLowest, t)}
		if p.tok.Kind == token.AS {
			p.next()
			s.Name = p.name("expected a connection name after AS, like DBCONNECT url AS reports")
		}
		return s
	case token.LOG:
		p.next()
		s := &ast.LogStmt{At: t.Pos, Level: p.operand(precLowest, t)}
		p.expect(token.Comma, `LOG needs a level and a message, like LOG "INFO", "STARTED"`)
		s.Msg = p.operand(precLowest, t)
		if p.tok.Kind == token.Comma {
			p.next()
			s.Fields = p.operand(precLowest, t)
		}
		return s
	case token.EXPECT:
		p.next()
		s := &ast.ExpectStmt{At: t.Pos, Cond: p.operand(precLowest, t)}
		if p.tok.Kind == token.Comma {
			p.next()
			s.Msg = p.operand(precLowest, t)
		}
		return s
	case token.GOTO:
		p.next()
		return &ast.GotoStmt{At: t.Pos, Line: p.lineRef("GOTO")}
	case token.GOSUB:
		p.next()
		return &ast.GosubStmt{At: t.Pos, Line: p.lineRef("GOSUB")}
	case token.ON:
		p.next()
		s := &ast.OnGotoStmt{At: t.Pos, X: p.operand(precLowest, t)}
		if p.tok.Kind == token.GOSUB {
			p.fail(p.tok.Pos, "ON … GOSUB isn't supported; use ON … GOTO")
		}
		p.expect(token.GOTO, "expected GOTO after ON x, like ON x GOTO 100, 200")
		for {
			s.Lines = append(s.Lines, p.lineRef("GOTO"))
			if p.tok.Kind != token.Comma {
				return s
			}
			p.next()
		}
	case token.Number:
		if p.mode == token.Structured {
			p.fail(t.Pos, "expected a statement, not the number %s", t.Text)
		}
		p.fail(t.Pos, "expected a statement, not %s; line numbers go only at the start of a line", t.Text)
	case token.Colon:
		p.stmtEnd()
	}
	if t.Kind.IsKeyword() {
		p.fail(t.Pos, "%s can't start a statement", t.Kind)
	}
	p.fail(t.Pos, "expected a statement, not %s", describe(t))
	return nil
}

// name parses an identifier and returns it in canonical (upper) case.
func (p *parser) name(hint string) string {
	t := p.tok
	if t.Kind != token.Ident {
		if t.Kind.IsKeyword() {
			p.fail(t.Pos, "%s is a reserved word, so it can't be used as a name", t.Kind)
		}
		p.fail(t.Pos, "%s", hint)
	}
	p.next()
	return strings.ToUpper(t.Text)
}

// lineRef parses the line number after GOTO, GOSUB or ON … GOTO.
func (p *parser) lineRef(after string) int {
	t := p.tok
	if t.Kind != token.Number {
		p.fail(t.Pos, "expected a line number after %s", after)
	}
	n, err := strconv.Atoi(t.Text)
	if err != nil {
		p.fail(t.Pos, "%s isn't a line number; line numbers are whole numbers", t.Text)
	}
	p.next()
	return n
}

// assign parses [LET] target = value, or a call used as a statement.
func (p *parser) assign(at token.Pos, hasLet bool) ast.Stmt {
	t := p.tok
	if t.Kind != token.Ident {
		p.name("expected a variable name after LET")
	}
	target := p.postfix(p.primary())
	if p.tok.Kind == token.Eq {
		eq := p.tok
		if _, ok := target.(*ast.CallExpr); ok {
			p.fail(eq.Pos, "can't assign to a function call")
		}
		p.next()
		return &ast.LetStmt{At: at, Target: target, Value: p.operand(precLowest, eq)}
	}
	if call, ok := target.(*ast.CallExpr); ok && !hasLet {
		return &ast.ExprStmt{At: at, X: call}
	}
	if hasLet {
		p.fail(p.tok.Pos, "expected = after LET %s", t.Text)
	}
	if _, ok := target.(*ast.Ident); ok {
		p.fail(p.tok.Pos, "expected = after %s to give it a value, or ( to call it; is it a misspelled keyword?", t.Text)
	}
	p.fail(p.tok.Pos, "expected = to assign a value here")
	return nil
}

func (p *parser) print() ast.Stmt {
	s := &ast.PrintStmt{At: p.tok.Pos}
	p.next()
	for !p.endOfStmt() {
		s.Args = append(s.Args, p.expr())
		if p.tok.Kind != token.Comma && p.tok.Kind != token.Semicolon {
			break
		}
		p.next()
	}
	return s
}

func (p *parser) typeRef() ast.TypeRef {
	t := p.tok
	switch t.Kind {
	case token.Ident:
		p.next()
		name := strings.ToUpper(t.Text)
		if name == "ANY" {
			name = ""
		}
		return ast.TypeRef{Name: name}
	case token.NIL:
		p.next()
		return ast.TypeRef{Name: "NIL"}
	}
	p.fail(t.Pos, "expected a type after AS, like NUMBER, INTEGER, STRING, BOOLEAN, MAP, ARRAY, DECIMAL or DATE")
	return ast.TypeRef{}
}

func (p *parser) dim(at token.Pos, shared bool) ast.Stmt {
	p.next() // DIM
	s := &ast.DimStmt{At: at, Shared: shared}
	s.Name = p.name("expected a variable name after DIM")
	p.expect(token.AS, "expected AS and a type after DIM "+s.Name+", like DIM "+s.Name+" AS INTEGER")
	s.Type = p.typeRef()
	if p.tok.Kind == token.Eq {
		eq := p.tok
		p.next()
		s.Value = p.operand(precLowest, eq)
	}
	return s
}

func (p *parser) constDecl(at token.Pos, export bool) ast.Stmt {
	p.next() // CONST
	s := &ast.ConstDecl{At: at, Export: export}
	s.Name = p.name("expected a name after CONST")
	eq := p.expect(token.Eq, "expected = and a value after CONST "+s.Name+", like CONST "+s.Name+" = 3")
	s.Value = p.operand(precLowest, eq)
	return s
}

func (p *parser) funcDecl(at token.Pos, export bool) ast.Stmt {
	kw := p.tok
	d := &ast.FuncDecl{At: at, IsSub: kw.Kind == token.SUB, Export: export}
	what := kw.Kind.String()
	p.header(func() {
		p.next()
		d.Name = p.name("expected a name after " + what)
		p.expect(token.LParen, "expected ( after "+what+" "+d.Name+", even when it takes no parameters")
		for p.tok.Kind != token.RParen {
			var prm ast.Param
			prm.Name = p.name("expected a parameter name")
			if p.tok.Kind == token.AS {
				p.next()
				prm.Type = p.typeRef()
			}
			if p.tok.Kind == token.Eq {
				eq := p.tok
				p.next()
				prm.Default = p.operand(precLowest, eq)
			}
			d.Params = append(d.Params, prm)
			if p.tok.Kind != token.Comma {
				break
			}
			p.next()
		}
		p.expect(token.RParen, "expected , or ) in the parameter list")
		if p.tok.Kind == token.AS {
			if d.IsSub {
				p.fail(p.tok.Pos, "a SUB returns nothing, so it has no AS type; make it a FUNCTION")
			}
			p.next()
			d.Result = p.typeRef()
		}
	})
	body, closed := p.block(kw.Kind, at)
	d.Body = body
	if !closed {
		p.unclosed(at, what, "END "+what)
		return d
	}
	p.closeWith2()
	return d
}

func (p *parser) importDecl() {
	imp := &ast.Import{At: p.tok.Pos}
	p.next()
	switch p.tok.Kind {
	case token.String:
		imp.Path = p.tok.Text
		p.next()
		p.f.Imports = append(p.f.Imports, imp)
		return
	case token.LBrace:
		p.next()
		var raw []string
		p.skipNewlines()
		for p.tok.Kind != token.RBrace {
			t := p.tok
			if t.Kind != token.Ident {
				p.fail(t.Pos, "expected a name to import inside { }")
			}
			raw = append(raw, t.Text)
			p.next()
			if !p.listSep(token.RBrace, "expected , or } in the import list") {
				break
			}
		}
		p.expect(token.RBrace, "expected , or } in the import list")
		p.expect(token.FROM, `expected FROM after the names, like IMPORT { CalcTax } FROM "./billing.bas"`)
		if p.tok.Kind == token.GO {
			p.next()
			imp.FromGo = true
		}
		imp.Path = p.expect(token.String, "expected a path in quotes after FROM").Text
		for _, n := range raw {
			if !imp.FromGo {
				n = strings.ToUpper(n) // BASIC names are case-insensitive; Go names are not
			}
			imp.Names = append(imp.Names, n)
		}
		if imp.Names == nil {
			imp.Names = []string{}
		}
	case token.Ident:
		imp.Alias = strings.ToUpper(p.tok.Text)
		p.next()
		p.expect(token.FROM, `expected FROM after the package name, like IMPORT uuid FROM GO "github.com/google/uuid"`)
		p.expect(token.GO, `IMPORT name FROM … is only for Go packages (IMPORT uuid FROM GO "…"); for a .bas file use IMPORT { Name } FROM "./file.bas"`)
		imp.FromGo = true
		imp.Path = p.expect(token.String, "expected the Go package path in quotes").Text
	default:
		p.fail(p.tok.Pos, `expected a path in quotes or { names } after IMPORT, like IMPORT "./billing.bas"`)
	}
	p.f.Imports = append(p.f.Imports, imp)
}

func (p *parser) ifStmt() ast.Stmt {
	root := &ast.IfStmt{At: p.tok.Pos}
	blockForm := p.lineIsBlockIf()
	failed := false
	func() {
		defer p.recoverLine(&failed)
		p.next()
		root.Cond = p.cond("IF")
	}()
	if failed {
		// The condition was broken. Guess the IF's form from the shape of the
		// line, so a block IF's END IF still finds it.
		if blockForm {
			return p.ifBlock(root)
		}
		return root
	}
	if p.tok.Kind == token.Newline || p.tok.Kind == token.EOF {
		return p.ifBlock(root)
	}
	// Single-line IF: the rest of the line is the THEN part. In classic
	// mode, every :-separated statement on the line belongs to it.
	p.inline++
	defer func() { p.inline-- }()
	for {
		if s := p.stmtSafe(); s != nil {
			root.Then = append(root.Then, s)
		}
		if p.mode != token.Classic || p.tok.Kind != token.Colon {
			break
		}
		p.next()
		if p.endOfStmt() {
			break
		}
	}
	return root
}

// lineIsBlockIf reports whether the IF line at the current token looks like a
// block IF: it ends with THEN, or has no THEN at all.
func (p *parser) lineIsBlockIf() bool {
	last, then := token.Illegal, false
	for j := p.i; j < len(p.toks) && p.toks[j].Kind != token.Newline && p.toks[j].Kind != token.EOF; j++ {
		last = p.toks[j].Kind
		then = then || last == token.THEN
	}
	return last == token.THEN || !then
}

// cond parses an IF or ELSEIF condition and its THEN. A missing THEN is
// reported, but parsing continues as if it were there.
func (p *parser) cond(after string) ast.Expr {
	if p.endOfStmt() || p.tok.Kind == token.THEN {
		p.fail(p.tok.Pos, "expected a condition after %s", after)
	}
	c := p.expr()
	if p.tok.Kind == token.THEN {
		p.next()
		if p.tok.Kind == token.Number {
			p.fail(p.tok.Pos, "expected a statement after THEN; to jump, write THEN GOTO %s", p.tok.Text)
		}
		return c
	}
	p.errorAt(p.tok.Pos, "expected THEN after %s condition", after)
	if !p.endOfStmt() {
		panic(bailout{})
	}
	return c
}

func (p *parser) ifBlock(root *ast.IfStmt) ast.Stmt {
	p.blocks = append(p.blocks, block{token.IF, root.At})
	defer func() { p.blocks = p.blocks[:len(p.blocks)-1] }()
	cur := root
	var closed bool
	cur.Then, closed = p.blockBody(token.IF)
	seenElse := false
	for closed {
		t := p.tok
		switch t.Kind {
		case token.ELSEIF:
			if seenElse {
				p.errorAt(t.Pos, "ELSEIF can't come after ELSE")
			}
			nested := &ast.IfStmt{At: t.Pos}
			p.header(func() {
				p.next()
				nested.Cond = p.cond("ELSEIF")
			})
			cur.Else = []ast.Stmt{nested}
			cur = nested
			cur.Then, closed = p.blockBody(token.IF)
		case token.ELSE:
			if seenElse {
				p.errorAt(t.Pos, "this IF already has an ELSE")
			}
			seenElse = true
			p.header(func() {
				p.next()
				if p.tok.Kind == token.IF {
					p.fail(p.tok.Pos, "write ELSEIF as one word")
				}
			})
			cur.Else, closed = p.blockBody(token.IF)
		default: // END IF
			p.closeWith2()
			return root
		}
	}
	p.unclosed(root.At, "IF", "END IF")
	return root
}

// blockBody parses statements up to the next closer of the innermost open
// block, which has the given kind.
func (p *parser) blockBody(kind token.Kind) ([]ast.Stmt, bool) {
	var body []ast.Stmt
	closed := p.stmts(kind, func(s ast.Stmt) { body = append(body, s) })
	return body, closed
}

func (p *parser) forStmt() ast.Stmt {
	at := p.tok.Pos
	p.next()
	if p.tok.Kind == token.EACH {
		s := &ast.ForEachStmt{At: at}
		p.header(func() {
			p.next()
			s.Var = p.name("expected a variable name after FOR EACH")
			in := p.expect(token.IN, "expected IN after FOR EACH "+s.Var+", like FOR EACH item IN items")
			s.In = p.operand(precLowest, in)
		})
		var closed bool
		s.Body, closed = p.block(token.FOR, at)
		p.endFor(at, closed, s.Var)
		return s
	}
	s := &ast.ForStmt{At: at}
	p.header(func() {
		s.Var = p.name("expected a variable name after FOR, like FOR i = 1 TO 10")
		eq := p.expect(token.Eq, "expected = after FOR "+s.Var+", like FOR "+s.Var+" = 1 TO 10")
		s.From = p.operand(precLowest, eq)
		to := p.expect(token.TO, "expected TO after the start value, like FOR "+s.Var+" = 1 TO 10")
		s.To = p.operand(precLowest, to)
		if p.tok.Kind == token.STEP {
			step := p.tok
			p.next()
			s.Step = p.operand(precLowest, step)
		}
	})
	var closed bool
	s.Body, closed = p.block(token.FOR, at)
	p.endFor(at, closed, s.Var)
	return s
}

// endFor consumes NEXT [var].
func (p *parser) endFor(at token.Pos, closed bool, v string) {
	if !closed {
		p.unclosed(at, "FOR", "NEXT")
		return
	}
	p.next() // NEXT
	if t := p.tok; t.Kind == token.Ident {
		p.next()
		if v != "" && !strings.EqualFold(t.Text, v) {
			p.errorAt(t.Pos, "NEXT %s doesn't match FOR %s", t.Text, v)
		}
	}
}

func (p *parser) whileStmt() ast.Stmt {
	s := &ast.WhileStmt{At: p.tok.Pos}
	p.header(func() {
		w := p.tok
		p.next()
		s.Cond = p.operand(precLowest, w)
	})
	var closed bool
	s.Body, closed = p.block(token.WHILE, s.At)
	if !closed {
		p.unclosed(s.At, "WHILE", "WEND")
		return s
	}
	p.next()
	return s
}

func (p *parser) doStmt() ast.Stmt {
	s := &ast.DoStmt{At: p.tok.Pos}
	p.header(func() {
		p.next()
		if p.tok.Kind == token.WHILE || p.tok.Kind == token.UNTIL {
			p.fail(p.tok.Pos, "DO takes no condition; put it at the end with LOOP UNTIL, or use WHILE … WEND")
		}
	})
	var closed bool
	s.Body, closed = p.block(token.DO, s.At)
	if !closed {
		p.unclosed(s.At, "DO", "LOOP")
		return s
	}
	p.next()
	switch p.tok.Kind {
	case token.UNTIL:
		until := p.tok
		p.next()
		s.Until = p.operand(precLowest, until)
	case token.WHILE:
		p.fail(p.tok.Pos, "LOOP takes UNTIL, not WHILE; write LOOP UNTIL NOT (…), or use WHILE … WEND")
	}
	return s
}

func (p *parser) selectStmt() ast.Stmt {
	s := &ast.SelectStmt{At: p.tok.Pos}
	p.header(func() {
		p.next()
		c := p.expect(token.CASE, "expected CASE after SELECT, like SELECT CASE x")
		s.X = p.operand(precLowest, c)
	})
	p.blocks = append(p.blocks, block{token.SELECT, s.At})
	defer func() { p.blocks = p.blocks[:len(p.blocks)-1] }()

	stray, closed := p.blockBody(token.SELECT)
	if len(stray) > 0 {
		p.errorAt(stray[0].Pos(), "statements inside SELECT CASE must come after a CASE line")
	}
	seenElse := false
	for closed {
		t := p.tok
		if t.Kind == token.END {
			p.closeWith2()
			return s
		}
		// CASE
		var values []ast.Expr
		isElse := false
		p.header(func() {
			p.next()
			if p.tok.Kind == token.ELSE {
				p.next()
				isElse = true
				return
			}
			for {
				values = append(values, p.operand(precLowest, t))
				if p.tok.Kind != token.Comma {
					return
				}
				p.next()
			}
		})
		if seenElse {
			p.errorAt(t.Pos, "CASE ELSE must be the last CASE")
		}
		var body []ast.Stmt
		body, closed = p.blockBody(token.SELECT)
		if isElse {
			seenElse = true
			s.Else = body
		} else {
			s.Cases = append(s.Cases, ast.SelectCase{Values: values, Body: body})
		}
	}
	p.unclosed(s.At, "SELECT CASE", "END SELECT")
	return s
}

func (p *parser) tryStmt() ast.Stmt {
	s := &ast.TryStmt{At: p.tok.Pos}
	p.header(p.next)
	p.blocks = append(p.blocks, block{token.TRY, s.At})
	defer func() { p.blocks = p.blocks[:len(p.blocks)-1] }()

	var closed bool
	s.Body, closed = p.blockBody(token.TRY)
	seenCatch, seenFinally := false, false
	for closed {
		t := p.tok
		switch t.Kind {
		case token.CATCH:
			if seenCatch || seenFinally {
				p.errorAt(t.Pos, "a TRY has one CATCH, before any FINALLY")
			}
			seenCatch = true
			p.header(func() {
				p.next()
				s.CatchVar = p.name("expected a name after CATCH, like CATCH e")
			})
			s.Catch, closed = p.blockBody(token.TRY)
		case token.FINALLY:
			if seenFinally {
				p.errorAt(t.Pos, "a TRY has only one FINALLY")
			}
			seenFinally = true
			p.header(p.next)
			s.Finally, closed = p.blockBody(token.TRY)
		default: // END TRY
			if !seenCatch {
				p.errorAt(s.At, "this TRY has no CATCH; add CATCH e before END TRY")
			}
			p.closeWith2()
			return s
		}
	}
	p.unclosed(s.At, "TRY", "END TRY")
	return s
}

func (p *parser) transaction() ast.Stmt {
	s := &ast.TransactionStmt{At: p.tok.Pos}
	p.header(func() {
		p.next()
		if p.tok.Kind == token.ON {
			p.next()
			s.Name = p.name("expected a connection name after TRANSACTION ON")
		}
	})
	var closed bool
	s.Body, closed = p.block(token.TRANSACTION, s.At)
	if !closed {
		p.unclosed(s.At, "TRANSACTION", "END TRANSACTION")
		return s
	}
	p.closeWith2()
	return s
}

func (p *parser) testDecl() ast.Stmt {
	s := &ast.TestDecl{At: p.tok.Pos}
	p.header(func() {
		p.next()
		s.Name = p.expect(token.String, `expected the test's name in quotes, like TEST "ADDS TAX"`).Text
	})
	var closed bool
	s.Body, closed = p.block(token.TEST, s.At)
	if !closed {
		p.unclosed(s.At, "TEST", "END TEST")
		return s
	}
	p.closeWith2()
	return s
}

// methods are the HTTP methods ROUTE accepts.
var methods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true,
}

func (p *parser) route() ast.Stmt {
	at := p.tok.Pos
	p.next()
	if p.tok.Kind == token.GROUP {
		g := &ast.GroupStmt{At: at}
		p.header(func() {
			p.next()
			g.Prefix = p.expect(token.String, `expected a path prefix in quotes, like ROUTE GROUP "/v1"`).Text
		})
		body, closed := p.block(token.GROUP, at)
		for _, s := range body {
			if r, ok := s.(*ast.RouteStmt); ok {
				g.Routes = append(g.Routes, r)
			} else {
				p.errorAt(s.Pos(), "only ROUTE lines can go inside ROUTE GROUP")
			}
		}
		if !closed {
			p.unclosed(at, "ROUTE GROUP", "END GROUP")
			return g
		}
		p.closeWith2()
		return g
	}
	r := &ast.RouteStmt{At: at}
	m := p.tok
	if m.Kind != token.Ident || !methods[strings.ToUpper(m.Text)] {
		p.fail(m.Pos, "expected an HTTP method after ROUTE: GET, POST, PUT, PATCH, DELETE, HEAD or OPTIONS")
	}
	r.Method = strings.ToUpper(m.Text)
	p.next()
	r.Path = p.expect(token.String, `expected the route's path in quotes, like ROUTE GET "/hello" CALL Hello`).Text
	switch p.tok.Kind {
	case token.CALL:
		p.next()
		r.Handler = p.name("expected a function name after CALL")
	case token.GOSUB:
		p.next()
		r.GosubLine = p.lineRef("GOSUB")
	default:
		p.fail(p.tok.Pos, "expected CALL FunctionName (or GOSUB line in classic mode) after the route's path")
	}
	return r
}
