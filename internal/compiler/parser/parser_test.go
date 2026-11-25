package parser

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

func parseSrc(t *testing.T, file, src string) (*ast.File, diag.List) {
	t.Helper()
	toks, mode, lds := lexer.Lex(file, []byte(src))
	if lds.HasErrors() {
		return nil, lds
	}
	f, pds := Parse(file, toks, mode)
	return f, append(lds, pds...)
}

func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, ds := parseSrc(t, "test.bas", src)
	if len(ds) > 0 {
		t.Fatalf("unexpected diagnostics for:\n%s\ngot: %v", src, ds)
	}
	return f
}

// formatExpr formats an expression as an explicit S-expression to verify
// exact precedence, associativity, and AST node kinds.
func formatExpr(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.NumberLit:
		return x.Text
	case *ast.StringLit:
		return fmt.Sprintf("%q", x.Value)
	case *ast.BoolLit:
		if x.Value {
			return "TRUE"
		}
		return "FALSE"
	case *ast.NilLit:
		return "NIL"
	case *ast.UnaryExpr:
		return fmt.Sprintf("(%s %s)", x.Op, formatExpr(x.X))
	case *ast.BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", x.Op, formatExpr(x.X), formatExpr(x.Y))
	case *ast.MemberExpr:
		return fmt.Sprintf("(. %s %s)", formatExpr(x.X), x.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("([] %s %s)", formatExpr(x.X), formatExpr(x.Index))
	case *ast.CallExpr:
		args := make([]string, len(x.Args))
		for i, a := range x.Args {
			args[i] = formatExpr(a)
		}
		return fmt.Sprintf("(call %s %s)", formatExpr(x.Fn), strings.Join(args, " "))
	case *ast.ArrayLit:
		items := make([]string, len(x.Items))
		for i, it := range x.Items {
			items[i] = formatExpr(it)
		}
		return fmt.Sprintf("[%s]", strings.Join(items, " "))
	case *ast.MapLit:
		parts := make([]string, len(x.Keys))
		for i, k := range x.Keys {
			parts[i] = fmt.Sprintf("%q:%s", k, formatExpr(x.Values[i]))
		}
		return fmt.Sprintf("{%s}", strings.Join(parts, " "))
	default:
		return fmt.Sprintf("<%T>", e)
	}
}

func TestExprPrecedence(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"add_mul", "1 + 2 * 3", "(+ 1 (* 2 3))"},
		{"mul_add", "1 * 2 + 3", "(+ (* 1 2) 3)"},
		{"parens", "(1 + 2) * 3", "(* (+ 1 2) 3)"},
		{"left_assoc_sub", "10 - 3 - 2", "(- (- 10 3) 2)"},
		{"div_mod_intdiv", `10 \ 3 MOD 2 * 4 / 2`, `(/ (* (MOD (\ 10 3) 2) 4) 2)`},
		{"pow_over_mul", "2 * 3 ^ 2", "(* 2 (^ 3 2))"},
		{"pow_right_assoc", "2 ^ 3 ^ 2", "(^ 2 (^ 3 2))"},
		{"unary_over_pow", "-2 ^ 2", "(^ (- 2) 2)"},
		{"pow_unary_rhs", "2 ^ -3", "(^ 2 (- 3))"},
		{"double_neg", "--x", "(- (- X))"},
		{"unary_postfix", "-u.val", "(- (. U val))"},
		{"concat_below_add", "1 + 2 & 3", "(& (+ 1 2) 3)"},
		{"concat_rhs_add", "1 & 2 + 3", "(& 1 (+ 2 3))"},
		{"compare_below_concat", `"a" & "b" = "ab"`, `(= (& "a" "b") "ab")`},
		{"not_below_compare", "NOT a = b", "(NOT (= A B))"},
		{"not_above_and", "NOT a AND b", "(AND (NOT A) B)"},
		{"and_above_or", "a OR b AND c", "(OR A (AND B C))"},
		{"full_chain", "a OR b AND NOT c < d & e + f * g ^ -h.i[0](1)",
			"(OR A (AND B (NOT (< C (& D (+ E (* F (^ G (- (call ([] (. H i) 0) 1))))))))))"},
		{"case_sensitive_member", "user.firstName", "(. USER firstName)"},
		{"keyword_member", "r.end", "(. R end)"},
		{"rem_member", "r.rem", "(. R rem)"},
		{"map_and_array", `{"Name": [1, TRUE, NIL]}`, `{"Name":[1 TRUE NIL]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mustParse(t, "PRINT "+tt.expr)
			if len(f.Body) != 1 {
				t.Fatalf("got %d statements, want 1", len(f.Body))
			}
			ps, ok := f.Body[0].(*ast.PrintStmt)
			if !ok || len(ps.Args) != 1 {
				t.Fatalf("got %T, want PrintStmt with 1 arg", f.Body[0])
			}
			if got := formatExpr(ps.Args[0]); got != tt.want {
				t.Errorf("expr %s:\ngot  %s\nwant %s", tt.expr, got, tt.want)
			}
		})
	}
}

func TestNumberLitIsInt(t *testing.T) {
	f := mustParse(t, "PRINT 42, 3.14, 1e6, 2.5E-3")
	ps := f.Body[0].(*ast.PrintStmt)
	wantInt := []bool{true, false, false, false}
	if len(ps.Args) != len(wantInt) {
		t.Fatalf("got %d args, want %d", len(ps.Args), len(wantInt))
	}
	for i, w := range wantInt {
		nl := ps.Args[i].(*ast.NumberLit)
		if nl.IsInt != w {
			t.Errorf("number %s: IsInt=%v, want %v", nl.Text, nl.IsInt, w)
		}
	}
}

func TestMultilineLiterals(t *testing.T) {
	src := `
m = {
  "name": "Ada",
  "nums": [
    1,
    2,
    3,
  ],
}
`
	f := mustParse(t, src)
	letStmt := f.Body[0].(*ast.LetStmt)
	ml := letStmt.Value.(*ast.MapLit)
	if len(ml.Keys) != 2 || ml.Keys[0] != "name" || ml.Keys[1] != "nums" {
		t.Fatalf("unexpected map keys: %v", ml.Keys)
	}
	al := ml.Values[1].(*ast.ArrayLit)
	if len(al.Items) != 3 {
		t.Fatalf("got %d array items, want 3", len(al.Items))
	}
}

func TestStructuredStatements(t *testing.T) {
	t.Run("if_elseif_else", func(t *testing.T) {
		src := `
IF x > 10 THEN
  PRINT "A"
ELSEIF x > 5 THEN
  PRINT "B"
ELSE
  PRINT "C"
END IF
`
		f := mustParse(t, src)
		ifs := f.Body[0].(*ast.IfStmt)
		if len(ifs.Then) != 1 || len(ifs.Else) != 1 {
			t.Fatalf("unexpected IF structure: then=%d else=%d", len(ifs.Then), len(ifs.Else))
		}
		elif, ok := ifs.Else[0].(*ast.IfStmt)
		if !ok {
			t.Fatalf("ELSEIF should be nested IfStmt, got %T", ifs.Else[0])
		}
		if len(elif.Then) != 1 || len(elif.Else) != 1 {
			t.Fatalf("unexpected ELSEIF structure: then=%d else=%d", len(elif.Then), len(elif.Else))
		}
	})

	t.Run("if_single_line", func(t *testing.T) {
		f := mustParse(t, `IF x = 1 THEN PRINT "YES"`)
		ifs := f.Body[0].(*ast.IfStmt)
		if len(ifs.Then) != 1 || len(ifs.Else) != 0 {
			t.Fatalf("unexpected single-line IF: then=%d else=%d", len(ifs.Then), len(ifs.Else))
		}
	})

	t.Run("for_and_for_each", func(t *testing.T) {
		src := `
FOR i = 10 TO 1 STEP -2
  IF i = 4 THEN EXIT FOR
  PRINT i
NEXT i
FOR EACH item IN items
  PRINT item
NEXT
`
		f := mustParse(t, src)
		fs := f.Body[0].(*ast.ForStmt)
		if fs.Var != "I" || fs.Step == nil || len(fs.Body) != 2 {
			t.Fatalf("unexpected ForStmt: %+v", fs)
		}
		fe := f.Body[1].(*ast.ForEachStmt)
		if fe.Var != "ITEM" || len(fe.Body) != 1 {
			t.Fatalf("unexpected ForEachStmt: %+v", fe)
		}
	})

	t.Run("while_and_do", func(t *testing.T) {
		src := `
WHILE n < 10
  n = n + 1
  IF n = 5 THEN EXIT WHILE
WEND
DO
  n = n - 1
  IF n = 0 THEN EXIT DO
LOOP UNTIL n <= 0
DO
  EXIT DO
LOOP
`
		f := mustParse(t, src)
		if len(f.Body) != 3 {
			t.Fatalf("got %d stmts, want 3", len(f.Body))
		}
		if _, ok := f.Body[0].(*ast.WhileStmt); !ok {
			t.Fatalf("stmt 0: got %T, want WhileStmt", f.Body[0])
		}
		do1 := f.Body[1].(*ast.DoStmt)
		if do1.Until == nil {
			t.Fatal("expected Until expression on DO ... LOOP UNTIL")
		}
		do2 := f.Body[2].(*ast.DoStmt)
		if do2.Until != nil {
			t.Fatal("expected nil Until on bare DO ... LOOP")
		}
	})

	t.Run("select_case", func(t *testing.T) {
		src := `
SELECT CASE x
  CASE 1
    PRINT "ONE"
  CASE 2, 3
    PRINT "TWO OR THREE"
  CASE ELSE
    PRINT "OTHER"
END SELECT
`
		f := mustParse(t, src)
		ss := f.Body[0].(*ast.SelectStmt)
		if len(ss.Cases) != 2 || len(ss.Cases[1].Values) != 2 || len(ss.Else) != 1 {
			t.Fatalf("unexpected SelectStmt: %+v", ss)
		}
	})

	t.Run("func_sub_dim_const", func(t *testing.T) {
		src := `
EXPORT CONST TAX = 0.075
SHARED DIM hits AS INTEGER = 0
DIM raw AS ANY

EXPORT FUNCTION Add(a AS INTEGER, b AS INTEGER = 1) AS INTEGER
  RETURN a + b
END FUNCTION

SUB LogIt(msg AS STRING)
  PRINT msg
  RETURN
END SUB
`
		f := mustParse(t, src)
		if len(f.Body) != 5 {
			t.Fatalf("got %d decls, want 5", len(f.Body))
		}
		cd := f.Body[0].(*ast.ConstDecl)
		if !cd.Export || cd.Name != "TAX" {
			t.Fatalf("unexpected ConstDecl: %+v", cd)
		}
		sd := f.Body[1].(*ast.DimStmt)
		if !sd.Shared || sd.Name != "HITS" || sd.Type.Name != "INTEGER" || sd.Value == nil {
			t.Fatalf("unexpected Shared DimStmt: %+v", sd)
		}
		ad := f.Body[2].(*ast.DimStmt)
		if ad.Type.Name != "" {
			t.Fatalf("ANY should normalize to empty TypeRef.Name, got %q", ad.Type.Name)
		}
		fn := f.Body[3].(*ast.FuncDecl)
		if !fn.Export || fn.IsSub || fn.Name != "ADD" || len(fn.Params) != 2 || fn.Params[1].Default == nil || fn.Result.Name != "INTEGER" {
			t.Fatalf("unexpected FuncDecl: %+v", fn)
		}
		sub := f.Body[4].(*ast.FuncDecl)
		if !sub.IsSub || sub.Name != "LOGIT" || len(sub.Params) != 1 {
			t.Fatalf("unexpected Sub FuncDecl: %+v", sub)
		}
	})

	t.Run("imports_and_backend_stmts", func(t *testing.T) {
		src := `
IMPORT "./billing.bas"
IMPORT { CalcTax, CalcTotal } FROM "./billing.bas"
IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT { NewString } FROM GO "github.com/google/uuid"

DBCONNECT ENV("DB") AS main
USE REQUESTLOG()
USE APIKEY("K") ON "/v1/*"

ROUTE GROUP "/v1"
  ROUTE GET "/items/:id" CALL GetItem
  ROUTE POST "/items" CALL CreateItem
END GROUP

SERVE 8080

FUNCTION GetItem(req)
  TRY
    TRANSACTION ON main
      LOG "INFO", "FETCH", {"id": req.params.id}
    END TRANSACTION
    RESPOND 200, {"ok": TRUE}, {"x-test": "1"}
  CATCH e
    THROW HTTPERROR(500, e.message)
  FINALLY
    PRINT "DONE"
  END TRY
END FUNCTION

TEST "works"
  EXPECT 1 = 1, "math"
END TEST
`
		f := mustParse(t, src)
		if len(f.Imports) != 4 {
			t.Fatalf("got %d imports, want 4", len(f.Imports))
		}
		if f.Imports[0].Path != "./billing.bas" || f.Imports[0].Names != nil || f.Imports[0].FromGo {
			t.Fatalf("unexpected import 0: %+v", f.Imports[0])
		}
		if len(f.Imports[1].Names) != 2 || f.Imports[1].Names[0] != "CALCTAX" {
			t.Fatalf("unexpected import 1: %+v", f.Imports[1])
		}
		if f.Imports[2].Alias != "UUID" || !f.Imports[2].FromGo {
			t.Fatalf("unexpected import 2: %+v", f.Imports[2])
		}
		if !f.Imports[3].FromGo || f.Imports[3].Names[0] != "NewString" {
			t.Fatalf("Go named import should preserve case: %+v", f.Imports[3])
		}
	})
}

func TestClassicMode(t *testing.T) {
	t.Run("sort_and_duplicate_replacement", func(t *testing.T) {
		src := `
30 PRINT "THREE"
10 PRINT "ONE"
20 PRINT "OLD"
20 PRINT "TWO"
`
		f := mustParse(t, src)
		if f.Mode != token.Classic {
			t.Fatalf("mode=%v, want classic", f.Mode)
		}
		if len(f.Lines) != 3 {
			t.Fatalf("got %d lines, want 3", len(f.Lines))
		}
		wantNums := []int{10, 20, 30}
		wantText := []string{"ONE", "TWO", "THREE"}
		for i, ln := range f.Lines {
			if ln.Number != wantNums[i] {
				t.Errorf("line %d: number=%d, want %d", i, ln.Number, wantNums[i])
			}
			ps := ln.Stmts[0].(*ast.PrintStmt)
			sl := ps.Args[0].(*ast.StringLit)
			if sl.Value != wantText[i] {
				t.Errorf("line %d: printed %q, want %q", ln.Number, sl.Value, wantText[i])
			}
		}
	})

	t.Run("colon_separator_and_inline_if", func(t *testing.T) {
		src := `
10 A = 1 : B = 2 : PRINT A + B
20 IF A = 1 THEN PRINT "Y" : GOTO 40
30 GOSUB 100
40 ON A GOTO 10, 20, 30
50 END
100 REM SUBROUTINE
110 RETURN
`
		f := mustParse(t, src)
		if len(f.Lines) != 7 {
			t.Fatalf("got %d lines, want 7", len(f.Lines))
		}
		if len(f.Lines[0].Stmts) != 3 {
			t.Fatalf("line 10: got %d stmts, want 3", len(f.Lines[0].Stmts))
		}
		ifs := f.Lines[1].Stmts[0].(*ast.IfStmt)
		if len(ifs.Then) != 2 {
			t.Fatalf("line 20 single-line IF: got %d Then stmts, want 2", len(ifs.Then))
		}
		if _, ok := ifs.Then[1].(*ast.GotoStmt); !ok {
			t.Fatalf("line 20 second Then stmt: got %T, want GotoStmt", ifs.Then[1])
		}
		if gs := f.Lines[2].Stmts[0].(*ast.GosubStmt); gs.Line != 100 {
			t.Fatalf("line 30: got GOSUB %d, want 100", gs.Line)
		}
		og := f.Lines[3].Stmts[0].(*ast.OnGotoStmt)
		if len(og.Lines) != 3 || og.Lines[0] != 10 || og.Lines[2] != 30 {
			t.Fatalf("line 40: unexpected ON GOTO: %+v", og)
		}
		if len(f.Lines[5].Stmts) != 0 {
			t.Fatalf("line 100 REM line should have 0 stmts, got %d", len(f.Lines[5].Stmts))
		}
	})

	t.Run("multiline_block_in_classic", func(t *testing.T) {
		src := `
10 FOR I = 1 TO 3
20 PRINT I
30 NEXT
40 END
`
		f := mustParse(t, src)
		if len(f.Lines) != 2 {
			t.Fatalf("got %d top-level lines, want 2 (10 and 40)", len(f.Lines))
		}
		fs := f.Lines[0].Stmts[0].(*ast.ForStmt)
		if len(fs.Body) != 1 {
			t.Fatalf("FOR body has %d stmts, want 1", len(fs.Body))
		}
	})
}

func TestParserErrors(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantCount int
		wantLine  int
		wantSub   string
	}{
		{
			name:      "missing_then",
			src:       "IF 1 = 1\n  PRINT \"NO THEN\"\nEND IF\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "expected THEN after IF condition",
		},
		{
			name:      "missing_paren",
			src:       "x = 1\nPRINT (x + 2\n",
			wantCount: 1,
			wantLine:  2,
			wantSub:   "expected ) to close the (",
		},
		{
			name:      "next_without_for",
			src:       "PRINT \"A\"\nNEXT\n",
			wantCount: 1,
			wantLine:  2,
			wantSub:   "NEXT without FOR",
		},
		{
			name:      "wend_without_while",
			src:       "WEND\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "WEND without WHILE",
		},
		{
			name:      "end_for_hint",
			src:       "FOR i = 1 TO 3\n  PRINT i\nEND FOR\n",
			wantCount: 2,
			wantLine:  3,
			wantSub:   "NEXT, not END FOR",
		},
		{
			name:      "end_while_hint",
			src:       "WHILE FALSE\nEND WHILE\n",
			wantCount: 2,
			wantLine:  2,
			wantSub:   "WEND, not END WHILE",
		},
		{
			name:      "colon_in_structured",
			src:       "a = 1 : b = 2\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "classic mode",
		},
		{
			name:      "else_if_two_words",
			src:       "IF x = 1 THEN\n  PRINT 1\nELSE IF x = 2 THEN\n  PRINT 2\nEND IF\n",
			wantCount: 1,
			wantLine:  3,
			wantSub:   "ELSEIF as one word",
		},
		{
			name:      "then_number_hint",
			src:       "10 IF X = 1 THEN 20\n20 END\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "THEN GOTO 20",
		},
		{
			name:      "sub_return_type",
			src:       "SUB Foo() AS INTEGER\n  PRINT 1\nEND SUB\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "SUB returns nothing",
		},
		{
			name:      "do_while_hint",
			src:       "DO WHILE x < 5\n  x = x + 1\nLOOP\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "DO takes no condition",
		},
		{
			name:      "loop_while_hint",
			src:       "DO\n  x = x + 1\nLOOP WHILE x < 5\n",
			wantCount: 1,
			wantLine:  3,
			wantSub:   "LOOP takes UNTIL, not WHILE",
		},
		{
			name:      "loop_until_missing_expr",
			src:       "DO\n  x = 1\nLOOP UNTIL\n",
			wantCount: 1,
			wantLine:  3,
			wantSub:   "expected a value after UNTIL",
		},
		{
			name:      "on_gosub_hint",
			src:       "10 ON X GOSUB 100\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "ON … GOSUB isn't supported",
		},
		{
			name:      "next_var_mismatch",
			src:       "FOR i = 1 TO 3\n  PRINT i\nNEXT j\n",
			wantCount: 1,
			wantLine:  3,
			wantSub:   "NEXT j doesn't match FOR I",
		},
		{
			name:      "unclosed_if_inside_function_no_cascade",
			src:       "FUNCTION F()\n  IF x = 1 THEN\n    PRINT 1\nEND FUNCTION\n",
			wantCount: 1,
			wantLine:  2,
			wantSub:   "this IF never reaches its END IF",
		},
		{
			name:      "shared_inside_function",
			src:       "FUNCTION F()\n  SHARED DIM x AS INTEGER\nEND FUNCTION\n",
			wantCount: 1,
			wantLine:  2,
			wantSub:   "top level",
		},
		{
			name:      "assign_to_call",
			src:       "Foo() = 1\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "can't assign to a function call",
		},
		{
			name:      "non_string_map_key",
			src:       "m = {name: 1}\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "map keys are strings",
		},
		{
			name:      "try_without_catch",
			src:       "TRY\n  PRINT 1\nEND TRY\n",
			wantCount: 1,
			wantLine:  1,
			wantSub:   "no CATCH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := parseSrc(t, tt.name+".bas", tt.src)
			if len(ds) != tt.wantCount {
				t.Fatalf("got %d diagnostics %v, want %d", len(ds), ds, tt.wantCount)
			}
			found := false
			for _, d := range ds {
				if d.Kind == diag.Syntax && d.Pos.Line == tt.wantLine && strings.Contains(d.Message, tt.wantSub) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("got %v, want SYNTAX error on line %d containing %q", ds, tt.wantLine, tt.wantSub)
			}
		})
	}
}

func TestParseAllGoldenAndExamples(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "..", "testdata", "golden", "structured"),
		filepath.Join("..", "..", "..", "testdata", "golden", "classic"),
		filepath.Join("..", "..", "..", "examples"),
	}
	count := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".bas") {
				return nil
			}
			count++
			t.Run(filepath.ToSlash(p), func(t *testing.T) {
				src, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				toks, mode, lds := lexer.Lex(filepath.Base(p), src)
				if lds.HasErrors() {
					t.Fatalf("unexpected lex errors: %v", lds)
				}
				_, pds := Parse(filepath.Base(p), toks, mode)
				if pds.HasErrors() {
					t.Fatalf("unexpected parse errors: %v", pds)
				}
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count < 40 {
		t.Fatalf("expected to parse at least 40 golden+example .bas files, parsed %d", count)
	}
}
