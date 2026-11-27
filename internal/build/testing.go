package build

import (
	"go/format"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// Test builds (rip test, task 4.2). A test build is the program compiled
// with its tests/*.test.bas files (compiler.CompileTests), plus the file
// from TestModeFile, in its own build directory so that it never replaces
// the build that rip dev or rip run is using.

// TestModeFileName is the name of the file TestModeFile makes. It must not
// end in _test.go, or go build would skip it.
const TestModeFileName = "rip_testmode.go"

// TestBuildDir is the build directory of rip test in a project.
func TestBuildDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".rip", "test")
}

// TestBinary is the path of the rip test binary of the project named name.
func TestBinary(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".rip", "bin", name+"-test")
}

// TestModeFile returns the generated file that switches a build into test
// mode before any top-level code runs: every DBCONNECT goes to the test
// database, and the program runs its TESTs instead of serving. migrations
// (file name → SQL, as from MigrationsFS) are embedded, to be applied to
// the in-memory test database in name order.
func TestModeFile(migrations map[string][]byte) codegen.File {
	var b strings.Builder
	b.WriteString(codegen.Header + "\n\npackage main\n\nimport " + strconv.Quote(codegen.RuntimeImport) + "\n\n")
	b.WriteString("func init() {\n\trt.EnableTestMode(\n")
	for _, name := range SortedMigrationNames(migrations) {
		b.WriteString("\t\trt.Migration{Name: " + strconv.Quote("migrations/"+name) + ", SQL: " + strconv.Quote(string(migrations[name])) + "},\n")
	}
	b.WriteString("\t)\n}\n")
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		// Only quoted strings vary, so this cannot happen; keep the raw source.
		src = []byte(b.String())
	}
	return codegen.File{Name: TestModeFileName, Src: src}
}
