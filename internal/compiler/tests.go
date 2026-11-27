package compiler

import (
	"path/filepath"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// CompileTests compiles the program whose entry file is entryPath together
// with the test files testPaths (tests/*.test.bas) for rip test
// (Requirement 12). The entry file and its imports come first, so their
// top-level code (DBCONNECT, USE, ROUTE, SERVE, …) runs before the test
// files register their TESTs. Test files are named relative to the entry
// file's directory, like "tests/main.test.bas", in diagnostics and in
// runtime errors. As with Compile, an error is returned only for I/O
// problems.
func CompileTests(entryPath string, testPaths []string, opts Options) (*Result, error) {
	absEntry, err := filepath.Abs(entryPath)
	if err != nil {
		return nil, err
	}
	absEntry = filepath.Clean(absEntry)

	ld := &loader{
		entryDir: filepath.Dir(absEntry),
		state:    map[string]int{},
	}
	if err := ld.load(absEntry, true, token.Pos{}, "", ""); err != nil {
		return nil, err
	}
	for _, tp := range testPaths {
		abs, err := filepath.Abs(tp)
		if err != nil {
			return nil, err
		}
		abs = filepath.Clean(abs)
		if ld.state[fileKey(abs)] != 0 {
			continue // already loaded, e.g. listed twice
		}
		if err := ld.load(abs, false, token.Pos{}, filepath.Base(absEntry), filepath.ToSlash(tp)); err != nil {
			return nil, err
		}
	}

	res := &Result{Diagnostics: ld.diags}
	if res.Diagnostics.HasErrors() {
		return res, nil
	}

	prog := &ast.Program{Entry: filepath.Base(absEntry), Files: ld.files}
	res.Diagnostics = append(res.Diagnostics, check.Check(prog)...)
	if res.Diagnostics.HasErrors() {
		return res, nil
	}

	files, ds := codegen.Generate(prog, codegen.Options{Guard: opts.Guard})
	res.Diagnostics = append(res.Diagnostics, ds...)
	if ds.HasErrors() {
		return res, nil
	}
	res.Files = files
	return res, nil
}
