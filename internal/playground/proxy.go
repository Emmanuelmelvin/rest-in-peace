package playground

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
)

// watchdogGrace is how much longer than RequestTimeout the host waits before
// it decides a program has stopped responding (design §5).
const watchdogGrace = time.Second

// trappedBody is the 508 answer for a request the watchdog had to kill.
var trappedBody = map[string]string{"error": "TRAPPED IN AN ETERNAL GOTO LOOP"}

// proxy forwards /p/<id>/… to the program's process, with the /p/<id>
// prefix stripped. A request that outlives RequestTimeout + 1 s means the
// program is stuck: it is killed and restarted, and the request gets 508.
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sb := s.lookup(id)
	if sb == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "THIS PROGRAM HAS PASSED ON"})
		return
	}
	sb.touch()
	if sb.currentState() != stateRunning {
		s.makeRoom(sb)
	}
	if err := sb.ensureRunning(); err != nil {
		if errors.Is(err, errStartLoop) {
			writeJSON(w, http.StatusLoopDetected, trappedBody)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	port, cmd, ok := sb.target()
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "THIS PROGRAM IS NOT RESPONDING"})
		return
	}

	start := time.Now()
	prefix := "/p/" + id
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Limits.RequestTimeout+watchdogGrace)
	defer cancel()
	stuck := func() bool {
		return errors.Is(ctx.Err(), context.DeadlineExceeded) && r.Context().Err() == nil
	}
	trapped := false
	rec := &statusRecorder{ResponseWriter: w}
	rp := &httputil.ReverseProxy{
		Transport: s.proxyTransport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			pr.Out.URL.Path = stripPrefix(pr.In.URL.Path, prefix)
			if pr.In.URL.RawPath != "" {
				pr.Out.URL.RawPath = stripPrefix(pr.In.URL.RawPath, prefix)
			}
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Prefix", prefix)
		},
		ModifyResponse: func(resp *http.Response) error {
			// The program shares the playground's origin; a sandboxed CSP
			// keeps any HTML it serves away from that origin.
			resp.Header.Set("Content-Security-Policy", "sandbox")
			resp.Header.Set("X-Content-Type-Options", "nosniff")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if stuck() {
				trapped = true
				writeJSON(w, http.StatusLoopDetected, trappedBody)
				return
			}
			if r.Context().Err() != nil {
				return // the client went away
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "THIS PROGRAM IS NOT RESPONDING"})
		},
	}
	rp.ServeHTTP(rec, r.WithContext(ctx))

	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	sb.logs.add("request", fmt.Sprintf("%s %s → %d (%d MS)", r.Method, stripPrefix(r.URL.RequestURI(), prefix),
		status, time.Since(start).Milliseconds()))
	if trapped {
		// The 508 is written; kill and restart before this handler returns, so the
		// next request finds the new process.
		sb.restartIf(cmd, fmt.Sprintf("?WATCHDOG — %s %s ran longer than %s, so the program was killed and restarted",
			r.Method, stripPrefix(r.URL.Path, prefix), s.cfg.Limits.RequestTimeout+watchdogGrace))
	}
}

func stripPrefix(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest[0] != '/' {
		rest = "/" + rest
	}
	return rest
}

// statusRecorder remembers the status code for the request log line.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
