// Package playground is the hosted playground: it builds BASIC programs, runs
// each one as its own OS process, streams their logs over SSE, and proxies
// /p/<id>/ to them (Requirement 15, design §5).
package playground

import (
	"encoding/json"
	"net/http"
	"time"
)

// Limits are the per-program caps (Requirement 15.4).
type Limits struct {
	MemoryMB       int
	RequestTimeout time.Duration
	MaxPrograms    int
	Expiry         time.Duration
}

// DefaultLimits returns the limits from the spec.
func DefaultLimits() Limits {
	return Limits{MemoryMB: 128, RequestTimeout: 5 * time.Second, MaxPrograms: 20, Expiry: 7 * 24 * time.Hour}
}

// Config configures the host.
type Config struct {
	WebDir  string // built front end (playground/web/dist); empty serves the API only
	DataDir string // program sources, build directories and SQLite files
	Limits  Limits
}

// Server is the playground host.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New creates the host.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("POST /api/programs", s.createProgram)
	s.mux.HandleFunc("GET /api/programs/{id}/logs", s.streamLogs)
	s.mux.HandleFunc("/p/{id}/", s.proxy)
	if cfg.WebDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))
	}
	return s
}

// Handler returns the host's HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ALIVE"})
}

// createProgram compiles and deploys a program.
// Request: {"code": "…"}. Response: {"id": "…", "url": "/p/<id>/"}.
func (s *Server) createProgram(w http.ResponseWriter, r *http.Request) {
	// TODO(task 5.2): cap the body size; compile with Guard on; reject IMPORT FROM GO
	// packages that aren't Allowed (and FETCH targets via the SSRF guard); build
	// with internal/build; start a Sandbox; and evict the least recently used
	// program when MaxPrograms is reached.
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "RUN IS NOT BUILT YET (TASK 5.2)"})
}

// streamLogs streams a program's PRINT/LOG output and request lines over SSE.
func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	// TODO(task 5.2): text/event-stream, replaying recent lines first.
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "LOGS ARE NOT BUILT YET (TASK 5.2)"})
}

// proxy forwards /p/<id>/… to the program's process.
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	// TODO(task 5.2): httputil.ReverseProxy to the sandbox's port, with the /p/<id> prefix stripped.
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "THIS PROGRAM HAS PASSED ON"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
