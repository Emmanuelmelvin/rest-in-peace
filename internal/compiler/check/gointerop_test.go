package check_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

func checkSource(t *testing.T, src string) diag.List {
	t.Helper()
	toks, mode, ds := lexer.Lex("test.bas", []byte(src))
	if ds.HasErrors() {
		t.Fatalf("lex error: %v", ds)
	}
	f, ds := parser.Parse("test.bas", toks, mode)
	if ds.HasErrors() {
		t.Fatalf("parse error: %v", ds)
	}
	return check.Check(&ast.Program{Entry: "test.bas", Files: []*ast.File{f}})
}

func TestCheckGoImportsValid(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "alias stdlib and third-party",
			src: `IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT strings FROM GO "strings"
IMPORT url FROM GO "net/url"
IMPORT math FROM GO "math"
IMPORT http FROM GO "net/http"

id = uuid.NewString()
s = strings.ToUpper("hi")
u = NEW(url.URL)
p = math.Pi
req = http.NewRequestWithContext("GET", "https://example.com", NIL)
PRINT id, s, u, p, req
`,
		},
		{
			name: "named Go import",
			src: `IMPORT { NewString, Parse } FROM GO "github.com/google/uuid"
id = NewString()
u = Parse(id)
PRINT u
`,
		},
		{
			name: "variadic Go function",
			src: `IMPORT fmt FROM GO "fmt"
PRINT fmt.Sprintf("hello")
PRINT fmt.Sprintf("%s=%d", "x", 1)
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := checkSource(t, tc.src)
			if ds.HasErrors() {
				t.Fatalf("unexpected errors:\n%v", ds)
			}
		})
	}
}

func TestCheckGoImportsErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		kind diag.Kind
		want string
	}{
		{
			name: "unknown Go package",
			src:  `IMPORT foo FROM GO "example.com/no/such/go/pkg"`,
			kind: diag.Undefined,
			want: "cannot load Go package",
		},
		{
			name: "unknown named symbol with hint",
			src:  `IMPORT { newstring } FROM GO "github.com/google/uuid"`,
			kind: diag.Undefined,
			want: `did you mean "NewString"`,
		},
		{
			name: "unknown member function with hint",
			src: `IMPORT strings FROM GO "strings"
PRINT strings.toupper("hi")`,
			kind: diag.Undefined,
			want: `did you mean "ToUpper"`,
		},
		{
			name: "unsupported callback parameter in named import",
			src:  `IMPORT { Map } FROM GO "strings"`,
			kind: diag.TypeMismatch,
			want: "function/callback",
		},
		{
			name: "unsupported channel return at call site",
			src: `IMPORT time FROM GO "time"
ch = time.After(1)`,
			kind: diag.TypeMismatch,
			want: "channel",
		},
		{
			name: "unsupported generic function",
			src:  `IMPORT { Contains } FROM GO "slices"`,
			kind: diag.TypeMismatch,
			want: "generic",
		},
		{
			name: "wrong arity on Go function",
			src: `IMPORT strings FROM GO "strings"
PRINT strings.ToUpper("a", "b")`,
			kind: diag.Syntax,
			want: "expects 1 argument, got 2",
		},
		{
			name: "wrong arity with context.Context auto-injected",
			src: `IMPORT http FROM GO "net/http"
req = http.NewRequestWithContext("GET", "https://example.com")`,
			kind: diag.Syntax,
			want: "expects 3 arguments, got 2",
		},
		{
			name: "NEW on interface rejected",
			src: `IMPORT io FROM GO "io"
r = NEW(io.Reader)`,
			kind: diag.TypeMismatch,
			want: "interface",
		},
		{
			name: "NEW on unknown type with hint",
			src: `IMPORT url FROM GO "net/url"
u = NEW(url.url)`,
			kind: diag.Undefined,
			want: `did you mean "URL"`,
		},
		{
			name: "calling Go type without NEW",
			src: `IMPORT url FROM GO "net/url"
u = url.URL()`,
			kind: diag.TypeMismatch,
			want: "NEW(URL.URL)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := checkSource(t, tc.src)
			if !ds.HasErrors() {
				t.Fatalf("expected %s error containing %q, got none", tc.kind, tc.want)
			}
			for _, d := range ds {
				if d.Kind == tc.kind && strings.Contains(d.Message, tc.want) {
					return
				}
			}
			t.Fatalf("no %s error containing %q; got:\n%v", tc.kind, tc.want, ds)
		})
	}
}
