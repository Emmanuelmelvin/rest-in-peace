package golden

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestParseScript(t *testing.T) {
	src := "Free text before the first exchange is a comment.\r\n" +
		"GET /not-a-request\r\n" +
		"\r\n" +
		"### hello\r\n" +
		"# a comment\r\n" +
		"\r\n" +
		"GET /hello?x=1\r\n" +
		"=> 200\r\n" +
		"\r\n" +
		"{\"message\": \"HELLO WORLD\"}\r\n" +
		"\r\n" +
		"###\n" +
		"POST /todos\n" +
		"X-Api-Key: a:b\n" +
		"Content-Type: text/plain\n" +
		"\n" +
		"{\"title\":\n" +
		"# not a comment inside a body\n" +
		"\n" +
		"=> 201\n" +
		"Location: /todos/1\n" +
		"x-ghost: <any>\n" +
		"\n" +
		"{\"id\": \"1\"}\n" +
		"### no body expected\n" +
		"DELETE /todos/1\n" +
		"=> 204\n"
	got, err := ParseScript("t.http", src)
	if err != nil {
		t.Fatal(err)
	}
	want := []Exchange{
		{
			Name: "hello", Line: 7,
			Req:  Request{Method: "GET", Target: "/hello?x=1"},
			Want: Response{Status: 200, Body: `{"message": "HELLO WORLD"}`},
		},
		{
			Line: 13,
			Req: Request{
				Method: "POST", Target: "/todos",
				Headers: []Header{{"X-Api-Key", "a:b"}, {"Content-Type", "text/plain"}},
				Body:    "{\"title\":\n# not a comment inside a body",
			},
			Want: Response{Status: 201, Headers: []Header{{"Location", "/todos/1"}, {"x-ghost", "<any>"}}, Body: `{"id": "1"}`},
		},
		{
			Name: "no body expected", Line: 26,
			Req:  Request{Method: "DELETE", Target: "/todos/1"},
			Want: Response{Status: 204},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseScript =\n%#v\nwant\n%#v", got, want)
	}
}

func TestParseScriptErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"empty", "just a comment\n", "no exchanges"},
		{"no request line", "### a\n# only a comment\n### b\nGET /\n=> 200\n", "no request line"},
		{"lower-case method", "###\nget /hello\n=> 200\n", "want a request line"},
		{"no path", "###\nGET\n=> 200\n", "want a request line"},
		{"relative path", "###\nGET hello\n=> 200\n", "want a request line"},
		{"no status", "###\nGET /hello\n", "has no expected response"},
		{"bad status", "###\nGET /hello\n=> OK\n", "want => followed by a status code"},
		{"status out of range", "###\nGET /hello\n=> 42\n", "want => followed by a status code"},
		{"body without blank line", "###\nGET /hello\n=> 200\n{\"a\": 1}\n", "t.http:4: want a header line"},
		{"bad request header", "###\nGET /hello\nnot a header\n=> 200\n", "t.http:3: want a header line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseScript("t.http", tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestParseJSONKeepsOrder(t *testing.T) {
	v, err := ParseJSON(` {"b": 1, "a": [true, null, "x", {"z": 2.5, "y": {}}]} `)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := compactJSON(v), `{"b":1,"a":[true,null,"x",{"z":2.5,"y":{}}]}`; got != want {
		t.Errorf("round trip = %s, want %s", got, want)
	}
	for _, bad := range []string{"", "{", `{"a": 1} {}`, `{"a": 1,}`, "nope"} {
		if _, err := ParseJSON(bad); err == nil {
			t.Errorf("ParseJSON(%q) succeeded", bad)
		}
	}
}

func TestMatchJSON(t *testing.T) {
	for _, tc := range []struct {
		got, want string
		err       string // "" means it must match
	}{
		{`{"a":1,"b":[1,2]}`, `{"a": 1, "b": [1, 2]}`, ""},
		{`{"b":1,"a":2}`, `{"a": 2, "b": 1}`, `$ has keys in order ["b" "a"], want ["a" "b"]`},
		{`{"a":1}`, `{"a": 1, "b": 2}`, `$ has keys ["a"], want ["a" "b"]`},
		{`{"a":{"b":[1,{"c":"x"}]}}`, `{"a":{"b":[1,{"c":"y"}]}}`, `$.a.b[1].c is "x", want "y"`},
		{`[1,2,3]`, `[1,2]`, `$ is [1,2,3], want [1,2]`},
		{`1.0`, `1`, ""},
		{`1e2`, `100`, ""},
		{`2.5`, `2.50001`, `$ is 2.5, want 2.50001`},
		{`"1"`, `1`, `$ is "1", want 1`},
		{`null`, `{}`, `$ is null, want {}`},
		{`{}`, `{}`, ""},
		{`[]`, `[]`, ""},
		{`false`, `false`, ""},
		{`{"requestId":"abc-123"}`, `{"requestId": "<any>"}`, ""},
		{`{"requestId":null}`, `{"requestId": "<any>"}`, ""},
		{`{}`, `{"requestId": "<any>"}`, `$ has keys [], want ["requestId"]`},
		{`{"requestId":"abc"}`, `{"requestId": "<any string>"}`, ""},
		{`{"requestId":""}`, `{"requestId": "<any string>"}`, `$.requestId is "", want a non-empty string`},
		{`{"requestId":7}`, `{"requestId": "<any string>"}`, `$.requestId is 7, want a non-empty string`},
	} {
		g, err := ParseJSON(tc.got)
		if err != nil {
			t.Fatal(err)
		}
		w, err := ParseJSON(tc.want)
		if err != nil {
			t.Fatal(err)
		}
		err = MatchJSON(g, w)
		switch {
		case tc.err == "" && err != nil:
			t.Errorf("MatchJSON(%s, %s) = %v, want a match", tc.got, tc.want, err)
		case tc.err != "" && (err == nil || err.Error() != tc.err):
			t.Errorf("MatchJSON(%s, %s) = %v, want %q", tc.got, tc.want, err, tc.err)
		}
	}
}

func TestMatchResponse(t *testing.T) {
	got := Got{
		Status: 201,
		Header: http.Header{"Location": {"/todos/1"}, "X-Request-Id": {"r-1"}, "Vary": {"a", "b"}},
		Body:   "{\"id\":\"1\",\"done\":false}\n",
	}
	for _, tc := range []struct {
		name string
		want Response
		err  string
	}{
		{"match", Response{Status: 201, Headers: []Header{{"location", "/todos/1"}}, Body: `{"id": "1", "done": false}`}, ""},
		{"wildcard header", Response{Status: 201, Headers: []Header{{"x-request-id", "<any>"}, {"X-REQUEST-ID", "<any string>"}}, Body: "<any>"}, ""},
		{"joined header", Response{Status: 201, Headers: []Header{{"vary", "a, b"}}, Body: "<any>"}, ""},
		{"wrong status", Response{Status: 200, Body: "<any>"}, "status is 201, want 200"},
		{"missing header", Response{Status: 201, Headers: []Header{{"x-ghost", "<any>"}}, Body: "<any>"}, "header x-ghost is missing"},
		{"wrong header", Response{Status: 201, Headers: []Header{{"location", "/todos/2"}}, Body: "<any>"}, `header location is "/todos/1", want "/todos/2"`},
		{"wrong body", Response{Status: 201, Body: `{"id": "1", "done": true}`}, "body: $.done is false, want true"},
		{"empty body wanted", Response{Status: 201}, "body differs"},
		{"text body", Response{Status: 201, Body: "HELLO"}, "body differs"},
		{"several problems", Response{Status: 500, Body: `{}`}, "status is 201, want 500; body: $ has keys"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := MatchResponse(got, tc.want)
			switch {
			case tc.err == "" && err != nil:
				t.Errorf("unexpected mismatch: %v", err)
			case tc.err != "" && (err == nil || !strings.HasPrefix(err.Error(), tc.err)):
				t.Errorf("err = %v, want one starting %q", err, tc.err)
			}
		})
	}

	if err := MatchResponse(Got{Status: 200, Body: "HELLO\r\n"}, Response{Status: 200, Body: "HELLO"}); err != nil {
		t.Errorf("text body: %v", err)
	}
	if err := MatchResponse(Got{Status: 204}, Response{Status: 204}); err != nil {
		t.Errorf("empty body: %v", err)
	}
	if err := MatchResponse(Got{Status: 200, Body: "<html>"}, Response{Status: 200, Body: "{}"}); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("non-JSON body for a JSON want: %v", err)
	}
}

func TestHasReadyLine(t *testing.T) {
	for out, want := range map[string]bool{
		"":                                  false,
		"READY.":                            false, // not a complete line yet
		"READY.\n":                          true,
		"READY. listening on :8080\r\n":     true,
		"booting\nREADY. listening on :1\n": true,
		"NOT READY.\n":                      false,
		"  READY.\n":                        false,
	} {
		if got := hasReadyLine(out); got != want {
			t.Errorf("hasReadyLine(%q) = %v, want %v", out, got, want)
		}
	}
}
