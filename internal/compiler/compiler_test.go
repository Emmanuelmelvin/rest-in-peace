package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func compileAndRun(t *testing.T, entryPath string) (stdout, stderr string, exitCode int) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a Go program; skipped with -short")
	}
	if _, err := build.LookGo(); err != nil {
		t.Skip(err)
	}
	res, err := compiler.Compile(entryPath, compiler.Options{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("unexpected compile diagnostics:\n%v", res.Diagnostics)
	}
	buildDir := t.TempDir()
	if err := build.Prepare(buildDir, res.Files); err != nil {
		t.Fatal(err)
	}
	bin, err := build.Build(context.Background(), buildDir, build.Options{Output: filepath.Join(buildDir, "bin", "prog")})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	cmd := exec.Command(bin)
	cmd.Dir = buildDir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return strings.ReplaceAll(outBuf.String(), "\r\n", "\n"), strings.ReplaceAll(errBuf.String(), "\r\n", "\n"), exitCode
}

func TestCompileModulesEndToEnd(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		files map[string]string
		want  string
	}{
		{
			name:  "named_and_whole_imports",
			entry: "main.bas",
			files: map[string]string{
				"math.bas": `
EXPORT CONST TAX_RATE = DECIMAL("0.10")
EXPORT CONST CURRENCY = "USD"

FUNCTION helper(n)
  RETURN n * 1
END FUNCTION

EXPORT FUNCTION CalcTax(amount)
  RETURN ROUND(DECIMAL(amount) * TAX_RATE, 2)
END FUNCTION

EXPORT FUNCTION CalcTotal(amount)
  RETURN helper(DECIMAL(amount) + CalcTax(amount))
END FUNCTION

EXPORT SUB PrintBanner(title)
  PRINT "=== " & title & " ==="
END SUB
`,
				"main.bas": `
IMPORT { CalcTax, CalcTotal, PrintBanner, CURRENCY } FROM "./math.bas"

PrintBanner("INVOICE")
PRINT CURRENCY & " " & STR(CalcTax("100.00"))
PRINT CURRENCY & " " & STR(CalcTotal("100.00"))
`,
			},
			want: "=== INVOICE ===\nUSD 10.00\nUSD 110.00\n",
		},
		{
			name:  "whole_file_import",
			entry: "main.bas",
			files: map[string]string{
				"helper.bas": `
EXPORT CONST FACTOR = 4
EXPORT FUNCTION Times(n AS INTEGER) AS INTEGER
  RETURN n * FACTOR
END FUNCTION
EXPORT SUB Done()
  PRINT "DONE"
END SUB
`,
				"main.bas": `
IMPORT "./helper.bas"

PRINT Times(5)
PRINT FACTOR
Done()
`,
			},
			want: "20\n4\nDONE\n",
		},
		{
			name:  "transitive_and_diamond_imports_across_subdirs",
			entry: "main.bas",
			files: map[string]string{
				"leaf.bas": `
PRINT "LOAD LEAF"
EXPORT CONST OFFSET = 10
EXPORT FUNCTION AddOffset(x AS INTEGER) AS INTEGER
  RETURN x + OFFSET
END FUNCTION
`,
				"sub/mid.bas": `
IMPORT { AddOffset } FROM "../leaf.bas"
PRINT "LOAD MID"
EXPORT FUNCTION DoubleOffset(x AS INTEGER) AS INTEGER
  RETURN AddOffset(x) * 2
END FUNCTION
`,
				"main.bas": `
IMPORT { DoubleOffset } FROM "./sub/mid.bas"
IMPORT { OFFSET } FROM "./leaf.bas"
PRINT "LOAD MAIN"
PRINT DoubleOffset(5)
PRINT OFFSET
`,
			},
			// leaf.bas must load and initialize ONCE before mid.bas, and mid.bas before main.bas.
			want: "LOAD LEAF\nLOAD MID\nLOAD MAIN\n30\n10\n",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFiles(t, dir, c.files)
			stdout, stderr, code := compileAndRun(t, filepath.Join(dir, filepath.FromSlash(c.entry)))
			if code != 0 {
				t.Fatalf("exit %d, stderr:\n%s", code, stderr)
			}
			if stdout != c.want {
				t.Errorf("stdout:\n%q\nwant:\n%q", stdout, c.want)
			}
		})
	}
}

func TestCompileInvoicingBillingModule(t *testing.T) {
	billingSrc, err := os.ReadFile(filepath.Join("..", "..", "examples", "invoicing", "billing.bas"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"billing.bas": string(billingSrc),
		"caller.bas": `
IMPORT { InvoiceTotals } FROM "./billing.bas"

LET items = [{"unitPrice": "100.00", "quantity": 2}]
LET totals = InvoiceTotals(items, DECIMAL("0.075"))
PRINT totals.subtotal
PRINT totals.tax
PRINT totals.total

TRY
  LET bad = InvoiceTotals([{"unitPrice": "50.00", "quantity": -1}], DECIMAL("0.075"))
  PRINT bad
CATCH e
  PRINT e.status
  PRINT e.message
END TRY
`,
	})
	stdout, stderr, code := compileAndRun(t, filepath.Join(dir, "caller.bas"))
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	want := "200.00\n15.00\n215.00\n422\nQUANTITY MUST BE POSITIVE\n"
	if stdout != want {
		t.Errorf("stdout:\n%q\nwant:\n%q", stdout, want)
	}
}

func TestCompileModuleErrors(t *testing.T) {
	cases := []struct {
		name     string
		entry    string
		files    map[string]string
		wantKind diag.Kind
		wantFile string
		wantLine int
		wantSub  string
	}{
		{
			name:  "missing_file",
			entry: "main.bas",
			files: map[string]string{
				"main.bas": "IMPORT \"./nope.bas\"\nPRINT 1\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  `cannot find imported file "./nope.bas"`,
		},
		{
			name:  "missing_file_forgot_extension_hint",
			entry: "main.bas",
			files: map[string]string{
				"helper.bas": "EXPORT CONST X = 1\n",
				"main.bas":   "IMPORT \"./helper\"\nPRINT X\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  `did you mean "./helper.bas"?`,
		},
		{
			name:  "unexported_named_import",
			entry: "main.bas",
			files: map[string]string{
				"helper.bas": "FUNCTION Secret(x)\n  RETURN x * 2\nEND FUNCTION\n",
				"main.bas":   "IMPORT { Secret } FROM \"./helper.bas\"\nPRINT Secret(3)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 1,
			wantSub:  "SECRET is not exported by helper.bas; add EXPORT before FUNCTION SECRET",
		},
		{
			name:  "unexported_whole_file_import",
			entry: "main.bas",
			files: map[string]string{
				"helper.bas": "FUNCTION Secret(x)\n  RETURN x * 2\nEND FUNCTION\nEXPORT FUNCTION Public(x)\n  RETURN Secret(x)\nEND FUNCTION\n",
				"main.bas":   "IMPORT \"./helper.bas\"\nPRINT Secret(3)\n",
			},
			wantKind: diag.Undefined,
			wantFile: "main.bas",
			wantLine: 2,
			wantSub:  "SECRET is not exported by helper.bas; add EXPORT before FUNCTION SECRET",
		},
		{
			name:  "direct_circular_import",
			entry: "a.bas",
			files: map[string]string{
				"a.bas": "IMPORT \"./a.bas\"\nPRINT 1\n",
			},
			wantKind: diag.Syntax,
			wantFile: "a.bas",
			wantLine: 1,
			wantSub:  "circular IMPORT detected (a.bas -> a.bas)",
		},
		{
			name:  "indirect_circular_import",
			entry: "a.bas",
			files: map[string]string{
				"a.bas": "IMPORT \"./b.bas\"\nEXPORT CONST A = 1\n",
				"b.bas": "IMPORT \"./c.bas\"\nEXPORT CONST B = 2\n",
				"c.bas": "IMPORT \"./a.bas\"\nEXPORT CONST C = 3\n",
			},
			wantKind: diag.Syntax,
			wantFile: "c.bas",
			wantLine: 1,
			wantSub:  "circular IMPORT detected (a.bas -> b.bas -> c.bas -> a.bas)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, c.files)
			res, err := compiler.Compile(filepath.Join(dir, filepath.FromSlash(c.entry)), compiler.Options{})
			if err != nil {
				t.Fatalf("unexpected I/O error: %v", err)
			}
			if len(res.Files) != 0 {
				t.Fatalf("got %d files on error, want 0", len(res.Files))
			}
			for _, d := range res.Diagnostics {
				if d.Kind == c.wantKind && d.Pos.File == c.wantFile && d.Pos.Line == c.wantLine && strings.Contains(d.Message, c.wantSub) {
					return
				}
			}
			t.Fatalf("no %s diagnostic at %s:%d containing %q; got:\n%v", c.wantKind, c.wantFile, c.wantLine, c.wantSub, res.Diagnostics)
		})
	}
}

func TestCompileMissingEntryReturnsIOError(t *testing.T) {
	_, err := compiler.Compile(filepath.Join(t.TempDir(), "does_not_exist.bas"), compiler.Options{})
	if err == nil {
		t.Fatal("expected I/O error for missing entry file, got nil")
	}
}
