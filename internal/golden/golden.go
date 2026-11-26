// Package golden runs every program in testdata/golden (task 1.9).
//
// Each case is <name>.bas, plus one of:
//   - <name>.err: the expected compiler diagnostics, one per line;
//   - <name>.out and/or <name>.stderr: the expected stdout and, for a runtime
//     error, the expected stderr. A case with a .stderr file must exit non-zero;
//     a case without one must exit 0. A missing .out means stdout must be empty;
//   - <name>.http: a request script, for a program that serves HTTP (below).
//
// Lines in .err and .stderr files are compared with MatchLines: a line that
// stops after its location (?KIND ERROR IN file:line) leaves the hint after
// " — " free, so the golden files don't fix hint wording that is still settling.
//
// A case whose compile reports a NOT IMPLEMENTED diagnostic is skipped. So is
// an HTTP case whose server stops before it is ready because a runtime
// feature is "not built yet".
//
// # HTTP cases
//
// The runner builds the program, starts it with PORT set to a free port,
// waits (up to a timeout) for a stdout line starting with "READY.", sends the
// script's requests in order to 127.0.0.1, checks each response, and then
// kills the program.
//
// A .http script is a list of exchanges. Everything before the first "###"
// line is a comment. Each exchange looks like this:
//
//	### create a todo
//	# Comment lines may follow the ### line, before the request line.
//	POST /todos?notify=1
//	X-Api-Key: secret
//
//	{"title": "BURY THE HATCHET"}
//	=> 201
//	location: /todos/1
//
//	{"id": "1", "title": "BURY THE HATCHET", "done": false}
//
// That is:
//   - "###", optionally followed by a name for messages;
//   - the request line: an upper-case method and a target (the path and
//     query, sent exactly as written, so percent-escape it yourself);
//   - request headers, one "Name: value" per line, up to a blank line;
//   - an optional body, sent exactly as written (so it may be invalid JSON),
//     up to the "=> <status>" line. A body is sent as application/json unless
//     a Content-Type header says otherwise. No body means none is sent;
//   - "=> <status>": the expected status code;
//   - expected headers, one "Name: value" per line, up to a blank line. Only
//     these headers are checked: names case-insensitively, values exactly;
//   - the expected body, up to the next "###" line or the end of the file.
//
// Blank lines around bodies are ignored. An expected body that parses as JSON
// is compared structurally: whitespace doesn't matter, numbers compare by
// value, and object keys must come in the same order, because maps keep
// insertion order (Requirement 3.5). Any other expected body is compared as
// text, so an empty one means the body must be empty, and a body of just
// <any> is not checked. Two wildcards stand in for values that vary, as a
// JSON value or as a whole header value:
//   - "<any>" matches anything, as long as the key or header is present;
//   - "<any string>" matches any non-empty string.
//
// A mismatch is reported with the request, the expected response and the
// actual one.
package golden

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// HintSep separates a diagnostic's location from its hint.
const HintSep = " — "

// ErrTimeout is returned by BuildAndRun when the program runs past its timeout.
var ErrTimeout = errors.New("program did not finish in time")

// Outcome is what a program did when it ran.
type Outcome struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// BuildBinary writes files into dir as a build directory and builds them
// into a binary with internal/build. It returns the binary's path.
func BuildBinary(ctx context.Context, dir string, files []codegen.File) (string, error) {
	if err := build.Prepare(dir, files); err != nil {
		return "", err
	}
	return build.Build(ctx, dir, build.Options{Output: filepath.Join(dir, "bin", "prog")})
}

// BuildAndRun builds files with BuildBinary and runs the binary with no
// arguments and no stdin. timeout limits the run, not the build. A non-zero
// exit is not an error: it is reported in Outcome.ExitCode.
func BuildAndRun(ctx context.Context, dir string, files []codegen.File, timeout time.Duration) (*Outcome, error) {
	bin, err := BuildBinary(ctx, dir, files)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w: still running after %s; stdout so far:\n%s", ErrTimeout, timeout, stdout.String())
	}
	out := &Outcome{Stdout: stdout.String(), Stderr: stderr.String()}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		out.ExitCode = exitErr.ExitCode()
	case err != nil:
		return nil, err
	}
	return out, nil
}

// Normalize turns CRLF into LF and drops trailing newlines.
func Normalize(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// MatchLines compares got with want line by line, after normalizing both. A
// want line that contains HintSep must equal the got line exactly. One without
// it only has to equal the got line up to the got line's own HintSep, so
// "?SYNTAX ERROR IN x.bas:3" matches "?SYNTAX ERROR IN x.bas:3 — expected THEN".
func MatchLines(got, want string) error {
	g, w := splitLines(got), splitLines(want)
	ok := len(g) == len(w)
	for i := 0; ok && i < len(w); i++ {
		ok = MatchLine(g[i], w[i])
	}
	if !ok {
		return fmt.Errorf("lines differ\n--- got\n%s\n--- want\n%s", Normalize(got), Normalize(want))
	}
	return nil
}

// MatchLine applies the MatchLines rule to one line.
func MatchLine(got, want string) bool {
	if strings.Contains(want, HintSep) {
		return got == want
	}
	head, _, _ := strings.Cut(got, HintSep)
	return head == want
}

func splitLines(s string) []string {
	s = Normalize(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
