package check

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func checkProgram(t *testing.T, entry string, files map[string]string, order []string) diag.List {
	t.Helper()
	var parsed []*ast.File
	for _, name := range order {
		src, ok := files[name]
		if !ok {
			t.Fatalf("missing source for %s", name)
		}
		toks, mode, lds := lexer.Lex(name, []byte(src))
		if lds.HasErrors() {
			t.Fatalf("unexpected lex errors in %s: %v", name, lds)
		}
		f, pds := parser.Parse(name, toks, mode)
		if pds.HasErrors() {
			t.Fatalf("unexpected parse errors in %s: %v", name, pds)
		}
		parsed = append(parsed, f)
	}
	return Check(&ast.Program{Entry: entry, Files: parsed})
}

func TestModulesValid(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		files map[string]string
		order []string
	}{
		{
			name:  "named_imports_func_sub_const",
			entry: "main.bas",
			order: []string{"billing.bas", "main.bas"},
			files: map[string]string{
				"billing.bas": `
EXPORT CONST TAX_RATE = DECIMAL("0.075")
FUNCTION secret(x)
  RETURN x
END FUNCTION
EXPORT FUNCTION CalcTax(a)
  RETURN ROUND(DECIMAL(a) * TAX_RATE, 2)
END FUNCTION
EXPORT SUB LogBill(msg)
  PRINT msg
END SUB
`,
				"main.bas": `
IMPORT { CalcTax, LogBill, tax_rate } FROM "./billing.bas"
LogBill("START")
PRINT CalcTax("100.00")
PRINT TAX_RATE
`,
			},
		},
		{
			name:  "whole_file_import",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": `
EXPORT CONST RATE = 10
EXPORT FUNCTION Double(n AS INTEGER) AS INTEGER
  RETURN n * 2
END FUNCTION
EXPORT SUB Say(s)
  PRINT s
END SUB
`,
				"main.bas": `
IMPORT "./helper.bas"
Say("HI")
PRINT Double(RATE)
`,
			},
		},
		{
			name:  "transitive_imports",
			entry: "main.bas",
			order: []string{"leaf.bas", "mid.bas", "main.bas"},
			files: map[string]string{
				"leaf.bas": `
EXPORT CONST BASE = 5
EXPORT FUNCTION AddBase(x)
  RETURN x + BASE
END FUNCTION
`,
				"mid.bas": `
IMPORT { AddBase } FROM "./leaf.bas"
EXPORT FUNCTION Compute(x)
  RETURN AddBase(x) * 2
END FUNCTION
`,
				"main.bas": `
IMPORT { Compute } FROM "./mid.bas"
PRINT Compute(3)
`,
			},
		},
		{
			name:  "subdir_and_parent_relative_paths",
			entry: "tests/app.test.bas",
			order: []string{"lib/math.bas", "tests/app.test.bas"},
			files: map[string]string{
				"lib/math.bas": `
EXPORT FUNCTION Square(n)
  RETURN n * n
END FUNCTION
`,
				"tests/app.test.bas": `
IMPORT { Square } FROM "../lib/math.bas"
TEST "squares"
  EXPECT Square(4) = 16
END TEST
`,
			},
		},
		{
			name:  "imported_route_handler_and_middleware",
			entry: "main.bas",
			order: []string{"handlers.bas", "main.bas"},
			files: map[string]string{
				"handlers.bas": `
EXPORT SUB Auth(req)
  PRINT req.path
END SUB
EXPORT FUNCTION Ping(req)
  RETURN {"ok": TRUE}
END FUNCTION
`,
				"main.bas": `
IMPORT { Auth, Ping } FROM "./handlers.bas"
USE Auth
ROUTE GET "/ping" CALL Ping
SERVE
`,
			},
		},
		{
			name:  "whole_file_import_with_log_and_try",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": `
EXPORT FUNCTION Work(x)
  RETURN x + 1
END FUNCTION
`,
				"main.bas": `
IMPORT "./helper.bas"
LOG INFO, "running"
TRY
  PRINT Work(1)
CATCH e
  PRINT e.message
END TRY
`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := checkProgram(t, tt.entry, tt.files, tt.order)
			if len(ds) != 0 {
				t.Fatalf("got unexpected diagnostics:\n%v", ds)
			}
		})
	}
}

func TestModulesErrors(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		order    []string
		files    map[string]string
		wantKind diag.Kind
		wantFile string
		wantLine int
		wantSub  string
	}{
		{
			name:  "missing_imported_file",
			entry: "main.bas",
			order: []string{"main.bas"},
			files: map[string]string{
				"main.bas": "IMPORT \"./missing.bas\"\nPRINT 1\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  `cannot find imported file "./missing.bas"`,
		},
		{
			name:  "named_import_unexported_function",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "FUNCTION Secret(x)\n  RETURN x\nEND FUNCTION\n",
				"main.bas":   "IMPORT { Secret } FROM \"./helper.bas\"\nPRINT Secret(1)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "SECRET is not exported by helper.bas; add EXPORT before FUNCTION SECRET",
		},
		{
			name:  "named_import_unexported_sub",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "SUB Quiet()\n  PRINT 1\nEND SUB\n",
				"main.bas":   "IMPORT { Quiet } FROM \"./helper.bas\"\nQuiet()\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "QUIET is not exported by helper.bas; add EXPORT before SUB QUIET",
		},
		{
			name:  "named_import_unexported_const",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "CONST HIDDEN = 42\n",
				"main.bas":   "IMPORT { HIDDEN } FROM \"./helper.bas\"\nPRINT HIDDEN\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "HIDDEN is not exported by helper.bas; add EXPORT before CONST HIDDEN",
		},
		{
			name:  "named_import_module_variable",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "DIM count AS INTEGER = 0\n",
				"main.bas":   "IMPORT { count } FROM \"./helper.bas\"\nPRINT count\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "COUNT is a variable in helper.bas, not an exported declaration",
		},
		{
			name:  "named_import_nonexistent_with_suggestion",
			entry: "main.bas",
			order: []string{"billing.bas", "main.bas"},
			files: map[string]string{
				"billing.bas": "EXPORT FUNCTION CalcTotal(x)\n  RETURN x\nEND FUNCTION\n",
				"main.bas":    "IMPORT { CalcTotl } FROM \"./billing.bas\"\nPRINT CalcTotl(1)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "billing.bas has no export named CALCTOTL; did you mean CALCTOTAL?",
		},
		{
			name:  "whole_file_import_calls_unexported_function",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "FUNCTION PrivateFn(x)\n  RETURN x\nEND FUNCTION\nEXPORT FUNCTION PublicFn(x)\n  RETURN x\nEND FUNCTION\n",
				"main.bas":   "IMPORT \"./helper.bas\"\nPRINT PrivateFn(1)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "PRIVATEFN is not exported by helper.bas; add EXPORT before FUNCTION PRIVATEFN",
		},
		{
			name:  "whole_file_import_reads_unexported_const",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "CONST SECRET = 7\nEXPORT CONST PUBLIC_K = 1\n",
				"main.bas":   "IMPORT \"./helper.bas\"\nPRINT SECRET\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "SECRET is not exported by helper.bas; add EXPORT before CONST SECRET",
		},
		{
			name:  "whole_file_import_undefined_call",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT FUNCTION PublicFn(x)\n  RETURN x\nEND FUNCTION\n",
				"main.bas":   "IMPORT \"./helper.bas\"\nPRINT PublicF(1)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "no function named PUBLICF; did you mean PUBLICFN?",
		},
		{
			name:  "transitive_export_not_visible_without_import",
			entry: "main.bas",
			order: []string{"leaf.bas", "mid.bas", "main.bas"},
			files: map[string]string{
				"leaf.bas": "EXPORT FUNCTION LeafFn()\n  RETURN 1\nEND FUNCTION\n",
				"mid.bas":  "IMPORT { LeafFn } FROM \"./leaf.bas\"\nEXPORT FUNCTION MidFn()\n  RETURN LeafFn()\nEND FUNCTION\n",
				"main.bas": "IMPORT { MidFn } FROM \"./mid.bas\"\nPRINT LeafFn()\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "no function named LEAFFN",
		},
		{
			name:  "duplicate_name_in_single_import",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT FUNCTION Foo()\n  RETURN 1\nEND FUNCTION\n",
				"main.bas":   "IMPORT { Foo, foo } FROM \"./helper.bas\"\nPRINT Foo()\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "FOO is listed more than once",
		},
		{
			name:  "conflicting_whole_file_imports",
			entry: "main.bas",
			order: []string{"a.bas", "b.bas", "main.bas"},
			files: map[string]string{
				"a.bas":    "EXPORT FUNCTION Dup()\n  RETURN 1\nEND FUNCTION\n",
				"b.bas":    "EXPORT FUNCTION Dup()\n  RETURN 2\nEND FUNCTION\n",
				"main.bas": "IMPORT \"./a.bas\"\nIMPORT \"./b.bas\"\nPRINT Dup()\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "DUP is imported from both a.bas and b.bas",
		},
		{
			name:  "whole_file_import_collides_with_local_decl",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT FUNCTION Foo()\n  RETURN 1\nEND FUNCTION\n",
				"main.bas":   "IMPORT \"./helper.bas\"\nFUNCTION Foo()\n  RETURN 2\nEND FUNCTION\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "FOO is already imported from helper.bas",
		},
		{
			name:  "reassign_imported_const",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT CONST RATE = 5\n",
				"main.bas":   "IMPORT { RATE } FROM \"./helper.bas\"\nRATE = 10\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "cannot reassign imported CONST RATE",
		},
		{
			name:  "call_imported_const_as_function",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT CONST RATE = 5\n",
				"main.bas":   "IMPORT { RATE } FROM \"./helper.bas\"\nPRINT RATE()\n",
			},
			wantKind: diag.TypeMismatch,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "RATE is a CONST imported from helper.bas, not a function",
		},
		{
			name:  "imported_sub_used_as_expression",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT SUB Say(s)\n  PRINT s\nEND SUB\n",
				"main.bas":   "IMPORT { Say } FROM \"./helper.bas\"\nLET x = Say(\"hi\")\n",
			},
			wantKind: diag.TypeMismatch,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "SUB SAY (imported from helper.bas) returns no value",
		},
		{
			name:  "imported_function_wrong_arity",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT FUNCTION Add(a, b)\n  RETURN a + b\nEND FUNCTION\n",
				"main.bas":   "IMPORT { Add } FROM \"./helper.bas\"\nPRINT Add(1)\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "ADD expects 2 arguments, got 1",
		},
		{
			name:  "circular_import_direct_check",
			entry: "a.bas",
			order: []string{"a.bas", "b.bas"},
			files: map[string]string{
				"a.bas": "IMPORT \"./b.bas\"\nEXPORT CONST A = 1\n",
				"b.bas": "IMPORT \"./a.bas\"\nEXPORT CONST B = 2\n",
			},
			wantKind: diag.Syntax,
			wantFile: "b.bas",
			wantLine: 1,
			wantSub:  "circular IMPORT detected (a.bas -> b.bas -> a.bas)",
		},
		{
			name:  "empty_named_import",
			entry: "main.bas",
			order: []string{"helper.bas", "main.bas"},
			files: map[string]string{
				"helper.bas": "EXPORT CONST A = 1\n",
				"main.bas":   "IMPORT {} FROM \"./helper.bas\"\nPRINT 1\n",
			},
			wantKind: diag.Syntax,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "IMPORT { } lists no names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := checkProgram(t, tt.entry, tt.files, tt.order)
			for _, d := range ds {
				if d.Kind == tt.wantKind && d.Pos.File == tt.wantFile && d.Pos.Line == tt.wantLine && strings.Contains(d.Message, tt.wantSub) {
					return
				}
			}
			t.Fatalf("no %s diagnostic at %s:%d containing %q; got:\n%v", tt.wantKind, tt.wantFile, tt.wantLine, tt.wantSub, ds)
		})
	}
}
