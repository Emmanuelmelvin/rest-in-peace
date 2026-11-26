package golden

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotReady is returned by StartServer when the program exits, or runs past
// its timeout, without printing its READY. line.
var ErrNotReady = errors.New("server did not print READY.")

// Server is a running program started by StartServer.
type Server struct {
	URL    string // http://127.0.0.1:<port>
	Port   int
	cmd    *exec.Cmd
	stdout *readyWriter
	stderr *lockedBuffer
	done   chan struct{} // closed when the process has exited
	err    error         // from cmd.Wait, set before done is closed
	once   sync.Once
}

// StartServer runs bin in dir with PORT set to a free port, and waits until
// it prints a line starting with "READY." on stdout. If the port turns out to
// be taken (another program grabbed it first), it tries again with a new one.
// The caller must call Stop.
func StartServer(bin, dir string, readyTimeout time.Duration) (*Server, error) {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var s *Server
		s, err = startOnce(bin, dir, readyTimeout)
		if err == nil {
			return s, nil
		}
		if !errors.Is(err, errPortTaken) {
			return nil, err
		}
	}
	return nil, err
}

var errPortTaken = errors.New("port is taken")

func startOnce(bin, dir string, readyTimeout time.Duration) (*Server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	s := &Server{
		URL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		Port:   port,
		stdout: newReadyWriter(),
		stderr: &lockedBuffer{},
		done:   make(chan struct{}),
	}
	s.cmd = exec.Command(bin)
	s.cmd.Dir = dir
	s.cmd.Env = append(os.Environ(), "PORT="+strconv.Itoa(port))
	s.cmd.Stdout = s.stdout
	s.cmd.Stderr = s.stderr
	s.cmd.WaitDelay = time.Second
	if err := s.cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		s.err = s.cmd.Wait()
		close(s.done)
	}()

	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()
	select {
	case <-s.stdout.ready:
		return s, nil
	case <-s.done:
		// The process may have printed READY. just before exiting.
		select {
		case <-s.stdout.ready:
			return s, nil
		default:
		}
		stdout, stderr := s.Output()
		if strings.Contains(stderr, "bind") {
			return nil, fmt.Errorf("%w: %s", errPortTaken, stderr)
		}
		return nil, &NotReadyError{Reason: fmt.Sprintf("exited (%v) before printing READY.", s.err), Stdout: stdout, Stderr: stderr}
	case <-timer.C:
		s.Stop()
		stdout, stderr := s.Output()
		return nil, &NotReadyError{Reason: fmt.Sprintf("did not print READY. within %s", readyTimeout), Stdout: stdout, Stderr: stderr}
	}
}

// NotReadyError says why a server never became ready, with what it printed.
type NotReadyError struct {
	Reason         string
	Stdout, Stderr string
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("server %s\n--- stdout\n%s\n--- stderr\n%s", e.Reason, Normalize(e.Stdout), Normalize(e.Stderr))
}

// Unwrap makes errors.Is(err, ErrNotReady) true.
func (e *NotReadyError) Unwrap() error { return ErrNotReady }

// Stop kills the process and waits for it to exit. It is safe to call more
// than once. The program is the binary itself, not a "go run" wrapper, so
// killing it leaves no child process behind on any OS.
func (s *Server) Stop() {
	s.once.Do(func() {
		select {
		case <-s.done:
		default:
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	})
}

// Exited reports whether the process has exited, and how.
func (s *Server) Exited() (bool, error) {
	select {
	case <-s.done:
		return true, s.err
	default:
		return false, nil
	}
}

// Output returns what the program has printed so far.
func (s *Server) Output() (stdout, stderr string) {
	return s.stdout.String(), s.stderr.String()
}

// freePort asks the OS for a free loopback port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// requestTimeout limits each request in a script.
const requestTimeout = 10 * time.Second

// Send sends one request to baseURL. If the request has a body and no
// Content-Type header, it is sent as application/json.
func Send(ctx context.Context, client *http.Client, baseURL string, req Request) (Got, error) {
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, req.Method, baseURL+req.Target, body)
	if err != nil {
		return Got{}, err
	}
	for _, h := range req.Headers {
		if strings.EqualFold(h.Name, "Host") {
			r.Host = h.Value
			continue
		}
		r.Header.Add(h.Name, h.Value)
	}
	if req.Body != "" && r.Header.Get("Content-Type") == "" {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(r)
	if err != nil {
		return Got{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return Got{}, err
	}
	return Got{Status: resp.StatusCode, Header: resp.Header, Body: string(b)}, nil
}

// RunScript sends each exchange to baseURL in order and checks its response.
// It returns one error per failed exchange, each showing the request, the
// expected response and the actual one. It keeps going after a mismatch, but
// stops if a request can't be sent at all.
func RunScript(ctx context.Context, baseURL string, exs []Exchange) []error {
	client := &http.Client{
		// Report redirects as they are, don't follow them.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	var errs []error
	for _, ex := range exs {
		got, err := Send(ctx, client, baseURL, ex.Req)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s\n%s\n--- error\n%v", exchangeTitle(ex), formatRequest(ex.Req), err))
			return errs
		}
		if err := MatchResponse(got, ex.Want); err != nil {
			errs = append(errs, fmt.Errorf("%s: %v\n%s\n--- want\n%s\n--- got\n%s",
				exchangeTitle(ex), err, formatRequest(ex.Req), formatWant(ex.Want), formatGot(got, ex.Want)))
		}
	}
	return errs
}

func exchangeTitle(ex Exchange) string {
	t := fmt.Sprintf("line %d", ex.Line)
	if ex.Name != "" {
		t += " (" + ex.Name + ")"
	}
	return t
}

func formatRequest(r Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- request\n%s %s\n", r.Method, r.Target)
	for _, h := range r.Headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if r.Body != "" {
		b.WriteString("\n" + r.Body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatWant(w Response) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=> %d\n", w.Status)
	for _, h := range w.Headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if w.Body != "" {
		b.WriteString("\n" + w.Body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatGot shows the actual response: the headers the script asked about
// first, then the rest, then the body.
func formatGot(g Got, w Response) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=> %d\n", g.Status)
	shown := map[string]bool{}
	for _, h := range w.Headers {
		k := http.CanonicalHeaderKey(h.Name)
		if vals, ok := g.Header[k]; ok && !shown[k] {
			fmt.Fprintf(&b, "%s: %s\n", strings.ToLower(k), strings.Join(vals, ", "))
		}
		shown[k] = true
	}
	var rest []string
	for k := range g.Header {
		if !shown[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		fmt.Fprintf(&b, "%s: %s\n", strings.ToLower(k), strings.Join(g.Header[k], ", "))
	}
	if body := strings.TrimRight(g.Body, "\r\n"); body != "" {
		b.WriteString("\n" + body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// readyWriter collects stdout and closes ready once a line starting with
// "READY." has been written.
type readyWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	ready chan struct{}
	seen  bool
}

func newReadyWriter() *readyWriter { return &readyWriter{ready: make(chan struct{})} }

func (w *readyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if !w.seen && hasReadyLine(w.buf.String()) {
		w.seen = true
		close(w.ready)
	}
	return len(p), nil
}

func (w *readyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// hasReadyLine reports whether out has a complete line starting with "READY.".
func hasReadyLine(out string) bool {
	lines := strings.Split(out, "\n")
	for _, l := range lines[:len(lines)-1] { // the last element is not a complete line
		if strings.HasPrefix(l, "READY.") {
			return true
		}
	}
	return false
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
