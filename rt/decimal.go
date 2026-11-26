package rt

// DECIMAL: exact money math (Requirement 3.7, task 2.5).
//
// This file is owned by the Kiro task 2.5 (.kiro/steering/structure.md). It
// ships with stub bodies: shared code (ops.go, value.go, codegen) already
// calls every function below, so implementing them here is all DECIMAL needs.
// Do not change the signatures without updating the callers listed on each.

import "github.com/shopspring/decimal"

// Decimal is the DECIMAL type. A variable, parameter or result declared
// AS DECIMAL compiles to a native rt.Decimal (Requirement 3.3); the zero value
// rt.Decimal{} is 0. Boxed in an rt.Value, a DECIMAL is an rt.Decimal too.
type Decimal = decimal.Decimal

// DecimalOf implements DECIMAL(x): x is a STRING like "19.99", an INTEGER, a
// NUMBER or a DECIMAL. Called by generated code for DECIMAL(…).
func DecimalOf(v Value) Decimal {
	// TODO(task 2.5): parse strings exactly; convert INTEGER and NUMBER; a bad
	// string raises ?TYPE MISMATCH ERROR naming the value.
	panic(notBuilt("DECIMAL", "2.5"))
}

// AsDecimal converts a value stored into something declared AS DECIMAL. A
// value of the wrong type raises ?TYPE MISMATCH ERROR (Requirement 3.2).
// Called by generated code, like AsInt and AsNum in convert.go.
func AsDecimal(v Value) Decimal {
	// TODO(task 2.5): accept DECIMAL, INTEGER and NUMBER (Requirement 3.7.3).
	panic(notBuilt("the DECIMAL type", "2.5"))
}

// Round implements ROUND(x [, places]). Called by generated code for ROUND(…).
func Round(v Value, places ...Value) Value {
	// TODO(task 2.5): round DECIMALs (and NUMBERs); rounding mode per the
	// "Decisions to confirm" table in requirements.md.
	panic(notBuilt("ROUND", "2.5"))
}

// decimalArith computes a op b (op is '+', '-', '*' or '/') when either side
// is a DECIMAL, and reports ok = false otherwise so the caller falls back to
// INTEGER/NUMBER math. Called by arith in ops.go (which Add, Sub, Mul and Div use).
func decimalArith(op byte, a, b Value) (result Value, ok bool) {
	// TODO(task 2.5): DECIMAL op DECIMAL, and mixing with INTEGER/NUMBER gives
	// DECIMAL (Requirement 3.7.3). Division by zero raises ?DIVISION BY ZERO ERROR.
	return nil, false
}

// decimalCompare compares a and b (-1, 0 or 1) when either side is a DECIMAL,
// and reports ok = false otherwise. Called by Eq and Lt in ops.go, which every
// comparison operator uses.
func decimalCompare(a, b Value) (cmp int, ok bool) {
	// TODO(task 2.5)
	return 0, false
}

// decimalNeg implements unary minus when a is a DECIMAL. Called by Neg in ops.go.
func decimalNeg(a Value) (result Value, ok bool) {
	// TODO(task 2.5)
	return nil, false
}

// decimalString formats a DECIMAL for PRINT and string concatenation, and
// reports ok = false for any other value. Called by ToString in ops.go.
func decimalString(v Value) (s string, ok bool) {
	// TODO(task 2.5): keep the scale, e.g. "215.00".
	return "", false
}

// decimalJSON encodes a DECIMAL in JSON responses and logs, and reports
// ok = false for any other value. Called by marshalValue in value.go, which
// Map and Array use for every element.
func decimalJSON(v Value) (data []byte, ok bool) {
	// TODO(task 2.5): a string that keeps the scale, "215.00" (requirements.md,
	// "Decisions to confirm").
	return nil, false
}
