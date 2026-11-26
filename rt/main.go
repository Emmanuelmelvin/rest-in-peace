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
func Main(run func(c *Ctx)) {
	c := NewCtx(context.Background())
	if e := runTop(c, run); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
	// TODO(task 2.1): start the app if the program called SERVE.
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
			if buildRoot != "/" && strings.HasPrefix(file, buildRoot) {
				file = file[len(buildRoot):]
			} else if j := strings.LastIndex(file, "/"); j >= 0 {
				file = file[j+1:]
			}
			return file, f.Line
		}
		if !more {
			return "", 0
		}
	}
}
