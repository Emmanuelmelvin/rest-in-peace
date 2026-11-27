package rt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func catchOSError(t *testing.T, fn func()) *Error {
	t.Helper()
	var got *Error
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			e, ok := r.(*Error)
			if !ok {
				t.Fatalf("expected *rt.Error panic, got %T: %v", r, r)
			}
			got = e
		}()
		fn()
	}()
	return got
}

// TestOSHelperProcess is invoked as a subprocess by TestExec* tests.
func TestOSHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_OS_HELPER_PROCESS") != "1" {
		return
	}
	// Find arguments after "--".
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(0)
	}
	switch args[0] {
	case "echo":
		fmt.Fprint(os.Stdout, strings.Join(args[1:], " "))
		os.Exit(0)
	case "stderr-exit":
		fmt.Fprint(os.Stdout, "out-data")
		fmt.Fprint(os.Stderr, "err-data")
		os.Exit(42)
	case "pwd":
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Fprint(os.Stdout, dir)
		os.Exit(0)
	case "env":
		for _, k := range args[1:] {
			fmt.Fprintf(os.Stdout, "%s=%s\n", k, os.Getenv(k))
		}
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "bigout":
		chunk := strings.Repeat("a", 64*1024)
		for range 20 { // 1.25 MB
			fmt.Fprint(os.Stdout, chunk)
			fmt.Fprint(os.Stderr, chunk)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s", args[0])
		os.Exit(2)
	}
}

func helperArgs(mode string, extra ...string) *Array {
	items := []Value{"-test.run=^TestOSHelperProcess$", "--", mode}
	for _, s := range extra {
		items = append(items, s)
	}
	return &Array{Items: items}
}

func TestOSCwd(t *testing.T) {
	want, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if got := Cwd(); got != want {
		t.Fatalf("Cwd() = %q, want %q", got, want)
	}
}

func TestOSArgs(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{name: "empty_when_only_prog", argv: []string{"prog"}, want: []string{}},
		{name: "empty_when_nil", argv: nil, want: []string{}},
		{name: "multiple_args", argv: []string{"prog", "--flag", "val", "file.bas"}, want: []string{"--flag", "val", "file.bas"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.argv
			got := Args()
			if got == nil {
				t.Fatal("Args() returned nil")
			}
			if got.Len() != len(tc.want) {
				t.Fatalf("Args().Len() = %d, want %d", got.Len(), len(tc.want))
			}
			for i, w := range tc.want {
				if got.Items[i] != w {
					t.Errorf("Args()[%d] = %v, want %q", i, got.Items[i], w)
				}
			}
		})
	}
}

func TestOSPid(t *testing.T) {
	want := int64(os.Getpid())
	got := Pid()
	if got != want || got <= 0 {
		t.Fatalf("Pid() = %d, want %d", got, want)
	}
}

func TestOSPlatform(t *testing.T) {
	if got := Platform(); got != runtime.GOOS {
		t.Fatalf("Platform() = %q, want %q", got, runtime.GOOS)
	}
}

func TestOSHostname(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}
	if got := Hostname(); got != want {
		t.Fatalf("Hostname() = %q, want %q", got, want)
	}
}

func TestOSCPUs(t *testing.T) {
	want := int64(runtime.NumCPU())
	got := CPUs()
	if got != want || got < 1 {
		t.Fatalf("CPUs() = %d, want %d", got, want)
	}
}

func TestOSPlaygroundGuards(t *testing.T) {
	t.Setenv("RIP_PLAYGROUND", "1")

	blocked := []struct {
		name string
		fn   func()
	}{
		{name: "Cwd", fn: func() { Cwd() }},
		{name: "Args", fn: func() { Args() }},
		{name: "Pid", fn: func() { Pid() }},
		{name: "Hostname", fn: func() { Hostname() }},
		{name: "Exec", fn: func() { Exec(NewCtx(context.Background()), "echo") }},
	}

	for _, tc := range blocked {
		t.Run(tc.name, func(t *testing.T) {
			err := catchOSError(t, tc.fn)
			if err == nil {
				t.Fatalf("%s did not panic in playground mode", tc.name)
			}
			if err.Kind != KindNotAllowed {
				t.Fatalf("%s error kind = %q, want %q", tc.name, err.Kind, KindNotAllowed)
			}
			if !strings.Contains(err.Message, "not allowed in the playground") {
				t.Fatalf("%s error message = %q, want playground hint", tc.name, err.Message)
			}
		})
	}

	// Platform and CPUs remain allowed in playground mode.
	if got := Platform(); got != runtime.GOOS {
		t.Fatalf("Platform() in playground = %q, want %q", got, runtime.GOOS)
	}
	if got := CPUs(); got < 1 {
		t.Fatalf("CPUs() in playground = %d, want >= 1", got)
	}
}

func TestExecBasicAndExitCodes(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	t.Setenv("GO_WANT_OS_HELPER_PROCESS", "1")
	c := NewCtx(context.Background())

	t.Run("echo_stdout_code_0", func(t *testing.T) {
		res := Exec(c, exe, helperArgs("echo", "hello", "rip"))
		if code, _ := res.Get("code"); code != int64(0) {
			t.Fatalf("code = %v, want 0", code)
		}
		if out, _ := res.Get("stdout"); out != "hello rip" {
			t.Fatalf("stdout = %q, want %q", out, "hello rip")
		}
		if errOut, _ := res.Get("stderr"); errOut != "" {
			t.Fatalf("stderr = %q, want empty", errOut)
		}
	})

	t.Run("nil_ctx_and_nil_args", func(t *testing.T) {
		// Calling with nil Ctx and nil args should not panic on arg validation.
		res := Exec(nil, exe, helperArgs("echo", "ok"), nil)
		if code, _ := res.Get("code"); code != int64(0) {
			t.Fatalf("code = %v, want 0", code)
		}
		if out, _ := res.Get("stdout"); out != "ok" {
			t.Fatalf("stdout = %q, want %q", out, "ok")
		}
	})

	t.Run("nonzero_exit_no_panic", func(t *testing.T) {
		res := Exec(c, exe, helperArgs("stderr-exit"))
		if code, _ := res.Get("code"); code != int64(42) {
			t.Fatalf("code = %v, want 42", code)
		}
		if out, _ := res.Get("stdout"); out != "out-data" {
			t.Fatalf("stdout = %q, want %q", out, "out-data")
		}
		if errOut, _ := res.Get("stderr"); errOut != "err-data" {
			t.Fatalf("stderr = %q, want %q", errOut, "err-data")
		}
	})
}

func TestExecOptions(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	c := NewCtx(context.Background())

	t.Run("cwd", func(t *testing.T) {
		tmp := t.TempDir()
		opts := MapOf(
			"cwd", tmp,
			"env", MapOf("GO_WANT_OS_HELPER_PROCESS", "1"),
		)
		res := Exec(c, exe, helperArgs("pwd"), opts)
		if code, _ := res.Get("code"); code != int64(0) {
			t.Fatalf("code = %v, want 0", code)
		}
		gotDir, _ := res.Get("stdout")
		// Resolve symlinks on both sides for macOS /tmp -> /private/tmp portability.
		wantReal, _ := filepath.EvalSymlinks(tmp)
		gotReal, _ := filepath.EvalSymlinks(gotDir.(string))
		if gotReal != wantReal {
			t.Fatalf("cwd = %q (real %q), want %q (real %q)", gotDir, gotReal, tmp, wantReal)
		}
	})

	t.Run("env_appended", func(t *testing.T) {
		t.Setenv("RIP_TEST_INHERITED", "inherited_val")
		opts := MapOf(
			"env", MapOf(
				"GO_WANT_OS_HELPER_PROCESS", "1",
				"RIP_TEST_CUSTOM", "custom_val",
			),
		)
		res := Exec(c, exe, helperArgs("env", "RIP_TEST_INHERITED", "RIP_TEST_CUSTOM"), opts)
		if code, _ := res.Get("code"); code != int64(0) {
			t.Fatalf("code = %v, want 0", code)
		}
		out, _ := res.Get("stdout")
		want := "RIP_TEST_INHERITED=inherited_val\nRIP_TEST_CUSTOM=custom_val\n"
		if out != want {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	})

	t.Run("timeout_exceeded", func(t *testing.T) {
		opts := MapOf(
			"timeout", 0.15,
			"env", MapOf("GO_WANT_OS_HELPER_PROCESS", "1"),
		)
		rtErr := catchOSError(t, func() {
			Exec(c, exe, helperArgs("sleep"), opts)
		})
		if rtErr == nil {
			t.Fatal("expected timeout panic, got none")
		}
		if rtErr.Kind != KindInternal {
			t.Fatalf("error kind = %q, want %q", rtErr.Kind, KindInternal)
		}
	})

	t.Run("output_capped_at_1mb", func(t *testing.T) {
		opts := MapOf(
			"env", MapOf("GO_WANT_OS_HELPER_PROCESS", "1"),
		)
		res := Exec(c, exe, helperArgs("bigout"), opts)
		if code, _ := res.Get("code"); code != int64(0) {
			t.Fatalf("code = %v, want 0", code)
		}
		stdout, _ := res.Get("stdout")
		stderr, _ := res.Get("stderr")
		if len(stdout.(string)) != execMaxOutput {
			t.Fatalf("stdout len = %d, want %d", len(stdout.(string)), execMaxOutput)
		}
		if len(stderr.(string)) != execMaxOutput {
			t.Fatalf("stderr len = %d, want %d", len(stderr.(string)), execMaxOutput)
		}
	})
}

func TestExecErrors(t *testing.T) {
	c := NewCtx(context.Background())

	t.Run("command_not_found", func(t *testing.T) {
		rtErr := catchOSError(t, func() {
			Exec(c, "rip-definitely-nonexistent-command-xyz-99999")
		})
		if rtErr == nil {
			t.Fatal("expected panic for missing command")
		}
		if rtErr.Kind != KindInternal {
			t.Fatalf("error kind = %q, want %q", rtErr.Kind, KindInternal)
		}
	})

	typeMismatchCases := []struct {
		name string
		rest []Value
	}{
		{name: "args_not_array", rest: []Value{"not-an-array"}},
		{name: "args_element_not_string", rest: []Value{ArrayOf("ok", int64(123))}},
		{name: "opts_not_map", rest: []Value{nil, "not-a-map"}},
		{name: "opts_cwd_not_string", rest: []Value{nil, MapOf("cwd", int64(1))}},
		{name: "opts_timeout_not_number", rest: []Value{nil, MapOf("timeout", "5s")}},
		{name: "opts_timeout_zero", rest: []Value{nil, MapOf("timeout", int64(0))}},
		{name: "opts_timeout_negative", rest: []Value{nil, MapOf("timeout", -1.5)}},
		{name: "opts_env_not_map", rest: []Value{nil, MapOf("env", ArrayOf("A=B"))}},
		{name: "opts_env_value_not_string", rest: []Value{nil, MapOf("env", MapOf("A", int64(1)))}},
	}

	for _, tc := range typeMismatchCases {
		t.Run(tc.name, func(t *testing.T) {
			rtErr := catchOSError(t, func() {
				Exec(c, "echo", tc.rest...)
			})
			if rtErr == nil {
				t.Fatalf("%s: expected panic, got none", tc.name)
			}
			if rtErr.Kind != KindTypeMismatch {
				t.Fatalf("%s: error kind = %q, want %q", tc.name, rtErr.Kind, KindTypeMismatch)
			}
		})
	}
}
