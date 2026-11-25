package rt

import (
	"math"
	"testing"
)

func TestIntDivModPow(t *testing.T) {
	tests := []struct {
		name string
		got  Value
		want Value
	}{
		{`7 \ 2`, IntDiv(int64(7), int64(2)), int64(3)},
		{`-7 \ 2`, IntDiv(int64(-7), int64(2)), int64(-3)},
		{`7.9 \ 2`, IntDiv(7.9, int64(2)), int64(3)},
		{"7 MOD 2", Mod(int64(7), int64(2)), int64(1)},
		{"-7 MOD 2", Mod(int64(-7), int64(2)), int64(-1)},
		{"7.5 MOD 2", Mod(7.5, int64(2)), 1.5},
		{"2 ^ 10", Pow(int64(2), int64(10)), int64(1024)},
		{"2 ^ 64", Pow(int64(2), int64(64)), math.Pow(2, 64)},
		{"-3 ^ 3", Pow(int64(-3), int64(3)), int64(-27)},
		{"2 ^ -1", Pow(int64(2), int64(-1)), 0.5},
		{"4 ^ 0.5", Pow(int64(4), 0.5), 2.0},
		{"-x", Neg(int64(3)), int64(-3)},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v (%T), want %v (%T)", tt.name, tt.got, tt.got, tt.want, tt.want)
		}
	}
}

func TestDivisionByZero(t *testing.T) {
	for name, f := range map[string]func(){
		`\`:       func() { IntDiv(int64(1), int64(0)) },
		`\ NUM`:   func() { IntDiv(1.5, 0.0) },
		"MOD":     func() { Mod(int64(1), int64(0)) },
		"MOD NUM": func() { Mod(1.5, 0.0) },
		"/":       func() { Div(int64(1), int64(0)) },
		"DivNum":  func() { DivNum(1, 0) },
		"0 ^ -1":  func() { Pow(int64(0), int64(-1)) },
	} {
		if k := panicKind(f); k != KindDivisionByZero {
			t.Errorf("%s: got %q, want DIVISION BY ZERO", name, k)
		}
	}
}

func TestBuiltins(t *testing.T) {
	tests := []struct {
		name string
		got  Value
		want Value
	}{
		{"LEN string", Len("héllo"), int64(5)},
		{"LEN array", Len(ArrayOf(int64(1), int64(2))), int64(2)},
		{"LEN map", Len(MapOf("a", int64(1))), int64(1)},
		{"LEN NIL", Len(nil), int64(0)},
		{"UPPER", Upper("abc"), "ABC"},
		{"LOWER", Lower("ABC"), "abc"},
		{"TRIM", Trim("  a b  "), "a b"},
		{"LEFT", Left("HELLO", int64(2)), "HE"},
		{"LEFT too many", Left("HI", int64(5)), "HI"},
		{"RIGHT", Right("HELLO", int64(3)), "LLO"},
		{"RIGHT unicode", Right("héllo", int64(4)), "éllo"},
		{"MID", Mid("HELLO", int64(2), int64(3)), "ELL"},
		{"MID rest", Mid("HELLO", int64(3)), "LLO"},
		{"MID past end", Mid("HI", int64(5)), ""},
		{"STR int", Str(int64(42)), "42"},
		{"STR float", Str(2.5), "2.5"},
		{"STR bool", Str(true), "TRUE"},
		{"VAL int", Val("42"), int64(42)},
		{"VAL prefix", Val("12abc"), int64(12)},
		{"VAL float", Val(" -3.5e2 "), -350.0},
		{"VAL none", Val("abc"), int64(0)},
		{"VAL number", Val(2.5), 2.5},
		{"INT floor", Int(-2.5), int64(-3)},
		{"INT int", Int(int64(7)), int64(7)},
		{"ABS int", Abs(int64(-3)), int64(3)},
		{"ABS float", Abs(-2.5), 2.5},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v (%T), want %v (%T)", tt.name, tt.got, tt.got, tt.want, tt.want)
		}
	}
}

func TestBuiltinErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		f    func()
		kind Kind
	}{
		"LEN number":  {func() { Len(int64(1)) }, KindTypeMismatch},
		"UPPER NIL":   {func() { Upper(nil) }, KindTypeMismatch},
		"MID start 0": {func() { Mid("x", int64(0)) }, KindBadSubscript},
		"LEFT -1":     {func() { Left("x", int64(-1)) }, KindBadSubscript},
		"INT string":  {func() { Int("1") }, KindTypeMismatch},
		"ABS string":  {func() { Abs("1") }, KindTypeMismatch},
		"AsInt 2.5":   {func() { AsInt(2.5) }, KindTypeMismatch},
		"AsStr 1":     {func() { AsStr(int64(1)) }, KindTypeMismatch},
		"Iter NIL":    {func() { Iter(nil) }, KindNilAccess},
		"Set on NIL":  {func() { SetMember(nil, "a", int64(1)) }, KindNilAccess},
	} {
		if k := panicKind(tc.f); k != tc.kind {
			t.Errorf("%s: got %q, want %q", name, k, tc.kind)
		}
	}
}

func TestForCond(t *testing.T) {
	if !ForCond(int64(1), int64(3), int64(1)) || ForCond(int64(4), int64(3), int64(1)) {
		t.Error("positive STEP")
	}
	if !ForCond(int64(3), int64(1), int64(-1)) || ForCond(int64(0), int64(1), int64(-1)) {
		t.Error("negative STEP")
	}
	if !ForCond(1.5, int64(2), 0.5) {
		t.Error("mixed NUMBER and INTEGER")
	}
}

func TestFromPanicKinds(t *testing.T) {
	var arr []int
	i := 3
	err := func() (e *Error) {
		defer func() { e = FromPanic(recover()) }()
		_ = arr[i]
		return nil
	}()
	if err.Kind != KindBadSubscript {
		t.Errorf("index out of range → %s, want BAD SUBSCRIPT", err.Kind)
	}
}

func panicKind(f func()) (k Kind) {
	defer func() {
		if e, ok := recover().(*Error); ok {
			k = e.Kind
		}
	}()
	f()
	return ""
}
