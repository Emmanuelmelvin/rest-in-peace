package codegen

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"

// goImportState holds per-file state for IMPORT … FROM GO (task 3.5).
type goImportState struct{}

// goImport handles IMPORT alias FROM GO "pkg" and IMPORT { … } FROM GO "pkg"
// (Requirements 5.4–5.7, design §4.10, task 3.5), called from gen.generate.
func (g *gen) goImport(im *ast.Import) {
	// TODO(task 3.5): load the Go package via internal/compiler/gointerop and
	// record its alias, Go import path, and exported symbols on g.goImports.
	g.todo(im.At, "3.5", "IMPORT … FROM GO")
}

// extraGoImports returns extra Go import lines (e.g. `go_uuid "github.com/google/uuid"`)
// to emit in the generated file's import block.
func (g *gen) extraGoImports() []string {
	return nil
}

// goMember handles member access on a Go package alias (e.g. pkg.Const).
// Returns (expr, true) when e.X is a Go package alias, or (bad, false) to
// fall through to normal rt.Get member access.
func (g *gen) goMember(e *ast.MemberExpr, sc *scope) (gexpr, bool) {
	return bad, false
}

// goIdentCall handles calls to Go named imports (e.g. NewString()) and NEW(pkg.Type).
// Returns (expr, true) if handled, or (bad, false) to fall through.
func (g *gen) goIdentCall(e *ast.CallExpr, name string, sc *scope, asStmt bool) (gexpr, bool) {
	if name == "NEW" {
		g.todo(e.At, "3.5", "NEW")
		return bad, true
	}
	return bad, false
}

// goMethodCall handles calls whose callee is not a plain identifier:
// pkg.Func(…) on a Go package alias, or val.Method(…) on an rt.Host value.
func (g *gen) goMethodCall(e *ast.CallExpr, sc *scope, asStmt bool) gexpr {
	// TODO(task 3.5): emit typed Go function calls or reflective rt.HostCall.
	g.todo(e.At, "3.5", "calling a method or a function value")
	return bad
}
