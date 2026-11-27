package playground

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

// entryName is the file name a playground program is compiled as.
const entryName = "main.bas"

// notAllowed is the classic error word for code the playground refuses.
const notAllowed diag.Kind = "NOT ALLOWED"

// checkImports refuses what the playground can't run safely before the
// compiler sees the program: a playground program is one file, so IMPORT of
// a .bas path (which would read the host's files) is refused, and IMPORT …
// FROM GO is limited to the allowlist (Requirement 15.5). Lex and parse
// errors are left for the compiler to report.
func checkImports(src []byte) diag.List {
	toks, mode, ds := lexer.Lex(entryName, src)
	if ds.HasErrors() {
		return nil
	}
	f, ds := parser.Parse(entryName, toks, mode)
	if ds.HasErrors() {
		return nil
	}
	var out diag.List
	for _, imp := range f.Imports {
		switch {
		case !imp.FromGo:
			out = append(out, diag.Errorf(imp.At, notAllowed,
				"a playground program is a single file, so IMPORT %q can't be used here; paste that module into this file, or use the rip CLI", imp.Path))
		case !Allowed(imp.Path):
			out = append(out, diag.Errorf(imp.At, notAllowed,
				"the playground only allows IMPORT … FROM GO for %s; %q works with the rip CLI",
				strings.Join(GoAllowlist, " and "), imp.Path))
		}
	}
	return out
}

// compileProgram checks and compiles the program in dir/main.bas with loop
// guards on. It returns the generated files, or nil and the diagnostics.
func compileProgram(dir string, src []byte) ([]codegen.File, diag.List, error) {
	if ds := checkImports(src); ds.HasErrors() {
		return nil, ds, nil
	}
	path := filepath.Join(dir, entryName)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		return nil, nil, err
	}
	res, err := compiler.Compile(path, compiler.Options{Guard: true})
	if err != nil {
		return nil, nil, err
	}
	if res.Diagnostics.HasErrors() {
		return nil, res.Diagnostics, nil
	}
	return res.Files, res.Diagnostics, nil
}

// goBuild builds the generated files into dir/bin and removes the build
// directory afterwards. It returns the binary's path.
func goBuild(ctx context.Context, dir string, files []codegen.File) (string, error) {
	buildDir := filepath.Join(dir, "build")
	defer os.RemoveAll(buildDir)
	if err := build.Prepare(buildDir, files); err != nil {
		return "", err
	}
	bin, err := build.Build(ctx, buildDir, build.Options{Output: filepath.Join(dir, "bin", "program"), Release: true})
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("?BUILD ERROR — the build took too long; try again in a moment")
		}
		return "", err
	}
	return bin, nil
}

// warmProgram uses Go interop and the database (decimal is in rt), so that
// building it fills the Go module and build caches for real programs.
const warmProgram = `IMPORT { NewString } FROM GO "github.com/google/uuid"
DBCONNECT ENV("DATABASE_URL")
ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD", "id": NewString()}
END FUNCTION
`

// Warm builds one program in a scratch directory under dataDir, so the
// first RUN after a deploy doesn't pay for downloading modules and compiling
// the runtime (deploy/playground.Dockerfile runs it at image build time).
func Warm(ctx context.Context, dataDir string) error {
	dir, err := os.MkdirTemp(dataDir, "warm-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	files, ds, err := compileProgram(dir, []byte(warmProgram))
	if err != nil {
		return err
	}
	if files == nil {
		return fmt.Errorf("warm-up program does not compile: %s", firstError(ds))
	}
	_, err = goBuild(ctx, dir, files)
	return err
}
