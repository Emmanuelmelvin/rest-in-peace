package lexer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// show renders tokens compactly: KIND(text)@line:col, with Newline as ⏎.
func show(toks []token.Token) string {
	var parts []string
	for _, t := range toks {
		switch t.Kind {
		case token.Newline:
			parts = append(parts, "⏎")
		case token.EOF:
			parts = append(parts, "EOF")
		case token.Ident, token.Number, token.String, token.LineNumber:
			parts = append(parts, fmt.Sprintf("%v(%s)", t.Kind, t.Text))
		default:
			parts = append(parts, t.Kind.String())
		}
	}
	return strings.Join(parts, " ")
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"print", `PRINT "HI"`, `PRINT STRING(HI) ⏎ EOF`},
		{"keywords any case", "print Print pRiNt", `PRINT PRINT PRINT ⏎ EOF`},
		{"names keep text", "customerId = 1", `NAME(customerId) = NUMBER(1) ⏎ EOF`},
		{"numbers", "PRINT 42 3.14 1e6 2.5E-3", `PRINT NUMBER(42) NUMBER(3.14) NUMBER(1e6) NUMBER(2.5E-3) ⏎ EOF`},
		{"escapes", `"a\"b\\c\nd"`, `STRING(a"b\c\nd) ⏎ EOF`},
		{"operators", `+ - * / \ ^ & = <> < > <= >= ( ) [ ] { } , . ; :`,
			`+ - * / \ ^ & = <> < > <= >= ( ) [ ] { } , . ; : ⏎ EOF`},
		{"rem comment", "REM hello\nPRINT 1 ' trailing\nrem lower", `PRINT NUMBER(1) ⏎ EOF`},
		{"remainder is a name", "REMAINDER = 1", `NAME(REMAINDER) = NUMBER(1) ⏎ EOF`},
		{"quote in string", `PRINT "DON'T STOP ' HERE"`, `PRINT STRING(DON'T STOP ' HERE) ⏎ EOF`},
		{"blank lines collapse", "A\n\n\nB\n", `NAME(A) ⏎ NAME(B) ⏎ EOF`},
		{"crlf", "A\r\nB\r\n", `NAME(A) ⏎ NAME(B) ⏎ EOF`},
		{"classic", "10 A = 1 : PRINT A\n20 GOTO 10", `LINE NUMBER(10) NAME(A) = NUMBER(1) : PRINT NAME(A) ⏎ LINE NUMBER(20) GOTO NUMBER(10) ⏎ EOF`},
		{"classic with comment lines", "' header\n10 PRINT 1\nREM note\n20 END", `LINE NUMBER(10) PRINT NUMBER(1) ⏎ LINE NUMBER(20) END ⏎ EOF`},
		{"multi-line literal", "x = [1,\n 2,\n 3]", `NAME(x) = [ NUMBER(1) , ⏎ NUMBER(2) , ⏎ NUMBER(3) ] ⏎ EOF`},
		{"member keyword", "r.end", `NAME(r) . END ⏎ EOF`},
		{"member rem", "r.rem", `NAME(r) . NAME(rem) ⏎ EOF`},
		{"empty", "", `EOF`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, _, ds := Lex("t.bas", []byte(tt.src))
			if len(ds) > 0 {
				t.Fatalf("unexpected diagnostics: %v", ds)
			}
			if got := show(toks); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	src := "x = 1\n  PRINT \"é\", y\n"
	toks, _, _ := Lex("p.bas", []byte(src))
	want := []struct {
		kind      token.Kind
		line, col int
	}{
		{token.Ident, 1, 1}, {token.Eq, 1, 3}, {token.Number, 1, 5}, {token.Newline, 1, 6},
		{token.PRINT, 2, 3}, {token.String, 2, 9}, {token.Comma, 2, 12}, {token.Ident, 2, 14},
		{token.Newline, 2, 15}, {token.EOF, 3, 1},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens (%s), want %d", len(toks), show(toks), len(want))
	}
	for i, w := range want {
		g := toks[i]
		if g.Kind != w.kind || g.Pos.Line != w.line || g.Pos.Col != w.col || g.Pos.File != "p.bas" {
			t.Errorf("token %d: got %v at %s:%d:%d, want %v at %d:%d", i, g.Kind, g.Pos.File, g.Pos.Line, g.Pos.Col, w.kind, w.line, w.col)
		}
	}
}

func TestMode(t *testing.T) {
	tests := []struct {
		src  string
		want token.Mode
	}{
		{"PRINT 1", token.Structured},
		{"10 PRINT 1", token.Classic},
		{"REM intro\n' more\n\n10 PRINT 1", token.Classic},
		{"' intro\nPRINT 1", token.Structured},
		{"", token.Structured},
	}
	for _, tt := range tests {
		if _, got, _ := Lex("m.bas", []byte(tt.src)); got != tt.want {
			t.Errorf("%q: mode %v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name, src string
		want      []string // diagnostic prefixes, one per error
	}{
		{"unterminated string", "PRINT \"OK\"\nPRINT \"NO END\n", []string{"?SYNTAX ERROR IN e.bas:2 — this string never ends"}},
		{"classic then structured", "10 PRINT 1\nPRINT 2\nPRINT 3", []string{"?SYNTAX ERROR IN e.bas:2 — this line has no line number"}},
		{"structured then classic", "PRINT 1\n20 PRINT 2", []string{"?SYNTAX ERROR IN e.bas:2 — this line starts with a line number"}},
		{"illegal character", "PRINT 1 # 2\nPRINT @", []string{"?SYNTAX ERROR IN e.bas:1 — '#'", "?SYNTAX ERROR IN e.bas:2 — '@'"}},
		{"digit name", "PRINT 2x", []string{"?SYNTAX ERROR IN e.bas:1 — 2x isn't a number"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ds := Lex("e.bas", []byte(tt.src))
			if len(ds) != len(tt.want) {
				t.Fatalf("got %d diagnostics %v, want %d", len(ds), ds, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.HasPrefix(ds[i].String(), w) {
					t.Errorf("got %q, want prefix %q", ds[i].String(), w)
				}
			}
		})
	}
}
