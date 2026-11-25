package rt

import "math"

// Conversions and control-flow helpers that generated code calls.

// AsInt converts a value for an INTEGER variable, parameter or return value.
// A whole NUMBER is accepted; anything else is a TYPE MISMATCH (Requirement 3.2).
func AsInt(v Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		if x == math.Trunc(x) && x >= math.MinInt64 && x < math.MaxInt64 {
			return int64(x)
		}
	}
	panic(Errorf(KindTypeMismatch, "expected an INTEGER, got %s %s", TypeName(v), ToString(v)))
}

// AsNum converts a value for a NUMBER variable, parameter or return value.
func AsNum(v Value) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	panic(Errorf(KindTypeMismatch, "expected a NUMBER, got %s", TypeName(v)))
}

// AsStr converts a value for a STRING variable, parameter or return value.
func AsStr(v Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	panic(Errorf(KindTypeMismatch, "expected a STRING, got %s — use STR() to turn a value into text", TypeName(v)))
}

// AsBool converts a value for a BOOLEAN variable, parameter or return value.
func AsBool(v Value) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	panic(Errorf(KindTypeMismatch, "expected a BOOLEAN, got %s", TypeName(v)))
}

// ForCond reports whether a dynamic FOR loop should run another time: v <= to
// for a positive (or zero) STEP, v >= to for a negative one.
func ForCond(v, to, step Value) bool {
	_, sf, _, ok := num(step)
	if !ok {
		panic(Errorf(KindTypeMismatch, "FOR … STEP needs a NUMBER, got %s", TypeName(step)))
	}
	if _, _, _, ok := num(v); !ok {
		panic(Errorf(KindTypeMismatch, "the FOR variable must be a NUMBER, got %s", TypeName(v)))
	}
	if _, _, _, ok := num(to); !ok {
		panic(Errorf(KindTypeMismatch, "FOR … TO needs a NUMBER, got %s", TypeName(to)))
	}
	if sf < 0 {
		return !Lt(v, to)
	}
	return !Lt(to, v)
}

// Iter returns what FOR EACH walks over: an array's items, or a map's keys in
// insertion order. The slice is a snapshot, so changing the array inside the
// loop doesn't change how many times it runs.
func Iter(v Value) []Value {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "FOR EACH cannot loop over NIL"))
	case *Array:
		if x == nil {
			panic(Errorf(KindNilAccess, "FOR EACH cannot loop over NIL"))
		}
		return append([]Value(nil), x.Items...)
	case *Map:
		keys := x.Keys()
		out := make([]Value, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return out
	}
	panic(Errorf(KindTypeMismatch, "FOR EACH needs an ARRAY or a MAP, got %s", TypeName(v)))
}

// OnIndex converts the selector of ON x GOTO to a 1-based choice. A NUMBER is
// truncated, as in classic BASIC; out-of-range choices fall through.
func OnIndex(v Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(math.Trunc(x))
	}
	panic(Errorf(KindTypeMismatch, "ON … GOTO needs a NUMBER, got %s", TypeName(v)))
}

// endSignal is raised by END and recovered by Main.
type endSignal struct{}

// End implements END: the program stops normally.
func End() { panic(endSignal{}) }
