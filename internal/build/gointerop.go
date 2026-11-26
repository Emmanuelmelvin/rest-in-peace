package build

import (
	"runtime/debug"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// extraGoModRequires returns additional module requirements needed by Go
// packages imported via IMPORT … FROM GO in files (task 3.5). Called from
// goModFor in build.go.
func extraGoModRequires(files []codegen.File) []*debug.Module {
	// TODO(task 3.5): inspect third-party imports in generated files and resolve
	// their module versions from the project's go.mod or build info.
	return nil
}
