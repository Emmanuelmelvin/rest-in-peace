package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// The seams for the Kiro tasks 2.4 and 2.5 (.kiro/steering/structure.md):
// LOG, ENV, DECIMAL and ROUND compile, and run through the functions in
// rt/log.go, rt/env.go and rt/decimal.go. Until task 2.5 fills in
// rt/decimal.go, DECIMAL values fail at run time with a "not built yet" error.

func TestLogAndEnv(t *testing.T) {
	r := runProgram(t, structured("seams.bas",
		&ast.LogStmt{At: at(1), Level: str("INFO"), Msg: str("STARTED"), Fields: mapl("port", num(8080), "password", str("hunter2"))},
		&ast.LogStmt{At: at(2), Level: str("DEBUG"), Msg: str("NO FIELDS")},
		pr(3, call("ENV", str("RIP_SEAMS_TEST_UNSET"), str("fallback"))),
		pr(4, bin(call("ENV", str("RIP_SEAMS_TEST_UNSET")), token.Eq, nilv())),
	))
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("stdout:\n%s", r.stdout)
	}
	for _, want := range []string{`"level":"INFO"`, `"msg":"STARTED"`, `"port":8080`, `"password":"[REDACTED]"`} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LOG line %s does not contain %s", lines[0], want)
		}
	}
	if !strings.Contains(lines[1], `"msg":"NO FIELDS"`) {
		t.Errorf("LOG without fields: %s", lines[1])
	}
	if lines[2] != "fallback" {
		t.Errorf("ENV with a default printed %q", lines[2])
	}
	if lines[3] != "TRUE" {
		t.Errorf("ENV of an unset variable = NIL printed %q", lines[3])
	}
}

func TestDecimalSeams(t *testing.T) {
	// DIM AS DECIMAL compiles to a native rt.Decimal with a zero value.
	r := runProgram(t, structured("dec.bas",
		dim(1, "price", "DECIMAL", nil),
		pr(2, str("COMPILED")),
	))
	if r.exit != 0 || r.stdout != "COMPILED\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.exit, r.stdout, r.stderr)
	}

	for _, c := range []struct {
		name string
		x    ast.Expr
		want string
	}{
		{"DECIMAL()", call("DECIMAL", str("19.99")), "DECIMAL is not built yet (task 2.5"},
		{"ROUND()", call("ROUND", flt("1.5"), num(0)), "ROUND is not built yet (task 2.5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runProgram(t, structured("dec.bas", pr(1, c.x)))
			if r.exit == 0 || !strings.Contains(r.stderr, c.want) || !strings.Contains(r.stderr, "dec.bas:1") {
				t.Errorf("exit %d, stderr %q; want an error containing %q at dec.bas:1", r.exit, r.stderr, c.want)
			}
		})
	}
}

// TestMiddlewareSeams verifies that USE with each built-in middleware compiles
// to the Kiro-owned factory functions in rt/middleware.go.
func TestMiddlewareSeams(t *testing.T) {
	p := structured("mw.bas",
		&ast.UseStmt{At: at(1), Fn: call("CORS", mapl("origins", str("*")))},
		&ast.UseStmt{At: at(2), Fn: call("REQUESTLOG")},
		&ast.UseStmt{At: at(3), Fn: call("APIKEY", str("API_KEY")), Scope: "/v1/*"},
		&ast.UseStmt{At: at(4), Fn: call("RATELIMIT", num(60))},
		&ast.UseStmt{At: at(5), Fn: call("BEARERJWT", str("JWT_SECRET")), Scope: "/admin/*"},
	)
	files, ds := codegen.Generate(p, codegen.Options{})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	src := string(files[1].Src)
	for _, want := range []string{
		`c.App().Use(rt.CORS(rt.MapOf("origins", "*")), "")`,
		`c.App().Use(rt.RequestLog(), "")`,
		`c.App().Use(rt.APIKey("API_KEY"), "/v1/*")`,
		`c.App().Use(rt.RateLimit(int64(60)), "")`,
		`c.App().Use(rt.BearerJWT("JWT_SECRET"), "/admin/*")`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q:\n%s", want, src)
		}
	}
}
