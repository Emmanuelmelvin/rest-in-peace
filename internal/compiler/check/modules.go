package check

import (
	"path"
	"sort"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// checkModules validates IMPORT and EXPORT across .bas files in p
// (Requirements 5.1–5.3, task 3.4).
//
// Called from Check in check.go.
func checkModules(p *ast.Program) diag.List {
	if p == nil || len(p.Files) == 0 {
		return nil
	}

	indices := make(map[*ast.File]*fileIndex, len(p.Files))
	for _, f := range p.Files {
		if f == nil {
			continue
		}
		indices[f] = indexFile(f)
	}

	var all diag.List

	// Detect circular .bas imports when Check is called directly (compiler.Compile
	// also catches cycles during loading before Check runs).
	all = append(all, checkImportCycles(p.Files)...)

	for _, f := range p.Files {
		if f == nil || len(f.Imports) == 0 {
			continue
		}
		mc := &modChecker{
			prog:      p,
			file:      f,
			fidx:      indices[f],
			indices:   indices,
			imported:  map[string]*importedSym{},
			goImports: map[string]bool{},
		}
		mc.check()
		sort.SliceStable(mc.diags, func(i, j int) bool {
			if mc.diags[i].Pos.Line != mc.diags[j].Pos.Line {
				return mc.diags[i].Pos.Line < mc.diags[j].Pos.Line
			}
			return mc.diags[i].Pos.Col < mc.diags[j].Pos.Col
		})
		all = append(all, mc.diags...)
	}

	return all
}

type exportDecl struct {
	name   string
	kind   symKind
	export bool
	pos    token.Pos
	fn     *ast.FuncDecl
	cnst   *ast.ConstDecl
}

type fileIndex struct {
	file       *ast.File
	decls      map[string]*exportDecl // all top-level declarations & vars
	exports    map[string]*exportDecl // exported FUNCTION, SUB, CONST
	exportList []string               // sorted exported names
	localDecls map[string]*exportDecl // top-level FUNCTION, SUB, CONST, DIM
}

func indexFile(f *ast.File) *fileIndex {
	idx := &fileIndex{
		file:       f,
		decls:      map[string]*exportDecl{},
		exports:    map[string]*exportDecl{},
		localDecls: map[string]*exportDecl{},
	}
	if f.Mode == token.Classic {
		return idx
	}
	for _, s := range f.Body {
		switch d := s.(type) {
		case *ast.FuncDecl:
			name := strings.ToUpper(d.Name)
			if name == "" {
				continue
			}
			kind := symFunc
			if d.IsSub {
				kind = symSub
			}
			ed := &exportDecl{name: name, kind: kind, export: d.Export, pos: d.At, fn: d}
			if _, exists := idx.decls[name]; !exists {
				idx.decls[name] = ed
			}
			if _, exists := idx.localDecls[name]; !exists {
				idx.localDecls[name] = ed
			}
			if d.Export {
				if _, exists := idx.exports[name]; !exists {
					idx.exports[name] = ed
					idx.exportList = append(idx.exportList, name)
				}
			}
		case *ast.ConstDecl:
			name := strings.ToUpper(d.Name)
			if name == "" {
				continue
			}
			ed := &exportDecl{name: name, kind: symConst, export: d.Export, pos: d.At, cnst: d}
			if _, exists := idx.decls[name]; !exists {
				idx.decls[name] = ed
			}
			if _, exists := idx.localDecls[name]; !exists {
				idx.localDecls[name] = ed
			}
			if d.Export {
				if _, exists := idx.exports[name]; !exists {
					idx.exports[name] = ed
					idx.exportList = append(idx.exportList, name)
				}
			}
		case *ast.DimStmt:
			name := strings.ToUpper(d.Name)
			if name == "" {
				continue
			}
			ed := &exportDecl{name: name, kind: symVar, pos: d.At}
			if _, exists := idx.decls[name]; !exists {
				idx.decls[name] = ed
			}
			if _, exists := idx.localDecls[name]; !exists {
				idx.localDecls[name] = ed
			}
		}
	}
	collectTopLevelVars(f.Body, idx.decls)
	sort.Strings(idx.exportList)
	return idx
}

func collectTopLevelVars(stmts []ast.Stmt, decls map[string]*exportDecl) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.LetStmt:
			if id, ok := st.Target.(*ast.Ident); ok {
				name := strings.ToUpper(id.Name)
				if _, exists := decls[name]; !exists {
					decls[name] = &exportDecl{name: name, kind: symVar, pos: st.At}
				}
			}
		case *ast.ForStmt:
			name := strings.ToUpper(st.Var)
			if name != "" {
				if _, exists := decls[name]; !exists {
					decls[name] = &exportDecl{name: name, kind: symVar, pos: st.At}
				}
			}
			collectTopLevelVars(st.Body, decls)
		case *ast.ForEachStmt:
			name := strings.ToUpper(st.Var)
			if name != "" {
				if _, exists := decls[name]; !exists {
					decls[name] = &exportDecl{name: name, kind: symVar, pos: st.At}
				}
			}
			collectTopLevelVars(st.Body, decls)
		case *ast.IfStmt:
			collectTopLevelVars(st.Then, decls)
			collectTopLevelVars(st.Else, decls)
		case *ast.WhileStmt:
			collectTopLevelVars(st.Body, decls)
		case *ast.DoStmt:
			collectTopLevelVars(st.Body, decls)
		case *ast.SelectStmt:
			for _, sc := range st.Cases {
				collectTopLevelVars(sc.Body, decls)
			}
			collectTopLevelVars(st.Else, decls)
		case *ast.TryStmt:
			collectTopLevelVars(st.Body, decls)
			collectTopLevelVars(st.Catch, decls)
			collectTopLevelVars(st.Finally, decls)
		case *ast.TransactionStmt:
			collectTopLevelVars(st.Body, decls)
		}
	}
}

func checkImportCycles(files []*ast.File) diag.List {
	var ds diag.List
	state := map[*ast.File]int{} // 0=unvisited, 1=visiting, 2=done
	var stack []*ast.File

	var visit func(f *ast.File)
	visit = func(f *ast.File) {
		state[f] = 1
		stack = append(stack, f)
		for _, imp := range f.Imports {
			if imp.FromGo {
				continue
			}
			target := resolveBasImport(f, imp.Path, files)
			if target == nil {
				continue
			}
			switch state[target] {
			case 1:
				idx := 0
				for i, sf := range stack {
					if sf == target {
						idx = i
						break
					}
				}
				names := make([]string, 0, len(stack)-idx+1)
				for _, sf := range stack[idx:] {
					names = append(names, sf.Name)
				}
				names = append(names, target.Name)
				ds = append(ds, diag.Errorf(imp.At, diag.Syntax,
					"circular IMPORT detected (%s); break the cycle by moving shared code into a third .bas file",
					strings.Join(names, " -> ")))
			case 0:
				visit(target)
			}
		}
		stack = stack[:len(stack)-1]
		state[f] = 2
	}

	for _, f := range files {
		if f != nil && state[f] == 0 {
			visit(f)
		}
	}
	return ds
}

// resolveBasImport matches impPath (resolved relative to from.Name) to a loaded
// *ast.File in files, normalizing path separators for cross-platform safety.
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
	// Pass 2: case-insensitive path match (unique).
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
	// Pass 3: unique basename fallback (e.g. cross-parent relative paths or
	// synthetic test ASTs).
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

type importedSym struct {
	name      string
	decl      *exportDecl
	fromFile  *ast.File
	importPos token.Pos
	isNamed   bool
}

type modChecker struct {
	prog          *ast.Program
	file          *ast.File
	fidx          *fileIndex
	indices       map[*ast.File]*fileIndex
	imported      map[string]*importedSym
	goImports     map[string]bool
	wildTargets   []*fileIndex
	hasBasImport  bool
	hasWild       bool
	hasMissingImp bool
	diags         diag.List

	modScope *scope
	curScope *scope
}

func (m *modChecker) errorf(pos token.Pos, kind diag.Kind, format string, args ...any) {
	m.diags = append(m.diags, diag.Errorf(pos, kind, format, args...))
}

func (m *modChecker) check() {
	for _, imp := range m.file.Imports {
		if imp.FromGo {
			if imp.Alias != "" {
				m.goImports[strings.ToUpper(imp.Alias)] = true
			}
			for _, n := range imp.Names {
				m.goImports[strings.ToUpper(n)] = true
			}
			continue
		}
		m.hasBasImport = true

		if m.file.Mode == token.Classic {
			m.errorf(imp.At, diag.Syntax, "IMPORT is only supported in structured mode; remove line numbers to use modules")
			continue
		}
		if strings.TrimSpace(imp.Path) == "" {
			m.hasMissingImp = true
			m.errorf(imp.At, diag.Syntax, `IMPORT path cannot be empty; write a relative .bas path like IMPORT "./billing.bas"`)
			continue
		}

		target := resolveBasImport(m.file, imp.Path, m.prog.Files)
		if target == nil {
			m.hasMissingImp = true
			m.errorf(imp.At, diag.Undefined, "cannot find imported file %q; check the path relative to %s", imp.Path, m.file.Name)
			continue
		}
		if target == m.file {
			// Cycle already reported by checkImportCycles.
			continue
		}
		if target.Mode == token.Classic {
			m.errorf(imp.At, diag.Syntax, "cannot IMPORT classic-mode file %q; remove line numbers from %s to export declarations", imp.Path, target.Name)
			continue
		}

		tidx := m.indices[target]
		if imp.Names == nil {
			// Whole-file import: IMPORT "./billing.bas"
			m.hasWild = true
			m.wildTargets = append(m.wildTargets, tidx)
			for _, name := range tidx.exportList {
				ed := tidx.exports[name]
				if prev, dup := m.imported[name]; dup {
					if prev.fromFile != target {
						m.errorf(imp.At, diag.Syntax, "%s is imported from both %s and %s; use named IMPORT { ... } FROM ... to avoid ambiguity", name, prev.fromFile.Name, target.Name)
					}
					continue
				}
				if loc := m.fidx.localDecls[name]; loc != nil {
					m.errorf(loc.pos, diag.Syntax, "%s is already imported from %s; rename it or use named IMPORT { ... } FROM %q", name, target.Name, imp.Path)
					continue
				}
				m.imported[name] = &importedSym{
					name:      name,
					decl:      ed,
					fromFile:  target,
					importPos: imp.At,
					isNamed:   false,
				}
			}
			continue
		}

		if len(imp.Names) == 0 {
			// IMPORT { } FROM "./x.bas": check.go sets hasWild = true when len(Names) == 0.
			m.hasWild = true
			m.errorf(imp.At, diag.Syntax, "IMPORT { } lists no names; list at least one exported name or use IMPORT %q", imp.Path)
			continue
		}

		seenInThis := map[string]bool{}
		for _, raw := range imp.Names {
			name := strings.ToUpper(raw)
			if seenInThis[name] {
				m.errorf(imp.At, diag.Syntax, "%s is listed more than once in IMPORT from %s", name, target.Name)
				continue
			}
			seenInThis[name] = true

			ed := tidx.exports[name]
			if ed == nil {
				m.reportMissingExport(imp.At, name, tidx)
				continue
			}
			if prev, dup := m.imported[name]; dup {
				if prev.fromFile != target {
					m.errorf(imp.At, diag.Syntax, "%s is imported from both %s and %s; remove one of the imports", name, prev.fromFile.Name, target.Name)
				} else {
					m.errorf(imp.At, diag.Syntax, "%s is already imported from %s at line %d", name, target.Name, prev.importPos.Line)
				}
				continue
			}
			// Collisions with local FuncDecl/ConstDecl/DimStmt are already reported by check.go
			// because registerImports defines named imports into modScope with declared: true.
			m.imported[name] = &importedSym{
				name:      name,
				decl:      ed,
				fromFile:  target,
				importPos: imp.At,
				isNamed:   true,
			}
		}
	}

	if !m.hasBasImport || m.file.Mode == token.Classic {
		return
	}

	m.checkUsages()
}

func (m *modChecker) reportMissingExport(pos token.Pos, name string, tidx *fileIndex) {
	if unexp := tidx.decls[name]; unexp != nil {
		if unexp.kind == symVar {
			m.errorf(pos, diag.Undefined, "%s is a variable in %s, not an exported declaration; only EXPORT FUNCTION, EXPORT SUB and EXPORT CONST can be imported", name, tidx.file.Name)
		} else {
			m.errorf(pos, diag.Undefined, "%s is not exported by %s; add EXPORT before %s %s", name, tidx.file.Name, kindWord(unexp.kind), name)
		}
		return
	}
	if hint := suggest(name, tidx.exportList); hint != "" {
		m.errorf(pos, diag.Undefined, "%s has no export named %s; did you mean %s?", tidx.file.Name, name, hint)
		return
	}
	m.errorf(pos, diag.Undefined, "%s has no export named %s", tidx.file.Name, name)
}

func (m *modChecker) checkUsages() {
	m.modScope = newScope(nil)
	m.curScope = m.modScope

	// Pass 1: register top-level functions and subs.
	for _, s := range m.file.Body {
		fn, ok := s.(*ast.FuncDecl)
		if !ok || fn.Name == "" {
			continue
		}
		kind := symFunc
		if fn.IsSub {
			kind = symSub
		}
		name := strings.ToUpper(fn.Name)
		if m.modScope.lookupLocal(name) == nil {
			m.modScope.define(&symbol{name: name, kind: kind, pos: fn.At, declared: true, fn: fn})
		}
	}

	// Pass 2: check top-level statements in source order.
	for _, s := range m.file.Body {
		switch s.(type) {
		case *ast.FuncDecl, *ast.TestDecl:
			continue
		}
		m.walkStmt(s)
	}

	// Pass 3: check function, sub, and test bodies.
	for _, s := range m.file.Body {
		switch d := s.(type) {
		case *ast.FuncDecl:
			m.walkFuncBody(d)
		case *ast.TestDecl:
			m.walkTestBody(d)
		}
	}
}

func (m *modChecker) walkFuncBody(fn *ast.FuncDecl) {
	local := newScope(m.modScope)
	for _, prm := range fn.Params {
		if prm.Default != nil {
			prev := m.curScope
			m.curScope = m.modScope
			m.walkExpr(prm.Default)
			m.curScope = prev
		}
		name := strings.ToUpper(prm.Name)
		if local.lookupLocal(name) == nil {
			local.define(&symbol{name: name, kind: symParam, pos: fn.At, declared: true})
		}
	}
	prev := m.curScope
	m.curScope = local
	for _, s := range fn.Body {
		m.walkStmt(s)
	}
	m.curScope = prev
}

func (m *modChecker) walkTestBody(td *ast.TestDecl) {
	prev := m.curScope
	m.curScope = newScope(m.modScope)
	for _, s := range td.Body {
		m.walkStmt(s)
	}
	m.curScope = prev
}

func (m *modChecker) walkStmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch st := s.(type) {
	case *ast.ConstDecl:
		m.walkExpr(st.Value)
		name := strings.ToUpper(st.Name)
		if m.curScope.lookupLocal(name) == nil {
			m.curScope.define(&symbol{name: name, kind: symConst, pos: st.At, declared: true})
		}
	case *ast.DimStmt:
		if st.Value != nil {
			m.walkExpr(st.Value)
		}
		name := strings.ToUpper(st.Name)
		if m.curScope.lookupLocal(name) == nil {
			m.curScope.define(&symbol{name: name, kind: symVar, pos: st.At, declared: true})
		}
	case *ast.LetStmt:
		m.walkExpr(st.Value)
		m.walkAssignTarget(st.At, st.Target)
	case *ast.PrintStmt:
		for _, a := range st.Args {
			m.walkExpr(a)
		}
	case *ast.ExprStmt:
		m.walkCallOrExpr(st.X, true)
	case *ast.IfStmt:
		m.walkExpr(st.Cond)
		for _, b := range st.Then {
			m.walkStmt(b)
		}
		for _, b := range st.Else {
			m.walkStmt(b)
		}
	case *ast.ForStmt:
		m.walkExpr(st.From)
		m.walkExpr(st.To)
		if st.Step != nil {
			m.walkExpr(st.Step)
		}
		m.walkVarAssign(st.At, strings.ToUpper(st.Var))
		for _, b := range st.Body {
			m.walkStmt(b)
		}
	case *ast.ForEachStmt:
		m.walkExpr(st.In)
		m.walkVarAssign(st.At, strings.ToUpper(st.Var))
		for _, b := range st.Body {
			m.walkStmt(b)
		}
	case *ast.WhileStmt:
		m.walkExpr(st.Cond)
		for _, b := range st.Body {
			m.walkStmt(b)
		}
	case *ast.DoStmt:
		for _, b := range st.Body {
			m.walkStmt(b)
		}
		if st.Until != nil {
			m.walkExpr(st.Until)
		}
	case *ast.SelectStmt:
		m.walkExpr(st.X)
		for _, sc := range st.Cases {
			for _, v := range sc.Values {
				m.walkExpr(v)
			}
			for _, b := range sc.Body {
				m.walkStmt(b)
			}
		}
		for _, b := range st.Else {
			m.walkStmt(b)
		}
	case *ast.ReturnStmt:
		if st.Value != nil {
			m.walkExpr(st.Value)
		}
	case *ast.ThrowStmt:
		m.walkExpr(st.Value)
	case *ast.TryStmt:
		for _, b := range st.Body {
			m.walkStmt(b)
		}
		if st.CatchVar != "" || len(st.Catch) > 0 {
			prev := m.curScope
			catchScope := newScope(prev)
			if st.CatchVar != "" {
				catchScope.define(&symbol{name: strings.ToUpper(st.CatchVar), kind: symVar, pos: st.At, declared: true})
			}
			m.curScope = catchScope
			for _, b := range st.Catch {
				m.walkStmt(b)
			}
			m.curScope = prev
		}
		for _, b := range st.Finally {
			m.walkStmt(b)
		}
	case *ast.RouteStmt:
		if st.Handler != "" {
			m.checkHandlerName(st.At, strings.ToUpper(st.Handler))
		}
	case *ast.GroupStmt:
		for _, r := range st.Routes {
			if r.Handler != "" {
				m.checkHandlerName(r.At, strings.ToUpper(r.Handler))
			}
		}
	case *ast.UseStmt:
		switch fn := st.Fn.(type) {
		case *ast.Ident:
			name := strings.ToUpper(fn.Name)
			if _, isBuilt := middlewareBuiltins[name]; !isBuilt {
				m.checkHandlerName(fn.At, name)
			}
		case *ast.CallExpr:
			for _, a := range fn.Args {
				m.walkExpr(a)
			}
		}
	case *ast.RespondStmt:
		m.walkExpr(st.Status)
		m.walkExpr(st.Value)
		if st.Headers != nil {
			m.walkExpr(st.Headers)
		}
	case *ast.ServeStmt:
		if st.Port != nil {
			m.walkExpr(st.Port)
		}
	case *ast.DBConnectStmt:
		m.walkExpr(st.URL)
	case *ast.TransactionStmt:
		for _, b := range st.Body {
			m.walkStmt(b)
		}
	case *ast.LogStmt:
		if id, ok := st.Level.(*ast.Ident); ok && logLevels[strings.ToUpper(id.Name)] && m.curScope.lookup(strings.ToUpper(id.Name)) == nil && m.imported[strings.ToUpper(id.Name)] == nil {
			// Bare log level name such as LOG INFO, "msg".
		} else {
			m.walkExpr(st.Level)
		}
		m.walkExpr(st.Msg)
		if st.Fields != nil {
			m.walkExpr(st.Fields)
		}
	case *ast.ExpectStmt:
		m.walkExpr(st.Cond)
		if st.Msg != nil {
			m.walkExpr(st.Msg)
		}
	}
}

func (m *modChecker) walkAssignTarget(pos token.Pos, target ast.Expr) {
	switch t := target.(type) {
	case *ast.Ident:
		m.walkVarAssign(pos, strings.ToUpper(t.Name))
	case *ast.MemberExpr:
		m.walkExpr(t.X)
	case *ast.IndexExpr:
		m.walkExpr(t.X)
		m.walkExpr(t.Index)
	default:
		m.walkExpr(target)
	}
}

func (m *modChecker) walkVarAssign(pos token.Pos, name string) {
	if name == "" {
		return
	}
	if m.curScope.lookupLocal(name) == nil {
		if imp := m.imported[name]; imp != nil {
			switch imp.decl.kind {
			case symConst:
				m.errorf(pos, diag.Syntax, "cannot reassign imported CONST %s (from %s)", name, imp.fromFile.Name)
				return
			case symFunc, symSub:
				m.errorf(pos, diag.Syntax, "cannot assign to %s because it is an imported %s (from %s)", name, kindWord(imp.decl.kind), imp.fromFile.Name)
				return
			}
		}
		m.curScope.define(&symbol{name: name, kind: symVar, pos: pos})
	}
}

func (m *modChecker) walkExpr(e ast.Expr) {
	m.walkCallOrExpr(e, false)
}

func (m *modChecker) walkCallOrExpr(e ast.Expr, asStmt bool) {
	if e == nil {
		return
	}
	switch ex := e.(type) {
	case *ast.Ident:
		m.checkIdent(ex)
	case *ast.ArrayLit:
		for _, item := range ex.Items {
			m.walkExpr(item)
		}
	case *ast.MapLit:
		for _, v := range ex.Values {
			m.walkExpr(v)
		}
	case *ast.MemberExpr:
		m.walkExpr(ex.X)
	case *ast.IndexExpr:
		m.walkExpr(ex.X)
		m.walkExpr(ex.Index)
	case *ast.UnaryExpr:
		m.walkExpr(ex.X)
	case *ast.BinaryExpr:
		m.walkExpr(ex.X)
		m.walkExpr(ex.Y)
	case *ast.CallExpr:
		m.checkCall(ex, asStmt)
	}
}

func (m *modChecker) checkIdent(id *ast.Ident) {
	if !m.hasWild || m.hasMissingImp {
		return
	}
	name := strings.ToUpper(id.Name)
	if m.curScope.lookup(name) != nil || m.imported[name] != nil || m.goImports[name] || builtins[name] {
		return
	}
	if unexp, from := m.findUnexportedInWild(name); unexp != nil {
		if unexp.kind == symVar {
			m.errorf(id.At, diag.Undefined, "%s is a variable in %s, not an exported declaration; only EXPORT FUNCTION, EXPORT SUB and EXPORT CONST can be imported", name, from.Name)
		} else {
			m.errorf(id.At, diag.Undefined, "%s is not exported by %s; add EXPORT before %s %s", name, from.Name, kindWord(unexp.kind), name)
		}
		return
	}
	if hint := suggest(name, m.valueNames()); hint != "" {
		m.errorf(id.At, diag.Undefined, "%s is not defined; did you mean %s?", name, hint)
		return
	}
	m.errorf(id.At, diag.Undefined, "%s is not defined", name)
}

func (m *modChecker) checkCall(call *ast.CallExpr, asStmt bool) {
	for _, a := range call.Args {
		m.walkExpr(a)
	}
	id, ok := call.Fn.(*ast.Ident)
	if !ok {
		m.walkExpr(call.Fn)
		return
	}
	name := strings.ToUpper(id.Name)
	if m.curScope.lookup(name) != nil {
		return
	}
	if imp := m.imported[name]; imp != nil {
		switch imp.decl.kind {
		case symConst:
			m.errorf(call.At, diag.TypeMismatch, "%s is a CONST imported from %s, not a function", name, imp.fromFile.Name)
		case symFunc, symSub:
			if imp.decl.kind == symSub && !asStmt {
				m.errorf(call.At, diag.TypeMismatch, "SUB %s (imported from %s) returns no value; call it as a statement or make it a FUNCTION", name, imp.fromFile.Name)
			}
			m.checkImportedArity(call, imp.decl.fn)
		}
		return
	}
	if !m.hasWild || m.hasMissingImp {
		return
	}
	if m.goImports[name] || builtins[name] {
		return
	}
	if alt, ok := classicBuiltins[name]; ok {
		m.errorf(call.At, diag.Undefined, "%s is only available in classic mode; in structured mode read %s", name, alt)
		return
	}
	if unexp, from := m.findUnexportedInWild(name); unexp != nil {
		if unexp.kind == symVar {
			m.errorf(call.At, diag.Undefined, "%s is a variable in %s, not an exported declaration; only EXPORT FUNCTION, EXPORT SUB and EXPORT CONST can be imported", name, from.Name)
		} else {
			m.errorf(call.At, diag.Undefined, "%s is not exported by %s; add EXPORT before %s %s", name, from.Name, kindWord(unexp.kind), name)
		}
		return
	}
	if hint := suggest(name, m.callableNames()); hint != "" {
		m.errorf(call.At, diag.Undefined, "no function named %s; did you mean %s?", name, hint)
		return
	}
	m.errorf(call.At, diag.Undefined, "no function named %s", name)
}

func (m *modChecker) checkHandlerName(pos token.Pos, name string) {
	if m.modScope.lookup(name) != nil {
		return
	}
	if imp := m.imported[name]; imp != nil {
		switch imp.decl.kind {
		case symFunc, symSub:
			if imp.decl.fn != nil && len(imp.decl.fn.Params) != 1 {
				m.errorf(pos, diag.Syntax, "%s (imported from %s) takes %d parameter(s), so it cannot be a handler or middleware; expected 1 parameter (req)", name, imp.fromFile.Name, len(imp.decl.fn.Params))
			}
		default:
			m.errorf(pos, diag.TypeMismatch, "%s is a CONST imported from %s, not a function", name, imp.fromFile.Name)
		}
		return
	}
	if !m.hasWild || m.hasMissingImp || m.goImports[name] {
		return
	}
	if unexp, from := m.findUnexportedInWild(name); unexp != nil {
		m.errorf(pos, diag.Undefined, "%s is not exported by %s; add EXPORT before %s %s", name, from.Name, kindWord(unexp.kind), name)
		return
	}
	if hint := suggest(name, m.callableNames()); hint != "" {
		m.errorf(pos, diag.Undefined, "no function named %s; did you mean %s?", name, hint)
		return
	}
	m.errorf(pos, diag.Undefined, "no function named %s", name)
}

func (m *modChecker) checkImportedArity(call *ast.CallExpr, fn *ast.FuncDecl) {
	if fn == nil {
		return
	}
	minArgs := 0
	for _, p := range fn.Params {
		if p.Default == nil {
			minArgs++
		}
	}
	maxArgs := len(fn.Params)
	got := len(call.Args)
	if got >= minArgs && got <= maxArgs {
		return
	}
	if minArgs == maxArgs {
		m.errorf(call.At, diag.Syntax, "%s expects %s, got %d", fn.Name, pluralArgs(minArgs), got)
		return
	}
	if got < minArgs {
		m.errorf(call.At, diag.Syntax, "%s expects at least %s, got %d", fn.Name, pluralArgs(minArgs), got)
		return
	}
	m.errorf(call.At, diag.Syntax, "%s expects at most %s, got %d", fn.Name, pluralArgs(maxArgs), got)
}

func (m *modChecker) findUnexportedInWild(name string) (*exportDecl, *ast.File) {
	for _, wt := range m.wildTargets {
		if ed := wt.decls[name]; ed != nil && !ed.export {
			return ed, wt.file
		}
	}
	return nil, nil
}

func (m *modChecker) callableNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for s := m.curScope; s != nil; s = s.parent {
		for name, sym := range s.syms {
			if sym.kind == symFunc || sym.kind == symSub {
				add(name)
			}
		}
	}
	for name, imp := range m.imported {
		if imp.decl.kind == symFunc || imp.decl.kind == symSub {
			add(name)
		}
	}
	for name := range m.goImports {
		add(name)
	}
	for name := range builtins {
		add(name)
	}
	return out
}

func (m *modChecker) valueNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for s := m.curScope; s != nil; s = s.parent {
		for name := range s.syms {
			add(name)
		}
	}
	for name := range m.imported {
		add(name)
	}
	for name := range m.goImports {
		add(name)
	}
	return out
}
