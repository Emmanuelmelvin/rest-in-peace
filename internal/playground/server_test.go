package playground

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// TestMain doubles as a fake program: the sandbox runs this test binary with
// the program environment (PORT and RIP_PLAYGROUND=1), and it behaves the way
// the "' fake:<mode>" comment in its main.bas says. This keeps most tests
// free of real Go builds.
func TestMain(m *testing.M) {
	if os.Getenv("RIP_PLAYGROUND") == "1" && os.Getenv("PORT") != "" {
		fakeProgram()
		return
	}
	os.Exit(m.Run())
}

func fakeProgram() {
	src, _ := os.ReadFile(filepath.Join("..", entryName)) // cwd is <program>/data
	mode := "serve"
	if _, rest, ok := strings.Cut(string(src), "' fake:"); ok {
		mode, _, _ = strings.Cut(rest, "\n")
		mode = strings.TrimSpace(mode)
	}
	port := os.Getenv("PORT")
	switch mode {
	case "end":
		fmt.Println("HELLO FROM THE GRAVE")
		os.Exit(0)
	case "loop":
		fmt.Println("LOOPING FOREVER")
		for {
			time.Sleep(time.Hour)
		}
	case "port":
		fmt.Println(readyPrefix + "1")
		for {
			time.Sleep(time.Hour)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "HELLO WORLD", "path": r.URL.Path, "prefix": r.Header.Get("X-Forwarded-Prefix")})
	})
	mux.HandleFunc("/print", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("PRINTED", r.URL.Query().Get("msg"))
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})
	mux.HandleFunc("/env", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, os.Environ())
	})
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) { select {} })
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(readyPrefix + port)
	_ = http.Serve(ln, mux)
}

// fakeBuild "builds" a program by linking this test binary into place.
func fakeBuild(ctx context.Context, dir string, files []codegen.File) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "bin", "program")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return "", err
	}
	if err := os.Link(self, bin); err == nil {
		return bin, nil
	}
	in, err := os.Open(self)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(bin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	return bin, out.Close()
}

const helloCode = `' fake:serve
ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
`

func fakeCode(mode string) string {
	return strings.Replace(helloCode, "fake:serve", "fake:"+mode, 1)
}

type testHost struct {
	t   *testing.T
	pg  *Server
	web *httptest.Server
}

func newTestHost(t *testing.T, mutate func(*Limits)) *testHost {
	t.Helper()
	limits := DefaultLimits()
	if mutate != nil {
		mutate(&limits)
	}
	dir := t.TempDir()
	pg := New(Config{DataDir: dir, Limits: limits})
	pg.build = fakeBuild
	web := httptest.NewServer(pg.Handler())
	t.Cleanup(func() {
		web.Close()
		pg.Close()
	})
	return &testHost{t: t, pg: pg, web: web}
}

func (h *testHost) create(code string) (int, createResponse) {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"code": code})
	res, err := http.Post(h.web.URL+"/api/programs", "application/json", strings.NewReader(string(body)))
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var out createResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		h.t.Fatalf("decoding the create response: %v", err)
	}
	return res.StatusCode, out
}

func (h *testHost) mustCreate(code string) createResponse {
	h.t.Helper()
	status, out := h.create(code)
	if status != http.StatusCreated {
		h.t.Fatalf("create: %d %+v", status, out)
	}
	return out
}

func (h *testHost) get(path string) (int, http.Header, string) {
	h.t.Helper()
	res, err := http.Get(h.web.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}

// waitForLog reads the SSE stream until a line contains want.
func (h *testHost) waitForLog(id, want string) []LogLine {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.web.URL+"/api/programs/"+id+"/logs", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		h.t.Fatalf("logs Content-Type = %q", ct)
	}
	var seen []LogLine
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var l LogLine
		if err := json.Unmarshal([]byte(data), &l); err != nil {
			h.t.Fatalf("bad event %q: %v", data, err)
		}
		seen = append(seen, l)
		if strings.Contains(l.Text, want) {
			return seen
		}
	}
	h.t.Fatalf("log line %q never arrived; saw %+v", want, seen)
	return nil
}

func TestHealth(t *testing.T) {
	srv := New(Config{DataDir: t.TempDir(), Limits: DefaultLimits()})
	defer srv.Close()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ALIVE") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestEnvDoesNotLeakHost(t *testing.T) {
	t.Setenv("HOST_SECRET", "do-not-leak")
	for _, kv := range Env(9000, "/tmp/p1", DefaultLimits()) {
		if strings.Contains(kv, "do-not-leak") {
			t.Fatalf("program env leaked a host variable: %s", kv)
		}
	}
}

func TestAllowlist(t *testing.T) {
	if !Allowed("github.com/google/uuid") || Allowed("os/exec") {
		t.Fatal("allowlist is wrong")
	}
}

func TestCreateRejectsBadRequests(t *testing.T) {
	h := newTestHost(t, nil)
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"not json", "PRINT 1", http.StatusBadRequest, "NOT VALID JSON"},
		{"empty", `{"code": "  "}`, http.StatusBadRequest, "NO PROGRAM"},
		{"too large", `{"code": "` + strings.Repeat("A", maxCodeBytes+10) + `"}`, http.StatusRequestEntityTooLarge, "TOO LARGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := http.Post(h.web.URL+"/api/programs", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.status || !strings.Contains(string(b), tc.want) {
				t.Fatalf("got %d %s, want %d containing %q", res.StatusCode, b, tc.status, tc.want)
			}
		})
	}
}

func TestCreateReportsDiagnostics(t *testing.T) {
	h := newTestHost(t, nil)
	status, out := h.create("IF 1 = 1\n  PRINT \"X\"\nEND IF\n")
	if status != http.StatusBadRequest {
		t.Fatalf("status %d %+v", status, out)
	}
	if !strings.HasPrefix(out.Error, "?SYNTAX ERROR IN main.bas:1 — ") || len(out.Diagnostics) == 0 {
		t.Fatalf("want a classic diagnostic, got %+v", out)
	}
	entries, _ := os.ReadDir(filepath.Join(h.pg.cfg.DataDir, "programs"))
	if len(entries) != 0 {
		t.Fatalf("a failed program left %d directories behind", len(entries))
	}
}

func TestCreateRefusesImports(t *testing.T) {
	h := newTestHost(t, nil)
	for _, tc := range []struct{ code, want string }{
		{"IMPORT exec FROM GO \"os/exec\"\nPRINT 1\n", `?NOT ALLOWED ERROR IN main.bas:1 — the playground only allows IMPORT … FROM GO for github.com/google/uuid and github.com/shopspring/decimal; "os/exec"`},
		{"IMPORT \"../../etc/passwd\"\nPRINT 1\n", `?NOT ALLOWED ERROR IN main.bas:1 — a playground program is a single file`},
	} {
		status, out := h.create(tc.code)
		if status != http.StatusBadRequest || !strings.HasPrefix(out.Error, tc.want) {
			t.Errorf("code %q: got %d %+v, want error starting %q", tc.code, status, out, tc.want)
		}
	}
	if ds := checkImports([]byte("IMPORT { NewString } FROM GO \"github.com/google/uuid\"\nPRINT NewString()\n")); len(ds) != 0 {
		t.Fatalf("an allowlisted import was refused: %v", ds)
	}
}

func TestRunProxiesAndStreamsLogs(t *testing.T) {
	h := newTestHost(t, nil)
	out := h.mustCreate(helloCode)
	if out.Status != "RUNNING" || out.URL != "/p/"+out.ID+"/" {
		t.Fatalf("create: %+v", out)
	}

	status, hdr, body := h.get(out.URL + "hello")
	if status != http.StatusOK || !strings.Contains(body, `"path":"/hello"`) || !strings.Contains(body, `"prefix":"/p/`+out.ID) {
		t.Fatalf("proxy: %d %s", status, body)
	}
	if hdr.Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("proxied responses must be sandboxed, got CSP %q", hdr.Get("Content-Security-Policy"))
	}
	if status, _, _ := h.get(out.URL + "print?msg=BOO"); status != http.StatusOK {
		t.Fatalf("print: %d", status)
	}

	// A new subscriber gets the replay: READY, PRINT output and request lines.
	lines := h.waitForLog(out.ID, "GET /print?msg=BOO → 200")
	var text []string
	for _, l := range lines {
		text = append(text, l.Stream+": "+l.Text)
	}
	joined := strings.Join(text, "\n")
	for _, want := range []string{"stdout: READY. listening on :", "request: GET /hello → 200", "stdout: PRINTED BOO"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log is missing %q:\n%s", want, joined)
		}
	}

	if status, _, _ := h.get("/p/0123456789abcdef/hello"); status != http.StatusNotFound {
		t.Fatalf("unknown program: %d", status)
	}
	if status, _, _ := h.get("/api/programs/0123456789abcdef/logs"); status != http.StatusNotFound {
		t.Fatalf("unknown program logs: %d", status)
	}
}

func TestProgramGetsOnlyItsOwnEnvironment(t *testing.T) {
	t.Setenv("HOST_SECRET", "do-not-leak")
	h := newTestHost(t, nil)
	out := h.mustCreate(helloCode)
	_, _, body := h.get(out.URL + "env")
	var env []string
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("env: %v %s", err, body)
	}
	allowed := map[string]bool{"PORT": true, "RIP_PLAYGROUND": true, "GOMEMLIMIT": true, "GOMAXPROCS": true, "DATABASE_URL": true}
	if runtime.GOOS == "windows" {
		allowed["SYSTEMROOT"] = true // os/exec adds it on Windows
	}
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !allowed[strings.ToUpper(k)] {
			t.Errorf("program saw %q", kv)
		}
	}
	if !strings.Contains(body, "RIP_PLAYGROUND=1") {
		t.Fatalf("program env is missing RIP_PLAYGROUND=1: %s", body)
	}
}

func TestWatchdogKillsAndRestarts(t *testing.T) {
	h := newTestHost(t, func(l *Limits) { l.RequestTimeout = 200 * time.Millisecond })
	out := h.mustCreate(helloCode)
	start := time.Now()
	status, _, body := h.get(out.URL + "hang")
	if status != http.StatusLoopDetected || !strings.Contains(body, "TRAPPED IN AN ETERNAL GOTO LOOP") {
		t.Fatalf("hang: %d %s", status, body)
	}
	if d := time.Since(start); d < 1200*time.Millisecond || d > 10*time.Second {
		t.Fatalf("watchdog fired after %v, want about RequestTimeout + 1s", d)
	}
	if status, _, body := h.get(out.URL + "hello"); status != http.StatusOK {
		t.Fatalf("after restart: %d %s", status, body)
	}
	h.waitForLog(out.ID, "RESTARTED. READY.")
}

func TestLeastRecentlyUsedIsStopped(t *testing.T) {
	h := newTestHost(t, func(l *Limits) { l.MaxPrograms = 1 })
	a := h.mustCreate(helloCode)
	b := h.mustCreate(helloCode)
	sa, sb := h.pg.lookup(a.ID), h.pg.lookup(b.ID)
	if sa.currentState() != stateStopped || sb.currentState() != stateRunning {
		t.Fatalf("states: a=%d b=%d", sa.currentState(), sb.currentState())
	}
	// The stopped program wakes on its next request, and makes room in turn.
	if status, _, body := h.get(a.URL + "hello"); status != http.StatusOK {
		t.Fatalf("wake: %d %s", status, body)
	}
	if sa.currentState() != stateRunning || sb.currentState() != stateStopped {
		t.Fatalf("after wake: a=%d b=%d", sa.currentState(), sb.currentState())
	}
}

func TestExpiredProgramsAreDeleted(t *testing.T) {
	h := newTestHost(t, nil)
	out := h.mustCreate(helloCode)
	dir := h.pg.programDir(out.ID)
	h.pg.sweep(time.Now().Add(h.pg.cfg.Limits.Expiry - time.Hour))
	if h.pg.lookup(out.ID) == nil {
		t.Fatal("swept before expiry")
	}
	h.pg.sweep(time.Now().Add(h.pg.cfg.Limits.Expiry + time.Hour))
	if status, _, _ := h.get(out.URL + "hello"); status != http.StatusNotFound {
		t.Fatalf("expired program still answers: %d", status)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expired program's files remain: %v", err)
	}
}

func TestOldestStoredProgramIsDeleted(t *testing.T) {
	h := newTestHost(t, func(l *Limits) { l.MaxStored = 2 })
	a := h.mustCreate(fakeCode("end"))
	h.mustCreate(fakeCode("end"))
	h.mustCreate(fakeCode("end"))
	if h.pg.lookup(a.ID) != nil {
		t.Fatal("the oldest program should have been deleted")
	}
	h.pg.mu.Lock()
	n := len(h.pg.programs)
	h.pg.mu.Unlock()
	if n != 2 {
		t.Fatalf("%d programs stored, want 2", n)
	}
}

func TestProgramWithoutServe(t *testing.T) {
	h := newTestHost(t, nil)
	out := h.mustCreate(fakeCode("end"))
	if out.Status != "ENDED" {
		t.Fatalf("status %q", out.Status)
	}
	h.waitForLog(out.ID, "HELLO FROM THE GRAVE")
	status, _, body := h.get(out.URL)
	if status != http.StatusBadGateway || !strings.Contains(body, "NOT SERVING") {
		t.Fatalf("proxy to an ended program: %d %s", status, body)
	}
}

func TestStartupLoopIsTrapped(t *testing.T) {
	saved := startupTimeout
	startupTimeout = 500 * time.Millisecond
	defer func() { startupTimeout = saved }()
	h := newTestHost(t, nil)
	status, out := h.create(fakeCode("loop"))
	if status != http.StatusLoopDetected || out.Error != "TRAPPED IN AN ETERNAL GOTO LOOP" {
		t.Fatalf("got %d %+v", status, out)
	}
	if !strings.Contains(strings.Join(out.Log, "\n"), "LOOPING FOREVER") {
		t.Fatalf("the response should carry the program's output: %+v", out.Log)
	}
}

func TestServeWithAPortIsRefused(t *testing.T) {
	h := newTestHost(t, nil)
	status, out := h.create(fakeCode("port"))
	if status != http.StatusUnprocessableEntity || !strings.Contains(out.Error, "SERVE without a port") {
		t.Fatalf("got %d %+v", status, out)
	}
}

func TestProgramsSurviveAHostRestart(t *testing.T) {
	dir := t.TempDir()
	pg := New(Config{DataDir: dir, Limits: DefaultLimits()})
	pg.build = fakeBuild
	web := httptest.NewServer(pg.Handler())
	h := &testHost{t: t, pg: pg, web: web}
	out := h.mustCreate(helloCode)
	web.Close()
	pg.Close()

	pg2 := New(Config{DataDir: dir, Limits: DefaultLimits()})
	web2 := httptest.NewServer(pg2.Handler())
	defer func() {
		web2.Close()
		pg2.Close()
	}()
	h2 := &testHost{t: t, pg: pg2, web: web2}
	if status, _, body := h2.get(out.URL + "hello"); status != http.StatusOK {
		t.Fatalf("after host restart: %d %s", status, body)
	}
}

func TestLogRing(t *testing.T) {
	r := newLogRing()
	for i := 1; i <= logCapacity+5; i++ {
		r.add("stdout", fmt.Sprint(i))
	}
	all := r.snapshot()
	if len(all) != logCapacity || all[0].Text != "6" || all[len(all)-1].Text != fmt.Sprint(logCapacity+5) {
		t.Fatalf("ring kept %d lines, first %q", len(all), all[0].Text)
	}
	replay, ch, cancel := r.subscribe(all[len(all)-2].Seq)
	if len(replay) != 1 {
		t.Fatalf("replay after a seq: %d lines", len(replay))
	}
	r.add("host", "LIVE")
	if l := <-ch; l.Text != "LIVE" {
		t.Fatalf("live line %q", l.Text)
	}
	cancel()
	r.close()

	w := &lineWriter{stream: "stdout", ring: newLogRing()}
	fmt.Fprint(w, "par")
	fmt.Fprint(w, "tial\r\nsecond\nthi")
	w.flush()
	var got []string
	for _, l := range w.ring.snapshot() {
		got = append(got, l.Text)
	}
	if strings.Join(got, "|") != "partial|second|thi" {
		t.Fatalf("lines %q", got)
	}
}

func TestStripPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"/p/abc/":         "/",
		"/p/abc/hello":    "/hello",
		"/p/abc/a/b?x=1":  "/a/b?x=1",
		"/p/abc":          "/",
		"/p/abc/%2Fslash": "/%2Fslash",
	} {
		if got := stripPrefix(in, "/p/abc"); got != want {
			t.Errorf("stripPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEndToEnd builds real programs with the Go toolchain: the Hello World
// sample, and the GOTO Hell sample, whose stuck request the watchdog answers.
func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real programs; skipped with -short")
	}
	limits := DefaultLimits()
	limits.RequestTimeout = time.Second
	pg := New(Config{DataDir: t.TempDir(), Limits: limits})
	web := httptest.NewServer(pg.Handler())
	t.Cleanup(func() {
		web.Close()
		pg.Close()
	})
	h := &testHost{t: t, pg: pg, web: web}

	out := h.mustCreate(`ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  PRINT "SOMEONE KNOCKED"
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
`)
	status, _, body := h.get(out.URL + "hello")
	if status != http.StatusOK || !strings.Contains(body, "HELLO WORLD") {
		t.Fatalf("hello: %d %s", status, body)
	}
	h.waitForLog(out.ID, "SOMEONE KNOCKED")

	hell := h.mustCreate(`10 REM GOTO HELL — THE LOOP GUARD DEMO
20 ROUTE GET "/hell" GOSUB 100
30 SERVE
100 PRINT "ABANDON ALL HOPE"
110 GOTO 110
`)
	status, _, body = h.get(hell.URL + "hell")
	if status != http.StatusLoopDetected || !strings.Contains(body, "TRAPPED IN AN ETERNAL GOTO LOOP") {
		t.Fatalf("hell: %d %s", status, body)
	}
}
