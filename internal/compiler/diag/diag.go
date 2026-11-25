// Package diag holds compiler errors and warnings in classic BASIC form:
//
//	?SYNTAX ERROR IN billing.bas:42 — expected THEN after IF condition
package diag

import (
	"fmt"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// Severity separates errors, which stop compilation, from warnings.
type Severity int

const (
	Error Severity = iota
	Warning
)

// Kind is the classic error word.
type Kind string

const (
	Syntax         Kind = "SYNTAX"
	TypeMismatch   Kind = "TYPE MISMATCH"
	Undefined      Kind = "UNDEFINED"
	InjectionRisk  Kind = "INJECTION RISK" // warning, Requirement 9.4
	SharedState    Kind = "SHARED STATE"   // warning, Requirement 14.4
	NotImplemented Kind = "NOT IMPLEMENTED"
)

// Diagnostic is one compiler message. Message is the hint after the em dash.
type Diagnostic struct {
	Severity Severity
	Kind     Kind
	Pos      token.Pos
	Message  string
}

// String formats the diagnostic, e.g. "?SYNTAX ERROR IN x.bas:3 — expected THEN".
func (d Diagnostic) String() string {
	word := "ERROR"
	if d.Severity == Warning {
		word = "WARNING"
	}
	return fmt.Sprintf("?%s %s IN %s:%d — %s", d.Kind, word, d.Pos.File, d.Pos.Line, d.Message)
}

// Errorf builds an error diagnostic.
func Errorf(pos token.Pos, kind Kind, format string, args ...any) Diagnostic {
	return Diagnostic{Severity: Error, Kind: kind, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// Warnf builds a warning diagnostic.
func Warnf(pos token.Pos, kind Kind, format string, args ...any) Diagnostic {
	return Diagnostic{Severity: Warning, Kind: kind, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// Todo marks a compiler stage that is still a stub.
func Todo(pos token.Pos, task, what string) Diagnostic {
	return Errorf(pos, NotImplemented, "%s is not built yet (task %s in .kiro/specs/rip-basic/tasks.md)", what, task)
}

// List is a list of diagnostics in source order.
type List []Diagnostic

// HasErrors reports whether any diagnostic is an error.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// NotImplemented returns the first not-implemented diagnostic, if any.
func (l List) NotImplemented() (Diagnostic, bool) {
	for _, d := range l {
		if d.Kind == NotImplemented {
			return d, true
		}
	}
	return Diagnostic{}, false
}
