package codegen_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// HTTP end-to-end tests: BASIC source → compiler.Compile → build → start the
// binary with PORT=0 → wait for its READY. line → send real requests.

// server is a running compiled program.
type server struct {
	base   string // http://127.0.0.1:<port>
	stderr *bytes.Buffer
}

var readyLine = regexp.MustCompile(`^READY\. listening on :(\d+)$`)

// startProgram compiles and starts the BASIC program src, saved as file.
func startProgram(t *testing.T, file, src string) *server {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a Go program; skipped with -short")
	}
	if _, err := build.LookGo(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	entry := filepath.Join(dir, file)
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := compiler.Compile(entry, compiler.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("compile errors:\n%v", res.Diagnostics)
	}
	bdir := filepath.Join(dir, "build")
	if err := build.Prepare(bdir, res.Files); err != nil {
		t.Fatal(err)
	}
	bin, err := build.Build(context.Background(), bdir, build.Options{Output: filepath.Join(bdir, "bin", "prog")})
	if err != nil {
		t.Fatalf("%v\n\ngenerated source:\n%s", err, dump(res.Files))
	}
	if os.Getenv("RIP_DUMP") != "" {
		t.Logf("generated source:\n%s", dump(res.Files))
	}

	cmd := exec.Command(bin)
	cmd.Dir = bdir
	cmd.Env = append(os.Environ(), "PORT=0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &server{stderr: &bytes.Buffer{}}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	port := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if m := readyLine.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
				port <- m[1]
				break
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case p := <-port:
		s.base = "http://127.0.0.1:" + p
	case <-time.After(30 * time.Second):
		t.Fatalf("no READY. line within 30 s; stderr:\n%s", s.stderr.String())
	}
	return s
}

// call sends one request and returns the status and the body without its trailing newline.
func (s *server) call(t *testing.T, method, path, body string, header ...string) (int, string, http.Header) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, s.base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v\nstderr:\n%s", method, path, err, s.stderr.String())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSuffix(normalize(string(b)), "\n"), resp.Header
}

type exchange struct {
	method, path, body string
	header             []string
	code               int
	want               string // exact body, or a regexp when it starts with ~
}

func (s *server) check(t *testing.T, xs []exchange) {
	t.Helper()
	for _, x := range xs {
		code, body, _ := s.call(t, x.method, x.path, x.body, x.header...)
		ok := body == x.want
		if strings.HasPrefix(x.want, "~") {
			ok = regexp.MustCompile(x.want[1:]).MatchString(body)
		}
		if code != x.code || !ok {
			t.Errorf("%s %s %s: got %d %s, want %d %s", x.method, x.path, x.body, code, body, x.code, x.want)
		}
	}
}

func readExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHTTPExampleHello(t *testing.T) {
	t.Parallel()
	s := startProgram(t, "hello.bas", readExample(t, "hello.bas"))
	s.check(t, []exchange{
		{method: "GET", path: "/hello", code: 200, want: `{"message":"HELLO WORLD"}`},
		{method: "GET", path: "/_health", code: 200, want: `{"status":"ALIVE"}`},
		{method: "GET", path: "/nope", code: 404, want: `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
		{method: "POST", path: "/hello", code: 404, want: `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
	})
}

func TestHTTPExampleClassicTodo(t *testing.T) {
	t.Parallel()
	s := startProgram(t, "classic-todo.bas", readExample(t, "classic-todo.bas"))
	s.check(t, []exchange{
		{method: "GET", path: "/todos", code: 200, want: `{}`},
		{method: "POST", path: "/todos", body: `{"title": "dig"}`, code: 201, want: `{"id":"1","title":"dig","done":false}`},
		{method: "POST", path: "/todos", body: `{"title": "rest"}`, code: 201, want: `{"id":"2","title":"rest","done":false}`},
		{method: "POST", path: "/todos", body: `{}`, code: 400, want: `{"error":"A TODO NEEDS A TITLE"}`},
		{method: "POST", path: "/todos", code: 400, want: `{"error":"A TODO NEEDS A TITLE"}`},
		{method: "POST", path: "/todos", body: `{"title":`, code: 400, want: `{"error":"BODY IS NOT VALID JSON"}`},
		{method: "PATCH", path: "/todos/2", code: 200, want: `{"id":"2","title":"rest","done":true}`},
		{method: "PATCH", path: "/todos/99", code: 404, want: `{"error":"THAT TODO IS DEAD"}`},
		{method: "GET", path: "/todos", code: 200, want: `{"1":{"id":"1","title":"dig","done":false},"2":{"id":"2","title":"rest","done":true}}`},
		{method: "GET", path: "/_health", code: 200, want: `{"status":"ALIVE"}`},
	})
}

func TestHTTPStructured(t *testing.T) {
	t.Parallel()
	src := `ROUTE GET "/echo/:id/*" CALL Echo
ROUTE POST "/made" CALL Made
ROUTE GET "/typed/:n" CALL Typed
ROUTE GET "/sub" CALL Answer
ROUTE GET "/dyn" CALL Dynamic
ROUTE GET "/crash" CALL Crash
ROUTE GET "/" CALL Root
ROUTE GET "/_health" CALL Health
PRINT "ROUTES READY"
SERVE

FUNCTION Echo(req)
  RETURN {"method": req.method, "path": req.path, "id": req.params.id, "rest": req.params.rest, "q": req.query.q, "agent": req.headers["x-agent"], "body": req.body, "ctx": req.ctx, "hasId": LEN(req.id) > 0}
END FUNCTION

FUNCTION Made(req)
  RESPOND 201, {"got": req.body.name}, {"Location": "/made/1"}
  RETURN "never"
END FUNCTION

FUNCTION Typed(req AS MAP) AS STRING
  RETURN "N=" & req.params.n
END FUNCTION

SUB Answer(req)
  RESPOND 202, "ACCEPTED"
END SUB

FUNCTION Dynamic(req)
  DIM s AS INTEGER = 200
  s = s + 3
  RESPOND s, NIL
END FUNCTION

FUNCTION Crash(req)
  x = NIL
  RETURN x.boom
END FUNCTION

FUNCTION Root(req)
  RETURN "ROOT"
END FUNCTION

FUNCTION Health(req)
  RETURN {"status": "UNDEAD"}
END FUNCTION
`
	s := startProgram(t, "api.bas", src)
	s.check(t, []exchange{
		{method: "GET", path: "/echo/7/a/b?q=hi", body: `{"k": [1, 2.5]}`, header: []string{"X-Agent", "ghost"}, code: 200,
			want: `{"method":"GET","path":"/echo/7/a/b","id":"7","rest":"a/b","q":"hi","agent":"ghost","body":{"k":[1,2.5]},"ctx":{},"hasId":true}`},
		{method: "GET", path: "/echo/7/x", code: 200,
			want: `{"method":"GET","path":"/echo/7/x","id":"7","rest":"x","q":null,"agent":null,"body":{},"ctx":{},"hasId":true}`},
		{method: "POST", path: "/made", body: `{"name": "casper"}`, code: 201, want: `{"got":"casper"}`},
		{method: "POST", path: "/made", body: `not json`, code: 400, want: `{"error":"BODY IS NOT VALID JSON"}`},
		{method: "GET", path: "/typed/5", code: 200, want: `"N=5"`},
		{method: "GET", path: "/sub", code: 202, want: `"ACCEPTED"`},
		{method: "GET", path: "/dyn", code: 203, want: `null`},
		{method: "GET", path: "/crash", code: 500, want: `~"INTERNAL ERROR"`},
		{method: "GET", path: "/", code: 200, want: `"ROOT"`},
		{method: "GET", path: "/other", code: 404, want: `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
		{method: "GET", path: "/_health", code: 200, want: `{"status":"UNDEAD"}`},
	})
	if _, _, h := s.call(t, "POST", "/made", `{}`); h.Get("Location") != "/made/1" {
		t.Errorf("RESPOND headers: Location = %q", h.Get("Location"))
	}
}

// Classic handlers: GOSUB inside a handler, RETURN from the handler's own
// GOSUB ending the request, and many concurrent requests each with their own
// GOSUB stack (Requirement 7.10).
func TestHTTPClassic(t *testing.T) {
	t.Parallel()
	src := `10 HITS = 0
20 ROUTE GET "/hit/:n" GOSUB 100
30 ROUTE GET "/ret" GOSUB 300
40 ROUTE GET "/fall" GOSUB 400
50 ROUTE GET "/q" GOSUB 500
60 SERVE
70 PRINT "NEVER RUNS"
100 N = VAL(PARAM("n"))
110 GOSUB 200
120 GOSUB 200
130 RESPOND 200, {"n": N, "hits": HITS}
200 HITS = HITS + 1
210 N = N * 2
220 RETURN
300 HITS = HITS + 100
310 RETURN
400 X = 1
500 RESPOND 200, {"q": QUERY("q"), "h": HEADER("x-ghost"), "b": BODY()}
`
	s := startProgram(t, "classic.bas", src)
	s.check(t, []exchange{
		{method: "GET", path: "/hit/3", code: 200, want: `{"n":12,"hits":2}`},
		{method: "GET", path: "/ret", code: 200, want: `null`},
		{method: "GET", path: "/hit/1", code: 200, want: `{"n":4,"hits":104}`},
		{method: "GET", path: "/q?q=boo", header: []string{"X-Ghost", "yes"}, code: 200, want: `{"q":"boo","h":"yes","b":{}}`},
		{method: "GET", path: "/fall", code: 200, want: `{"q":null,"h":null,"b":{}}`},
	})

	// Concurrent requests: classic globals stay consistent because handlers
	// run one at a time, and no request sees another's GOSUB stack.
	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, body, _ := s.call(t, "GET", fmt.Sprintf("/hit/%d", i), "")
			if code != 200 || !strings.HasPrefix(body, fmt.Sprintf(`{"n":%d,`, i*4)) {
				t.Errorf("/hit/%d: got %d %s", i, code, body)
			}
		}(i)
	}
	wg.Wait()
	if _, body, _ := s.call(t, "GET", "/hit/0", ""); body != fmt.Sprintf(`{"n":0,"hits":%d}`, 104+2*n+2) {
		t.Errorf("after %d concurrent requests: %s", n, body)
	}
}

// Compile errors for HTTP statements that codegen catches itself.
func TestHTTPDiagnostics(t *testing.T) {
	route := func(line int, path, handler string) *ast.RouteStmt {
		return &ast.RouteStmt{At: at(line), Method: "GET", Path: path, Handler: handler}
	}
	cases := []struct {
		name string
		prog *ast.Program
		kind diag.Kind
		want string
		line int
	}{
		{"handler without a parameter", structured("d.bas", route(1, "/a", "H"), function(2, "H", nil, "", ret(3, num(1)))),
			diag.Syntax, "so it takes one parameter, the request: FUNCTION H(req)", 1},
		{"handler with two parameters", structured("d.bas", route(1, "/a", "H"), function(2, "H", params(param("a", "", nil), param("b", "", nil)), "")),
			diag.Syntax, "takes one parameter", 1},
		{"ROUTE inside a FUNCTION", structured("d.bas", function(1, "H", params(param("req", "", nil)), "", route(2, "/a", "H"))),
			diag.Syntax, "ROUTE goes in top-level code", 2},
		{"ROUTE GOSUB in structured mode", structured("d.bas", &ast.RouteStmt{At: at(4), Method: "GET", Path: "/a", GosubLine: 10}),
			diag.Syntax, "ROUTE … GOSUB only works in classic mode", 4},
		{"ROUTE CALL in classic mode", classic("d.bas", ln(10, 1, route(1, "/a", "H"))),
			diag.Syntax, "classic mode has no FUNCTIONs", 1},
		{"RESPOND status out of range", structured("d.bas", &ast.RespondStmt{At: at(6), Status: num(1000), Value: nilv()}),
			diag.TypeMismatch, "RESPOND needs an HTTP status from 100 to 599", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, ds := codegen.Generate(c.prog, codegen.Options{})
			if files != nil {
				t.Errorf("got %d files, want none when there are errors", len(files))
			}
			for _, d := range ds {
				if d.Kind == c.kind && strings.Contains(d.Message, c.want) {
					if d.Pos.Line != c.line || d.Pos.File != "d.bas" {
						t.Errorf("diagnostic at %s, want d.bas:%d", d.Pos, c.line)
					}
					return
				}
			}
			t.Fatalf("no %s diagnostic containing %q; got:\n%v", c.kind, c.want, ds)
		})
	}
}

// The classic function starts at a ROUTE … GOSUB line through its entry switch.
func TestClassicEntrySwitch(t *testing.T) {
	p := classic("e.bas",
		ln(10, 1, &ast.RouteStmt{At: at(1), Method: "GET", Path: "/a", GosubLine: 100}),
		ln(20, 2, &ast.ServeStmt{At: at(2)}),
		ln(100, 3, &ast.RespondStmt{At: at(3), Status: num(200), Value: str("A")}),
	)
	files, ds := codegen.Generate(p, codegen.Options{})
	if ds.HasErrors() {
		t.Fatal(ds)
	}
	src := string(files[1].Src)
	for _, want := range []string{
		`c.App().RouteGosub("GET", "/a", 100, rip_e_classic)`,
		"switch entry {\n\tcase 100:\n\t\tgoto L100\n\t}",
		"c.App().Serve(nil)\n\treturn",
		"rt.RespondNow(c, 200, \"A\")",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q:\n%s", want, src)
		}
	}
}
