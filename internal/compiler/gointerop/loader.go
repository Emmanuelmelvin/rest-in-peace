// Package gointerop loads Go packages with go/packages and inspects their
// exported types and functions for IMPORT … FROM GO (Requirements 5.4–5.7,
// design §4.10, task 3.5).
package gointerop

import (
	"errors"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// Package holds the loaded type information for one Go import path.
type Package struct {
	Path string
	Pkg  *packages.Package
}

type cacheKey struct {
	importPath string
	dir        string
}

var (
	cacheMu sync.RWMutex
	cache   = map[cacheKey]*Package{}
)

// Load loads the Go package at importPath relative to dir using go/packages.
// Successful loads are cached in memory keyed by (importPath, dir).
func Load(importPath, dir string) (*Package, error) {
	if strings.TrimSpace(importPath) == "" {
		return nil, errors.New("Go package path cannot be empty")
	}
	if strings.HasPrefix(importPath, ".") || strings.HasPrefix(importPath, "/") {
		return nil, fmt.Errorf("invalid Go package path %q (relative paths are for .bas imports)", importPath)
	}

	absDir := ""
	if dir != "" {
		if a, err := filepath.Abs(dir); err == nil {
			absDir = a
		} else {
			absDir = dir
		}
	} else if wd, err := os.Getwd(); err == nil {
		absDir = wd
	}

	key := cacheKey{importPath: importPath, dir: absDir}
	cacheMu.RLock()
	if cached, ok := cache[key]; ok {
		cacheMu.RUnlock()
		return cached, nil
	}
	cacheMu.RUnlock()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Dir:  absDir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Go package %q: %w", importPath, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("Go package %q not found", importPath)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		msgs := make([]string, len(pkg.Errors))
		for i, pe := range pkg.Errors {
			msgs[i] = pe.Msg
		}
		return nil, fmt.Errorf("failed to load Go package %q: %s", importPath, strings.Join(msgs, "; "))
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("Go package %q has no type information", importPath)
	}

	loaded := &Package{
		Path: pkg.PkgPath,
		Pkg:  pkg,
	}
	if loaded.Path == "" {
		loaded.Path = importPath
	}

	cacheMu.Lock()
	cache[key] = loaded
	cacheMu.Unlock()
	return loaded, nil
}

// Name returns the Go package identifier (e.g. "uuid" for "github.com/google/uuid").
func (p *Package) Name() string {
	if p == nil || p.Pkg == nil {
		return ""
	}
	if p.Pkg.Name != "" {
		return p.Pkg.Name
	}
	if p.Pkg.Types != nil {
		return p.Pkg.Types.Name()
	}
	return ""
}

// Lookup returns the exported package-scope object with the exact name, or nil
// if it does not exist or is unexported.
func (p *Package) Lookup(name string) types.Object {
	if p == nil || p.Pkg == nil || p.Pkg.Types == nil {
		return nil
	}
	obj := p.Pkg.Types.Scope().Lookup(name)
	if obj == nil || !obj.Exported() {
		return nil
	}
	return obj
}

// LookupFunc returns the exported package-level function with exact name.
func (p *Package) LookupFunc(name string) (*types.Func, bool) {
	fn, ok := p.Lookup(name).(*types.Func)
	return fn, ok
}

// LookupType returns the exported package-level type with exact name.
func (p *Package) LookupType(name string) (*types.TypeName, bool) {
	tn, ok := p.Lookup(name).(*types.TypeName)
	return tn, ok
}

// LookupConst returns the exported package-level constant with exact name.
func (p *Package) LookupConst(name string) (*types.Const, bool) {
	c, ok := p.Lookup(name).(*types.Const)
	return c, ok
}

// LookupVar returns the exported package-level variable with exact name.
func (p *Package) LookupVar(name string) (*types.Var, bool) {
	v, ok := p.Lookup(name).(*types.Var)
	return v, ok
}

// ExportedNames returns all exported top-level names in sorted order.
func (p *Package) ExportedNames() []string {
	if p == nil || p.Pkg == nil || p.Pkg.Types == nil {
		return nil
	}
	scope := p.Pkg.Types.Scope()
	var out []string
	for _, n := range scope.Names() {
		if obj := scope.Lookup(n); obj != nil && obj.Exported() {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// SuggestName returns the closest exported symbol name in p for typo hints,
// checking case-insensitive matches first and then small edit distances.
func (p *Package) SuggestName(want string) string {
	names := p.ExportedNames()
	if len(names) == 0 || want == "" {
		return ""
	}
	for _, n := range names {
		if strings.EqualFold(n, want) {
			return n
		}
	}
	best := ""
	bestDist := 3
	lw := strings.ToLower(want)
	for _, n := range names {
		d := editDistance(lw, strings.ToLower(n))
		if d < bestDist {
			bestDist = d
			best = n
		}
	}
	return best
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			m := prev[j] + 1
			if cur[j-1]+1 < m {
				m = cur[j-1] + 1
			}
			if prev[j-1]+cost < m {
				m = prev[j-1] + cost
			}
			cur[j] = m
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}
