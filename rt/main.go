package rt

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Main runs a program's top-level statements (design §4.8). The generated
// main function is:
//
//	func main() { rt.Main(func(c *rt.Ctx) { … }) }
//
// An error that escapes is printed to stderr in the classic form,
// ?KIND ERROR IN file:line — message, and the program exits with status 1.
//
// A program with TEST blocks, or a rip test build, runs its tests after the
// top-level code instead of starting the server (expect.go).
func Main(run func(c *Ctx)) {
	c := NewCtx(context.Background())
	app := c.App()
	if e := runTop(c, run); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
	// Under test, run the TESTs instead of serving: SERVE never binds a port
	// (Requirement 12.3).
	if testRun() {
		os.Exit(runTests(c, os.Stdout, os.Stderr))
	}
	// SERVE only recorded the port; listen now that all top-level code has run.
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runTop(c *Ctx, run func(c *Ctx)) (err *Error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(endSignal); ok {
			return
		}
		e := FromPanic(r)
		if e.File == "" {
			e.File, e.Line = basLocation()
		}
		err = e
	}()
	run(c)
	return nil
}

// buildRoot is the build directory (or module path, under -trimpath) that
// the compiler resolved relative //line file names against. It is found
// from this file's own path: <root>/rt/main.go.
var buildRoot = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	file = strings.ReplaceAll(file, `\`, "/")
	for i := 0; i < 2; i++ {
		if j := strings.LastIndex(file, "/"); j >= 0 {
			file = file[:j]
		}
	}
	return file + "/"
}()

// cleanBasFile normalizes a .bas frame path from runtime.CallersFrames so
// runtime errors report the project-relative BASIC file name (such as
// "tests/fail.test.bas") consistently on every OS, with or without -trimpath.
func cleanBasFile(file, root string) string {
	file = strings.ReplaceAll(file, `\`, "/")
	if root != "" && root != "/" && strings.HasPrefix(file, root) {
		return file[len(root):]
	}
	// Only strip leading directories from absolute paths that did not match
	// buildRoot; relative paths (like "tests/fail.test.bas" on Linux without
	// -trimpath) are already project-relative.
	if strings.HasPrefix(file, "/") || (len(file) >= 2 && file[1] == ':') {
		if j := strings.LastIndex(file, "/"); j >= 0 {
			return file[j+1:]
		}
	}
	return file
}

// basLocation finds the innermost BASIC statement on the panicking stack.
// The //line directives make those frames report .bas files (design §4.5).
// It must be called from a deferred function while the panic is unwinding.
func basLocation() (string, int) {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		file := strings.ReplaceAll(f.File, `\`, "/")
		if strings.HasSuffix(file, ".bas") {
			return cleanBasFile(file, buildRoot), f.Line
		}
		if !more {
			return "", 0
		}
	}
}
