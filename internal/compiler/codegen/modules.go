package codegen

import "github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"

// basImportState holds per-file state for .bas module imports (task 3.4).
type basImportState struct{}

// basImport handles IMPORT "./x.bas" and IMPORT { … } FROM "./x.bas"
// (Requirements 5.1–5.3, task 3.4), called from gen.generate in codegen.go.
func (g *gen) basImport(im *ast.Import) {
	// TODO(task 3.4): record imported .bas module paths and selected names.
	g.todo(im.At, "3.4", "IMPORT")
}

// bindBasImports binds exported FUNCTION, SUB and CONST declarations from
// imported .bas files into g.funcs and g.globals (without adding them to
// g.funcOrder / g.globOrder so they are not re-emitted). Called from
// structuredFile in structured.go after local declarations are collected.
func (g *gen) bindBasImports() {
	// TODO(task 3.4): look up exported declarations in g.prog.Files and bind
	// their mangled b_<mod>_<name> symbols into g.funcs / g.globals.
}
