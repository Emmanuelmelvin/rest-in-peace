package rt

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRoutePattern(t *testing.T) {
	cases := []struct{ in, want, err string }{
		{"/hello", "/hello", ""},
		{"/", "/{$}", ""},
		{"/todos/", "/todos/{$}", ""},
		{"/todos/:id", "/todos/{id}", ""},
		{"/a/:x/b/:y", "/a/{x}/b/{y}", ""},
		{"/files/*", "/files/{rest...}", ""},
		{"*", "", "starts with /"},
		{"hello", "", "starts with /"},
		{"/a/*/b", "", "last part"},
		{"/a/:", "", "not a valid path parameter"},
		{"/a/:1x", "", "not a valid path parameter"},
		{"/a/:id/:id", "", "appears twice"},
		{"/a/:rest/*", "", "appears twice"},
		{"/a/{id}", "", "cannot contain {"},
		{"/a//b", "", "empty part"},
	}
	for _, c := range cases {
		got, err := RoutePattern(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("RoutePattern(%q) error = %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("RoutePattern(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

// do sends one request to the app's handler.
func do(t *testing.T, h http.Handler, method, target, body string, header ...string) (int, string, http.Header) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, rd)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, strings.TrimSuffix(w.Body.String(), "\n"), w.Header()
}

func json1(t *testing.T, v Value) string {
	t.Helper()
	b, err := marshalValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStructuredRequest(t *testing.T) {
	a := NewApp("main.bas")
	var got *Map
	a.Route("POST", "/users/:id/files/*", func(c *Ctx, req Value) Value {
		got = req.(*Map)
		return MapOf("ok", true)
	})
	code, body, hdr := do(t, a.Handler(), "POST", "/users/42/files/a/b.txt?b=2&a=1&b=3&x=%20y",
		`{"z": 1, "a": [1, 2.5, "s", null, true], "n": {"k": "v"}}`, "X-Custom", "Yes")
	if code != 200 || body != `{"ok":true}` {
		t.Fatalf("got %d %s", code, body)
	}
	if ct := hdr.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	want := map[string]string{
		"method": `"POST"`,
		"path":   `"/users/42/files/a/b.txt"`,
		"params": `{"id":"42","rest":"a/b.txt"}`,
		"query":  `{"b":"2","a":"1","x":" y"}`,
		"body":   `{"z":1,"a":[1,2.5,"s",null,true],"n":{"k":"v"}}`,
		"ctx":    `{}`,
	}
	for k, w := range want {
		v, _ := got.Get(k)
		if s := json1(t, v); s != w {
			t.Errorf("req.%s = %s, want %s", k, s, w)
		}
	}
	h, _ := got.Get("headers")
	if v := Get(h, "x-custom"); v != "Yes" {
		t.Errorf("req.headers[\"x-custom\"] = %v", v)
	}
	if id, _ := got.Get("id"); len(ToString(id)) != 16 {
		t.Errorf("req.id = %v, want 16 hex characters", id)
	}
	z, _ := Get(got, "body").(*Map).Get("z")
	if _, ok := z.(int64); !ok {
		t.Errorf("whole JSON numbers should be INTEGER, got %T", z)
	}
}

func TestResponses(t *testing.T) {
	a := NewApp("main.bas")
	a.Route("GET", "/ret", func(c *Ctx, req Value) Value { return ArrayOf(int64(1), "two") })
	a.Route("GET", "/nil", func(c *Ctx, req Value) Value { return nil })
	a.Route("POST", "/respond", func(c *Ctx, req Value) Value {
		RespondNow(c, 201, MapOf("created", true), MapOf("X-Thing", "yes", "Location", "/x/1"))
		return "not reached"
	})
	a.Route("GET", "/text", func(c *Ctx, req Value) Value {
		RespondNow(c, 200, "<h1>BOO</h1>", MapOf("Content-Type", "text/html"))
		return nil
	})
	a.Route("GET", "/empty", func(c *Ctx, req Value) Value {
		RespondNow(c, 204, "ignored")
		return nil
	})
	a.Route("GET", "/httperror", func(c *Ctx, req Value) Value {
		panic(HTTPError(418, "I AM A TEAPOT", nil))
	})
	a.Route("GET", "/crash", func(c *Ctx, req Value) Value {
		var m map[string]int
		m["x"] = 1 // a Go runtime panic
		return nil
	})
	a.Route("GET", "/end", func(c *Ctx, req Value) Value { End(); return 1 })
	a.Route("GET", "/body", func(c *Ctx, req Value) Value { return Get(req, "body") })
	h := a.Handler()

	cases := []struct {
		method, target, body string
		code                 int
		want                 string
	}{
		{"GET", "/ret", "", 200, `[1,"two"]`},
		{"GET", "/nil", "", 200, `null`},
		{"POST", "/respond", "", 201, `{"created":true}`},
		{"GET", "/text", "", 200, `<h1>BOO</h1>`},
		{"GET", "/empty", "", 204, ``},
		{"GET", "/end", "", 200, `null`},
		{"GET", "/body", "", 200, `{}`},
		{"GET", "/body", "  \n", 200, `{}`},
		{"GET", "/body", "[1]", 200, `[1]`},
		{"GET", "/body", "{bad", 400, `{"error":"BODY IS NOT VALID JSON"}`},
		{"GET", "/body", `{"a":1} {"b":2}`, 400, `{"error":"BODY IS NOT VALID JSON"}`},
		{"GET", "/nope", "", 404, `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
		{"DELETE", "/ret", "", 404, `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
		{"GET", "/ret/", "", 404, `{"error":"ROUTE NOT FOUND IN THIS REALM"}`},
		{"GET", "/_health", "", 200, `{"status":"ALIVE"}`},
	}
	for _, c := range cases {
		code, body, _ := do(t, h, c.method, c.target, c.body)
		if code != c.code || body != c.want {
			t.Errorf("%s %s %q: got %d %s, want %d %s", c.method, c.target, c.body, code, body, c.code, c.want)
		}
	}

	_, _, hdr := do(t, h, "POST", "/respond", "")
	if hdr.Get("X-Thing") != "yes" || hdr.Get("Location") != "/x/1" {
		t.Errorf("RESPOND headers not set: %v", hdr)
	}
	_, _, hdr = do(t, h, "GET", "/text", "")
	if hdr.Get("Content-Type") != "text/html" {
		t.Errorf("Content-Type = %q", hdr.Get("Content-Type"))
	}

	// The bodies of error responses belong to ErrorResponse (task 2.2); the
	// server only has to route escaped errors through it.
	if code, body, _ := do(t, h, "GET", "/httperror", ""); code != 418 || !strings.HasPrefix(body, `{"error":"I AM A TEAPOT"`) {
		t.Errorf("HTTPERROR: got %d %s", code, body)
	}
	if code, body, _ := do(t, h, "GET", "/crash", ""); code != 500 || !strings.Contains(body, `"INTERNAL ERROR"`) {
		t.Errorf("Go panic: got %d %s", code, body)
	}
}

func TestHealthOverride(t *testing.T) {
	a := NewApp("main.bas")
	a.Route("GET", "/_health", func(c *Ctx, req Value) Value { return MapOf("status", "UNDEAD") })
	if code, body, _ := do(t, a.Handler(), "GET", "/_health", ""); code != 200 || body != `{"status":"UNDEAD"}` {
		t.Errorf("got %d %s", code, body)
	}
}

func TestBodyCap(t *testing.T) {
	a := NewApp("main.bas")
	a.MaxBody = 10
	a.Route("POST", "/x", func(c *Ctx, req Value) Value { return Get(req, "body") })
	h := a.Handler()
	if code, body, _ := do(t, h, "POST", "/x", `{"a":"0123456789"}`); code != 413 || body != `{"error":"BODY IS TOO LARGE"}` {
		t.Errorf("too large: got %d %s", code, body)
	}
	if code, body, _ := do(t, h, "POST", "/x", `{"a":1}`); code != 200 || body != `{"a":1}` {
		t.Errorf("small: got %d %s", code, body)
	}
}

func TestBodyCapFromEnv(t *testing.T) {
	if a := NewApp(""); a.MaxBody != DefaultMaxBody {
		t.Errorf("default MaxBody = %d", a.MaxBody)
	}
	t.Setenv(MaxBodyEnv, "2048")
	if a := NewApp(""); a.MaxBody != 2048 {
		t.Errorf("MaxBody = %d, want 2048", a.MaxBody)
	}
}

func TestRouteErrors(t *testing.T) {
	mustPanic := func(name, want string, f func()) {
		t.Helper()
		defer func() {
			r := recover()
			e, ok := r.(*Error)
			if !ok || !strings.Contains(e.Message, want) {
				t.Errorf("%s: panic %v, want an *Error containing %q", name, r, want)
			}
		}()
		f()
	}
	h := func(c *Ctx, req Value) Value { return nil }
	a := NewApp("")
	a.Route("GET", "/a/:id", h)
	mustPanic("clash", "clashes with another route", func() { a.Route("GET", "/a/:key", h) })
	mustPanic("bad path", "starts with /", func() { a.Route("GET", "a", h) })
	a.Handler()
	mustPanic("after start", "after the server started", func() { a.Route("GET", "/b", h) })
}

func TestStatus(t *testing.T) {
	if Status(int64(201)) != 201 || Status(float64(404)) != 404 {
		t.Error("Status of a valid number")
	}
	for _, v := range []Value{int64(99), int64(600), 200.5, "200", nil} {
		func() {
			defer func() {
				if e, ok := recover().(*Error); !ok || e.Kind != KindTypeMismatch {
					t.Errorf("Status(%v): want a TYPE MISMATCH", v)
				}
			}()
			Status(v)
		}()
	}
}

// Classic handlers run one at a time, and each request has its own GOSUB stack.
func TestClassicSerializedWithOwnGosubStack(t *testing.T) {
	a := NewApp("")
	var running, maxRunning int32
	entry := func(c *Ctx, line int) {
		n := atomic.AddInt32(&running, 1)
		if n > atomic.LoadInt32(&maxRunning) {
			atomic.StoreInt32(&maxRunning, n)
		}
		if len(c.gosub) != 0 {
			t.Errorf("request started with GOSUB stack %v", c.gosub)
		}
		c.GosubPush(line)
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		RespondNow(c, 200, MapOf("line", int64(line), "id", Param(c, "id")))
	}
	a.RouteGosub("GET", "/t/:id", 100, entry)
	h := a.Handler()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, body, _ := do(t, h, "GET", "/t/7", ""); code != 200 || body != `{"line":100,"id":"7"}` {
				t.Errorf("got %d %s", code, body)
			}
		}()
	}
	wg.Wait()
	if maxRunning != 1 {
		t.Errorf("%d classic handlers ran at once, want 1", maxRunning)
	}
}

func TestClassicHelpers(t *testing.T) {
	a := NewApp("")
	a.RouteGosub("POST", "/h/:id", 10, func(c *Ctx, line int) {
		RespondNow(c, 200, ArrayOf(Param(c, "id"), Param(c, "nope"), Query(c, "q"), Header(c, "X-API-KEY"), Body(c)))
	})
	a.RouteGosub("GET", "/fall", 20, func(c *Ctx, line int) {})
	h := a.Handler()
	code, body, _ := do(t, h, "POST", "/h/5?q=hi", `{"t":1}`, "x-api-key", "secret")
	if code != 200 || body != `["5",null,"hi","secret",{"t":1}]` {
		t.Errorf("got %d %s", code, body)
	}
	if code, body, _ := do(t, h, "GET", "/fall", ""); code != 200 || body != "null" {
		t.Errorf("falling off a classic handler: got %d %s", code, body)
	}

	defer func() {
		if e, ok := recover().(*Error); !ok || !strings.Contains(e.Message, "PARAM() only works") {
			t.Errorf("PARAM outside a request: %v", e)
		}
	}()
	Param(NewCtx(nil), "id")
}

func TestGracefulShutdown(t *testing.T) {
	a := NewApp("")
	started := make(chan struct{})
	a.Route("GET", "/slow", func(c *Ctx, req Value) Value {
		close(started)
		time.Sleep(200 * time.Millisecond)
		return "DONE"
	})
	var closed atomic.Bool
	a.OnShutdown(func() { closed.Store(true) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- a.serveOn(ctx, ln) }()

	type resp struct {
		code int
		body string
		err  error
	}
	got := make(chan resp, 1)
	go func() {
		r, err := http.Get("http://" + ln.Addr().String() + "/slow")
		if err != nil {
			got <- resp{err: err}
			return
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		got <- resp{code: r.StatusCode, body: string(b)}
	}()
	<-started
	cancel() // as if SIGTERM arrived while /slow is in flight

	r := <-got
	if r.err != nil || r.code != 200 || r.body != "\"DONE\"\n" {
		t.Errorf("in-flight request: %d %q %v", r.code, r.body, r.err)
	}
	if err := <-served; err != nil {
		t.Errorf("serveOn: %v", err)
	}
	if !closed.Load() {
		t.Error("OnShutdown hooks did not run")
	}
	if _, err := http.Get("http://" + ln.Addr().String() + "/slow"); err == nil {
		t.Error("server still accepts connections after shutdown")
	}
}

func TestDecodeJSONKeepsOrder(t *testing.T) {
	v, err := decodeJSON([]byte(`{"b": 1, "a": {"y": 1e3, "x": -2}, "c": []}`))
	if err != nil {
		t.Fatal(err)
	}
	if s := json1(t, v); s != `{"b":1,"a":{"y":1000,"x":-2},"c":[]}` {
		t.Errorf("got %s", s)
	}
	if _, ok := Get(Get(v, "a"), "y").(float64); !ok {
		t.Errorf("1e3 should be a NUMBER")
	}
	for _, bad := range []string{`{`, `{"a" 1}`, `[1,]`, `nul`, `1 2`} {
		if _, err := decodeJSON([]byte(bad)); err == nil {
			t.Errorf("decodeJSON(%q) should fail", bad)
		}
	}
}
