// Package golden runs every program in testdata/golden (task 1.9).
//
// Each case is <name>.bas, plus either:
//   - <name>.err: the expected compiler diagnostics, one per line, or
//   - <name>.out and/or <name>.stderr: the expected stdout and, for a runtime
//     error, the expected stderr. A case with a .stderr file must exit non-zero;
//     a case without one must exit 0. A missing .out means stdout must be empty.
//
// Lines in .err and .stderr files are compared with MatchLines: a line that
// stops after its location (?KIND ERROR IN file:line) leaves the hint after
// " — " free, so the golden files don't fix hint wording that is still settling.
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

// BuildAndRun writes files into dir as a build directory, builds them into a
// binary with internal/build, and runs it with no arguments and no stdin.
// timeout limits the run, not the build. A non-zero exit is not an error: it
// is reported in Outcome.ExitCode.
func BuildAndRun(ctx context.Context, dir string, files []codegen.File, timeout time.Duration) (*Outcome, error) {
	if err := build.Prepare(dir, files); err != nil {
		return nil, err
	}
	bin, err := build.Build(ctx, dir, build.Options{Output: filepath.Join(dir, "bin", "prog")})
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
