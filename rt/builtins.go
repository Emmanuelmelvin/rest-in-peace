package rt

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Phase 1 built-in functions (language.md). Strings are measured and sliced
// in characters (runes), not bytes. LEFT, RIGHT and MID count from 1, as in
// classic BASIC. ROUND and DECIMAL arrive with task 2.5.

// Len implements LEN: characters in a STRING, items in an ARRAY, keys in a
// MAP, and 0 for NIL.
func Len(v Value) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case string:
		return int64(utf8.RuneCountInString(x))
	case *Array:
		return int64(x.Len())
	case *Map:
		return int64(x.Len())
	}
	panic(Errorf(KindTypeMismatch, "LEN needs a STRING, ARRAY or MAP, got %s", TypeName(v)))
}

// Upper implements UPPER.
func Upper(v Value) string { return strings.ToUpper(strArg("UPPER", v)) }

// Lower implements LOWER.
func Lower(v Value) string { return strings.ToLower(strArg("LOWER", v)) }

// Trim implements TRIM: leading and trailing white space is removed.
func Trim(v Value) string { return strings.TrimSpace(strArg("TRIM", v)) }

// Left implements LEFT(s, n): the first n characters.
func Left(s, n Value) string {
	r := []rune(strArg("LEFT", s))
	k := countArg("LEFT", n)
	if k > int64(len(r)) {
		k = int64(len(r))
	}
	return string(r[:k])
}

// Right implements RIGHT(s, n): the last n characters.
func Right(s, n Value) string {
	r := []rune(strArg("RIGHT", s))
	k := countArg("RIGHT", n)
	if k > int64(len(r)) {
		k = int64(len(r))
	}
	return string(r[int64(len(r))-k:])
}

// Mid implements MID(s, start [, n]): n characters (or the rest) from the
// 1-based position start.
func Mid(s, start Value, n ...Value) string {
	r := []rune(strArg("MID", s))
	st, ok := toInt(start)
	if !ok {
		panic(Errorf(KindTypeMismatch, "MID needs a whole NUMBER for the start, got %s", TypeName(start)))
	}
	if st < 1 {
		panic(Errorf(KindBadSubscript, "MID starts counting at 1, got %d", st))
	}
	if st > int64(len(r)) {
		return ""
	}
	rest := r[st-1:]
	if len(n) == 0 {
		return string(rest)
	}
	k := countArg("MID", n[0])
	if k > int64(len(rest)) {
		k = int64(len(rest))
	}
	return string(rest[:k])
}

// Str implements STR: the value as PRINT shows it.
func Str(v Value) string { return ToString(v) }

// Val implements VAL: the number at the start of a STRING, or 0 if there is
// none, as in classic BASIC. Leading and trailing spaces are ignored.
func Val(v Value) Value {
	switch x := v.(type) {
	case int64, float64:
		return x
	case string:
		return parseLeadingNumber(strings.TrimSpace(x))
	}
	panic(Errorf(KindTypeMismatch, "VAL needs a STRING, got %s", TypeName(v)))
}

// Int implements INT: the largest whole number not greater than the value
// (so INT(-2.5) = -3), as an INTEGER.
func Int(v Value) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		f := math.Floor(x)
		if f >= math.MinInt64 && f < math.MaxInt64 {
			return int64(f)
		}
		panic(Errorf(KindTypeMismatch, "INT(%s) is too big for an INTEGER", ToString(v)))
	}
	panic(Errorf(KindTypeMismatch, "INT needs a NUMBER, got %s", TypeName(v)))
}

// Abs implements ABS. An INTEGER stays an INTEGER.
func Abs(v Value) Value {
	switch x := v.(type) {
	case int64:
		if x < 0 {
			return -x
		}
		return x
	case float64:
		return math.Abs(x)
	}
	panic(Errorf(KindTypeMismatch, "ABS needs a NUMBER, got %s", TypeName(v)))
}

func strArg(fn string, v Value) string {
	s, ok := v.(string)
	if !ok {
		panic(Errorf(KindTypeMismatch, "%s needs a STRING, got %s", fn, TypeName(v)))
	}
	return s
}

func countArg(fn string, v Value) int64 {
	n, ok := toInt(v)
	if !ok {
		panic(Errorf(KindTypeMismatch, "%s needs a whole NUMBER for the length, got %s", fn, TypeName(v)))
	}
	if n < 0 {
		panic(Errorf(KindBadSubscript, "%s cannot take %d characters", fn, n))
	}
	return n
}

// parseLeadingNumber reads [sign] digits [. digits] [e [sign] digits] from the
// start of s. Whole numbers become INTEGERs, the rest NUMBERs.
func parseLeadingNumber(s string) Value {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	isFloat := false
	if i < len(s) && s[i] == '.' {
		j := i + 1
		frac := 0
		for j < len(s) && isDigit(s[j]) {
			j++
			frac++
		}
		if digits+frac > 0 {
			i, digits, isFloat = j, digits+frac, true
		}
	}
	if digits == 0 {
		return int64(0)
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			i, isFloat = j, true
		}
	}
	text := s[:i]
	if !isFloat {
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n
		}
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil && f == 0 {
		return int64(0)
	}
	return f
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
