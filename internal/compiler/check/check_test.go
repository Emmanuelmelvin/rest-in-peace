package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func checkSrc(t *testing.T, file, src string) diag.List {
	t.Helper()
	toks, mode, lds := lexer.Lex(file, []byte(src))
	if lds.HasErrors() {
		t.Fatalf("unexpected lex errors for %s: %v", file, lds)
	}
	f, pds := parser.Parse(file, toks, mode)
	if pds.HasErrors() {
		t.Fatalf("unexpected parse errors for %s: %v", file, pds)
	}
	return Check(&ast.Program{Entry: file, Files: []*ast.File{f}})
}

func TestUndefinedAndHints(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantLine int
		wantSub  string
	}{
		{
			name:     "undefined_function_no_match",
			src:      "PRINT \"A\"\nPRINT Nope(1)\n",
			wantLine: 2,
			wantSub:  "no function named NOPE",
		},
		{
			name:     "undefined_function_did_you_mean",
			src:      "FUNCTION CalcTotal(x)\n  RETURN x\nEND FUNCTION\nPRINT CalcTax(1)\n",
			wantLine: 4,
			wantSub:  "no function named CALCTAX; did you mean CALCTOTAL?",
		},
		{
			name:     "undefined_builtin_did_you_mean",
			src:      "PRINT UPPR(\"hi\")\n",
			wantLine: 1,
			wantSub:  "no function named UPPR; did you mean UPPER?",
		},
		{
			name:     "undefined_variable_did_you_mean",
			src:      "total = 10\nPRINT totl\n",
			wantLine: 2,
			wantSub:  "TOTL is not defined; did you mean TOTAL?",
		},
		{
			name:     "undefined_variable_use_before_assign",
			src:      "PRINT x\nx = 1\n",
			wantLine: 1,
			wantSub:  "X is not defined",
		},
		{
			name:     "undefined_self_init",
			src:      "x = x + 1\n",
			wantLine: 1,
			wantSub:  "X is not defined",
		},
		{
			name:     "undefined_route_handler_hint",
			src:      "ROUTE GET \"/hi\" CALL Helo\nFUNCTION Hello(req)\n  RETURN 1\nEND FUNCTION\n",
			wantLine: 1,
			wantSub:  "no function named HELO; did you mean HELLO?",
		},
		{
			name:     "classic_helper_in_structured_hint",
			src:      "FUNCTION F(req)\n  RETURN PARAM(\"id\")\nEND FUNCTION\n",
			wantLine: 2,
			wantSub:  "req.params",
		},
		{
			name:     "local_does_not_leak_to_top_level",
			src:      "SUB MakeLocal()\n  secret = 1\n  PRINT secret\nEND SUB\nPRINT secret\n",
			wantLine: 5,
			wantSub:  "SECRET is not defined",
		},
		{
			name:     "unknown_type_alias_hint",
			src:      "DIM n AS INT\n",
			wantLine: 1,
			wantSub:  "unknown type INT; did you mean INTEGER?",
		},
		{
			name:     "unknown_type_typo_hint",
			src:      "DIM s AS STRNG\n",
			wantLine: 1,
			wantSub:  "unknown type STRNG; did you mean STRING?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := checkSrc(t, tt.name+".bas", tt.src)
			if len(ds) != 1 {
				t.Fatalf("got %d diagnostics %v, want 1", len(ds), ds)
			}
			d := ds[0]
			if d.Kind != diag.Undefined || d.Pos.Line != tt.wantLine || !strings.Contains(d.Message, tt.wantSub) {
				t.Errorf("got %q, want UNDEFINED on line %d containing %q", d.String(), tt.wantLine, tt.wantSub)
			}
		})
	}
}

func TestDeclsConstAndScopingErrors(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKind diag.Kind
		wantLine int
		wantSub  string
	}{
		{
			name:     "duplicate_function",
			src:      "FUNCTION F()\n  RETURN 1\nEND FUNCTION\nFUNCTION f()\n  RETURN 2\nEND FUNCTION\n",
			wantKind: diag.Syntax,
			wantLine: 4,
			wantSub:  "already declared",
		},
		{
			name:     "duplicate_const",
			src:      "CONST A = 1\nCONST A = 2\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "already declared",
		},
		{
			name:     "duplicate_dim",
			src:      "DIM x AS INTEGER\nDIM x AS INTEGER\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "already declared",
		},
		{
			name:     "dim_collides_with_param",
			src:      "FUNCTION F(x)\n  DIM x AS INTEGER\n  RETURN x\nEND FUNCTION\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "already a parameter",
		},
		{
			name:     "duplicate_param",
			src:      "FUNCTION F(a, A)\n  RETURN a\nEND FUNCTION\n",
			wantKind: diag.Syntax,
			wantLine: 1,
			wantSub:  "duplicate parameter A",
		},
		{
			name:     "required_after_default_param",
			src:      "FUNCTION F(a = 1, b)\n  RETURN a + b\nEND FUNCTION\n",
			wantKind: diag.Syntax,
			wantLine: 1,
			wantSub:  "parameter B has no default, but follows parameter A",
		},
		{
			name:     "reassign_const_top_level",
			src:      "CONST PI = 3.14\nPI = 4\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "cannot reassign CONST PI",
		},
		{
			name:     "reassign_module_const_in_func",
			src:      "CONST PI = 3.14\nSUB Bad()\n  PI = 4\nEND SUB\n",
			wantKind: diag.Syntax,
			wantLine: 3,
			wantSub:  "cannot reassign CONST PI",
		},
		{
			name:     "reassign_const_as_for_var",
			src:      "CONST I = 1\nFOR I = 1 TO 3\n  PRINT I\nNEXT\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "cannot reassign CONST I",
		},
		{
			name:     "assign_to_function",
			src:      "FUNCTION F()\n  RETURN 1\nEND FUNCTION\nF = 2\n",
			wantKind: diag.Syntax,
			wantLine: 4,
			wantSub:  "cannot assign to F",
		},
		{
			name:     "call_sub_as_value",
			src:      "SUB Greet(w)\n  PRINT w\nEND SUB\nx = Greet(\"Ada\")\n",
			wantKind: diag.TypeMismatch,
			wantLine: 4,
			wantSub:  "SUB GREET returns no value",
		},
		{
			name:     "call_const_as_func",
			src:      "CONST N = 5\nPRINT N(1)\n",
			wantKind: diag.TypeMismatch,
			wantLine: 2,
			wantSub:  "N is a CONST, not a function",
		},
		{
			name:     "too_few_args",
			src:      "FUNCTION Add(a, b)\n  RETURN a + b\nEND FUNCTION\nPRINT Add(1)\n",
			wantKind: diag.Syntax,
			wantLine: 4,
			wantSub:  "ADD expects 2 arguments, got 1",
		},
		{
			name:     "too_many_args_with_default",
			src:      "FUNCTION Label(t, p = \"*\")\n  RETURN p & t\nEND FUNCTION\nPRINT Label(\"a\", \"b\", \"c\")\n",
			wantKind: diag.Syntax,
			wantLine: 4,
			wantSub:  "LABEL expects at most 2 arguments, got 3",
		},
		{
			name:     "return_value_in_sub",
			src:      "SUB S()\n  RETURN 1\nEND SUB\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "SUB S returns nothing",
		},
		{
			name:     "return_at_top_level",
			src:      "RETURN 1\n",
			wantKind: diag.Syntax,
			wantLine: 1,
			wantSub:  "inside a FUNCTION or SUB",
		},
		{
			name:     "exit_for_outside_loop",
			src:      "EXIT FOR\n",
			wantKind: diag.Syntax,
			wantLine: 1,
			wantSub:  "EXIT FOR can only be used inside a FOR loop",
		},
		{
			name:     "exit_for_inside_while",
			src:      "WHILE FALSE\n  EXIT FOR\nWEND\n",
			wantKind: diag.Syntax,
			wantLine: 2,
			wantSub:  "EXIT FOR can only be used inside a FOR loop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := checkSrc(t, tt.name+".bas", tt.src)
			if len(ds) != 1 {
				t.Fatalf("got %d diagnostics %v, want 1", len(ds), ds)
			}
			d := ds[0]
			if d.Kind != tt.wantKind || d.Pos.Line != tt.wantLine || !strings.Contains(d.Message, tt.wantSub) {
				t.Errorf("got %q, want %s on line %d containing %q", d.String(), tt.wantKind, tt.wantLine, tt.wantSub)
			}
		})
	}
}

func TestClassicAndModeRestrictions(t *testing.T) {
	t.Run("goto_in_structured", func(t *testing.T) {
		ds := checkSrc(t, "a.bas", "PRINT \"HI\"\nGOTO 10\n")
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || ds[0].Pos.Line != 2 {
			t.Fatalf("got %v, want SYNTAX error on line 2", ds)
		}
	})

	t.Run("gosub_in_structured", func(t *testing.T) {
		ds := checkSrc(t, "a.bas", "PRINT \"HI\"\nGOSUB 100\n")
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || ds[0].Pos.Line != 2 {
			t.Fatalf("got %v, want SYNTAX error on line 2", ds)
		}
	})

	t.Run("on_goto_in_structured", func(t *testing.T) {
		ds := checkSrc(t, "a.bas", "x = 1\nON x GOTO 10, 20\n")
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || ds[0].Pos.Line != 2 {
			t.Fatalf("got %v, want SYNTAX error on line 2", ds)
		}
	})

	t.Run("route_gosub_in_structured", func(t *testing.T) {
		ds := checkSrc(t, "a.bas", "ROUTE GET \"/a\" GOSUB 100\n")
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || ds[0].Pos.Line != 1 {
			t.Fatalf("got %v, want SYNTAX error on line 1", ds)
		}
	})

	t.Run("missing_goto_target", func(t *testing.T) {
		ds := checkSrc(t, "c.bas", "10 GOTO 999\n20 END\n")
		if len(ds) != 1 || ds[0].Kind != diag.Undefined || !strings.Contains(ds[0].Message, "line 999 does not exist") {
			t.Fatalf("got %v, want UNDEFINED line 999", ds)
		}
	})

	t.Run("missing_gosub_target", func(t *testing.T) {
		ds := checkSrc(t, "c.bas", "10 GOSUB 500\n20 END\n")
		if len(ds) != 1 || ds[0].Kind != diag.Undefined || !strings.Contains(ds[0].Message, "line 500 does not exist") {
			t.Fatalf("got %v, want UNDEFINED line 500", ds)
		}
	})

	t.Run("missing_on_goto_target", func(t *testing.T) {
		ds := checkSrc(t, "c.bas", "10 X = 1\n20 ON X GOTO 10, 999\n")
		if len(ds) != 1 || ds[0].Kind != diag.Undefined || !strings.Contains(ds[0].Message, "line 999 does not exist") {
			t.Fatalf("got %v, want UNDEFINED line 999", ds)
		}
	})

	t.Run("missing_route_gosub_target", func(t *testing.T) {
		ds := checkSrc(t, "c.bas", "10 ROUTE GET \"/hi\" GOSUB 404\n20 SERVE\n")
		if len(ds) != 1 || ds[0].Kind != diag.Undefined || !strings.Contains(ds[0].Message, "line 404 does not exist") {
			t.Fatalf("got %v, want UNDEFINED line 404", ds)
		}
	})

	t.Run("jump_into_block_body_rejected", func(t *testing.T) {
		src := "10 FOR I = 1 TO 3\n20 PRINT I\n30 NEXT\n40 GOTO 20\n"
		ds := checkSrc(t, "c.bas", src)
		if len(ds) != 1 || ds[0].Kind != diag.Undefined || !strings.Contains(ds[0].Message, "line 20 does not exist") {
			t.Fatalf("got %v, want UNDEFINED line 20", ds)
		}
	})

	t.Run("classic_return_with_value_rejected", func(t *testing.T) {
		ds := checkSrc(t, "c.bas", "10 GOSUB 100\n20 END\n100 RETURN 5\n")
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || ds[0].Pos.Line != 3 {
			t.Fatalf("got %v, want SYNTAX error on line 3", ds)
		}
	})

	t.Run("classic_forward_var_via_gosub_ok", func(t *testing.T) {
		src := "10 GOSUB 100\n20 PRINT X\n30 END\n100 X = 42\n110 RETURN\n"
		if ds := checkSrc(t, "c.bas", src); len(ds) > 0 {
			t.Fatalf("unexpected diagnostics: %v", ds)
		}
	})

	t.Run("classic_const_reassign_rejected", func(t *testing.T) {
		src := "10 CONST C = 1\n20 C = 2\n"
		ds := checkSrc(t, "c.bas", src)
		if len(ds) != 1 || ds[0].Kind != diag.Syntax || !strings.Contains(ds[0].Message, "cannot reassign CONST C") {
			t.Fatalf("got %v, want SYNTAX cannot reassign CONST C", ds)
		}
	})
}

func TestValidPrograms(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "try_catch_twice",
			src: `
TRY
  THROW ERROR("boom")
CATCH e
  PRINT e.message
END TRY
TRY
  PRINT 1
CATCH e
  PRINT e.kind
FINALLY
  PRINT "done"
END TRY
`,
		},
		{
			name: "log_bare_and_string_levels",
			src: `
LOG INFO, "hello"
LOG "WARN", "careful", {"k": 1}
`,
		},
		{
			name: "local_var_named_body_in_structured",
			src: `
FUNCTION Handle(req)
  LET body = req.body
  RETURN body
END FUNCTION
`,
		},
		{
			name: "module_var_after_function",
			src: `
FUNCTION GetRate()
  RETURN RATE
END FUNCTION
CONST RATE = 3
PRINT GetRate()
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ds := checkSrc(t, tt.name+".bas", tt.src); len(ds) > 0 {
				t.Fatalf("unexpected diagnostics: %v", ds)
			}
		})
	}
}

func checkFileWithImports(t *testing.T, entryPath string) diag.List {
	t.Helper()
	absEntry, err := filepath.Abs(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	baseDir := filepath.Dir(absEntry)
	seen := map[string]bool{}
	var files []*ast.File

	var load func(absPath string)
	load = func(absPath string) {
		key := strings.ToLower(filepath.Clean(absPath))
		if seen[key] {
			return
		}
		seen[key] = true
		src, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("read %s: %v", absPath, err)
		}
		rel, err := filepath.Rel(baseDir, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		name := filepath.ToSlash(rel)
		toks, mode, lds := lexer.Lex(name, src)
		if lds.HasErrors() {
			t.Fatalf("unexpected lex errors for %s: %v", name, lds)
		}
		f, pds := parser.Parse(name, toks, mode)
		if pds.HasErrors() {
			t.Fatalf("unexpected parse errors for %s: %v", name, pds)
		}
		for _, imp := range f.Imports {
			if imp.FromGo {
				continue
			}
			target := filepath.Clean(filepath.Join(filepath.Dir(absPath), filepath.FromSlash(imp.Path)))
			load(target)
		}
		files = append(files, f)
	}
	load(absEntry)
	return Check(&ast.Program{Entry: filepath.Base(absEntry), Files: files})
}

func TestCheckAllGoldenAndExamples(t *testing.T) {
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
				if ds := checkFileWithImports(t, p); len(ds) > 0 {
					t.Fatalf("unexpected check diagnostics: %v", ds)
				}
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count < 40 {
		t.Fatalf("expected to check at least 40 golden+example .bas files, checked %d", count)
	}
}
