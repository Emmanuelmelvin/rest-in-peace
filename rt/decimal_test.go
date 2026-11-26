package rt

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// helpers

func dec(s string) Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func mustPanic(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected a panic, got none", label)
		}
	}()
	fn()
}

// ---- DecimalOf ----------------------------------------------------------

func TestDecimalOf(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string // scale-preserving formatted string
	}{
		{"string integer", "42", "42"},
		{"string with scale", "19.99", "19.99"},
		{"string large", "1234567890.12345", "1234567890.12345"},
		{"string zero scale", "215.00", "215.00"},
		{"string negative", "-7.50", "-7.50"},
		{"INTEGER", int64(100), "100"},
		{"INTEGER negative", int64(-3), "-3"},
		{"NUMBER", float64(3.14), "3.14"},
		// DECIMAL passthrough: the input dec("5.50") has scale=2 preserved.
		{"DECIMAL passthrough", dec("5.50"), "5.50"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DecimalOf(tc.in)
			s := formatDecimal(got)
			if s != tc.want {
				t.Errorf("DecimalOf(%v) formatted = %q, want %q", tc.in, s, tc.want)
			}
		})
	}
}

func TestDecimalOf_InvalidString(t *testing.T) {
	mustPanic(t, "bad string", func() { DecimalOf("not-a-number") })
	mustPanic(t, "empty string", func() { DecimalOf("") })
}

func TestDecimalOf_WrongType(t *testing.T) {
	mustPanic(t, "bool", func() { DecimalOf(true) })
	mustPanic(t, "nil", func() { DecimalOf(nil) })
	mustPanic(t, "map", func() { DecimalOf(NewMap()) })
	mustPanic(t, "array", func() { DecimalOf(ArrayOf()) })
}

// ---- AsDecimal ----------------------------------------------------------

func TestAsDecimal(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
	}{
		{"DECIMAL", dec("12.34"), "12.34"},
		{"INTEGER", int64(7), "7"},
		{"NUMBER", float64(0.5), "0.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AsDecimal(tc.in)
			if got.String() != tc.want {
				t.Errorf("AsDecimal(%v) = %q, want %q", tc.in, got.String(), tc.want)
			}
		})
	}
}

func TestAsDecimal_WrongType(t *testing.T) {
	mustPanic(t, "string", func() { AsDecimal("19.99") })
	mustPanic(t, "bool", func() { AsDecimal(false) })
	mustPanic(t, "nil", func() { AsDecimal(nil) })
}

// ---- Round --------------------------------------------------------------

func TestRound(t *testing.T) {
	tests := []struct {
		name   string
		v      Value
		places []Value
		want   string
	}{
		// DECIMAL rounding — half away from zero
		{"2 places", dec("2.345"), []Value{int64(2)}, "2.35"},
		{"2 places down", dec("2.344"), []Value{int64(2)}, "2.34"},
		{"0 places", dec("2.5"), []Value{int64(0)}, "3"},
		{"0 places neg", dec("-2.5"), []Value{int64(0)}, "-3"}, // half away from zero
		{"negative places", dec("215.00"), []Value{int64(-1)}, "220"},
		{"scale preserved after round", dec("1.005"), []Value{int64(2)}, "1.01"},
		// no places arg defaults to 0 decimal places
		{"no places decimal", dec("3.7"), nil, "4"},
		{"no places integer passthrough", int64(5), nil, "5"},
		// NUMBER input
		{"number 2 places", float64(2.345), []Value{int64(2)}, "2.35"},
		// NUMBER places arg
		{"float places", dec("1.555"), []Value{float64(2)}, "1.56"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Round(tc.v, tc.places...)
			s, _ := decimalString(got)
			if s == "" {
				// integer result
				s = ToString(got)
			}
			if s != tc.want {
				t.Errorf("Round(%v, %v) = %q, want %q", tc.v, tc.places, s, tc.want)
			}
		})
	}
}

func TestRound_WrongType(t *testing.T) {
	mustPanic(t, "string value", func() { Round("hello") })
	mustPanic(t, "nil value", func() { Round(nil) })
	mustPanic(t, "bad places", func() { Round(dec("1.5"), "two") })
}

// ---- decimalArith -------------------------------------------------------

func TestDecimalArith(t *testing.T) {
	tests := []struct {
		name string
		op   byte
		a, b Value
		want string
		ok   bool
	}{
		// DECIMAL op DECIMAL
		{"decimal+decimal", '+', dec("10.00"), dec("5.50"), "15.50", true},
		{"decimal-decimal", '-', dec("10.00"), dec("5.50"), "4.50", true},
		{"decimal*decimal", '*', dec("3.00"), dec("2.50"), "7.5000", true},
		{"decimal/decimal", '/', dec("10.00"), dec("4.00"), "2.5", true},
		// DECIMAL op INTEGER
		{"decimal+int", '+', dec("10.00"), int64(5), "15.00", true},
		{"int+decimal", '+', int64(5), dec("10.00"), "15.00", true},
		// DECIMAL op NUMBER
		{"decimal+float", '+', dec("1.50"), float64(0.5), "2.00", true},
		{"float+decimal", '+', float64(0.5), dec("1.50"), "2.00", true},
		// INTEGER op INTEGER — no DECIMAL, falls through
		{"int+int fallthrough", '+', int64(2), int64(3), "", false},
		// NUMBER op NUMBER — falls through
		{"float+float fallthrough", '+', float64(1.0), float64(2.0), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decimalArith(tc.op, tc.a, tc.b)
			if ok != tc.ok {
				t.Errorf("decimalArith: ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			s, _ := decimalString(got)
			if s != tc.want {
				t.Errorf("decimalArith(%c, %v, %v) = %q, want %q", tc.op, tc.a, tc.b, s, tc.want)
			}
		})
	}
}

func TestDecimalArith_DivByZero(t *testing.T) {
	mustPanic(t, "decimal/0", func() { decimalArith('/', dec("5.00"), dec("0")) })
}

func TestDecimalArith_UnknownType(t *testing.T) {
	_, ok := decimalArith('+', "hello", dec("1"))
	if ok {
		t.Error("expected ok=false for string operand")
	}
	_, ok = decimalArith('+', dec("1"), "hello")
	if ok {
		t.Error("expected ok=false for string second operand")
	}
}

// ---- decimalCompare -----------------------------------------------------

func TestDecimalCompare(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Value
		wantCmp int
		wantOK  bool
	}{
		// DECIMAL vs DECIMAL
		{"eq", dec("1.00"), dec("1.0"), 0, true},
		{"lt", dec("1.00"), dec("2.00"), -1, true},
		{"gt", dec("2.00"), dec("1.00"), 1, true},
		// DECIMAL vs INTEGER
		{"decimal vs int eq", dec("5.0"), int64(5), 0, true},
		{"decimal vs int lt", dec("4.9"), int64(5), -1, true},
		// INTEGER vs DECIMAL
		{"int vs decimal gt", int64(6), dec("5.9"), 1, true},
		// DECIMAL vs NUMBER
		{"decimal vs float eq", dec("3.14"), float64(3.14), 0, true},
		// No DECIMAL involved — fall through
		{"int vs int fallthrough", int64(1), int64(2), 0, false},
		{"float vs float fallthrough", float64(1.0), float64(2.0), 0, false},
		// Non-numeric — fall through
		{"string fallthrough", "a", "b", 0, false},
		{"nil fallthrough", nil, nil, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmp, ok := decimalCompare(tc.a, tc.b)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && cmp != tc.wantCmp {
				t.Errorf("cmp = %d, want %d", cmp, tc.wantCmp)
			}
		})
	}
}

// ---- decimalNeg ---------------------------------------------------------

func TestDecimalNeg(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
		ok   bool
	}{
		{"positive", dec("5.50"), "-5.50", true},
		{"negative", dec("-3.00"), "3.00", true},
		{"zero", dec("0.00"), "0.00", true},
		{"int fallthrough", int64(1), "", false},
		{"float fallthrough", float64(1.0), "", false},
		{"nil fallthrough", nil, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decimalNeg(tc.in)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			s, _ := decimalString(got)
			if s != tc.want {
				t.Errorf("decimalNeg(%v) = %q, want %q", tc.in, s, tc.want)
			}
		})
	}
}

// ---- decimalString ------------------------------------------------------

func TestDecimalString(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string
		ok   bool
	}{
		{"215.00 keeps scale", dec("215.00"), "215.00", true},
		{"19.99", dec("19.99"), "19.99", true},
		{"zero", dec("0"), "0", true},
		{"zero with scale", dec("0.00"), "0.00", true},
		{"negative", dec("-7.50"), "-7.50", true},
		{"large", dec("1234567890.12345"), "1234567890.12345", true},
		{"int fallthrough", int64(5), "", false},
		{"float fallthrough", float64(3.14), "", false},
		{"string fallthrough", "hello", "", false},
		{"nil fallthrough", nil, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := decimalString(tc.in)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && s != tc.want {
				t.Errorf("decimalString(%v) = %q, want %q", tc.in, s, tc.want)
			}
		})
	}
}

// ---- decimalJSON --------------------------------------------------------

func TestDecimalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   Value
		want string // expected JSON bytes as string
		ok   bool
	}{
		{"215.00 as quoted string", dec("215.00"), `"215.00"`, true},
		{"19.99", dec("19.99"), `"19.99"`, true},
		{"zero", dec("0"), `"0"`, true},
		{"zero with scale", dec("0.00"), `"0.00"`, true},
		{"negative", dec("-7.50"), `"-7.50"`, true},
		{"int fallthrough", int64(5), "", false},
		{"float fallthrough", float64(3.14), "", false},
		{"nil fallthrough", nil, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, ok := decimalJSON(tc.in)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && string(data) != tc.want {
				t.Errorf("decimalJSON(%v) = %q, want %q", tc.in, data, tc.want)
			}
		})
	}
}

// ---- JSON marshaling via Map / Array ------------------------------------

func TestDecimalInMap_JSON(t *testing.T) {
	m := MapOf("price", dec("215.00"), "qty", int64(3))
	data, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("JSON unmarshal failed: %v — raw: %s", err, data)
	}
	if string(out["price"]) != `"215.00"` {
		t.Errorf("price = %s, want \"215.00\"", out["price"])
	}
	if string(out["qty"]) != `3` {
		t.Errorf("qty = %s, want 3", out["qty"])
	}
}

func TestDecimalInArray_JSON(t *testing.T) {
	a := ArrayOf(dec("1.10"), dec("2.20"), int64(3))
	data, err := a.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var out []json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("JSON unmarshal failed: %v — raw: %s", err, data)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if string(out[0]) != `"1.10"` {
		t.Errorf("[0] = %s, want \"1.10\"", out[0])
	}
	if string(out[1]) != `"2.20"` {
		t.Errorf("[1] = %s, want \"2.20\"", out[1])
	}
	if string(out[2]) != `3` {
		t.Errorf("[2] = %s, want 3", out[2])
	}
}

func TestNestedDecimalMap_JSON(t *testing.T) {
	inner := MapOf("tax", dec("0.08"), "total", dec("108.00"))
	outer := MapOf("invoice", inner, "id", int64(1))
	data, err := outer.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	// re-parse to validate structure
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	var inv map[string]json.RawMessage
	if err := json.Unmarshal(out["invoice"], &inv); err != nil {
		t.Fatalf("inner unmarshal: %v", err)
	}
	if string(inv["tax"]) != `"0.08"` {
		t.Errorf("tax = %s, want \"0.08\"", inv["tax"])
	}
	if string(inv["total"]) != `"108.00"` {
		t.Errorf("total = %s, want \"108.00\"", inv["total"])
	}
}

// ---- Integration: ops helpers (Add, Sub, Mul, Div, Neg, Eq, Lt, ToString) --

func TestOps_DecimalArithmetic(t *testing.T) {
	t.Run("Add decimal+decimal", func(t *testing.T) {
		got := Add(dec("10.00"), dec("5.50"))
		s, ok := decimalString(got)
		if !ok || s != "15.50" {
			t.Errorf("got %v, want 15.50", got)
		}
	})
	t.Run("Sub decimal-decimal", func(t *testing.T) {
		got := Sub(dec("10.00"), dec("5.50"))
		s, ok := decimalString(got)
		if !ok || s != "4.50" {
			t.Errorf("got %v, want 4.50", got)
		}
	})
	t.Run("Mul decimal*decimal", func(t *testing.T) {
		got := Mul(dec("3.00"), dec("2.50"))
		s, ok := decimalString(got)
		if !ok || s != "7.5000" {
			t.Errorf("got %v, want 7.5000", got)
		}
	})
	t.Run("Div decimal/decimal", func(t *testing.T) {
		got := Div(dec("10.00"), dec("4.00"))
		s, ok := decimalString(got)
		if !ok || s != "2.5" {
			t.Errorf("got %v, want 2.5", got)
		}
	})
	t.Run("Add decimal+int promotes", func(t *testing.T) {
		got := Add(dec("10.00"), int64(5))
		if _, isDecimal := got.(Decimal); !isDecimal {
			t.Errorf("expected Decimal, got %T", got)
		}
	})
	t.Run("Add int+decimal promotes", func(t *testing.T) {
		got := Add(int64(5), dec("10.00"))
		if _, isDecimal := got.(Decimal); !isDecimal {
			t.Errorf("expected Decimal, got %T", got)
		}
	})
	t.Run("Add decimal+float promotes", func(t *testing.T) {
		got := Add(dec("1.50"), float64(0.5))
		if _, isDecimal := got.(Decimal); !isDecimal {
			t.Errorf("expected Decimal, got %T", got)
		}
	})
}

func TestOps_DecimalDivByZero(t *testing.T) {
	mustPanic(t, "Div decimal by zero", func() { Div(dec("5.00"), dec("0")) })
}

func TestOps_DecimalNeg(t *testing.T) {
	got := Neg(dec("5.50"))
	s, ok := decimalString(got)
	if !ok || s != "-5.50" {
		t.Errorf("Neg(5.50) = %v, want -5.50", got)
	}
}

func TestOps_DecimalComparisons(t *testing.T) {
	t.Run("Eq same value different scale", func(t *testing.T) {
		if !Eq(dec("1.00"), dec("1.0")) {
			t.Error("1.00 should equal 1.0")
		}
	})
	t.Run("Eq decimal vs int", func(t *testing.T) {
		if !Eq(dec("5.00"), int64(5)) {
			t.Error("5.00 should equal INTEGER 5")
		}
	})
	t.Run("Lt decimal", func(t *testing.T) {
		if !Lt(dec("1.99"), dec("2.00")) {
			t.Error("1.99 should be < 2.00")
		}
	})
	t.Run("Gt decimal", func(t *testing.T) {
		if !Gt(dec("2.01"), dec("2.00")) {
			t.Error("2.01 should be > 2.00")
		}
	})
	t.Run("Le decimal", func(t *testing.T) {
		if !Le(dec("2.00"), dec("2.00")) {
			t.Error("2.00 should be <= 2.00")
		}
	})
	t.Run("Ge decimal", func(t *testing.T) {
		if !Ge(dec("2.01"), dec("2.00")) {
			t.Error("2.01 should be >= 2.00")
		}
	})
	t.Run("Ne decimal", func(t *testing.T) {
		if !Ne(dec("1.00"), dec("2.00")) {
			t.Error("1.00 should != 2.00")
		}
	})
}

func TestOps_DecimalToString(t *testing.T) {
	tests := []struct {
		in   Value
		want string
	}{
		{dec("215.00"), "215.00"},
		{dec("19.99"), "19.99"},
		{dec("0.00"), "0.00"},
		{dec("-7.50"), "-7.50"},
	}
	for _, tc := range tests {
		got := ToString(tc.in)
		if got != tc.want {
			t.Errorf("ToString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---- Mixed INTEGER / NUMBER / DECIMAL arithmetic ------------------------

func TestMixedArithmetic(t *testing.T) {
	// All mixed-decimal ops should return Decimal
	pairs := []struct {
		name string
		a, b Value
		op   byte
	}{
		{"decimal+int", dec("10.00"), int64(2), '+'},
		{"int+decimal", int64(2), dec("10.00"), '+'},
		{"decimal+float", dec("10.00"), float64(2.0), '+'},
		{"float+decimal", float64(2.0), dec("10.00"), '+'},
		{"decimal-int", dec("10.00"), int64(2), '-'},
		{"decimal*int", dec("10.00"), int64(2), '*'},
		{"decimal/int", dec("10.00"), int64(2), '/'},
	}
	for _, tc := range pairs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decimalArith(tc.op, tc.a, tc.b)
			if !ok {
				t.Fatal("expected ok=true")
			}
			if _, isDecimal := got.(Decimal); !isDecimal {
				t.Errorf("expected Decimal result, got %T", got)
			}
		})
	}
}

// ---- Error message quality ----------------------------------------------

func TestDecimalOf_ErrorContainsValue(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", r)
		}
		if e.Kind != KindTypeMismatch {
			t.Errorf("kind = %q, want %q", e.Kind, KindTypeMismatch)
		}
	}()
	DecimalOf("bad input value")
}

func TestAsDecimal_ErrorMessage(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", r)
		}
		if e.Kind != KindTypeMismatch {
			t.Errorf("kind = %q, want %q", e.Kind, KindTypeMismatch)
		}
	}()
	AsDecimal("not a decimal")
}

func TestRound_WrongTypePanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", r)
		}
		if e.Kind != KindTypeMismatch {
			t.Errorf("kind = %q, want %q", e.Kind, KindTypeMismatch)
		}
	}()
	Round(nil)
}

func TestDecimalDivByZero_ErrorKind(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", r)
		}
		if e.Kind != KindDivisionByZero {
			t.Errorf("kind = %q, want %q", e.Kind, KindDivisionByZero)
		}
	}()
	decimalArith('/', dec("1.00"), dec("0.00"))
}
