// Package compiler is the entry point to the RIP BASIC compiler:
// .bas → tokens → AST → checks → Go source.
package compiler

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/check"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/lexer"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/parser"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
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
	absEntry, err := filepath.Abs(entryPath)
	if err != nil {
		return nil, err
	}
	absEntry = filepath.Clean(absEntry)
	entryName := filepath.Base(absEntry)

	ld := &loader{
		entryDir: filepath.Dir(absEntry),
		state:    map[string]int{},
	}
	if err := ld.load(absEntry, true, token.Pos{}, "", ""); err != nil {
		return nil, err
	}

	res := &Result{Diagnostics: ld.diags}
	if res.Diagnostics.HasErrors() {
		return res, nil
	}

	prog := &ast.Program{Entry: entryName, Files: ld.files}

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

// loader recursively loads .bas modules in topological dependency order,
// lexing and parsing each file at most once and reporting missing files and
// import cycles as classic BASIC diagnostics.
type loader struct {
	entryDir  string
	state     map[string]int // 0=unvisited, 1=visiting, 2=done
	stackKeys []string
	stackName []string
	files     []*ast.File
	diags     diag.List
}

func (l *loader) load(absPath string, isEntry bool, importAt token.Pos, fromName, rawImport string) error {
	var src []byte
	var err error
	if isEntry {
		src, err = os.ReadFile(absPath)
		if err != nil {
			return err
		}
	} else {
		info, statErr := os.Stat(absPath)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				slashImp := strings.ReplaceAll(rawImport, `\`, "/")
				if !strings.HasSuffix(strings.ToLower(slashImp), ".bas") {
					if _, err2 := os.Stat(absPath + ".bas"); err2 == nil {
						l.diags = append(l.diags, diag.Errorf(importAt, diag.Undefined,
							"cannot find imported file %q; did you mean %q?", rawImport, rawImport+".bas"))
						return nil
					}
				}
				l.diags = append(l.diags, diag.Errorf(importAt, diag.Undefined,
					"cannot find imported file %q relative to %s", rawImport, fromName))
				return nil
			}
			return statErr
		}
		if info.IsDir() {
			l.diags = append(l.diags, diag.Errorf(importAt, diag.Undefined,
				"cannot import directory %q; point IMPORT at a .bas file", rawImport))
			return nil
		}
		src, err = os.ReadFile(absPath)
		if err != nil {
			return err
		}
	}

	name := filepath.Base(absPath)
	if !isEntry {
		name = displayName(l.entryDir, absPath)
	}
	key := fileKey(absPath)

	l.state[key] = 1
	l.stackKeys = append(l.stackKeys, key)
	l.stackName = append(l.stackName, name)
	defer func() {
		l.stackKeys = l.stackKeys[:len(l.stackKeys)-1]
		l.stackName = l.stackName[:len(l.stackName)-1]
		l.state[key] = 2
	}()

	toks, mode, ds := lexer.Lex(name, src)
	l.diags = append(l.diags, ds...)
	if ds.HasErrors() {
		return nil
	}

	f, pds := parser.Parse(name, toks, mode)
	l.diags = append(l.diags, pds...)
	if pds.HasErrors() {
		return nil
	}

	currDir := filepath.Dir(absPath)
	for _, imp := range f.Imports {
		if imp.FromGo {
			continue
		}
		if strings.TrimSpace(imp.Path) == "" {
			l.diags = append(l.diags, diag.Errorf(imp.At, diag.Syntax,
				`IMPORT path cannot be empty; write a relative .bas path like IMPORT "./billing.bas"`))
			continue
		}
		slashImp := strings.ReplaceAll(imp.Path, `\`, "/")
		targetAbs := filepath.Clean(filepath.Join(currDir, filepath.FromSlash(slashImp)))
		targetKey := fileKey(targetAbs)

		switch l.state[targetKey] {
		case 1:
			cycle := l.formatCycle(targetKey)
			l.diags = append(l.diags, diag.Errorf(imp.At, diag.Syntax,
				"circular IMPORT detected (%s); break the cycle by moving shared code into a third .bas file", cycle))
		case 2:
			// Already loaded and appended before this file.
		default:
			if err := l.load(targetAbs, false, imp.At, name, imp.Path); err != nil {
				return err
			}
		}
	}

	l.files = append(l.files, f)
	return nil
}

func (l *loader) formatCycle(targetKey string) string {
	idx := 0
	for i, k := range l.stackKeys {
		if k == targetKey {
			idx = i
			break
		}
	}
	parts := make([]string, 0, len(l.stackName)-idx+1)
	parts = append(parts, l.stackName[idx:]...)
	parts = append(parts, l.stackName[idx])
	return strings.Join(parts, " -> ")
}

func fileKey(absPath string) string {
	clean := filepath.Clean(absPath)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func displayName(entryDir, absPath string) string {
	rel, err := filepath.Rel(entryDir, absPath)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(absPath))
	}
	return filepath.ToSlash(filepath.Clean(rel))
}
