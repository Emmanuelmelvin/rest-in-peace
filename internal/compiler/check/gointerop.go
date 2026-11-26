package check

import (
	"go/types"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/gointerop"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

type namedGoSym struct {
	pkg *gointerop.Package
	obj types.Object
	sig *gointerop.FuncSig
}

// checkGoImports validates IMPORT … FROM GO declarations and usage
// (Requirements 5.4–5.7, task 3.5).
//
// Called from Check in check.go.
func checkGoImports(p *ast.Program) diag.List {
	if p == nil {
		return nil
	}
	var ds diag.List
	for _, f := range p.Files {
		if f == nil {
			continue
		}
		gc := &goChecker{
			file:    f,
			aliases: map[string]*gointerop.Package{},
			named:   map[string]namedGoSym{},
		}
		gc.checkFile()
		ds = append(ds, gc.diags...)
	}
	return ds
}

type goChecker struct {
	file    *ast.File
	aliases map[string]*gointerop.Package
	named   map[string]namedGoSym
	diags   diag.List
}

func (g *goChecker) errorf(pos token.Pos, kind diag.Kind, format string, args ...any) {
	if pos.File == "" {
		pos.File = g.file.Name
	}
	g.diags = append(g.diags, diag.Errorf(pos, kind, format, args...))
}

func (g *goChecker) checkFile() {
	for _, im := range g.file.Imports {
		if !im.FromGo {
			continue
		}
		if strings.TrimSpace(im.Path) == "" {
			g.errorf(im.At, diag.Syntax, "IMPORT … FROM GO requires a non-empty Go package path")
			continue
		}
		pkg, err := gointerop.Load(im.Path, "")
		if err != nil {
			g.errorf(im.At, diag.Undefined, "cannot load Go package %q: %v — check the package path and make sure it is available in go.mod", im.Path, err)
			continue
		}
		if im.Alias != "" {
			g.aliases[strings.ToUpper(im.Alias)] = pkg
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
			sym := namedGoSym{pkg: pkg, obj: obj}
			if fn, ok := obj.(*types.Func); ok {
				sig, err := gointerop.AnalyzeFunc(fn)
				if err != nil {
					g.errorf(im.At, diag.TypeMismatch, "%v", err)
					continue
				}
				sym.sig = sig
			}
			g.named[strings.ToUpper(name)] = sym
		}
	}

	if g.file.Mode == token.Classic {
		for _, ln := range g.file.Lines {
			g.walkStmts(ln.Stmts, nil)
		}
		return
	}
	g.walkStmts(g.file.Body, nil)
}

func (g *goChecker) walkStmts(stmts []ast.Stmt, shadowed map[string]bool) {
	for _, s := range stmts {
		g.walkStmt(s, shadowed)
	}
}

func (g *goChecker) walkStmt(s ast.Stmt, shadowed map[string]bool) {
	if s == nil {
		return
	}
	switch st := s.(type) {
	case *ast.FuncDecl:
		local := copyShadow(shadowed)
		for _, prm := range st.Params {
			if prm.Default != nil {
				g.walkExpr(prm.Default, nil)
			}
			local[strings.ToUpper(prm.Name)] = true
		}
		collectLocalShadow(st.Body, local)
		g.walkStmts(st.Body, local)
	case *ast.TestDecl:
		local := copyShadow(shadowed)
		collectLocalShadow(st.Body, local)
		g.walkStmts(st.Body, local)
	case *ast.ConstDecl:
		g.walkExpr(st.Value, shadowed)
	case *ast.DimStmt:
		g.walkExpr(st.Value, shadowed)
	case *ast.LetStmt:
		g.walkExpr(st.Value, shadowed)
		if _, ok := st.Target.(*ast.Ident); !ok {
			g.walkExpr(st.Target, shadowed)
		}
	case *ast.PrintStmt:
		for _, a := range st.Args {
			g.walkExpr(a, shadowed)
		}
	case *ast.ExprStmt:
		g.walkExpr(st.X, shadowed)
	case *ast.IfStmt:
		g.walkExpr(st.Cond, shadowed)
		g.walkStmts(st.Then, shadowed)
		g.walkStmts(st.Else, shadowed)
	case *ast.ForStmt:
		g.walkExpr(st.From, shadowed)
		g.walkExpr(st.To, shadowed)
		g.walkExpr(st.Step, shadowed)
		g.walkStmts(st.Body, shadowed)
	case *ast.ForEachStmt:
		g.walkExpr(st.In, shadowed)
		g.walkStmts(st.Body, shadowed)
	case *ast.WhileStmt:
		g.walkExpr(st.Cond, shadowed)
		g.walkStmts(st.Body, shadowed)
	case *ast.DoStmt:
		g.walkStmts(st.Body, shadowed)
		g.walkExpr(st.Until, shadowed)
	case *ast.SelectStmt:
		g.walkExpr(st.X, shadowed)
		for _, sc := range st.Cases {
			for _, v := range sc.Values {
				g.walkExpr(v, shadowed)
			}
			g.walkStmts(sc.Body, shadowed)
		}
		g.walkStmts(st.Else, shadowed)
	case *ast.ReturnStmt:
		g.walkExpr(st.Value, shadowed)
	case *ast.ThrowStmt:
		g.walkExpr(st.Value, shadowed)
	case *ast.TryStmt:
		g.walkStmts(st.Body, shadowed)
		catchShadow := shadowed
		if st.CatchVar != "" {
			catchShadow = copyShadow(shadowed)
			catchShadow[strings.ToUpper(st.CatchVar)] = true
		}
		g.walkStmts(st.Catch, catchShadow)
		g.walkStmts(st.Finally, shadowed)
	case *ast.RespondStmt:
		g.walkExpr(st.Status, shadowed)
		g.walkExpr(st.Value, shadowed)
		g.walkExpr(st.Headers, shadowed)
	case *ast.ServeStmt:
		g.walkExpr(st.Port, shadowed)
	case *ast.DBConnectStmt:
		g.walkExpr(st.URL, shadowed)
	case *ast.TransactionStmt:
		g.walkStmts(st.Body, shadowed)
	case *ast.LogStmt:
		g.walkExpr(st.Level, shadowed)
		g.walkExpr(st.Msg, shadowed)
		g.walkExpr(st.Fields, shadowed)
	case *ast.ExpectStmt:
		g.walkExpr(st.Cond, shadowed)
		g.walkExpr(st.Msg, shadowed)
	case *ast.OnGotoStmt:
		g.walkExpr(st.X, shadowed)
	}
}

func (g *goChecker) walkExpr(e ast.Expr, shadowed map[string]bool) {
	if e == nil {
		return
	}
	switch ex := e.(type) {
	case *ast.ArrayLit:
		for _, it := range ex.Items {
			g.walkExpr(it, shadowed)
		}
	case *ast.MapLit:
		for _, v := range ex.Values {
			g.walkExpr(v, shadowed)
		}
	case *ast.IndexExpr:
		g.walkExpr(ex.X, shadowed)
		g.walkExpr(ex.Index, shadowed)
	case *ast.UnaryExpr:
		g.walkExpr(ex.X, shadowed)
	case *ast.BinaryExpr:
		g.walkExpr(ex.X, shadowed)
		g.walkExpr(ex.Y, shadowed)
	case *ast.MemberExpr:
		if id, ok := ex.X.(*ast.Ident); ok && !shadowed[id.Name] {
			if pkg, ok := g.aliases[id.Name]; ok {
				g.checkGoMember(ex, id.Name, pkg)
				return
			}
		}
		g.walkExpr(ex.X, shadowed)
	case *ast.CallExpr:
		g.checkGoCall(ex, shadowed)
	}
}

func (g *goChecker) checkGoMember(mem *ast.MemberExpr, alias string, pkg *gointerop.Package) {
	obj := pkg.Lookup(mem.Name)
	if obj == nil {
		if hint := pkg.SuggestName(mem.Name); hint != "" {
			g.errorf(mem.At, diag.Undefined, "Go package %q has no exported symbol %q; did you mean %q?", pkg.Path, mem.Name, hint)
		} else {
			g.errorf(mem.At, diag.Undefined, "Go package %q has no exported symbol %q", pkg.Path, mem.Name)
		}
		return
	}
	switch obj.(type) {
	case *types.Const, *types.Var:
		return
	case *types.Func:
		g.errorf(mem.At, diag.TypeMismatch, "%s.%s is a Go function; call it with %s.%s(…)", alias, mem.Name, alias, mem.Name)
	case *types.TypeName:
		g.errorf(mem.At, diag.TypeMismatch, "%s.%s is a Go type; construct it with NEW(%s.%s)", alias, mem.Name, alias, mem.Name)
	}
}

func (g *goChecker) checkGoCall(call *ast.CallExpr, shadowed map[string]bool) {
	if id, ok := call.Fn.(*ast.Ident); ok {
		if id.Name == "NEW" {
			g.checkNewCall(call, shadowed)
			return
		}
		for _, a := range call.Args {
			g.walkExpr(a, shadowed)
		}
		if !shadowed[id.Name] {
			if sym, ok := g.named[id.Name]; ok {
				if sym.sig == nil {
					if _, isType := sym.obj.(*types.TypeName); isType {
						g.errorf(call.At, diag.TypeMismatch, "%s is a Go type, not a function — construct it with NEW(%s)", sym.obj.Name(), sym.obj.Name())
					} else {
						g.errorf(call.At, diag.TypeMismatch, "%s is not a Go function", sym.obj.Name())
					}
					return
				}
				g.checkGoArity(call.At, sym.obj.Name(), sym.sig, len(call.Args))
			}
		}
		return
	}

	if mem, ok := call.Fn.(*ast.MemberExpr); ok {
		if id, isId := mem.X.(*ast.Ident); isId && !shadowed[id.Name] {
			if pkg, ok := g.aliases[id.Name]; ok {
				for _, a := range call.Args {
					g.walkExpr(a, shadowed)
				}
				obj := pkg.Lookup(mem.Name)
				if obj == nil {
					if hint := pkg.SuggestName(mem.Name); hint != "" {
						g.errorf(call.At, diag.Undefined, "Go package %q has no exported function %q; did you mean %q?", pkg.Path, mem.Name, hint)
					} else {
						g.errorf(call.At, diag.Undefined, "Go package %q has no exported function %q", pkg.Path, mem.Name)
					}
					return
				}
				fn, ok := obj.(*types.Func)
				if !ok {
					if _, isType := obj.(*types.TypeName); isType {
						g.errorf(call.At, diag.TypeMismatch, "%s.%s is a Go type, not a function — construct it with NEW(%s.%s)", id.Name, mem.Name, id.Name, mem.Name)
					} else {
						g.errorf(call.At, diag.TypeMismatch, "%s.%s is not a Go function", id.Name, mem.Name)
					}
					return
				}
				sig, err := gointerop.AnalyzeFunc(fn)
				if err != nil {
					g.errorf(call.At, diag.TypeMismatch, "%v", err)
					return
				}
				g.checkGoArity(call.At, mem.Name, sig, len(call.Args))
				return
			}
		}
	}

	g.walkExpr(call.Fn, shadowed)
	for _, a := range call.Args {
		g.walkExpr(a, shadowed)
	}
}

func (g *goChecker) checkNewCall(call *ast.CallExpr, shadowed map[string]bool) {
	if len(call.Args) != 1 {
		g.errorf(call.At, diag.Syntax, "NEW expects 1 argument (a Go type like NEW(url.URL)), got %d", len(call.Args))
		return
	}
	switch arg := call.Args[0].(type) {
	case *ast.MemberExpr:
		id, ok := arg.X.(*ast.Ident)
		if !ok || shadowed[id.Name] {
			g.errorf(call.At, diag.Syntax, "NEW expects a Go package type like NEW(url.URL)")
			return
		}
		pkg, ok := g.aliases[id.Name]
		if !ok {
			g.errorf(call.At, diag.Undefined, "no Go package imported as %s; add IMPORT %s FROM GO \"…\"", id.Name, strings.ToLower(id.Name))
			return
		}
		obj := pkg.Lookup(arg.Name)
		if obj == nil {
			if hint := pkg.SuggestName(arg.Name); hint != "" {
				g.errorf(call.At, diag.Undefined, "Go package %q has no exported type %q; did you mean %q?", pkg.Path, arg.Name, hint)
			} else {
				g.errorf(call.At, diag.Undefined, "Go package %q has no exported type %q", pkg.Path, arg.Name)
			}
			return
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			g.errorf(call.At, diag.TypeMismatch, "%s.%s is not a Go type", id.Name, arg.Name)
			return
		}
		if err := gointerop.ValidateNewType(id.Name, tn); err != nil {
			g.errorf(call.At, diag.TypeMismatch, "%v", err)
		}
	case *ast.Ident:
		if !shadowed[arg.Name] {
			if sym, ok := g.named[arg.Name]; ok {
				tn, ok := sym.obj.(*types.TypeName)
				if !ok {
					g.errorf(call.At, diag.TypeMismatch, "%s is not a Go type", sym.obj.Name())
					return
				}
				if err := gointerop.ValidateNewType(sym.pkg.Name(), tn); err != nil {
					g.errorf(call.At, diag.TypeMismatch, "%v", err)
				}
				return
			}
		}
		g.errorf(call.At, diag.Syntax, "NEW expects a Go type like NEW(url.URL)")
	default:
		g.errorf(call.At, diag.Syntax, "NEW expects a Go type like NEW(url.URL)")
	}
}

func (g *goChecker) checkGoArity(pos token.Pos, name string, sig *gointerop.FuncSig, got int) {
	if sig.MaxArgs < 0 {
		if got < sig.MinArgs {
			g.errorf(pos, diag.Syntax, "Go function %s expects at least %s, got %d", name, pluralArgs(sig.MinArgs), got)
		}
		return
	}
	if got != sig.MinArgs {
		g.errorf(pos, diag.Syntax, "Go function %s expects %s, got %d", name, pluralArgs(sig.MinArgs), got)
	}
}

func copyShadow(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func collectLocalShadow(stmts []ast.Stmt, m map[string]bool) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.DimStmt:
			m[strings.ToUpper(st.Name)] = true
		case *ast.ConstDecl:
			m[strings.ToUpper(st.Name)] = true
		case *ast.LetStmt:
			if id, ok := st.Target.(*ast.Ident); ok {
				m[strings.ToUpper(id.Name)] = true
			}
		case *ast.ForStmt:
			m[strings.ToUpper(st.Var)] = true
			collectLocalShadow(st.Body, m)
		case *ast.ForEachStmt:
			m[strings.ToUpper(st.Var)] = true
			collectLocalShadow(st.Body, m)
		case *ast.IfStmt:
			collectLocalShadow(st.Then, m)
			collectLocalShadow(st.Else, m)
		case *ast.WhileStmt:
			collectLocalShadow(st.Body, m)
		case *ast.DoStmt:
			collectLocalShadow(st.Body, m)
		case *ast.SelectStmt:
			for _, c := range st.Cases {
				collectLocalShadow(c.Body, m)
			}
			collectLocalShadow(st.Else, m)
		case *ast.TryStmt:
			collectLocalShadow(st.Body, m)
			collectLocalShadow(st.Catch, m)
			collectLocalShadow(st.Finally, m)
		case *ast.TransactionStmt:
			collectLocalShadow(st.Body, m)
		}
	}
}
