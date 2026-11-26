package codegen

import (
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/gointerop"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

type goImportEntry struct {
	path    string
	goAlias string
	pkg     *gointerop.Package
	used    bool
}

type goNamedSym struct {
	entry *goImportEntry
	obj   types.Object
	sig   *gointerop.FuncSig // non-nil when obj is *types.Func
}

// goImportState holds per-file state for IMPORT … FROM GO (task 3.5).
type goImportState struct {
	byPath  map[string]*goImportEntry // Go import path → entry
	byAlias map[string]*goImportEntry // canonical BASIC alias (UPPER) → entry
	named   map[string]goNamedSym     // canonical BASIC symbol name (UPPER) → symbol
	usedID  map[string]bool           // allocated Go import identifiers
}

func (g *gen) initGoImps() {
	if g.goImps.byPath == nil {
		g.goImps.byPath = map[string]*goImportEntry{}
		g.goImps.byAlias = map[string]*goImportEntry{}
		g.goImps.named = map[string]goNamedSym{}
		g.goImps.usedID = map[string]bool{}
	}
}

func (g *gen) allocGoImport(path, preferred string, pkg *gointerop.Package) *goImportEntry {
	g.initGoImps()
	if existing, ok := g.goImps.byPath[path]; ok {
		if existing.pkg == nil && pkg != nil {
			existing.pkg = pkg
		}
		return existing
	}
	base := sanitize(strings.ToLower(preferred))
	if base == "" {
		base = "pkg"
	}
	goAlias := "go_" + base
	for i := 2; g.goImps.usedID[goAlias]; i++ {
		goAlias = "go_" + base + "_" + strconv.Itoa(i)
	}
	g.goImps.usedID[goAlias] = true
	entry := &goImportEntry{
		path:    path,
		goAlias: goAlias,
		pkg:     pkg,
	}
	g.goImps.byPath[path] = entry
	return entry
}

func (g *gen) ensureGoImport(importPath, pkgName string) string {
	entry := g.allocGoImport(importPath, pkgName, nil)
	entry.used = true
	return entry.goAlias
}

// goImport handles IMPORT alias FROM GO "pkg" and IMPORT { … } FROM GO "pkg"
// (Requirements 5.4–5.7, design §4.10, task 3.5), called from gen.generate.
func (g *gen) goImport(im *ast.Import) {
	g.initGoImps()
	if strings.TrimSpace(im.Path) == "" {
		g.errorf(im.At, diag.Syntax, "IMPORT … FROM GO requires a non-empty Go package path")
		return
	}
	pkg, err := gointerop.Load(im.Path, "")
	if err != nil {
		g.errorf(im.At, diag.Undefined, "cannot load Go package %q: %v — check the package path and make sure it is available in go.mod", im.Path, err)
		return
	}
	pref := im.Alias
	if pref == "" {
		pref = pkg.Name()
	}
	entry := g.allocGoImport(pkg.Path, pref, pkg)
	if im.Alias != "" {
		g.goImps.byAlias[canon(im.Alias)] = entry
	}
	for _, name := range im.Names {
		obj := pkg.Lookup(name)
		if obj == nil {
			if hint := pkg.SuggestName(name); hint != "" {
				g.errorf(im.At, diag.Undefined, "Go package %q has no exported symbol %q; did you mean %q?", im.Path, name, hint)
			} else {
				g.errorf(im.At, diag.Undefined, "Go package %q has no exported symbol %q", im.Path, name)
			}
			continue
		}
		sym := goNamedSym{entry: entry, obj: obj}
		if fn, ok := obj.(*types.Func); ok {
			sig, err := gointerop.AnalyzeFunc(fn)
			if err != nil {
				g.errorf(im.At, diag.TypeMismatch, "%v", err)
				continue
			}
			sym.sig = sig
		}
		g.goImps.named[canon(name)] = sym
	}
}

// extraGoImports returns extra Go import lines (e.g. `go_uuid "github.com/google/uuid"`)
// to emit in the generated file's import block. Only packages actually referenced
// during code generation are emitted, so unused BASIC imports never fail Go compilation.
func (g *gen) extraGoImports() []string {
	if g.goImps == nil || len(g.goImps.byPath) == 0 {
		return nil
	}
	var used []*goImportEntry
	for _, entry := range g.goImps.byPath {
		if entry.used {
			used = append(used, entry)
		}
	}
	if len(used) == 0 {
		return nil
	}
	sort.Slice(used, func(i, j int) bool {
		return used[i].path < used[j].path
	})
	out := make([]string, len(used))
	for i, e := range used {
		out[i] = e.goAlias + " " + strconv.Quote(e.path)
	}
	return out
}

func (g *gen) goAliasEntry(id *ast.Ident, sc *scope) (*goImportEntry, bool) {
	if g.goImps == nil || len(g.goImps.byAlias) == 0 {
		return nil, false
	}
	name := canon(id.Name)
	if sc != nil && sc.fn != nil && sc.fn.locals[name] != nil {
		return nil, false
	}
	if g.file.Mode == token.Structured && g.globals[name] != nil {
		return nil, false
	}
	entry, ok := g.goImps.byAlias[name]
	return entry, ok
}

// goMember handles member access on a Go package alias (e.g. pkg.Const, pkg.Var).
// Returns (expr, true) when e.X is a Go package alias, or (bad, false) to
// fall through to normal rt.Get member access.
func (g *gen) goMember(e *ast.MemberExpr, sc *scope) (gexpr, bool) {
	id, ok := e.X.(*ast.Ident)
	if !ok {
		return bad, false
	}
	entry, ok := g.goAliasEntry(id, sc)
	if !ok {
		return bad, false
	}
	obj := entry.pkg.Lookup(e.Name)
	if obj == nil {
		if hint := entry.pkg.SuggestName(e.Name); hint != "" {
			g.errorf(e.At, diag.Undefined, "Go package %q has no exported symbol %q; did you mean %q?", entry.path, e.Name, hint)
		} else {
			g.errorf(e.At, diag.Undefined, "Go package %q has no exported symbol %q", entry.path, e.Name)
		}
		return bad, true
	}
	switch x := obj.(type) {
	case *types.Const:
		entry.used = true
		return g.goConstExpr(entry, x), true
	case *types.Var:
		entry.used = true
		return g.wrapGoResult(entry.goAlias+"."+x.Name(), x.Type(), false), true
	case *types.Func:
		g.errorf(e.At, diag.TypeMismatch, "%s.%s is a Go function; call it with %s.%s(…)", id.Name, e.Name, id.Name, e.Name)
		return bad, true
	case *types.TypeName:
		g.errorf(e.At, diag.TypeMismatch, "%s.%s is a Go type; construct it with NEW(%s.%s)", id.Name, e.Name, id.Name, e.Name)
		return bad, true
	default:
		g.errorf(e.At, diag.TypeMismatch, "cannot read %s.%s as a value", id.Name, e.Name)
		return bad, true
	}
}

func (g *gen) goConstExpr(entry *goImportEntry, c *types.Const) gexpr {
	ref := entry.goAlias + "." + c.Name()
	u := types.Unalias(c.Type())
	if b, ok := u.(*types.Basic); ok {
		switch b.Kind() {
		case types.Bool, types.UntypedBool:
			return gexpr{code: "bool(" + ref + ")", t: tBool, konst: true}
		case types.String, types.UntypedString:
			return gexpr{code: "string(" + ref + ")", t: tStr, konst: true}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.UntypedInt, types.UntypedRune:
			if _, exact := constant.Int64Val(c.Val()); exact {
				return gexpr{code: "int64(" + ref + ")", t: tInt, konst: true}
			}
			return gexpr{code: "float64(" + ref + ")", t: tNum, konst: true}
		case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			if _, exact := constant.Int64Val(c.Val()); exact {
				return gexpr{code: "int64(" + ref + ")", t: tInt, konst: true}
			}
			return gexpr{code: "float64(" + ref + ")", t: tNum, konst: true}
		case types.Float32, types.Float64, types.UntypedFloat:
			return gexpr{code: "float64(" + ref + ")", t: tNum, konst: true}
		}
	}
	return gexpr{code: "rt.FromGo(" + ref + ")", t: tAny}
}

// goIdentCall handles calls to Go named imports (e.g. NewString()) and NEW(pkg.Type).
// Returns (expr, true) if handled, or (bad, false) to fall through.
func (g *gen) goIdentCall(e *ast.CallExpr, name string, sc *scope, asStmt bool) (gexpr, bool) {
	if name == "NEW" {
		return g.emitNew(e, sc), true
	}
	if g.goImps == nil || len(g.goImps.named) == 0 {
		return bad, false
	}
	if sc != nil && sc.fn != nil && sc.fn.locals[name] != nil {
		return bad, false
	}
	sym, ok := g.goImps.named[name]
	if !ok {
		return bad, false
	}
	if sym.sig == nil {
		if _, isType := sym.obj.(*types.TypeName); isType {
			g.errorf(e.At, diag.TypeMismatch, "%s is a Go type, not a function — construct it with NEW(%s)", sym.obj.Name(), sym.obj.Name())
		} else {
			g.errorf(e.At, diag.TypeMismatch, "%s is not a Go function", sym.obj.Name())
		}
		return bad, true
	}
	return g.emitGoCall(e.At, sym.entry, sym.obj.Name(), sym.sig, e.Args, sc, asStmt), true
}

// goMethodCall handles calls whose callee is not a plain identifier:
// pkg.Func(…) on a Go package alias, or val.Method(…) on an rt.Host value.
func (g *gen) goMethodCall(e *ast.CallExpr, sc *scope, asStmt bool) gexpr {
	mem, ok := e.Fn.(*ast.MemberExpr)
	if !ok {
		g.errorf(e.At, diag.TypeMismatch, "cannot call a non-function value")
		return bad
	}
	if id, isId := mem.X.(*ast.Ident); isId {
		if entry, ok := g.goAliasEntry(id, sc); ok {
			obj := entry.pkg.Lookup(mem.Name)
			if obj == nil {
				if hint := entry.pkg.SuggestName(mem.Name); hint != "" {
					g.errorf(e.At, diag.Undefined, "Go package %q has no exported function %q; did you mean %q?", entry.path, mem.Name, hint)
				} else {
					g.errorf(e.At, diag.Undefined, "Go package %q has no exported function %q", entry.path, mem.Name)
				}
				return bad
			}
			fn, ok := obj.(*types.Func)
			if !ok {
				if _, isType := obj.(*types.TypeName); isType {
					g.errorf(e.At, diag.TypeMismatch, "%s.%s is a Go type, not a function — construct it with NEW(%s.%s)", id.Name, mem.Name, id.Name, mem.Name)
				} else {
					g.errorf(e.At, diag.TypeMismatch, "%s.%s is not a Go function", id.Name, mem.Name)
				}
				return bad
			}
			sig, err := gointerop.AnalyzeFunc(fn)
			if err != nil {
				g.errorf(e.At, diag.TypeMismatch, "%v", err)
				return bad
			}
			return g.emitGoCall(e.At, entry, fn.Name(), sig, e.Args, sc, asStmt)
		}
	}

	// Reflective method call on an rt.Host value (or time.Time).
	recv := g.expr(mem.X, sc)
	parts := make([]string, 0, len(e.Args)+3)
	parts = append(parts, "c", recv.code, strconv.Quote(mem.Name))
	for _, a := range e.Args {
		parts = append(parts, g.expr(a, sc).code)
	}
	return gexpr{code: "rt.HostCall(" + strings.Join(parts, ", ") + ")", t: tAny}
}

func (g *gen) emitNew(e *ast.CallExpr, sc *scope) gexpr {
	if len(e.Args) != 1 {
		g.errorf(e.At, diag.Syntax, "NEW expects 1 argument (a Go type like NEW(url.URL)), got %d", len(e.Args))
		return bad
	}
	var (
		entry    *goImportEntry
		tn       *types.TypeName
		pkgLabel string
	)
	switch arg := e.Args[0].(type) {
	case *ast.MemberExpr:
		id, ok := arg.X.(*ast.Ident)
		if !ok {
			g.errorf(e.At, diag.Syntax, "NEW expects a Go package type like NEW(url.URL)")
			return bad
		}
		var found bool
		entry, found = g.goAliasEntry(id, sc)
		if !found {
			g.errorf(e.At, diag.Undefined, "no Go package imported as %s; add IMPORT %s FROM GO \"…\"", id.Name, strings.ToLower(id.Name))
			return bad
		}
		obj := entry.pkg.Lookup(arg.Name)
		if obj == nil {
			if hint := entry.pkg.SuggestName(arg.Name); hint != "" {
				g.errorf(e.At, diag.Undefined, "Go package %q has no exported type %q; did you mean %q?", entry.path, arg.Name, hint)
			} else {
				g.errorf(e.At, diag.Undefined, "Go package %q has no exported type %q", entry.path, arg.Name)
			}
			return bad
		}
		var isType bool
		tn, isType = obj.(*types.TypeName)
		if !isType {
			g.errorf(e.At, diag.TypeMismatch, "%s.%s is not a Go type", id.Name, arg.Name)
			return bad
		}
		pkgLabel = id.Name

	case *ast.Ident:
		if g.goImps != nil {
			if sym, ok := g.goImps.named[canon(arg.Name)]; ok {
				var isType bool
				tn, isType = sym.obj.(*types.TypeName)
				if !isType {
					g.errorf(e.At, diag.TypeMismatch, "%s is not a Go type", sym.obj.Name())
					return bad
				}
				entry = sym.entry
				pkgLabel = entry.pkg.Name()
			}
		}
		if tn == nil {
			g.errorf(e.At, diag.Syntax, "NEW expects a Go type like NEW(url.URL)")
			return bad
		}

	default:
		g.errorf(e.At, diag.Syntax, "NEW expects a Go type like NEW(url.URL)")
		return bad
	}

	if err := gointerop.ValidateNewType(pkgLabel, tn); err != nil {
		g.errorf(e.At, diag.TypeMismatch, "%v", err)
		return bad
	}
	entry.used = true
	typeRef := entry.goAlias + "." + tn.Name()
	switch types.Unalias(tn.Type()).Underlying().(type) {
	case *types.Struct:
		return gexpr{code: "rt.WrapHost(&" + typeRef + "{})", t: tAny}
	case *types.Map:
		return gexpr{code: "rt.WrapHost(make(" + typeRef + "))", t: tAny}
	case *types.Slice:
		return gexpr{code: "rt.WrapHost(make(" + typeRef + ", 0))", t: tAny}
	default:
		return gexpr{code: "rt.WrapHost(new(" + typeRef + "))", t: tAny}
	}
}

func (g *gen) emitGoCall(pos token.Pos, entry *goImportEntry, fnName string, sig *gointerop.FuncSig, args []ast.Expr, sc *scope, asStmt bool) gexpr {
	if sig.MaxArgs < 0 {
		if len(args) < sig.MinArgs {
			g.errorf(pos, diag.Syntax, "Go function %s expects at least %d argument(s), got %d", fnName, sig.MinArgs, len(args))
			return bad
		}
	} else if len(args) != sig.MinArgs {
		g.errorf(pos, diag.Syntax, "Go function %s expects %d argument(s), got %d", fnName, sig.MinArgs, len(args))
		return bad
	}

	entry.used = true
	var goArgs []string
	argIdx := 0
	for _, p := range sig.Params {
		switch {
		case p.IsContext:
			goArgs = append(goArgs, "c.Context()")
		case p.Variadic:
			for ; argIdx < len(args); argIdx++ {
				ge := g.expr(args[argIdx], sc)
				goArgs = append(goArgs, g.convertArgToGo(ge, p.ElemType))
			}
		default:
			ge := g.expr(args[argIdx], sc)
			goArgs = append(goArgs, g.convertArgToGo(ge, p.Type))
			argIdx++
		}
	}

	callCode := entry.goAlias + "." + fnName + "(" + strings.Join(goArgs, ", ") + ")"
	switch sig.ResultKind {
	case gointerop.ResultVoid:
		if asStmt {
			return gexpr{code: callCode, t: tAny}
		}
		return gexpr{code: "func() rt.Value { " + callCode + "; return nil }()", t: tAny}
	case gointerop.ResultError:
		if asStmt {
			return gexpr{code: "rt.CheckGoErr(" + callCode + ")", t: tAny}
		}
		return gexpr{code: "func() rt.Value { rt.CheckGoErr(" + callCode + "); return nil }()", t: tAny}
	case gointerop.ResultValue:
		return g.wrapGoResult(callCode, sig.ResultType, asStmt)
	case gointerop.ResultValueErr:
		return g.wrapGoResultErr(callCode, sig.ResultType)
	}
	return bad
}

func (g *gen) wrapGoResult(code string, resultType types.Type, asStmt bool) gexpr {
	if asStmt {
		return gexpr{code: code, t: tAny}
	}
	u := types.Unalias(resultType)
	if b, ok := u.(*types.Basic); ok {
		switch b.Kind() {
		case types.String:
			return gexpr{code: code, t: tStr}
		case types.Bool:
			return gexpr{code: code, t: tBool}
		case types.Int64:
			return gexpr{code: code, t: tInt}
		case types.Int, types.Int8, types.Int16, types.Int32:
			return gexpr{code: "int64(" + code + ")", t: tInt}
		case types.Float64:
			return gexpr{code: code, t: tNum}
		case types.Float32:
			return gexpr{code: "float64(" + code + ")", t: tNum}
		}
	}
	return gexpr{code: "rt.FromGo(" + code + ")", t: tAny}
}

func (g *gen) wrapGoResultErr(callCode string, resultType types.Type) gexpr {
	u := types.Unalias(resultType)
	if b, ok := u.(*types.Basic); ok {
		switch b.Kind() {
		case types.String:
			return gexpr{code: "rt.GoResultStr(" + callCode + ")", t: tStr}
		case types.Bool:
			return gexpr{code: "rt.GoResultBool(" + callCode + ")", t: tBool}
		case types.Int64:
			return gexpr{code: "rt.GoResultInt64(" + callCode + ")", t: tInt}
		case types.Float64:
			return gexpr{code: "rt.GoResultFloat64(" + callCode + ")", t: tNum}
		}
	}
	return gexpr{code: "rt.GoResult(" + callCode + ")", t: tAny}
}

func (g *gen) convertArgToGo(arg gexpr, target types.Type) string {
	u := types.Unalias(target)
	if b, ok := u.(*types.Basic); ok {
		switch b.Kind() {
		case types.String:
			if arg.t == tStr {
				return arg.code
			}
			return "rt.ToGoString(" + arg.code + ")"
		case types.Bool:
			if arg.t == tBool {
				return arg.code
			}
			return "rt.ToGoBool(" + arg.code + ")"
		case types.Int64:
			if arg.t == tInt {
				return arg.code
			}
			return "rt.ToGoInt64(" + arg.code + ")"
		case types.Int:
			if arg.t == tInt {
				return "int(" + arg.code + ")"
			}
			return "rt.ToGoInt(" + arg.code + ")"
		case types.Int8:
			return "rt.ToGoInt8(" + arg.code + ")"
		case types.Int16:
			return "rt.ToGoInt16(" + arg.code + ")"
		case types.Int32:
			return "rt.ToGoInt32(" + arg.code + ")"
		case types.Uint:
			return "rt.ToGoUint(" + arg.code + ")"
		case types.Uint8:
			return "rt.ToGoUint8(" + arg.code + ")"
		case types.Uint16:
			return "rt.ToGoUint16(" + arg.code + ")"
		case types.Uint32:
			return "rt.ToGoUint32(" + arg.code + ")"
		case types.Uint64:
			return "rt.ToGoUint64(" + arg.code + ")"
		case types.Uintptr:
			return "rt.ToGoUintptr(" + arg.code + ")"
		case types.Float64:
			if arg.t == tNum {
				return arg.code
			}
			if arg.t == tInt {
				return "float64(" + arg.code + ")"
			}
			return "rt.ToGoFloat64(" + arg.code + ")"
		case types.Float32:
			return "rt.ToGoFloat32(" + arg.code + ")"
		}
	}

	if gointerop.IsByteSlice(u) {
		return "rt.ToGoBytes(" + arg.code + ")"
	}
	if sl, ok := u.(*types.Slice); ok {
		return "rt.ToGoSlice[" + g.goTypeString(sl.Elem()) + "](" + arg.code + ")"
	}
	if mp, ok := u.(*types.Map); ok {
		if kb, ok := types.Unalias(mp.Key()).(*types.Basic); ok && kb.Kind() == types.String {
			return "rt.ToGoMap[" + g.goTypeString(mp.Elem()) + "](" + arg.code + ")"
		}
		return "rt.ToGo[" + g.goTypeString(u) + "](" + arg.code + ")"
	}
	if iface, ok := u.(*types.Interface); ok && iface.NumMethods() == 0 && iface.NumEmbeddeds() == 0 {
		return "rt.ToGoAny(" + arg.code + ")"
	}
	if _, ok := u.Underlying().(*types.Struct); ok {
		return "rt.ToStruct[" + g.goTypeString(u) + "](" + arg.code + ")"
	}
	if ptr, ok := u.(*types.Pointer); ok {
		if _, ok := types.Unalias(ptr.Elem()).Underlying().(*types.Struct); ok {
			return "rt.ToStructPtr[" + g.goTypeString(ptr.Elem()) + "](" + arg.code + ")"
		}
	}
	return "rt.ToGo[" + g.goTypeString(u) + "](" + arg.code + ")"
}

func (g *gen) goTypeString(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string {
		return g.ensureGoImport(p.Path(), p.Name())
	})
}
