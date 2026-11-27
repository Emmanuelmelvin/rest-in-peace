// Package playground is the hosted playground: it builds BASIC programs, runs
// each one as its own OS process, streams their logs over SSE, and proxies
// /p/<id>/ to them (Requirement 15, design §5).
package playground

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// Limits are the per-program caps (Requirement 15.4).
type Limits struct {
	MemoryMB       int
	RequestTimeout time.Duration
	MaxPrograms    int           // processes running at once; the least recently used is stopped to make room
	Expiry         time.Duration // programs are deleted this long after their deploy
	CPUSeconds     int           // RLIMIT_CPU for each process (Linux); 0 = no cap
	MaxStored      int           // programs kept on disk; the oldest is deleted to make room
}

// DefaultLimits returns the limits from the spec.
func DefaultLimits() Limits {
	return Limits{
		MemoryMB: 128, RequestTimeout: 5 * time.Second, MaxPrograms: 20, Expiry: 7 * 24 * time.Hour,
		CPUSeconds: 300, MaxStored: 200,
	}
}

// Config configures the host.
type Config struct {
	WebDir  string // built front end (playground/web/dist); empty serves the API only
	DataDir string // program sources, binaries and SQLite files
	Limits  Limits
}

const (
	maxCodeBytes   = 64 << 10 // a playground program is one file
	buildTimeout   = 3 * time.Minute
	maxBuilds      = 2 // concurrent go builds; more requests wait
	maxErrorBytes  = 4 << 10
	sweepInterval  = time.Minute
	heartbeatEvery = 15 * time.Second
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Server is the playground host.
type Server struct {
	cfg Config
	mux *http.ServeMux

	mu       sync.Mutex
	programs map[string]*Sandbox

	builds chan struct{} // semaphore of maxBuilds
	// build turns generated files into a binary in dir; tests replace it.
	build func(ctx context.Context, dir string, files []codegen.File) (string, error)

	proxyTransport *http.Transport
	done           chan struct{}
	closeOnce      sync.Once
}

// New creates the host. Programs left in DataDir by an earlier run are
// picked up again and start on their next request.
func New(cfg Config) *Server {
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(os.TempDir(), "rip-playground")
	}
	s := &Server{
		cfg:            cfg,
		mux:            http.NewServeMux(),
		programs:       map[string]*Sandbox{},
		builds:         make(chan struct{}, maxBuilds),
		build:          goBuild,
		proxyTransport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second},
		done:           make(chan struct{}),
	}
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("POST /api/programs", s.createProgram)
	s.mux.HandleFunc("GET /api/programs/{id}/logs", s.streamLogs)
	s.mux.HandleFunc("/p/{id}/", s.proxy)
	if cfg.WebDir != "" {
		s.mux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))
	}
	s.loadPrograms()
	go s.janitor()
	return s
}

// Handler returns the host's HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

// Close stops every program process. Their files stay for the next run.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.mu.Lock()
		all := make([]*Sandbox, 0, len(s.programs))
		for _, sb := range s.programs {
			all = append(all, sb)
		}
		s.mu.Unlock()
		for _, sb := range all {
			sb.Stop()
		}
		s.proxyTransport.CloseIdleConnections()
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ALIVE"})
}

// createResponse is the body of POST /api/programs.
type createResponse struct {
	ID          string   `json:"id,omitempty"`
	URL         string   `json:"url,omitempty"`
	Status      string   `json:"status,omitempty"` // RUNNING, or ENDED for a program without SERVE
	Error       string   `json:"error,omitempty"`
	Diagnostics []string `json:"diagnostics,omitempty"` // classic ?KIND ERROR/WARNING lines
	Log         []string `json:"log,omitempty"`         // the program's output, when it failed to start
}

// createProgram compiles and deploys a program.
// Request: {"code": "…"}. Response: {"id": "…", "url": "/p/<id>/", "status": "RUNNING"}.
func (s *Server) createProgram(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCodeBytes+4096) // room for the JSON around it
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, createResponse{Error: "PROGRAM IS TOO LARGE (64 KB MAX)"})
			return
		}
		writeJSON(w, http.StatusBadRequest, createResponse{Error: `BODY IS NOT VALID JSON — send {"code": "…"}`})
		return
	}
	if len(req.Code) > maxCodeBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, createResponse{Error: "PROGRAM IS TOO LARGE (64 KB MAX)"})
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, createResponse{Error: "THERE IS NO PROGRAM TO RUN"})
		return
	}

	id := newID()
	dir := s.programDir(id)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		s.internalError(w, err)
		return
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()

	files, ds, err := compileProgram(dir, []byte(req.Code))
	if err != nil {
		s.internalError(w, err)
		return
	}
	if files == nil {
		writeJSON(w, http.StatusBadRequest, createResponse{Error: firstError(ds), Diagnostics: diagStrings(ds)})
		return
	}
	warnings := diagStrings(ds)

	select {
	case s.builds <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), buildTimeout)
	bin, err := s.build(ctx, dir, files)
	cancel()
	<-s.builds
	if err != nil {
		if r.Context().Err() != nil {
			return // the browser went away
		}
		msg := truncate(err.Error(), maxErrorBytes)
		log.Printf("playground: build %s failed: %s", id, msg)
		writeJSON(w, http.StatusInternalServerError, createResponse{Error: firstLine(msg), Diagnostics: append(warnings, msg)})
		return
	}

	created := time.Now().UTC()
	rel, err := filepath.Rel(dir, bin)
	if err != nil || strings.HasPrefix(rel, "..") {
		s.internalError(w, fmt.Errorf("binary %s is outside %s", bin, dir))
		return
	}
	if err := writeMeta(dir, meta{ID: id, Bin: filepath.ToSlash(rel), Created: created}); err != nil {
		s.internalError(w, err)
		return
	}
	sb := newSandbox(id, bin, dataDir, created, s.cfg.Limits)
	s.makeStoreRoom()
	s.mu.Lock()
	s.programs[id] = sb
	s.mu.Unlock()
	keep = true

	s.makeRoom(sb)
	resp := createResponse{ID: id, URL: "/p/" + id + "/", Diagnostics: warnings}
	if err := sb.start(); err != nil {
		resp.Error = err.Error()
		resp.Log = logText(sb.logs.snapshot())
		status := http.StatusUnprocessableEntity
		if errors.Is(err, errStartLoop) {
			status = http.StatusLoopDetected // 508
		}
		writeJSON(w, status, resp)
		return
	}
	resp.Status = "RUNNING"
	if sb.currentState() != stateRunning {
		resp.Status = "ENDED"
	}
	writeJSON(w, http.StatusCreated, resp)
}

// streamLogs streams a program's PRINT/LOG output and request lines over SSE.
// Recent lines are replayed first; Last-Event-ID resumes after a reconnect.
func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	sb := s.lookup(r.PathValue("id"))
	if sb == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "THIS PROGRAM HAS PASSED ON"})
		return
	}
	sb.touch()
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	replay, lines, cancel := sb.logs.subscribe(after)
	defer cancel()

	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	for _, l := range replay {
		writeEvent(w, l)
	}
	if rc.Flush() != nil {
		return
	}

	ping := time.NewTicker(heartbeatEvery)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case l, ok := <-lines:
			if !ok {
				return // fell behind or expired; EventSource reconnects with Last-Event-ID
			}
			writeEvent(w, l)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}

func writeEvent(w http.ResponseWriter, l LogLine) {
	data, _ := json.Marshal(l)
	fmt.Fprintf(w, "id: %d\ndata: %s\n\n", l.Seq, data)
}

func (s *Server) lookup(id string) *Sandbox {
	if !idPattern.MatchString(id) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.programs[id]
}

func (s *Server) programDir(id string) string {
	return filepath.Join(s.cfg.DataDir, "programs", id)
}

// makeRoom stops the least recently used running programs until starting
// keep stays within MaxPrograms. A stopped program starts on its next request.
func (s *Server) makeRoom(keep *Sandbox) {
	max := s.cfg.Limits.MaxPrograms
	if max <= 0 {
		return
	}
	for {
		s.mu.Lock()
		var running []*Sandbox
		for _, sb := range s.programs {
			if sb != keep && sb.currentState() == stateRunning {
				running = append(running, sb)
			}
		}
		s.mu.Unlock()
		if len(running) < max {
			return
		}
		lru := running[0]
		for _, sb := range running[1:] {
			if sb.lastUse().Before(lru.lastUse()) {
				lru = sb
			}
		}
		lru.Stop()
		lru.logs.add("host", "STOPPED TO MAKE ROOM FOR OTHER PROGRAMS — IT WAKES UP ON ITS NEXT REQUEST")
	}
}

// makeStoreRoom deletes the oldest programs so a new one fits in MaxStored.
func (s *Server) makeStoreRoom() {
	max := s.cfg.Limits.MaxStored
	if max <= 0 {
		return
	}
	s.mu.Lock()
	all := make([]*Sandbox, 0, len(s.programs))
	for _, sb := range s.programs {
		all = append(all, sb)
	}
	s.mu.Unlock()
	if len(all) < max {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Created.Before(all[j].Created) })
	for _, sb := range all[:len(all)-max+1] {
		s.deleteProgram(sb)
	}
}

func (s *Server) deleteProgram(sb *Sandbox) {
	s.mu.Lock()
	delete(s.programs, sb.ID)
	s.mu.Unlock()
	sb.remove()
	if err := os.RemoveAll(s.programDir(sb.ID)); err != nil {
		log.Printf("playground: removing %s: %v", sb.ID, err)
	}
}

// sweep deletes programs older than Expiry (Requirement 15.8).
func (s *Server) sweep(now time.Time) {
	if s.cfg.Limits.Expiry <= 0 {
		return
	}
	s.mu.Lock()
	var expired []*Sandbox
	for _, sb := range s.programs {
		if now.Sub(sb.Created) > s.cfg.Limits.Expiry {
			expired = append(expired, sb)
		}
	}
	s.mu.Unlock()
	for _, sb := range expired {
		s.deleteProgram(sb)
	}
}

func (s *Server) janitor() {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case now := <-t.C:
			s.sweep(now)
		}
	}
}

// meta is what the host remembers about a program across restarts.
type meta struct {
	ID      string    `json:"id"`
	Bin     string    `json:"bin"` // relative to the program's directory
	Created time.Time `json:"created"`
}

func writeMeta(dir string, m meta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
}

// loadPrograms registers the programs an earlier run of the host left in
// DataDir, and deletes broken or expired ones.
func (s *Server) loadPrograms() {
	root := filepath.Join(s.cfg.DataDir, "programs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var m meta
		data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		if err == nil {
			err = json.Unmarshal(data, &m)
		}
		bin := filepath.Join(dir, filepath.FromSlash(m.Bin))
		if err == nil && (m.ID != e.Name() || m.Bin == "" || strings.Contains(m.Bin, "..") || filepath.IsAbs(m.Bin)) {
			err = errors.New("bad meta.json")
		}
		if err == nil {
			_, err = os.Stat(bin)
		}
		expired := s.cfg.Limits.Expiry > 0 && time.Since(m.Created) > s.cfg.Limits.Expiry
		if err != nil || expired {
			os.RemoveAll(dir)
			continue
		}
		sb := newSandbox(m.ID, bin, filepath.Join(dir, "data"), m.Created, s.cfg.Limits)
		sb.logs.add("host", "THE HOST RESTARTED — THIS PROGRAM WAKES UP ON ITS NEXT REQUEST")
		s.programs[m.ID] = sb
	}
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("playground: %v", err)
	writeJSON(w, http.StatusInternalServerError, createResponse{Error: "INTERNAL ERROR"})
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return hex.EncodeToString(b)
}

func diagStrings(ds diag.List) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.String())
	}
	return out
}

func firstError(ds diag.List) string {
	for _, d := range ds {
		if d.Severity == diag.Error {
			return d.String()
		}
	}
	return "?SYNTAX ERROR"
}

func logText(lines []LogLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
