package playground

import (
	"bytes"
	"sync"
	"time"
)

// LogLine is one line of a program's log: its PRINT/LOG output, the
// requests the host proxied to it, and the host's own notes.
type LogLine struct {
	Seq    int64     `json:"seq"`
	Time   time.Time `json:"time"`
	Stream string    `json:"stream"` // "stdout", "stderr", "request" or "host"
	Text   string    `json:"text"`
}

const (
	logCapacity   = 1000    // lines kept per program and replayed to new subscribers
	maxLineBytes  = 4 << 10 // longer lines are cut
	subscriberBuf = 256     // a subscriber this far behind is dropped and must reconnect
)

// logRing is a fixed-size ring of recent log lines with live subscribers.
type logRing struct {
	mu     sync.Mutex
	lines  []LogLine // at most logCapacity; once full, lines[head] is the oldest
	head   int
	seq    int64
	subs   map[chan LogLine]struct{}
	closed bool
}

func newLogRing() *logRing {
	return &logRing{subs: map[chan LogLine]struct{}{}}
}

// add appends a line and fans it out to subscribers.
func (l *logRing) add(stream, text string) {
	if len(text) > maxLineBytes {
		text = text[:maxLineBytes] + "…"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.seq++
	line := LogLine{Seq: l.seq, Time: time.Now().UTC(), Stream: stream, Text: text}
	if len(l.lines) < logCapacity {
		l.lines = append(l.lines, line)
	} else {
		l.lines[l.head] = line
		l.head = (l.head + 1) % logCapacity
	}
	for ch := range l.subs {
		select {
		case ch <- line:
		default:
			// Too slow: drop it. EventSource reconnects with Last-Event-ID
			// and gets the missed lines from the replay.
			delete(l.subs, ch)
			close(ch)
		}
	}
}

// subscribe returns the kept lines newer than after, and a channel of new
// lines. The channel is closed when the subscriber falls behind or the ring
// is closed; cancel must be called when done.
func (l *logRing) subscribe(after int64) (replay []LogLine, ch <-chan LogLine, cancel func()) {
	c := make(chan LogLine, subscriberBuf)
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.ordered() {
		if line.Seq > after {
			replay = append(replay, line)
		}
	}
	if l.closed {
		close(c)
		return replay, c, func() {}
	}
	l.subs[c] = struct{}{}
	return replay, c, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if _, ok := l.subs[c]; ok {
			delete(l.subs, c)
			close(c)
		}
	}
}

// snapshot returns a copy of the kept lines.
func (l *logRing) snapshot() []LogLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ordered()
}

// ordered returns the kept lines oldest first, as a new slice. l.mu must be held.
func (l *logRing) ordered() []LogLine {
	out := make([]LogLine, 0, len(l.lines))
	out = append(out, l.lines[l.head:]...)
	return append(out, l.lines[:l.head]...)
}

// close ends every subscription; later lines are dropped.
func (l *logRing) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	for ch := range l.subs {
		delete(l.subs, ch)
		close(ch)
	}
}

// lineWriter is an io.Writer that splits a process's output into lines.
type lineWriter struct {
	mu     sync.Mutex
	buf    []byte
	stream string
	ring   *logRing
	onLine func(string) // optional, called for every complete line
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.buf = append(w.buf, p...)
			if len(w.buf) > maxLineBytes {
				w.emit()
			}
			break
		}
		w.buf = append(w.buf, p[:i]...)
		w.emit()
		p = p[i+1:]
	}
	return n, nil
}

// flush emits a final line that had no newline.
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit()
	}
}

func (w *lineWriter) emit() {
	line := string(bytes.TrimRight(w.buf, "\r"))
	w.buf = w.buf[:0]
	w.ring.add(w.stream, line)
	if w.onLine != nil {
		w.onLine(line)
	}
}
