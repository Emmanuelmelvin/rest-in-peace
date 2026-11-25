package playground

// GoAllowlist lists the Go packages that playground programs may IMPORT … FROM GO
// (Requirement 15.5). The CLI has no such limit.
var GoAllowlist = []string{
	"github.com/google/uuid",
	"github.com/shopspring/decimal",
}

// Allowed reports whether a Go import path may be used in the playground.
func Allowed(importPath string) bool {
	for _, p := range GoAllowlist {
		if p == importPath {
			return true
		}
	}
	return false
}
