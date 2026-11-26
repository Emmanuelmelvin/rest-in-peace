package rt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Handler is a compiled structured-mode handler or middleware: FUNCTION Name(req).
type Handler func(c *Ctx, req Value) Value

// ClassicEntry runs a classic program starting at a GOSUB line (design §4.7).
type ClassicEntry func(c *Ctx, line int)

// DefaultMaxBody is the request body cap (Requirement 7.9).
const DefaultMaxBody = 1 << 20

// MaxBodyEnv names the environment variable that changes the body cap, in bytes.
const MaxBodyEnv = "RIP_MAX_BODY_BYTES"

// kindRoute reports a bad ROUTE found at startup that the compiler could not
// see, such as two routes that overlap.
const kindRoute Kind = "SYNTAX"

// ShutdownTimeout is how long in-flight requests get to finish on SIGTERM (Requirement 14.5).
const ShutdownTimeout = 10 * time.Second

// middlewareEntry is one registered USE middleware with its optional path scope.
type middlewareEntry struct {
	fn    Handler
	scope string
}

// App is the HTTP application that a program's top-level statements build.
type App struct {
	entry string
	mux   *http.ServeMux
	port  string
	serve bool

	// MaxBody caps request bodies, in bytes. NewApp reads it from
	// RIP_MAX_BODY_BYTES, defaulting to 1 MB.
	MaxBody int64

	middleware []middlewareEntry // ordered USE middleware (Requirement 8.1)

	classicMu sync.Mutex // classic handlers run one at a time (Requirement 7.10)
	started   bool       // set once Handler has been built; ROUTE/USE after that is an error

	shutdownMu sync.Mutex
	onShutdown []func()
}

// NewApp creates the app for a program whose entry file is entry (e.g. "main.bas").
func NewApp(entry string) *App {
	a := &App{entry: entry, mux: http.NewServeMux(), MaxBody: DefaultMaxBody}
	if v, ok := os.LookupEnv(MaxBodyEnv); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
			a.MaxBody = n
		}
	}
	return a
}

// Route implements ROUTE <METHOD> "<path>" CALL fn (Requirement 7.1).
func (a *App) Route(method, path string, h Handler) {
	a.handle(method, path, func(c *Ctx) Value {
		return h(c, c.Req.Value())
	})
}

// RouteGosub implements ROUTE <METHOD> "<path>" GOSUB <line> (classic mode).
// Each request has its own Ctx, so its own GOSUB stack, and classic handlers
// run one at a time per program because classic variables are global
// (Requirement 7.10).
func (a *App) RouteGosub(method, path string, line int, run ClassicEntry) {
	a.handle(method, path, func(c *Ctx) Value {
		a.classicMu.Lock()
		defer a.classicMu.Unlock()
		run(c, line)
		return nil
	})
}

// Use implements USE fn [ON "/scope/*"] (Requirement 8.1). Middleware runs
// in registration order before every matching route handler.
func (a *App) Use(mw Handler, scope string) {
	if a.started {
		panic(Errorf(KindInternal, "USE ran after the server started; USE goes in top-level code, before SERVE"))
	}
	if mw == nil {
		panic(Errorf(KindInternal, "USE requires a middleware handler"))
	}
	if scope != "" {
		if err := validateScope(scope); err != nil {
			panic(Errorf(kindRoute, "%v", err))
		}
	}
	a.middleware = append(a.middleware, middlewareEntry{fn: mw, scope: scope})
}

func validateScope(scope string) error {
	if !strings.HasPrefix(scope, "/") {
		return fmt.Errorf(`a USE scope starts with /, like "/admin/*"; got %q`, scope)
	}
	if strings.ContainsAny(scope, "{}") {
		return fmt.Errorf(`a USE scope cannot contain { or }; got %q`, scope)
	}
	if strings.Contains(scope, "//") {
		return fmt.Errorf(`the USE scope %q has an empty part (//)`, scope)
	}
	trimmed := strings.TrimSuffix(scope, "/*")
	if strings.Contains(trimmed, "*") {
		return fmt.Errorf(`* only works at the end of a USE scope, like "/admin/*"`)
	}
	return nil
}

// scopeMatches reports whether a USE scope applies to path. An empty scope
// matches every path; "/admin/*" matches "/admin" and anything under "/admin/";
// any other scope matches that exact path.
func scopeMatches(scope, path string) bool {
	if scope == "" || scope == "/*" {
		return true
	}
	if strings.HasSuffix(scope, "/*") {
		prefix := strings.TrimSuffix(scope, "/*")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return path == scope
}

func (a *App) matchingMiddleware(path string) []Handler {
	if len(a.middleware) == 0 {
		return nil
	}
	var out []Handler
	for _, m := range a.middleware {
		if scopeMatches(m.scope, path) {
			out = append(out, m.fn)
		}
	}
	return out
}

// Serve implements SERVE [port]. It only records the port: Run starts
// listening after all top-level statements have run (design §4.8).
func (a *App) Serve(port Value) {
	a.serve = true
	switch {
	case port != nil:
		a.port = ToString(port)
	case Env("PORT") != nil:
		a.port = ToString(Env("PORT"))
	default:
		a.port = "8080"
	}
}

// OnShutdown registers f to run after the server has stopped and every
// in-flight request has finished (Requirement 14.5).
//
// TODO(task 3.1): DBCONNECT registers each pool's Close here, so that a
// graceful shutdown closes the database pools.
func (a *App) OnShutdown(f func()) {
	a.shutdownMu.Lock()
	a.onShutdown = append(a.onShutdown, f)
	a.shutdownMu.Unlock()
}

// Handler returns the app as an http.Handler. Tests use this so that no port is bound.
func (a *App) Handler() http.Handler {
	a.started = true
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := a.mux.Handler(r); pattern == "" {
			if r.URL.Path == "/_health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				writeJSON(w, r, http.StatusOK, MapOf("status", "ALIVE"))
				return
			}
			// Let matching middleware handle OPTIONS preflight (e.g. CORS).
			if r.Method == http.MethodOptions && len(a.matchingMiddleware(r.URL.Path)) > 0 {
				a.serveRequest(w, r, func(c *Ctx) Value {
					RespondNow(c, http.StatusNotFound, MapOf("error", "ROUTE NOT FOUND IN THIS REALM"))
					return nil
				})
				return
			}
			writeJSON(w, r, http.StatusNotFound, MapOf("error", "ROUTE NOT FOUND IN THIS REALM"))
			return
		}
		a.mux.ServeHTTP(w, r)
	})
}

// Run starts the server if the program called SERVE, and blocks until
// SIGTERM or an interrupt shuts it down gracefully.
func (a *App) Run() error {
	if !a.serve {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ln, err := net.Listen("tcp", ":"+a.port)
	if err != nil {
		return Errorf(KindInternal, "cannot listen on port %s (%v) — is another program using it? Set PORT or pass a port to SERVE", a.port, err)
	}
	fmt.Printf("READY. listening on :%d\n", ln.Addr().(*net.TCPAddr).Port)
	return a.serveOn(ctx, ln)
}

// serveOn serves on ln until ctx is done, then stops accepting connections,
// gives in-flight requests up to ShutdownTimeout to finish, and runs the
// OnShutdown hooks.
func (a *App) serveOn(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	var err error
	select {
	case err = <-errc:
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		err = srv.Shutdown(sctx)
		cancel()
	}
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.DeadlineExceeded) {
		err = nil // a request that outlived the timeout is cut off, as Requirement 14.5 allows
	}
	a.shutdownMu.Lock()
	hooks := a.onShutdown
	a.shutdownMu.Unlock()
	for _, f := range hooks {
		f()
	}
	return err
}

// handle registers one route. run executes the handler for a request whose
// Ctx is ready, and returns the value for a 200 response.
func (a *App) handle(method, path string, run func(c *Ctx) Value) {
	if a.started {
		panic(Errorf(KindInternal, "ROUTE %s %s ran after the server started; routes go in top-level code, before SERVE", method, path))
	}
	pattern, err := RoutePattern(path)
	if err != nil {
		panic(Errorf(kindRoute, "%v", err))
	}
	method = strings.ToUpper(method)
	if err := a.register(method+" "+pattern, func(w http.ResponseWriter, r *http.Request) {
		a.serveRequest(w, r, run)
	}); err != nil {
		panic(Errorf(kindRoute, "ROUTE %s %q clashes with another route: %v", method, path, err))
	}
}

// register adds a pattern to the mux, turning the mux's panic on a clash into an error.
func (a *App) register(pattern string, h http.HandlerFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	a.mux.HandleFunc(pattern, h)
	return nil
}

// RoutePattern turns a ROUTE path into a net/http pattern: :param becomes
// {param}, a trailing * becomes {rest...}, and a trailing / matches only
// itself ({$}).
func RoutePattern(path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("a route path starts with /, like \"/hello\"; got %q", path)
	}
	segs := strings.Split(path[1:], "/")
	seen := map[string]bool{}
	var b strings.Builder
	for i, s := range segs {
		b.WriteByte('/')
		last := i == len(segs)-1
		switch {
		case s == "" && last:
			b.WriteString("{$}")
		case s == "*" && last:
			b.WriteString("{rest...}")
		case strings.HasPrefix(s, ":"):
			name := s[1:]
			if !isParamName(name) {
				return "", fmt.Errorf("%q is not a valid path parameter; use letters, digits and _, like :id", s)
			}
			if seen[name] || (name == "rest" && segs[len(segs)-1] == "*") {
				return "", fmt.Errorf("the path parameter :%s appears twice in %q", name, path)
			}
			seen[name] = true
			b.WriteString("{" + name + "}")
		case strings.Contains(s, "*"):
			return "", fmt.Errorf("* only works as the last part of a route path, like \"/files/*\"")
		case strings.ContainsAny(s, "{}"):
			return "", fmt.Errorf("a route path cannot contain { or }; write path parameters as :name")
		case s == "" && !last:
			return "", fmt.Errorf("the route path %q has an empty part (//)", path)
		default:
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

func isParamName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// serveRequest parses the request, runs matching USE middleware in order,
// runs the handler on a fresh Ctx, writes the response, and triggers OnFinish.
func (a *App) serveRequest(w http.ResponseWriter, r *http.Request, run func(c *Ctx) Value) {
	req, status, body := a.parseRequest(w, r)
	if req == nil {
		writeJSON(w, r, status, body)
		return
	}
	c := NewCtx(r.Context())
	c.app = a
	c.Req = req
	mws := a.matchingMiddleware(r.URL.Path)
	exec := run
	if len(mws) > 0 {
		reqVal := req.Value()
		exec = func(c *Ctx) Value {
			for _, mw := range mws {
				mw(c, reqVal)
			}
			return run(c)
		}
	}
	status, body, headers := callHandler(c, exec)
	status = writeResponse(w, r, c, status, body, headers)
	c.runOnFinish(status)
}

// callHandler runs a handler and turns how it ended into a response:
// RETURN v (or falling off the end) → 200 v, RESPOND → that response, END →
// 200 NIL, and any error → ErrorResponse (Requirements 6.4, 6.5, 7.4, 7.5).
func callHandler(c *Ctx, run func(c *Ctx) Value) (status int, body Value, headers *Map) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch x := r.(type) {
		case *Respond:
			status, body, headers = x.Status, x.Body, x.Headers
			return
		case endSignal:
			status, body, headers = http.StatusOK, nil, nil
			return
		}
		e := FromPanic(r)
		if e.File == "" {
			e.File, e.Line = basLocation()
		}
		status, body = ErrorResponse(c, e)
		headers = nil
	}()
	return http.StatusOK, run(c), nil
}

// parseRequest builds the Request. If the body is too large or is not JSON,
// it returns a nil Request and the error response instead.
func (a *App) parseRequest(w http.ResponseWriter, r *http.Request) (*Request, int, Value) {
	req := &Request{
		ID:         newRequestID(),
		Method:     r.Method,
		Path:       r.URL.Path,
		RemoteAddr: r.RemoteAddr,
		Params:     NewMap(),
		Query:      parseQuery(r.URL.RawQuery),
		Headers:    NewMap(),
		Body:       NewMap(),
		Ctx:        NewMap(),
	}
	for _, name := range patternParams(r.Pattern) {
		req.Params.Set(name, r.PathValue(name))
	}
	for _, k := range sortedHeaderKeys(r.Header) {
		req.Headers.Set(strings.ToLower(k), strings.Join(r.Header[k], ", "))
	}

	limit := a.MaxBody
	if limit <= 0 {
		limit = DefaultMaxBody
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, http.StatusRequestEntityTooLarge, MapOf("error", "BODY IS TOO LARGE")
		}
		return nil, http.StatusBadRequest, MapOf("error", "BODY IS NOT VALID JSON")
	}
	if len(strings.TrimSpace(string(data))) > 0 {
		v, err := decodeJSON(data)
		if err != nil {
			return nil, http.StatusBadRequest, MapOf("error", "BODY IS NOT VALID JSON")
		}
		req.Body = v
	}
	return req, 0, nil
}

// patternParams lists the wildcard names in a net/http pattern, in order.
func patternParams(pattern string) []string {
	var names []string
	for {
		i := strings.IndexByte(pattern, '{')
		if i < 0 {
			return names
		}
		j := strings.IndexByte(pattern[i:], '}')
		if j < 0 {
			return names
		}
		name := strings.TrimSuffix(pattern[i+1:i+j], "...")
		if name != "$" {
			names = append(names, name)
		}
		pattern = pattern[i+j+1:]
	}
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// writeResponse writes a handler's response and returns the final HTTP status.
// Headers set via c.SetResponseHeader are applied first; explicit RESPOND
// headers override them. The body is JSON, except that a STRING body is
// written as-is when the headers set Content-Type.
func writeResponse(w http.ResponseWriter, r *http.Request, c *Ctx, status int, body Value, headers *Map) int {
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
		body = MapOf("error", "INTERNAL ERROR")
		headers = nil
	}
	if c != nil && len(c.respOrder) > 0 {
		for _, k := range c.respOrder {
			w.Header().Set(k, c.respHeaders[k])
		}
	}
	if headers != nil {
		for _, k := range headers.Keys() {
			v, _ := headers.Get(k)
			w.Header().Set(k, ToString(v))
		}
		if s, ok := body.(string); ok && w.Header().Get("Content-Type") != "" {
			w.WriteHeader(status)
			if bodyAllowed(r, status) {
				_, _ = io.WriteString(w, s)
			}
			return status
		}
	}
	return writeJSON(w, r, status, body)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v Value) int {
	data, err := marshalValue(v)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":"INTERNAL ERROR"}`)
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(status)
	if bodyAllowed(r, status) {
		_, _ = w.Write(append(data, '\n'))
	}
	return status
}

func bodyAllowed(r *http.Request, status int) bool {
	return r.Method != http.MethodHead && status != http.StatusNoContent && status != http.StatusNotModified && status >= 200
}

// Status converts RESPOND's status for RespondNow, checking it is an HTTP status.
func Status(v Value) int {
	n, ok := toInt(v)
	if !ok || n < 100 || n > 599 {
		panic(Errorf(KindTypeMismatch, "RESPOND needs an HTTP status from 100 to 599, got %s %s", TypeName(v), ToString(v)))
	}
	return int(n)
}
