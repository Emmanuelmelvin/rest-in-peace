// Package build turns generated Go source into a native binary (design §4.1).
//
// Layout of a build directory:
//
//	go.mod   module ripapp; requires the runtime's dependencies
//	rt/      runtime source, copied from the copy embedded in the CLI
//	*.go     generated program (package main)
package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	rip "github.com/Emmanuelmelvin/rest-in-peace"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// ModulePath is the module name of every build directory; the runtime is ModulePath + "/rt".
const ModulePath = "ripapp"

// GoVersion is the go directive written into generated go.mod files.
const GoVersion = "1.24"

// ErrNoGo is returned when the Go toolchain is not installed (Requirement 13.1).
var ErrNoGo = errors.New("?GO NOT FOUND ERROR — rip needs the Go toolchain (1.24 or newer) to build programs; install it from https://go.dev/dl/ (on Windows: winget install GoLang.Go)")

// LookGo finds the go command.
func LookGo() (string, error) {
	p, err := exec.LookPath("go")
	if err != nil {
		return "", ErrNoGo
	}
	return p, nil
}

// Options control the go build step.
type Options struct {
	Output  string // binary path; ".exe" is added for Windows targets
	OS      string // GOOS for cross-compilation; empty = this machine
	Arch    string // GOARCH for cross-compilation; empty = this machine
	Release bool   // static (CGO_ENABLED=0), stripped and -trimpath (rip build)
}

// Prepare writes dir as a build directory containing the runtime and the generated files.
// Old .go files are removed first, so deleted .bas modules don't linger.
func Prepare(dir string, files []codegen.File) error {
	rtDir := filepath.Join(dir, "rt")
	if err := os.MkdirAll(rtDir, 0o755); err != nil {
		return err
	}
	for _, d := range []string{dir, rtDir} {
		if err := removeGoFiles(d); err != nil {
			return err
		}
	}
	if err := writeRuntime(rtDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), goMod(), 0o644); err != nil {
		return err
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Src, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Build runs go mod tidy and go build in dir, and returns the binary's path.
func Build(ctx context.Context, dir string, opts Options) (string, error) {
	goBin, err := LookGo()
	if err != nil {
		return "", err
	}
	if opts.Output == "" {
		return "", errors.New("build: Options.Output is empty")
	}
	out, err := filepath.Abs(binaryPath(opts.Output, opts.OS))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}

	env := []string{"GOWORK=off"}
	if opts.OS != "" {
		env = append(env, "GOOS="+opts.OS)
	}
	if opts.Arch != "" {
		env = append(env, "GOARCH="+opts.Arch)
	}
	if opts.Release {
		env = append(env, "CGO_ENABLED=0")
	}

	if err := goCmd(ctx, dir, env, goBin, "mod", "tidy"); err != nil {
		return "", err
	}
	args := []string{"build", "-o", out}
	if opts.Release {
		args = append(args, "-trimpath", "-ldflags=-s -w")
	}
	args = append(args, ".")
	if err := goCmd(ctx, dir, env, goBin, args...); err != nil {
		return "", err
	}
	return out, nil
}

// binaryPath adds ".exe" to output when the target OS (goos, or this
// machine's when empty) is Windows.
func binaryPath(output, goos string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" && !strings.EqualFold(filepath.Ext(output), ".exe") {
		return output + ".exe"
	}
	return output
}

func goCmd(ctx context.Context, dir string, env []string, goBin string, args ...string) error {
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("?BUILD ERROR — `go %s` failed. This is a bug in rip's code generation, not in your BASIC:\n%s",
			strings.Join(args, " "), output.String())
	}
	return nil
}

func writeRuntime(dst string) error {
	entries, err := fs.ReadDir(rip.RuntimeFS, "rt")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := fs.ReadFile(rip.RuntimeFS, "rt/"+name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func removeGoFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func goMod() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "module %s\n\ngo %s\n", ModulePath, GoVersion)
	if reqs := runtimeRequires(); len(reqs) > 0 {
		b.WriteString("\nrequire (\n")
		for _, r := range reqs {
			fmt.Fprintf(&b, "\t%s %s\n", r.Path, r.Version)
		}
		b.WriteString(")\n")
	}
	// TODO(task 3.5): add the Go packages named in IMPORT … FROM GO.
	return b.Bytes()
}

// runtimeRequires returns the modules that the embedded rt imports, at the
// versions in the CLI's own build info, so programs use exactly the versions
// rip was built and tested with. A module is only in the build info if the CLI
// links it, so each module rt imports must also be imported (blank) by a
// package the CLI links; TestGoModPinsRuntimeImports fails until it is.
func runtimeRequires() []*debug.Module {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	imports, err := runtimeImports()
	if err != nil {
		return nil
	}
	var out []*debug.Module
	for _, d := range info.Deps {
		for _, imp := range imports {
			if imp == d.Path || strings.HasPrefix(imp, d.Path+"/") {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// runtimeImports returns the packages outside the standard library that the
// embedded rt source imports.
func runtimeImports() ([]string, error) {
	entries, err := fs.ReadDir(rip.RuntimeFS, "rt")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := fs.ReadFile(rip.RuntimeFS, "rt/"+name)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			// Standard-library paths have no dot in their first element.
			first, _, _ := strings.Cut(p, "/")
			if strings.Contains(first, ".") && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}
