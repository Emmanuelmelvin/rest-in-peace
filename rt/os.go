package rt

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	execDefaultTimeout = 30 * time.Second
	execMaxOutput      = 1 << 20 // 1 MB per stream
)

func requireNotPlayground(fn string) {
	if isPlayground() {
		panic(Errorf(KindNotAllowed, "%s is not allowed in the playground — use the rip CLI", fn))
	}
}

// Cwd implements CWD(): returns the current working directory.
func Cwd() string {
	requireNotPlayground("CWD")
	dir, err := os.Getwd()
	if err != nil {
		panic(Errorf(KindInternal, "CWD: %v", err))
	}
	return dir
}

// Args implements ARGS(): returns command-line arguments after the program name.
func Args() *Array {
	requireNotPlayground("ARGS")
	if len(os.Args) <= 1 {
		return &Array{Items: []Value{}}
	}
	items := make([]Value, len(os.Args)-1)
	for i, a := range os.Args[1:] {
		items[i] = a
	}
	return &Array{Items: items}
}

// Pid implements PID(): returns the current process ID.
func Pid() int64 {
	requireNotPlayground("PID")
	return int64(os.Getpid())
}

// Platform implements PLATFORM(): returns the operating system name (runtime.GOOS).
func Platform() string {
	return runtime.GOOS
}

// Hostname implements HOSTNAME(): returns the host name reported by the kernel.
func Hostname() string {
	requireNotPlayground("HOSTNAME")
	name, err := os.Hostname()
	if err != nil {
		panic(Errorf(KindInternal, "HOSTNAME: %v", err))
	}
	return name
}

// CPUs implements CPUS(): returns the number of logical CPUs usable by the current process.
func CPUs() int64 {
	return int64(runtime.NumCPU())
}

// Exec implements EXEC(cmd [, args [, opts]]): runs an external command and
// returns a *Map with "code" (int64), "stdout" (string), and "stderr" (string).
//
// Non-zero exit codes are returned in "code" without panicking. Failures to
// start the command or timeouts panic with KindInternal.
func Exec(c *Ctx, cmd string, rest ...Value) *Map {
	requireNotPlayground("EXEC")

	var args []string
	if len(rest) > 0 && rest[0] != nil {
		arr, ok := rest[0].(*Array)
		if !ok {
			panic(Errorf(KindTypeMismatch, "EXEC args must be an ARRAY of STRINGs, got %s", TypeName(rest[0])))
		}
		if arr != nil {
			args = make([]string, len(arr.Items))
			for i, item := range arr.Items {
				s, ok := item.(string)
				if !ok {
					panic(Errorf(KindTypeMismatch, "EXEC args[%d] must be a STRING, got %s", i, TypeName(item)))
				}
				args[i] = s
			}
		}
	}

	timeout := execDefaultTimeout
	var cwd string
	var env []string

	if len(rest) > 1 && rest[1] != nil {
		opts, ok := rest[1].(*Map)
		if !ok {
			panic(Errorf(KindTypeMismatch, "EXEC options must be a MAP, got %s", TypeName(rest[1])))
		}
		if opts != nil {
			if v, ok := opts.Get("cwd"); ok && v != nil {
				s, ok := v.(string)
				if !ok {
					panic(Errorf(KindTypeMismatch, "EXEC options.cwd must be a STRING, got %s", TypeName(v)))
				}
				cwd = s
			}
			if v, ok := opts.Get("timeout"); ok && v != nil {
				secs, _, _, numOk := numFloat(v)
				if !numOk {
					panic(Errorf(KindTypeMismatch, "EXEC options.timeout must be a NUMBER of seconds, got %s", TypeName(v)))
				}
				if math.IsNaN(secs) || math.IsInf(secs, 0) || secs <= 0 {
					panic(Errorf(KindTypeMismatch, "EXEC options.timeout must be greater than zero, got %g", secs))
				}
				timeout = time.Duration(secs * float64(time.Second))
			}
			if v, ok := opts.Get("env"); ok && v != nil {
				envMap, ok := v.(*Map)
				if !ok {
					panic(Errorf(KindTypeMismatch, "EXEC options.env must be a MAP of STRINGs, got %s", TypeName(v)))
				}
				if envMap != nil {
					env = append(os.Environ(), make([]string, 0, envMap.Len())...)
					for _, k := range envMap.Keys() {
						ev, _ := envMap.Get(k)
						s, ok := ev.(string)
						if !ok {
							panic(Errorf(KindTypeMismatch, "EXEC options.env[%q] must be a STRING, got %s", k, TypeName(ev)))
						}
						env = append(env, k+"="+s)
					}
				}
			}
		}
	}

	parentCtx := context.Background()
	if c != nil && c.Context() != nil {
		parentCtx = c.Context()
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	command := exec.CommandContext(ctx, cmd, args...)
	if cwd != "" {
		command.Dir = cwd
	}
	if env != nil {
		command.Env = env
	}

	stdout := &limitedBuffer{limit: execMaxOutput}
	stderr := &limitedBuffer{limit: execMaxOutput}
	command.Stdout = stdout
	command.Stderr = stderr

	err := command.Run()
	if err != nil {
		if ctx.Err() != nil {
			panic(Errorf(KindInternal, "EXEC %q: %v", cmd, ctx.Err()))
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return MapOf(
				"code", int64(exitErr.ExitCode()),
				"stdout", stdout.buf.String(),
				"stderr", stderr.buf.String(),
			)
		}
		panic(Errorf(KindInternal, "EXEC %q: %v", cmd, err))
	}

	return MapOf(
		"code", int64(0),
		"stdout", stdout.buf.String(),
		"stderr", stderr.buf.String(),
	)
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	rem := b.limit - b.buf.Len()
	if rem > 0 {
		if len(p) > rem {
			b.buf.Write(p[:rem])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}
