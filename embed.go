// Package rip holds assets shared by the rip toolchain.
package rip

import "embed"

// RuntimeFS holds the source of the rt package. The build step copies it into
// every program's build directory, so a program always links the runtime that
// matches the compiler that generated it (design §4.1).
//
//go:embed rt/*.go
var RuntimeFS embed.FS
