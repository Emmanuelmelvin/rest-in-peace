package build

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	rip "github.com/Emmanuelmelvin/rest-in-peace"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
)

// extraGoModRequires returns additional module requirements needed by Go
// packages imported via IMPORT … FROM GO in files (task 3.5). Called from
// goModFor in build.go.
func extraGoModRequires(files []codegen.File) []*debug.Module {
	imports := thirdPartyImports(files)
	if len(imports) == 0 {
		return nil
	}
	return resolveModules(imports, candidateModuleSources())
}

// thirdPartyImports extracts non-stdlib, non-ripapp import paths from generated Go files.
func thirdPartyImports(files []codegen.File) []string {
	fset := token.NewFileSet()
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f.Name, ".go") || len(f.Src) == 0 {
			continue
		}
		parsed, err := parser.ParseFile(fset, f.Name, f.Src, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range parsed.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if p == ModulePath || strings.HasPrefix(p, ModulePath+"/") {
				continue
			}
			// Standard-library paths have no dot in their first path element.
			first, _, _ := strings.Cut(p, "/")
			if !strings.Contains(first, ".") {
				continue
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// candidateModuleSources returns module lists in priority order:
// local project go.mod (walking up from cwd), CLI build info, and embedded rip.GoMod.
func candidateModuleSources() [][]*debug.Module {
	var sources [][]*debug.Module
	if local := localGoModRequires(); len(local) > 0 {
		sources = append(sources, local)
	}
	if info, ok := debug.ReadBuildInfo(); ok && len(info.Deps) > 0 {
		sources = append(sources, info.Deps)
	}
	if embedded := parseGoModRequires(rip.GoMod); len(embedded) > 0 {
		sources = append(sources, embedded)
	}
	return sources
}

func localGoModRequires() []*debug.Module {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	return localGoModRequiresFrom(wd)
}

func localGoModRequiresFrom(startDir string) []*debug.Module {
	dir := startDir
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			return parseGoModRequires(data)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func resolveModules(imports []string, sources [][]*debug.Module) []*debug.Module {
	byPath := map[string]*debug.Module{}
	for _, imp := range imports {
		if m := bestMatch(imp, sources); m != nil {
			if _, exists := byPath[m.Path]; !exists {
				byPath[m.Path] = m
			}
		}
	}
	if len(byPath) == 0 {
		return nil
	}
	out := make([]*debug.Module, 0, len(byPath))
	for _, m := range byPath {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func bestMatch(imp string, sources [][]*debug.Module) *debug.Module {
	for _, list := range sources {
		var best *debug.Module
		for _, m := range list {
			if m == nil || m.Path == "" || m.Version == "" {
				continue
			}
			if imp == m.Path || strings.HasPrefix(imp, m.Path+"/") {
				if best == nil || len(m.Path) > len(best.Path) {
					best = m
				}
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}
