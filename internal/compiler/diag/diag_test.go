package diag

import (
	"testing"

	"github.com/rip-basic/rip/internal/compiler/token"
)

func TestClassicFormat(t *testing.T) {
	d := Errorf(token.Pos{File: "billing.bas", Line: 42, Col: 7}, Syntax, "expected THEN after IF condition")
	want := "?SYNTAX ERROR IN billing.bas:42 — expected THEN after IF condition"
	if d.String() != want {
		t.Fatalf("got %q, want %q", d.String(), want)
	}
	w := Warnf(token.Pos{File: "main.bas", Line: 9}, InjectionRisk, "use $1 placeholders")
	if got := w.String(); got != "?INJECTION RISK WARNING IN main.bas:9 — use $1 placeholders" {
		t.Fatalf("got %q", got)
	}
}
