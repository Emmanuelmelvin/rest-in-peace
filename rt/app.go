package rt

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Handler is a compiled structured-mode handler or middleware: FUNCTION Name(req).
type Handler func(c *Ctx, req Value) Value

// ClassicEntry runs a classic program starting at a GOSUB line (design §4.7).
type ClassicEntry func(c *Ctx, line int)

// App is the HTTP application that a program's top-level statements build.
type App struct {
	entry string
	mux   *http.ServeMux
	port  string
	serve bool
}

// NewApp creates the app for a program whose entry file is entry (e.g. "main.bas").
func NewApp(entry string) *App {
	return &App{entry: entry, mux: http.NewServeMux()}
}

// Route implements ROUTE <METHOD> "<path>" CALL fn (Requirement 7.1).
func (a *App) Route(method, path string, h Handler) {
	// TODO(task 2.1): translate :param → {param} and a trailing * → {rest...}, then
	// wrap h with request parsing (body cap, invalid JSON → 400), middleware,
	// RESPOND/RETURN handling and error → response mapping.
	panic(notBuilt("ROUTE … CALL", "2.1"))
}

// RouteGosub implements ROUTE <METHOD> "<path>" GOSUB <line> (classic mode).
func (a *App) RouteGosub(method, path string, line int, run ClassicEntry) {
	// TODO(task 2.1, 2.6): per-request GOSUB stack; classic handlers serialized per program.
	panic(notBuilt("ROUTE … GOSUB", "2.1"))
}

// Use implements USE fn [ON "/scope/*"] (Requirement 8.1).
func (a *App) Use(mw Handler, scope string) {
	// TODO(task 2.3): ordered middleware chain with path scoping.
	panic(notBuilt("USE", "2.3"))
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

// Handler returns the app as an http.Handler. Tests use this so that no port is bound.
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := a.mux.Handler(r); pattern == "" {
			writeJSON(w, http.StatusNotFound, MapOf("error", "ROUTE NOT FOUND IN THIS REALM"))
			return
		}
		a.mux.ServeHTTP(w, r)
	})
}

// Run starts the server if the program called SERVE.
func (a *App) Run() error {
	if !a.serve {
		return nil
	}
	fmt.Printf("READY. listening on :%s\n", a.port)
	// TODO(task 2.9): graceful shutdown on SIGTERM (10 s), close DB pools, /_health.
	return http.ListenAndServe(":"+a.port, a.Handler())
}

func writeJSON(w http.ResponseWriter, status int, v Value) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
