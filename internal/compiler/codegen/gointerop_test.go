package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func parseAndCheck(t *testing.T, name, src string) *ast.Program {
	t.Helper()
	toks, mode, ds := lexer.Lex(name, []byte(src))
	if ds.HasErrors() {
		t.Fatalf("lex errors:\n%v", ds)
	}
	f, ds := parser.Parse(name, toks, mode)
	if ds.HasErrors() {
		t.Fatalf("parse errors:\n%v", ds)
	}
	prog := &ast.Program{Entry: name, Files: []*ast.File{f}}
	if ds := check.Check(prog); ds.HasErrors() {
		t.Fatalf("check errors:\n%v", ds)
	}
	return prog
}

func TestGoInteropGeneratedCode(t *testing.T) {
	src := `IMPORT math FROM GO "math"
IMPORT url FROM GO "net/url"
IMPORT http FROM GO "net/http"
IMPORT { NewString } FROM GO "github.com/google/uuid"

id = NewString()
u = NEW(url.URL)
req = http.NewRequestWithContext("GET", "https://example.com", NIL)
PRINT id, u.String(), req.Method
`
	prog := parseAndCheck(t, "gi.bas", src)
	files, ds := codegen.Generate(prog, codegen.Options{})
	if ds.HasErrors() {
		t.Fatalf("codegen errors:\n%v", ds)
	}
	mod := string(files[1].Src)

	// Unused "math" import must NOT be emitted.
	if strings.Contains(mod, `"math"`) {
		t.Errorf("unused Go import \"math\" should not be emitted:\n%s", mod)
	}

	for _, want := range []string{
		`go_uuid "github.com/google/uuid"`,
		`go_http "net/http"`,
		`go_url "net/url"`,
		`go_io "io"`,
		`go_uuid.NewString()`,
		`rt.WrapHost(&go_url.URL{})`,
		`rt.GoResult(go_http.NewRequestWithContext(c.Context(), "GET", "https://example.com", rt.ToGo[go_io.Reader](nil)))`,
		`rt.HostCall(c, b_gi_u, "String")`,
	} {
		if !strings.Contains(mod, want) {
			t.Errorf("generated Go missing %q:\n%s", want, mod)
		}
	}
}

func TestGoInteropEndToEnd(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		stdout string
	}{
		{
			name: "stdlib strings strconv math fmt hex and unused import",
			src: `IMPORT os FROM GO "os"
IMPORT strings FROM GO "strings"
IMPORT strconv FROM GO "strconv"
IMPORT math FROM GO "math"
IMPORT fmt FROM GO "fmt"
IMPORT hex FROM GO "encoding/hex"

PRINT strings.ToUpper("rip basic")
parts = strings.Split("a,b,c", ",")
PRINT LEN(parts)
PRINT strings.Join(parts, "-")
PRINT strconv.Atoi("42") + 8
PRINT math.Sqrt(81)
PRINT INT(math.Pi * 100)
PRINT fmt.Sprintf("%s=%d", "ok", 7)
enc = hex.EncodeToString("HI")
PRINT enc
PRINT hex.DecodeString(enc)
`,
			stdout: "RIP BASIC\n3\na-b-c\n50\n9\n314\nok=7\n4849\nHI\n",
		},
		{
			name: "uuid alias and named import plus TRY CATCH GO error",
			src: `IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT { NewString } FROM GO "github.com/google/uuid"

id1 = uuid.NewString()
id2 = NewString()
PRINT LEN(id1)
PRINT LEN(id2)
parsed = uuid.Parse(id1)
PRINT parsed.String() = id1

TRY
  bad = uuid.Parse("not-a-valid-uuid")
  PRINT "SHOULD NOT RUN"
CATCH e
  PRINT e.kind
  PRINT e.file
  PRINT e.line
END TRY
`,
			stdout: "36\n36\nTRUE\nGO\ngi_uuid.bas\n12\n",
		},
		{
			name: "NEW url.URL field write read method call and map to struct",
			src: `IMPORT url FROM GO "net/url"

u = NEW(url.URL)
u.Scheme = "https"
u.Host = "rip.dev"
u.Path = "/v1/invoices"
u.RawQuery = "page=2"
PRINT u.Scheme
PRINT u.Host
PRINT u.String()

resolved = u.ResolveReference({"path": "/v2/items", "rawQuery": "limit=5"})
PRINT resolved.String()
`,
			stdout: "https\nrip.dev\nhttps://rip.dev/v1/invoices?page=2\nhttps://rip.dev/v2/items?limit=5\n",
		},
		{
			name: "context.Context auto-injection and io.Reader interop",
			src: `IMPORT http FROM GO "net/http"
IMPORT strings FROM GO "strings"
IMPORT io FROM GO "io"

body = strings.NewReader("hello context")
req = http.NewRequestWithContext("POST", "https://example.com/api", body)
PRINT req.Method
PRINT req.URL.Host
PRINT req.URL.Path
PRINT io.ReadAll(req.Body)
`,
			stdout: "POST\nexample.com\n/api\nhello context\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fileName := "gi_e2e.bas"
			if strings.Contains(tc.name, "uuid") {
				fileName = "gi_uuid.bas"
			}
			prog := parseAndCheck(t, fileName, tc.src)
			r := runProgram(t, prog)
			if r.exit != 0 {
				t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", r.exit, r.stdout, r.stderr)
			}
			if r.stdout != tc.stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, tc.stdout)
			}
		})
	}
}
