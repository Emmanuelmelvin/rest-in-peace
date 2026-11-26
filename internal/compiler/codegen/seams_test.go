package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// The seams for the Kiro tasks 2.4, 2.5, and 2.3b/2.7b (.kiro/steering/structure.md):
// LOG, ENV, DECIMAL, ROUND and built-in middleware compile and run through
// rt/log.go, rt/env.go, rt/decimal.go and rt/middleware.go.

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
	// DIM AS DECIMAL compiles to a native rt.Decimal; DECIMAL() and ROUND()
	// work end to end now that Kiro task 2.5 is merged.
	r := runProgram(t, structured("dec.bas",
		dim(1, "price", "DECIMAL", call("DECIMAL", str("19.99"))),
		pr(2, bin(id("price"), token.Plus, call("DECIMAL", str("0.01")))),
		pr(3, call("ROUND", call("DECIMAL", str("2.345")), num(2))),
		pr(4, call("ROUND", flt("1.5"), num(0))),
	))
	if r.exit != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.exit, r.stdout, r.stderr)
	}
	want := "20.00\n2.35\n2\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
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
