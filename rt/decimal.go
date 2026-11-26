package rt

// DECIMAL: exact money math (Requirement 3.7, task 2.5).
//
// This file is owned by the Kiro task 2.5 (.kiro/steering/structure.md). It
// ships with stub bodies: shared code (ops.go, value.go, codegen) already
// calls every function below, so implementing them here is all DECIMAL needs.
// Do not change the signatures without updating the callers listed on each.

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Decimal is the DECIMAL type. A variable, parameter or result declared
// AS DECIMAL compiles to a native rt.Decimal (Requirement 3.3); the zero value
// rt.Decimal{} is 0. Boxed in an rt.Value, a DECIMAL is an rt.Decimal too.
type Decimal = decimal.Decimal

// DecimalOf implements DECIMAL(x): x is a STRING like "19.99", an INTEGER, a
// NUMBER or a DECIMAL. Called by generated code for DECIMAL(…).
func DecimalOf(v Value) Decimal {
	switch x := v.(type) {
	case Decimal:
		return x
	case int64:
		return decimal.NewFromInt(x)
	case float64:
		return decimal.NewFromFloat(x)
	case string:
		d, err := decimal.NewFromString(x)
		if err != nil {
			panic(Errorf(KindTypeMismatch,
				"DECIMAL(%q) is not a valid number — use a string like \"19.99\"", x))
		}
		return d
	}
	panic(Errorf(KindTypeMismatch,
		"DECIMAL() needs a STRING, INTEGER, NUMBER or DECIMAL, got %s", TypeName(v)))
}

// AsDecimal converts a value stored into something declared AS DECIMAL. A
// value of the wrong type raises ?TYPE MISMATCH ERROR (Requirement 3.2).
// Called by generated code, like AsInt and AsNum in convert.go.
func AsDecimal(v Value) Decimal {
	switch x := v.(type) {
	case Decimal:
		return x
	case int64:
		return decimal.NewFromInt(x)
	case float64:
		return decimal.NewFromFloat(x)
	}
	panic(Errorf(KindTypeMismatch,
		"expected a DECIMAL, got %s — use DECIMAL() to convert it", TypeName(v)))
}

// Round implements ROUND(x [, places]). Called by generated code for ROUND(…).
// Rounding mode: half away from zero (requirements.md "Decisions to confirm").
func Round(v Value, places ...Value) Value {
	var prec int32
	if len(places) > 0 && places[0] != nil {
		switch p := places[0].(type) {
		case int64:
			prec = int32(p)
		case float64:
			prec = int32(p)
		default:
			panic(Errorf(KindTypeMismatch,
				"ROUND() places must be an INTEGER or NUMBER, got %s", TypeName(places[0])))
		}
	}

	switch x := v.(type) {
	case Decimal:
		return x.Round(prec)
	case int64:
		if prec >= 0 {
			return x
		}
		// negative prec rounds to tens, hundreds, etc.
		d := decimal.NewFromInt(x)
		return d.Round(prec)
	case float64:
		d := decimal.NewFromFloat(x)
		return d.Round(prec)
	}
	panic(Errorf(KindTypeMismatch,
		"ROUND() needs a DECIMAL or NUMBER, got %s", TypeName(v)))
}

// decimalArith computes a op b (op is '+', '-', '*' or '/') when either side
// is a DECIMAL, and reports ok = false otherwise so the caller falls back to
// INTEGER/NUMBER math. Called by arith in ops.go (which Add, Sub, Mul and Div use).
func decimalArith(op byte, a, b Value) (result Value, ok bool) {
	var da, db Decimal
	var aIsDecimal, bIsDecimal bool

	switch x := a.(type) {
	case Decimal:
		da, aIsDecimal = x, true
	case int64:
		da = decimal.NewFromInt(x)
	case float64:
		da = decimal.NewFromFloat(x)
	default:
		return nil, false
	}

	switch x := b.(type) {
	case Decimal:
		db, bIsDecimal = x, true
	case int64:
		db = decimal.NewFromInt(x)
	case float64:
		db = decimal.NewFromFloat(x)
	default:
		return nil, false
	}

	// Only intercept when at least one side is a DECIMAL (Requirement 3.7.3).
	if !aIsDecimal && !bIsDecimal {
		return nil, false
	}

	switch op {
	case '+':
		return da.Add(db), true
	case '-':
		return da.Sub(db), true
	case '*':
		return da.Mul(db), true
	case '/':
		if db.IsZero() {
			panic(Errorf(KindDivisionByZero, "the right side of / is zero"))
		}
		return da.Div(db), true
	}
	return nil, false
}

// decimalCompare compares a and b (-1, 0 or 1) when either side is a DECIMAL,
// and reports ok = false otherwise. Called by Eq and Lt in ops.go, which every
// comparison operator uses.
func decimalCompare(a, b Value) (cmp int, ok bool) {
	var da, db Decimal
	var aIsDecimal, bIsDecimal bool

	switch x := a.(type) {
	case Decimal:
		da, aIsDecimal = x, true
	case int64:
		da = decimal.NewFromInt(x)
	case float64:
		da = decimal.NewFromFloat(x)
	default:
		return 0, false
	}

	switch x := b.(type) {
	case Decimal:
		db, bIsDecimal = x, true
	case int64:
		db = decimal.NewFromInt(x)
	case float64:
		db = decimal.NewFromFloat(x)
	default:
		return 0, false
	}

	if !aIsDecimal && !bIsDecimal {
		return 0, false
	}

	return da.Cmp(db), true
}

// decimalNeg implements unary minus when a is a DECIMAL. Called by Neg in ops.go.
func decimalNeg(a Value) (result Value, ok bool) {
	if d, isDecimal := a.(Decimal); isDecimal {
		return d.Neg(), true
	}
	return nil, false
}

// decimalString formats a DECIMAL for PRINT and string concatenation, keeping
// its scale (e.g. "215.00"), and reports ok = false for any other value.
// Called by ToString in ops.go.
func decimalString(v Value) (s string, ok bool) {
	if d, isDecimal := v.(Decimal); isDecimal {
		return formatDecimal(d), true
	}
	return "", false
}

// decimalJSON encodes a DECIMAL in JSON responses and logs, and reports
// ok = false for any other value. Called by marshalValue in value.go, which
// Map and Array use for every element.
// Per requirements.md "Decisions to confirm": serialises as a quoted string
// that keeps the scale, e.g. "215.00".
func decimalJSON(v Value) (data []byte, ok bool) {
	if d, isDecimal := v.(Decimal); isDecimal {
		s := formatDecimal(d)
		// Encode as a JSON string (quoted, scale-preserving).
		quoted := fmt.Sprintf("%q", s)
		return []byte(quoted), true
	}
	return nil, false
}

// formatDecimal returns the decimal's string representation with its scale
// preserved. shopspring's d.String() strips trailing zeros (so "215.00"
// becomes "215"), but d.Exponent() records the internal exponent. We restore
// trailing zeros only when the exponent is in a user-meaningful range (≤ 10
// decimal places). Division sets the exponent to −16, which should NOT be
// padded out — the natural significant digits from d.String() are correct
// there (e.g. "2.5" for 10.00/4.00).
const maxDecimalScale = int32(10)

func formatDecimal(d Decimal) string {
	exp := d.Exponent()
	if exp >= 0 {
		return d.String()
	}
	scale := -exp // positive number of fractional digits recorded internally
	if scale > maxDecimalScale {
		// Result of division or other high-precision operation: emit
		// only the significant digits.
		return d.String()
	}
	return d.StringFixed(scale)
}
