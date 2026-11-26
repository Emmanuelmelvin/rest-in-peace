// Package gointerop loads Go packages with go/packages and inspects their
// exported types and functions for IMPORT … FROM GO (Requirements 5.4–5.7,
// design §4.10, task 3.5).
package gointerop

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

// Package holds the loaded type information for one Go import path.
type Package struct {
	Path string
	Pkg  *packages.Package
}

// Load loads the Go package at importPath relative to dir using go/packages.
func Load(importPath, dir string) (*Package, error) {
	// TODO(task 3.5): configure packages.NeedName|NeedTypes|NeedTypesInfo|NeedModule
	// and load importPath, caching loaded packages across calls.
	_ = packages.NeedName
	return nil, fmt.Errorf("gointerop.Load(%q) is not built yet (task 3.5)", importPath)
}
