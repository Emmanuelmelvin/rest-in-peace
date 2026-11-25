// Package lexer turns .bas source into tokens and detects the file's mode.
package lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Lex splits one .bas file into tokens and reports whether it is classic or
// structured mode (Requirement 1.2). file is the name used in diagnostics.
//
// The token stream always ends with Newline, EOF. Comments produce no tokens.
// Colon is emitted in both modes, because map literals use it too; the parser
// treats it as a statement separator only in classic mode. In classic mode the
// first token of every numbered line is a LineNumber.
func Lex(file string, src []byte) ([]token.Token, token.Mode, diag.List) {
	l := &lexer{file: file, src: string(src), mode: detectMode(string(src))}
	l.run()
	return l.toks, l.mode, l.diags
}

// bom is the UTF-8 byte order mark, which some editors put at the start of a file.
var bom = string(rune(0xFEFF))

type lexer struct {
	file  string
	src   string
	mode  token.Mode
	off   int // byte offset of the next character
	line  int
	col   int // 1-based column of src[off]
	toks  []token.Token
	diags diag.List
	mixed bool // a mixed-mode error was already reported
}

// detectMode reports classic if the first line that is not blank and not a
// comment starts with a digit (Requirement 1.2).
func detectMode(src string) token.Mode {
	for _, ln := range strings.Split(src, "\n") {
		s := strings.TrimLeft(strings.TrimPrefix(ln, bom), " \t\r")
		if s == "" || s[0] == '\'' || isRemAt(s, 0) {
			continue
		}
		if isDigit(s[0]) {
			return token.Classic
		}
		return token.Structured
	}
	return token.Structured
}

func (l *lexer) run() {
	l.line, l.col = 1, 1
	if strings.HasPrefix(l.src, bom) { // UTF-8 byte order mark
		l.off = len(bom)
	}
	for l.off < len(l.src) {
		l.lexLine()
	}
	end := l.pos()
	if n := len(l.toks); n > 0 && l.toks[n-1].Kind != token.Newline {
		l.emit(token.Newline, "", end)
	}
	l.emit(token.EOF, "", end)
}

// lexLine lexes one physical line, including its trailing newline.
func (l *lexer) lexLine() {
	l.skipSpace()
	continuation := l.continues()
	if !continuation && !l.atLineEnd() && !l.atComment() {
		l.checkMode()
		if l.mode == token.Classic && l.off < len(l.src) && isDigit(l.src[l.off]) {
			p := l.pos()
			n := l.scanWhile(isDigit)
			l.emit(token.LineNumber, n, p)
		}
	}
	for {
		l.skipSpace()
		if l.off >= len(l.src) {
			break
		}
		c := l.src[l.off]
		if c == '\n' {
			l.emitNewline()
			l.advance(1)
			l.line++
			l.col = 1
			return
		}
		if l.atComment() {
			l.skipToLineEnd()
			continue
		}
		l.lexToken()
	}
	l.emitNewline()
}

// emitNewline emits a Newline unless the previous token already is one.
func (l *lexer) emitNewline() {
	if n := len(l.toks); n == 0 || l.toks[n-1].Kind == token.Newline {
		return
	}
	l.emit(token.Newline, "", l.pos())
}

// continues reports whether this physical line continues the previous one:
// the previous line ended with an opening bracket or a comma, as in a map
// literal written across several lines. Continuation lines are not checked
// for line numbers.
func (l *lexer) continues() bool {
	n := len(l.toks)
	if n < 2 || l.toks[n-1].Kind != token.Newline {
		return false
	}
	switch l.toks[n-2].Kind {
	case token.LParen, token.LBrack, token.LBrace, token.Comma:
		return true
	}
	return false
}

// checkMode reports the first line whose numbering disagrees with the
// file's mode (Requirement 1.3).
func (l *lexer) checkMode() {
	if l.mixed {
		return
	}
	numbered := isDigit(l.src[l.off])
	switch {
	case l.mode == token.Classic && !numbered:
		l.mixed = true
		l.errorf(l.pos(), "this line has no line number, but the file started in classic mode; number every line, or remove all the numbers for structured mode")
	case l.mode == token.Structured && numbered:
		l.mixed = true
		l.errorf(l.pos(), "this line starts with a line number, but the file started in structured mode; a file can't mix numbered and unnumbered lines")
	}
}

func (l *lexer) lexToken() {
	p := l.pos()
	c := l.src[l.off]
	switch {
	case isLetter(c):
		word := l.scanWhile(isIdentChar)
		l.emit(token.Lookup(word), word, p)
		return
	case isDigit(c):
		l.lexNumber(p)
		return
	case c == '"':
		l.lexString(p)
		return
	}
	two := ""
	if l.off+1 < len(l.src) {
		two = l.src[l.off : l.off+2]
	}
	switch two {
	case "<>":
		l.advance(2)
		l.emit(token.NotEq, two, p)
		return
	case "<=":
		l.advance(2)
		l.emit(token.LtEq, two, p)
		return
	case ">=":
		l.advance(2)
		l.emit(token.GtEq, two, p)
		return
	}
	if k, ok := singles[c]; ok {
		l.advance(1)
		l.emit(k, string(c), p)
		return
	}
	r, size := utf8.DecodeRuneInString(l.src[l.off:])
	l.advance(size)
	l.errorf(p, "%q isn't part of RIP BASIC; remove it or put it inside a string", r)
}

var singles = map[byte]token.Kind{
	'+': token.Plus, '-': token.Minus, '*': token.Star, '/': token.Slash,
	'\\': token.Backslash, '^': token.Caret, '&': token.Amp, '=': token.Eq,
	'<': token.Lt, '>': token.Gt, '(': token.LParen, ')': token.RParen,
	'[': token.LBrack, ']': token.RBrack, '{': token.LBrace, '}': token.RBrace,
	',': token.Comma, '.': token.Dot, ';': token.Semicolon, ':': token.Colon,
}

// lexNumber scans 42, 3.14 or 1e6 (with an optional exponent sign).
func (l *lexer) lexNumber(p token.Pos) {
	start := l.off
	l.scanWhile(isDigit)
	if l.off+1 < len(l.src) && l.src[l.off] == '.' && isDigit(l.src[l.off+1]) {
		l.advance(1)
		l.scanWhile(isDigit)
	}
	if l.off < len(l.src) && (l.src[l.off] == 'e' || l.src[l.off] == 'E') {
		i := l.off + 1
		if i < len(l.src) && (l.src[i] == '+' || l.src[i] == '-') {
			i++
		}
		if i < len(l.src) && isDigit(l.src[i]) {
			l.advance(i - l.off)
			l.scanWhile(isDigit)
		}
	}
	text := l.src[start:l.off]
	if l.off < len(l.src) && isLetter(l.src[l.off]) {
		l.scanWhile(isIdentChar)
		l.errorf(p, "%s isn't a number or a name; names can't start with a digit", l.src[start:l.off])
		return
	}
	l.emit(token.Number, text, p)
}

// lexString scans a double-quoted string. \" and \\ are escapes; any other
// backslash is kept as written. Strings end on the line they start.
func (l *lexer) lexString(p token.Pos) {
	l.advance(1) // opening quote
	var b strings.Builder
	for {
		if l.off >= len(l.src) || l.src[l.off] == '\n' || l.src[l.off] == '\r' && l.off+1 < len(l.src) && l.src[l.off+1] == '\n' {
			l.errorf(p, "this string never ends; add a closing \" before the end of the line")
			return
		}
		c := l.src[l.off]
		if c == '"' {
			l.advance(1)
			l.emit(token.String, b.String(), p)
			return
		}
		if c == '\\' && l.off+1 < len(l.src) && (l.src[l.off+1] == '"' || l.src[l.off+1] == '\\') {
			b.WriteByte(l.src[l.off+1])
			l.advance(2)
			continue
		}
		_, size := utf8.DecodeRuneInString(l.src[l.off:])
		b.WriteString(l.src[l.off : l.off+size])
		l.advance(size)
	}
}

func (l *lexer) atLineEnd() bool {
	return l.off >= len(l.src) || l.src[l.off] == '\n'
}

// atComment reports whether a ' or REM comment starts at the current offset.
func (l *lexer) atComment() bool {
	if l.off >= len(l.src) {
		return false
	}
	if l.src[l.off] == '\'' {
		return true
	}
	if len(l.toks) > 0 && l.toks[len(l.toks)-1].Kind == token.Dot {
		return false
	}
	return isRemAt(l.src, l.off)
}

// isRemAt reports whether the word REM (any case) starts at s[i] and is not
// the start of a longer name such as REMAINDER.
func isRemAt(s string, i int) bool {
	if len(s)-i < 3 || !strings.EqualFold(s[i:i+3], "REM") {
		return false
	}
	if i > 0 && isIdentChar(s[i-1]) {
		return false
	}
	return len(s) == i+3 || !isIdentChar(s[i+3])
}

func (l *lexer) skipToLineEnd() {
	for l.off < len(l.src) && l.src[l.off] != '\n' {
		l.advance(1)
	}
}

func (l *lexer) skipSpace() {
	for l.off < len(l.src) {
		switch l.src[l.off] {
		case ' ', '\t', '\r':
			l.advance(1)
		default:
			return
		}
	}
}

func (l *lexer) scanWhile(ok func(byte) bool) string {
	start := l.off
	for l.off < len(l.src) && ok(l.src[l.off]) {
		l.advance(1)
	}
	return l.src[start:l.off]
}

// advance moves n bytes forward on the current line. Columns count runes.
func (l *lexer) advance(n int) {
	l.col += utf8.RuneCountInString(l.src[l.off : l.off+n])
	l.off += n
}

func (l *lexer) pos() token.Pos { return token.Pos{File: l.file, Line: l.line, Col: l.col} }

func (l *lexer) emit(k token.Kind, text string, p token.Pos) {
	l.toks = append(l.toks, token.Token{Kind: k, Text: text, Pos: p})
}

func (l *lexer) errorf(p token.Pos, format string, args ...any) {
	l.diags = append(l.diags, diag.Errorf(p, diag.Syntax, format, args...))
}

func isDigit(c byte) bool     { return '0' <= c && c <= '9' }
func isLetter(c byte) bool    { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_' }
func isIdentChar(c byte) bool { return isLetter(c) || isDigit(c) }
