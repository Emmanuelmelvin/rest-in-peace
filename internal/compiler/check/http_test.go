package check

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

func TestHTTPChecks(t *testing.T) {
	cases := []struct {
		name, file, src string
		kind            diag.Kind
		want            string // "" means no diagnostics
		line            int
	}{
		{name: "hello", file: "a.bas",
			src: "ROUTE GET \"/hello\" CALL Hello\nROUTE GET \"/s/:id/*\" CALL Done\nSERVE\nFUNCTION Hello(req)\n  RETURN 1\nEND FUNCTION\nSUB Done(r)\n  RESPOND 204, NIL\nEND SUB\n"},
		{name: "classic hello", file: "c.bas",
			src: "10 ROUTE GET \"/hello\" GOSUB 100\n20 SERVE\n100 RESPOND 200, {\"m\": PARAM(\"x\")}\n"},
		{name: "handler without req", file: "a.bas",
			src:  "ROUTE GET \"/a\" CALL H\nFUNCTION H()\n  RETURN 1\nEND FUNCTION\n",
			kind: diag.Syntax, want: "H handles ROUTE GET \"/a\", so it takes one parameter, the request: FUNCTION H(req)", line: 1},
		{name: "handler with two params", file: "a.bas",
			src:  "ROUTE GET \"/a\" CALL H\nFUNCTION H(a, b)\n  RETURN 1\nEND FUNCTION\n",
			kind: diag.Syntax, want: "takes one parameter", line: 1},
		{name: "ROUTE inside a FUNCTION", file: "a.bas",
			src:  "FUNCTION H(req)\n  ROUTE GET \"/a\" CALL H\nEND FUNCTION\n",
			kind: diag.Syntax, want: "ROUTE goes in top-level code", line: 2},
		{name: "SERVE inside a SUB", file: "a.bas",
			src:  "SUB S()\n  SERVE\nEND SUB\n",
			kind: diag.Syntax, want: "SERVE goes in top-level code", line: 2},
		{name: "path without slash", file: "a.bas",
			src:  "ROUTE GET \"hello\" CALL H\nFUNCTION H(req)\nEND FUNCTION\n",
			kind: diag.Syntax, want: "a route path starts with /", line: 1},
		{name: "star in the middle", file: "a.bas",
			src:  "ROUTE GET \"/a/*/b\" CALL H\nFUNCTION H(req)\nEND FUNCTION\n",
			kind: diag.Syntax, want: "* only works as the last part", line: 1},
		{name: "bad param name", file: "a.bas",
			src:  "ROUTE GET \"/a/:-x\" CALL H\nFUNCTION H(req)\nEND FUNCTION\n",
			kind: diag.Syntax, want: "not a valid path parameter", line: 1},
		{name: "duplicate route", file: "a.bas",
			src:  "ROUTE GET \"/a/:id\" CALL H\nROUTE GET \"/a/:key\" CALL H\nFUNCTION H(req)\nEND FUNCTION\n",
			kind: diag.Syntax, want: "ROUTE GET \"/a/:key\" is already defined at line 1", line: 2},
		{name: "same path, other method", file: "a.bas",
			src: "ROUTE GET \"/a\" CALL H\nROUTE POST \"/a\" CALL H\nFUNCTION H(req)\nEND FUNCTION\n"},
		{name: "ROUTE CALL in classic mode", file: "c.bas",
			src:  "10 ROUTE GET \"/a\" CALL H\n20 SERVE\n",
			kind: diag.Syntax, want: "classic mode has no FUNCTIONs", line: 1},
		{name: "ROUTE GROUP valid", file: "a.bas",
			src: "ROUTE GROUP \"/v1\"\n  ROUTE GET \"/ping\" CALL H\nEND GROUP\nSERVE\nFUNCTION H(req)\n  RETURN 1\nEND FUNCTION\n"},
		{name: "ROUTE GROUP duplicate with top-level route", file: "a.bas",
			src:  "ROUTE GET \"/v1/ping\" CALL H\nROUTE GROUP \"/v1\"\n  ROUTE GET \"/ping\" CALL H\nEND GROUP\nFUNCTION H(req)\n  RETURN 1\nEND FUNCTION\n",
			kind: diag.Syntax, want: "ROUTE GET \"/v1/ping\" is already defined at line 1", line: 3},
		{name: "ROUTE GROUP bad prefix", file: "a.bas",
			src:  "ROUTE GROUP \"v1\"\n  ROUTE GET \"/ping\" CALL H\nEND GROUP\nFUNCTION H(req)\n  RETURN 1\nEND FUNCTION\n",
			kind: diag.Syntax, want: "a ROUTE GROUP prefix starts with /", line: 1},
		{name: "USE valid user and built-in middleware", file: "a.bas",
			src: "USE Auth\nUSE Guard ON \"/admin/*\"\nUSE CORS()\nUSE APIKEY(\"API_KEY\") ON \"/v1/*\"\nUSE REQUESTLOG()\nUSE RATELIMIT(60)\nUSE BEARERJWT(\"JWT_SECRET\") ON \"/v1/*\"\nSUB Auth(req)\nEND SUB\nFUNCTION Guard(req)\nEND FUNCTION\n"},
		{name: "USE handler wrong arity", file: "a.bas",
			src:  "USE Bad\nSUB Bad()\nEND SUB\n",
			kind: diag.Syntax, want: "middleware BAD takes one parameter", line: 1},
		{name: "USE bad scope", file: "a.bas",
			src:  "USE CORS() ON \"admin/*\"\n",
			kind: diag.Syntax, want: "a USE scope starts with /", line: 1},
		{name: "USE built-in without parens", file: "a.bas",
			src:  "USE CORS\n",
			kind: diag.Syntax, want: "CORS is a built-in middleware; call it with parentheses", line: 1},
		{name: "USE wrong arg count", file: "a.bas",
			src:  "USE APIKEY()\n",
			kind: diag.Syntax, want: "APIKEY takes 1 argument(s), got 0", line: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ds := checkSrc(t, c.file, c.src)
			if c.want == "" {
				if len(ds) != 0 {
					t.Fatalf("want no diagnostics, got %v", ds)
				}
				return
			}
			for _, d := range ds {
				if d.Kind == c.kind && strings.Contains(d.Message, c.want) && d.Pos.Line == c.line {
					return
				}
			}
			t.Fatalf("no %s diagnostic on line %d containing %q; got %v", c.kind, c.line, c.want, ds)
		})
	}
}
