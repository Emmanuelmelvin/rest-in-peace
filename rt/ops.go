package rt

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Operators on dynamic values. Codegen emits plain Go operators when both
// operands have native types (design §4.2), and these helpers otherwise.

// Add implements +. If either side is a STRING, it concatenates (Requirement 3.6).
func Add(a, b Value) Value {
	_, aStr := a.(string)
	_, bStr := b.(string)
	if aStr || bStr {
		return ToString(a) + ToString(b)
	}
	return arith('+', a, b)
}

// Sub implements -.
func Sub(a, b Value) Value { return arith('-', a, b) }

// Mul implements *.
func Mul(a, b Value) Value { return arith('*', a, b) }

// Div implements /, which always produces a NUMBER (use \ for integer division).
func Div(a, b Value) Value { return arith('/', a, b) }

// IntDiv implements \. The result is an INTEGER: the quotient truncated toward zero.
func IntDiv(a, b Value) Value {
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if !aok || !bok {
		panic(Errorf(KindTypeMismatch, `cannot use \ on %s and %s`, TypeName(a), TypeName(b)))
	}
	if aInt && bInt {
		return IntDivInt(ai, bi)
	}
	if bf == 0 {
		panic(divByZero(`\`))
	}
	q := math.Trunc(af / bf)
	if q < math.MinInt64 || q >= math.MaxInt64 || math.IsNaN(q) {
		panic(Errorf(KindTypeMismatch, `the result of \ is too big for an INTEGER`))
	}
	return int64(q)
}

// IntDivInt is \ on two native INTEGERs.
func IntDivInt(a, b int64) int64 {
	if b == 0 {
		panic(divByZero(`\`))
	}
	return a / b
}

// Mod implements MOD. The result has the sign of the left side, as in Go.
func Mod(a, b Value) Value {
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if !aok || !bok {
		panic(Errorf(KindTypeMismatch, "cannot use MOD on %s and %s", TypeName(a), TypeName(b)))
	}
	if aInt && bInt {
		return ModInt(ai, bi)
	}
	return ModNum(af, bf)
}

// ModInt is MOD on two native INTEGERs.
func ModInt(a, b int64) int64 {
	if b == 0 {
		panic(divByZero("MOD"))
	}
	return a % b
}

// ModNum is MOD on two native NUMBERs.
func ModNum(a, b float64) float64 {
	if b == 0 {
		panic(divByZero("MOD"))
	}
	return math.Mod(a, b)
}

// DivNum is / on two native NUMBERs.
func DivNum(a, b float64) float64 {
	if b == 0 {
		panic(divByZero("/"))
	}
	return a / b
}

// Pow implements ^. INTEGER ^ non-negative INTEGER stays an INTEGER while it
// fits; everything else is a NUMBER.
func Pow(a, b Value) Value {
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if !aok || !bok {
		panic(Errorf(KindTypeMismatch, "cannot use ^ on %s and %s", TypeName(a), TypeName(b)))
	}
	if aInt && bInt && bi >= 0 {
		if r, ok := powInt(ai, bi); ok {
			return r
		}
	}
	return PowNum(af, bf)
}

// PowNum is ^ on two native NUMBERs.
func PowNum(a, b float64) float64 {
	if a == 0 && b < 0 {
		panic(divByZero("^"))
	}
	return math.Pow(a, b)
}

// powInt computes a^b by squaring and reports false if it overflows int64.
func powInt(a, b int64) (int64, bool) {
	r := int64(1)
	for b > 0 {
		if b&1 == 1 {
			if !mulFits(r, a) {
				return 0, false
			}
			r *= a
		}
		b >>= 1
		if b > 0 {
			if !mulFits(a, a) {
				return 0, false
			}
			a *= a
		}
	}
	return r, true
}

func mulFits(a, b int64) bool {
	if a == 0 || b == 0 {
		return true
	}
	c := a * b
	return c/b == a && !(a == -1 && b == math.MinInt64) && !(b == -1 && a == math.MinInt64)
}

func divByZero(op string) *Error {
	return Errorf(KindDivisionByZero, "the right side of %s is zero", op)
}

// Neg implements unary minus.
func Neg(a Value) Value {
	switch x := a.(type) {
	case int64:
		return -x
	case float64:
		return -x
	}
	panic(Errorf(KindTypeMismatch, "cannot use - on a %s", TypeName(a)))
}

// Ne implements <>.
func Ne(a, b Value) bool { return !Eq(a, b) }

// Gt implements >. It evaluates its operands left to right, like the source.
func Gt(a, b Value) bool { return Lt(b, a) }

// Le implements <=.
func Le(a, b Value) bool { return !Lt(b, a) }

// Ge implements >=.
func Ge(a, b Value) bool { return !Lt(a, b) }

// Concat implements &, which always concatenates.
func Concat(a, b Value) string { return ToString(a) + ToString(b) }

// Eq implements = in expressions.
func Eq(a, b Value) bool {
	if ai, af, aInt, ok := num(a); ok {
		if bi, bf, bInt, ok2 := num(b); ok2 {
			if aInt && bInt {
				return ai == bi
			}
			return af == bf
		}
		return false
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	}
	// TODO(task 2.5): DECIMAL. Maps and arrays compare by identity for now.
	return a == b
}

// Lt implements <. The other comparisons are built from Lt and Eq.
func Lt(a, b Value) bool {
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if aok && bok {
		if aInt && bInt {
			return ai < bi
		}
		return af < bf
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return x < y
		}
	}
	// TODO(task 2.5): DECIMAL and DATE comparisons.
	panic(Errorf(KindTypeMismatch, "cannot compare %s with %s", TypeName(a), TypeName(b)))
}

// ToString converts a value the way PRINT and string concatenation show it.
func ToString(v Value) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

// Print implements PRINT.
// TODO(task 1.3): the ; and , separators of classic PRINT.
func Print(vals ...Value) {
	var b strings.Builder
	for _, v := range vals {
		b.WriteString(ToString(v))
	}
	b.WriteByte('\n')
	os.Stdout.WriteString(b.String())
}

func num(v Value) (i int64, f float64, isInt bool, ok bool) {
	switch x := v.(type) {
	case int64:
		return x, float64(x), true, true
	case float64:
		return 0, x, false, true
	}
	return 0, 0, false, false
}

func arith(op byte, a, b Value) Value {
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if !aok || !bok {
		// TODO(task 2.5): DECIMAL operands and DECIMAL/NUMBER mixing.
		panic(Errorf(KindTypeMismatch, "cannot use %c on %s and %s", op, TypeName(a), TypeName(b)))
	}
	if op == '/' {
		if bf == 0 {
			panic(Errorf(KindDivisionByZero, "the right side of / is zero"))
		}
		return af / bf
	}
	if aInt && bInt {
		switch op {
		case '+':
			return ai + bi
		case '-':
			return ai - bi
		case '*':
			return ai * bi
		}
	}
	switch op {
	case '+':
		return af + bf
	case '-':
		return af - bf
	case '*':
		return af * bf
	}
	panic(Errorf(KindInternal, "unknown operator %c", op))
}
