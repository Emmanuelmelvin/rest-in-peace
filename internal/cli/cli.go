// Package cli implements the rip command (Requirement 13).
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
)

type command struct {
	name, usage, summary string
	run                  func(args []string) int
}

var commands []command

func init() {
	commands = []command{
		{"new", "rip new <name>", "scaffold a new project", runNew},
		{"dev", "rip dev [file.bas]", "compile and run, rebuilding on change", runDev},
		{"run", "rip run [file.bas]", "compile and run once", runRun},
		{"test", "rip test", "run tests/*.test.bas", runTest},
		{"build", "rip build [--os X] [--arch Y] [-o file]", "build one static binary in dist/", runBuild},
		{"migrate", "rip migrate", "apply migrations/*.sql", runMigrate},
		{"check", "rip check [file.bas]", "compile only; print errors and warnings", runCheck},
		{"version", "rip version", "print the rip version", runVersion},
	}
}

// Run executes the rip command and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(os.Stdout)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "?UNDEFINED COMMAND ERROR — rip has no command %q; run `rip help`\n", args[0])
	return 2
}

func usage(w io.Writer) {
	fmt.Fprint(w, "REST in Peace — BASIC for production backends\n\nUsage:\n")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-42s %s\n", c.usage, c.summary)
	}
	fmt.Fprint(w, "\nREADY.\n")
}

func runVersion(args []string) int {
	v := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	fmt.Printf("rip %s (%s/%s)\n", v, runtime.GOOS, runtime.GOARCH)
	return 0
}

// project is a directory with an optional rip.json.
type project struct {
	Root string `json:"-"`
	Name string `json:"name"`
	Main string `json:"main"`
}

func loadProject(dir string) (*project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	p := &project{Root: abs, Name: filepath.Base(abs), Main: "main.bas"}
	data, err := os.ReadFile(filepath.Join(abs, "rip.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("?SYNTAX ERROR IN rip.json — %v", err)
	}
	return p, nil
}

// resolve finds the project and entry file: an explicit .bas argument, or the
// project in the current directory.
func resolve(args []string) (*project, string, error) {
	if len(args) > 0 && strings.HasSuffix(strings.ToLower(args[0]), ".bas") {
		entry, err := filepath.Abs(args[0])
		if err != nil {
			return nil, "", err
		}
		p, err := loadProject(filepath.Dir(entry))
		if err != nil {
			return nil, "", err
		}
		return p, entry, nil
	}
	p, err := loadProject(".")
	if err != nil {
		return nil, "", err
	}
	entry := filepath.Join(p.Root, p.Main)
	if _, err := os.Stat(entry); err != nil {
		return nil, "", fmt.Errorf("?FILE NOT FOUND ERROR — no %s here; run rip inside a project, or pass a .bas file", p.Main)
	}
	return p, entry, nil
}

// compile runs the compiler and prints its diagnostics. ok is false if there were errors.
func compile(entry string, guard bool) (res *compiler.Result, ok bool) {
	res, err := compiler.Compile(entry, compiler.Options{Guard: guard})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, false
	}
	for _, d := range res.Diagnostics {
		fmt.Fprintln(os.Stderr, d)
	}
	return res, !res.Diagnostics.HasErrors()
}

// compileAndBuild compiles entry and builds a binary in the project's .rip directory.
func compileAndBuild(p *project, entry string, opts build.Options) (string, bool) {
	res, ok := compile(entry, false)
	if !ok {
		return "", false
	}
	dir := filepath.Join(p.Root, ".rip", "build")
	if err := build.Prepare(dir, res.Files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return "", false
	}
	if opts.Output == "" {
		opts.Output = filepath.Join(p.Root, ".rip", "bin", p.Name)
	}
	bin, err := build.Build(context.Background(), dir, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return "", false
	}
	return bin, true
}

// execProgram runs a built binary in the foreground and returns its exit code.
func execProgram(bin string, extraEnv []string) int {
	cmd := exec.Command(bin)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// loadDotEnv reads KEY=VALUE lines for development runs (Requirement 10.2).
// Variables already set in the real environment win. Nothing is loaded when
// RIP_ENV=production.
func loadDotEnv(path string) []string {
	if os.Getenv("RIP_ENV") == "production" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, set := os.LookupEnv(k); set {
			continue
		}
		out = append(out, k+"="+strings.Trim(strings.TrimSpace(v), `"'`))
	}
	return out
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 1
}
