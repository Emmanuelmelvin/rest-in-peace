package codegen

import (
	"path"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// basImportState holds per-file state for .bas module imports (task 3.4).
type basImportState struct {
	imports []*ast.Import
}

// basImport handles IMPORT "./x.bas" and IMPORT { … } FROM "./x.bas"
// (Requirements 5.1–5.3, task 3.4), called from gen.generate in codegen.go.
func (g *gen) basImport(im *ast.Import) {
	if g.file.Mode == token.Classic {
		g.errorf(im.At, diag.Syntax, "IMPORT is only supported in structured mode; remove line numbers to use modules")
		return
	}
	g.mods.imports = append(g.mods.imports, im)
}

// moduleExports holds the exported declarations extracted from one .bas file.
type moduleExports struct {
	funcs      map[string]*fnInfo
	consts     map[string]*varInfo
	unexported map[string]string // canonical name -> "FUNCTION", "SUB", or "CONST"
	order      []string          // exported names in source order
}

// bindBasImports binds exported FUNCTION, SUB and CONST declarations from
// imported .bas files into g.funcs and g.globals (without adding them to
// g.funcOrder / g.globOrder so they are not re-emitted). Called from
// structuredFile in structured.go after local declarations are collected.
func (g *gen) bindBasImports() {
	if len(g.mods.imports) == 0 {
		return
	}

	cache := map[*ast.File]*moduleExports{}
	boundFrom := map[string]*ast.File{}

	bindOne := func(name string, pos token.Pos, fromTarget *ast.File, ex *moduleExports) {
		if prev, ok := boundFrom[name]; ok {
			if prev != fromTarget {
				g.errorf(pos, diag.Syntax, "%s is imported from both %s and %s; use named IMPORT { ... } FROM ... to avoid ambiguity", name, prev.Name, fromTarget.Name)
			}
			return
		}
		if g.funcs[name] != nil || g.globals[name] != nil {
			g.errorf(pos, diag.Syntax, "%s is already declared in %s and also imported from %s; rename one of them", name, g.file.Name, fromTarget.Name)
			return
		}
		if fn, ok := ex.funcs[name]; ok {
			g.funcs[name] = fn
			boundFrom[name] = fromTarget
			return
		}
		if v, ok := ex.consts[name]; ok {
			g.globals[name] = v
			boundFrom[name] = fromTarget
		}
	}

	for _, im := range g.mods.imports {
		if strings.TrimSpace(im.Path) == "" {
			g.errorf(im.At, diag.Syntax, `IMPORT path cannot be empty; write a relative .bas path like IMPORT "./billing.bas"`)
			continue
		}
		target := resolveBasImport(g.file, im.Path, g.prog.Files)
		if target == nil {
			g.errorf(im.At, diag.Undefined, "cannot find imported file %q; check the path relative to %s", im.Path, g.file.Name)
			continue
		}
		if target == g.file {
			g.errorf(im.At, diag.Syntax, "circular IMPORT detected (%s -> %s); a file cannot import itself", g.file.Name, g.file.Name)
			continue
		}
		if target.Mode == token.Classic {
			g.errorf(im.At, diag.Syntax, "cannot IMPORT classic-mode file %q; remove line numbers from %s to export declarations", im.Path, target.Name)
			continue
		}

		ex := cache[target]
		if ex == nil {
			ex = g.extractExports(target)
			cache[target] = ex
		}

		if im.Names == nil {
			// Whole-file import: bind all exports.
			for _, name := range ex.order {
				bindOne(name, im.At, target, ex)
			}
			continue
		}
		if len(im.Names) == 0 {
			g.errorf(im.At, diag.Syntax, "IMPORT { } lists no names; list at least one exported name or use IMPORT %q", im.Path)
			continue
		}
		for _, raw := range im.Names {
			name := canon(raw)
			if ex.funcs[name] == nil && ex.consts[name] == nil {
				if kind, ok := ex.unexported[name]; ok {
					g.errorf(im.At, diag.Undefined, "%s is not exported by %s; add EXPORT before %s %s", name, target.Name, kind, name)
				} else {
					g.errorf(im.At, diag.Undefined, "%s has no export named %s", target.Name, name)
				}
				continue
			}
			bindOne(name, im.At, target, ex)
		}
	}
}

// extractExports collects the target module's exported FUNCTION, SUB, and CONST
// metadata using the exact same declaration passes as structuredFile, so mangled
// Go names, native parameter/return types, and Go const vs var representation
// match the code emitted for target identically.
func (g *gen) extractExports(target *ast.File) *moduleExports {
	tg := newGen(g.prog, target, g.opts)
	tg.collectFuncs()
	tg.collectDecls(target.Body, &scope{}, tg.addGlobal)

	ex := &moduleExports{
		funcs:      map[string]*fnInfo{},
		consts:     map[string]*varInfo{},
		unexported: map[string]string{},
	}
	for _, s := range target.Body {
		switch d := s.(type) {
		case *ast.FuncDecl:
			name := canon(d.Name)
			if !d.Export {
				if d.IsSub {
					ex.unexported[name] = "SUB"
				} else {
					ex.unexported[name] = "FUNCTION"
				}
				continue
			}
			if fn := tg.funcs[name]; fn != nil {
				if _, dup := ex.funcs[name]; !dup {
					ex.funcs[name] = fn
					ex.order = append(ex.order, name)
				}
			}
		case *ast.ConstDecl:
			name := canon(d.Name)
			if !d.Export {
				ex.unexported[name] = "CONST"
				continue
			}
			if v := tg.globals[name]; v != nil {
				if _, dup := ex.consts[name]; !dup {
					ex.consts[name] = v
					ex.order = append(ex.order, name)
				}
			}
		}
	}
	return ex
}

// resolveBasImport matches impPath (relative to from.Name) against loaded files
// in files, using cleaned slash paths with case-insensitive and unique basename
// fallbacks.
func resolveBasImport(from *ast.File, impPath string, files []*ast.File) *ast.File {
	if strings.TrimSpace(impPath) == "" {
		return nil
	}
	fromNorm := normBasPath(from.Name)
	impNorm := strings.ReplaceAll(impPath, `\`, "/")
	var target string
	if path.IsAbs(impNorm) {
		target = path.Clean(impNorm)
	} else {
		target = path.Clean(path.Join(path.Dir(fromNorm), impNorm))
	}

	// Pass 1: exact normalized path match.
	for _, f := range files {
		if f != nil && normBasPath(f.Name) == target {
			return f
		}
	}
	// Pass 2: unique case-insensitive path match.
	var match *ast.File
	count := 0
	for _, f := range files {
		if f != nil && strings.EqualFold(normBasPath(f.Name), target) {
			match = f
			count++
		}
	}
	if count == 1 {
		return match
	}
	// Pass 3: unique basename fallback.
	wantBase := path.Base(target)
	match, count = nil, 0
	for _, f := range files {
		if f != nil && strings.EqualFold(path.Base(normBasPath(f.Name)), wantBase) {
			match = f
			count++
		}
	}
	if count == 1 {
		return match
	}
	return nil
}

func normBasPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, `\`, "/"))
}
