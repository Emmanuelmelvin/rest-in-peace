// Package compiler is the entry point to the RIP BASIC compiler:
// .bas → tokens → AST → checks → Go source.
package compiler

import (
	"os"
	"path/filepath"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
)

// Options control compilation.
type Options struct {
	// Guard compiles in loop guards for the playground.
	Guard bool
}

// Result is the output of a compilation. Files is empty if there were errors.
type Result struct {
	Files       []codegen.File
	Diagnostics diag.List
}

// Compile compiles the program whose entry file is entryPath. An error is
// returned only for I/O problems; BASIC mistakes are in Result.Diagnostics.
func Compile(entryPath string, opts Options) (*Result, error) {
	src, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(entryPath)
	res := &Result{}

	toks, mode, ds := lexer.Lex(name, src)
	res.Diagnostics = append(res.Diagnostics, ds...)
	if ds.HasErrors() {
		return res, nil
	}

	f, ds := parser.Parse(name, toks, mode)
	res.Diagnostics = append(res.Diagnostics, ds...)
	if ds.HasErrors() {
		return res, nil
	}

	// TODO(task 3.4): follow IMPORT "./x.bas" and lex and parse each imported file once.
	prog := &ast.Program{Entry: name, Files: []*ast.File{f}}

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
