package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

func exportFunc(line int, name string, ps []ast.Param, result string, body ...ast.Stmt) ast.Stmt {
	return &ast.FuncDecl{
		At:     at(line),
		Name:   strings.ToUpper(name),
		Params: ps,
		Result: ast.TypeRef{Name: result},
		Export: true,
		Body:   body,
	}
}

func exportSub(line int, name string, ps []ast.Param, body ...ast.Stmt) ast.Stmt {
	return &ast.FuncDecl{
		At:     at(line),
		Name:   strings.ToUpper(name),
		Params: ps,
		IsSub:  true,
		Export: true,
		Body:   body,
	}
}

func exportConst(line int, name string, value ast.Expr) ast.Stmt {
	return &ast.ConstDecl{
		At:     at(line),
		Name:   strings.ToUpper(name),
		Value:  value,
		Export: true,
	}
}

func TestModulesGeneratedSourceShape(t *testing.T) {
	billing := &ast.File{
		Name: "billing.bas",
		Mode: token.Structured,
		Body: []ast.Stmt{
			exportConst(1, "BASE_FEE", num(5)),
			exportConst(2, "TAX_RATE", call("DECIMAL", str("0.10"))),
			function(3, "internalHelper", params(param("n", "INTEGER", nil)), "INTEGER",
				ret(4, bin(id("n"), token.Plus, id("BASE_FEE"))),
			),
			exportFunc(6, "CalcTotal", params(param("n", "INTEGER", nil)), "INTEGER",
				ret(7, call("internalHelper", id("n"))),
			),
			exportSub(9, "Say", params(param("msg", "STRING", nil)),
				pr(10, id("msg")),
			),
		},
	}
	mainF := &ast.File{
		Name: "main.bas",
		Mode: token.Structured,
		Imports: []*ast.Import{
			{At: at(1), Path: "./billing.bas", Names: []string{"CALCTOTAL", "SAY", "BASE_FEE", "TAX_RATE"}},
		},
		Body: []ast.Stmt{
			do(2, call("Say", str("READY"))),
			pr(3, bin(call("CalcTotal", num(10)), token.Plus, id("BASE_FEE"))),
			pr(4, id("TAX_RATE")),
		},
	}

	prog := &ast.Program{Entry: "main.bas", Files: []*ast.File{billing, mainF}}
	files, ds := codegen.Generate(prog, codegen.Options{})
	if ds.HasErrors() {
		t.Fatalf("Generate failed:\n%v", ds)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files (%v), want 3", len(files), names(files))
	}

	byName := map[string]string{}
	for _, f := range files {
		byName[f.Name] = string(f.Src)
	}

	mainGo := byName["main.go"]
	billingIdx := strings.Index(mainGo, "rip_billing_run(c)")
	mainIdx := strings.Index(mainGo, "rip_main_run(c)")
	if billingIdx < 0 || mainIdx < 0 || billingIdx >= mainIdx {
		t.Errorf("main.go should run rip_billing_run(c) before rip_main_run(c):\n%s", mainGo)
	}

	mMain := byName["m_main.go"]
	for _, want := range []string{
		`b_billing_say(c, "READY")`,
		`(b_billing_calctotal(c, int64(10)) + b_billing_base_fee)`,
		`rt.Print(b_billing_tax_rate)`,
	} {
		if !strings.Contains(mMain, want) {
			t.Errorf("m_main.go missing %q:\n%s", want, mMain)
		}
	}
	for _, forbidden := range []string{
		"func b_billing_calctotal",
		"func b_billing_say",
		"b_billing_base_fee int64 =",
		"b_billing_tax_rate rt.Value",
	} {
		if strings.Contains(mMain, forbidden) {
			t.Errorf("m_main.go must not re-emit imported symbol %q:\n%s", forbidden, mMain)
		}
	}
}

func TestModulesRunEndToEnd(t *testing.T) {
	cases := []struct {
		name   string
		prog   *ast.Program
		stdout string
	}{
		{
			name: "named_import_func_sub_const",
			prog: &ast.Program{
				Entry: "main.bas",
				Files: []*ast.File{
					{
						Name: "helper.bas",
						Mode: token.Structured,
						Body: []ast.Stmt{
							exportConst(1, "MULTIPLIER", num(3)),
							exportConst(2, "TAX_RATE", call("DECIMAL", str("0.10"))),
							exportFunc(3, "Scale", params(param("x", "INTEGER", nil)), "INTEGER",
								ret(4, bin(id("x"), token.Star, id("MULTIPLIER"))),
							),
							exportSub(6, "Announce", params(param("who", "STRING", str("WORLD"))),
								pr(7, bin(str("HI "), token.Amp, id("who"))),
							),
						},
					},
					{
						Name: "main.bas",
						Mode: token.Structured,
						Imports: []*ast.Import{
							{At: at(1), Path: "./helper.bas", Names: []string{"SCALE", "ANNOUNCE", "MULTIPLIER", "TAX_RATE"}},
						},
						Body: []ast.Stmt{
							do(2, call("Announce")),
							do(3, call("Announce", str("ADA"))),
							pr(4, call("Scale", num(7))),
							pr(5, id("MULTIPLIER")),
							pr(6, call("ROUND", bin(call("DECIMAL", str("50.00")), token.Star, id("TAX_RATE")), num(2))),
						},
					},
				},
			},
			stdout: "HI WORLD\nHI ADA\n21\n3\n5.00\n",
		},
		{
			name: "whole_file_import_and_transitive_init_order",
			prog: &ast.Program{
				Entry: "main.bas",
				Files: []*ast.File{
					{
						Name: "leaf.bas",
						Mode: token.Structured,
						Body: []ast.Stmt{
							pr(1, str("INIT LEAF")),
							exportConst(2, "BASE", num(10)),
							exportFunc(3, "AddBase", params(param("n", "INTEGER", nil)), "INTEGER",
								ret(4, bin(id("n"), token.Plus, id("BASE"))),
							),
						},
					},
					{
						Name: "mid.bas",
						Mode: token.Structured,
						Imports: []*ast.Import{
							{At: at(1), Path: "./leaf.bas"},
						},
						Body: []ast.Stmt{
							pr(2, str("INIT MID")),
							exportFunc(3, "DoubleWithBase", params(param("n", "INTEGER", nil)), "INTEGER",
								ret(4, bin(call("AddBase", id("n")), token.Star, num(2))),
							),
						},
					},
					{
						Name: "main.bas",
						Mode: token.Structured,
						Imports: []*ast.Import{
							{At: at(1), Path: "./mid.bas"},
						},
						Body: []ast.Stmt{
							pr(2, str("INIT MAIN")),
							pr(3, call("DoubleWithBase", num(5))),
						},
					},
				},
			},
			stdout: "INIT LEAF\nINIT MID\nINIT MAIN\n30\n",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := runProgram(t, c.prog)
			if r.exit != 0 {
				t.Fatalf("exit %d, stderr:\n%s", r.exit, r.stderr)
			}
			if r.stdout != c.stdout {
				t.Errorf("stdout:\n%q\nwant:\n%q", r.stdout, c.stdout)
			}
		})
	}
}

func TestModulesDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		prog *ast.Program
		kind diag.Kind
		want string
	}{
		{
			name: "missing_file",
			prog: &ast.Program{
				Entry: "main.bas",
				Files: []*ast.File{
					{Name: "main.bas", Imports: []*ast.Import{{At: at(1), Path: "./missing.bas"}}},
				},
			},
			kind: diag.Undefined,
			want: `cannot find imported file "./missing.bas"`,
		},
		{
			name: "unexported_function",
			prog: &ast.Program{
				Entry: "main.bas",
				Files: []*ast.File{
					{Name: "helper.bas", Body: []ast.Stmt{function(1, "Secret", nil, "", ret(2, num(1)))}},
					{Name: "main.bas", Imports: []*ast.Import{{At: at(1), Path: "./helper.bas", Names: []string{"SECRET"}}}},
				},
			},
			kind: diag.Undefined,
			want: "SECRET is not exported by helper.bas",
		},
		{
			name: "conflicting_imports",
			prog: &ast.Program{
				Entry: "main.bas",
				Files: []*ast.File{
					{Name: "a.bas", Body: []ast.Stmt{exportConst(1, "K", num(1))}},
					{Name: "b.bas", Body: []ast.Stmt{exportConst(1, "K", num(2))}},
					{Name: "main.bas", Imports: []*ast.Import{{At: at(1), Path: "./a.bas"}, {At: at(2), Path: "./b.bas"}}},
				},
			},
			kind: diag.Syntax,
			want: "K is imported from both a.bas and b.bas",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, ds := codegen.Generate(c.prog, codegen.Options{})
			if files != nil {
				t.Fatalf("got %d files, want nil on error", len(files))
			}
			for _, d := range ds {
				if d.Kind == c.kind && strings.Contains(d.Message, c.want) {
					return
				}
			}
			t.Fatalf("no %s diagnostic containing %q; got:\n%v", c.kind, c.want, ds)
		})
	}
}
