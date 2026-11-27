package rt

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// JSONParse parses a JSON string into a RIP Value, preserving map key insertion
// order. Panics with KindTypeMismatch on invalid JSON.
func JSONParse(s string) Value {
	v, err := decodeJSON([]byte(s))
	if err != nil {
		panic(Errorf(KindTypeMismatch, "JSONPARSE invalid JSON: %v", err))
	}
	return v
}

// JSONStringify serializes a RIP Value to a JSON string, preserving map key
// insertion order. If an optional indent argument is provided, a truthy bool
// indents with 2 spaces, a positive integer indents with that many spaces, and
// a non-empty string is used directly as the indent prefix.
func JSONStringify(v Value, indent ...Value) string {
	raw, err := marshalValue(v)
	if err != nil {
		panic(Errorf(KindTypeMismatch, "JSONSTRINGIFY failed: %v", err))
	}
	if len(indent) == 0 || indent[0] == nil {
		return string(raw)
	}
	var indentStr string
	switch x := indent[0].(type) {
	case bool:
		if x {
			indentStr = "  "
		}
	case string:
		indentStr = x
	default:
		n, ok := toInt(indent[0])
		if !ok {
			panic(Errorf(KindTypeMismatch, "JSONSTRINGIFY indent must be a BOOLEAN, whole NUMBER, or STRING, got %s", TypeName(indent[0])))
		}
		if n > 0 {
			if n > 64 {
				n = 64
			}
			indentStr = strings.Repeat(" ", int(n))
		}
	}
	if indentStr == "" {
		return string(raw)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", indentStr); err != nil {
		panic(Errorf(KindInternal, "JSONSTRINGIFY indent failed: %v", err))
	}
	return out.String()
}

// Split splits s around each instance of sep and returns an *Array of strings.
func Split(s string, sep string) *Array {
	parts := strings.Split(s, sep)
	items := make([]Value, len(parts))
	for i, p := range parts {
		items[i] = p
	}
	return &Array{Items: items}
}

// Join joins the elements of an *Array (converted via ToString) with sep.
func Join(arr Value, sep string) string {
	a := arrayArg("JOIN", arr)
	parts := make([]string, len(a.Items))
	for i, item := range a.Items {
		parts[i] = ToString(item)
	}
	return strings.Join(parts, sep)
}

// Replace replaces all occurrences of old in s with newStr.
func Replace(s string, old string, newStr string) string {
	return strings.ReplaceAll(s, old, newStr)
}

// Contains reports whether haystack contains needle:
//   - string: substring check (needle must be a string)
//   - *Array: linear equality check using Eq
//   - *Map: key presence check (needle must be a string)
func Contains(haystack Value, needle Value) bool {
	switch h := haystack.(type) {
	case string:
		n, ok := needle.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "CONTAINS on STRING needs a STRING needle, got %s", TypeName(needle)))
		}
		return strings.Contains(h, n)
	case *Array:
		if h == nil {
			panic(Errorf(KindTypeMismatch, "CONTAINS needs a STRING, ARRAY, or MAP, got NIL"))
		}
		for _, item := range h.Items {
			if Eq(item, needle) {
				return true
			}
		}
		return false
	case *Map:
		if h == nil {
			panic(Errorf(KindTypeMismatch, "CONTAINS needs a STRING, ARRAY, or MAP, got NIL"))
		}
		k, ok := needle.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "CONTAINS on MAP needs a STRING key, got %s", TypeName(needle)))
		}
		_, exists := h.Get(k)
		return exists
	default:
		panic(Errorf(KindTypeMismatch, "CONTAINS needs a STRING, ARRAY, or MAP, got %s", TypeName(haystack)))
	}
}

// StartsWith reports whether s begins with prefix.
func StartsWith(s string, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// EndsWith reports whether s ends with suffix.
func EndsWith(s string, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// IndexOf returns the 0-based index of needle in haystack, or -1 if not found.
// For strings it returns the rune index; for arrays it compares items with Eq.
func IndexOf(haystack Value, needle Value) int64 {
	switch h := haystack.(type) {
	case string:
		n, ok := needle.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "INDEXOF on STRING needs a STRING needle, got %s", TypeName(needle)))
		}
		idx := strings.Index(h, n)
		if idx < 0 {
			return -1
		}
		return int64(utf8.RuneCountInString(h[:idx]))
	case *Array:
		if h == nil {
			panic(Errorf(KindTypeMismatch, "INDEXOF needs a STRING or ARRAY, got NIL"))
		}
		for i, item := range h.Items {
			if Eq(item, needle) {
				return int64(i)
			}
		}
		return -1
	default:
		panic(Errorf(KindTypeMismatch, "INDEXOF needs a STRING or ARRAY, got %s", TypeName(haystack)))
	}
}

// Keys returns an *Array of m's keys in insertion order.
func Keys(m Value) *Array {
	mp := mapArg("KEYS", m)
	ks := mp.Keys()
	items := make([]Value, len(ks))
	for i, k := range ks {
		items[i] = k
	}
	return &Array{Items: items}
}

// Values returns an *Array of m's values in insertion order.
func Values(m Value) *Array {
	mp := mapArg("VALUES", m)
	ks := mp.Keys()
	items := make([]Value, len(ks))
	for i, k := range ks {
		v, _ := mp.Get(k)
		items[i] = v
	}
	return &Array{Items: items}
}

// Push appends v to arr in place and returns the new length.
func Push(arr Value, v Value) int64 {
	a := arrayArg("PUSH", arr)
	a.Items = append(a.Items, v)
	return int64(len(a.Items))
}

// Pop removes and returns the last item of arr, or nil if arr is empty.
func Pop(arr Value) Value {
	a := arrayArg("POP", arr)
	n := len(a.Items)
	if n == 0 {
		return nil
	}
	last := a.Items[n-1]
	a.Items[n-1] = nil
	a.Items = a.Items[:n-1]
	return last
}

// Slice returns a new *Array containing items from start (inclusive) to end
// (exclusive), clamped to [0, len] with JS-style negative index support.
func Slice(arr Value, start Value, end ...Value) *Array {
	a := arrayArg("SLICE", arr)
	n := int64(len(a.Items))
	st, ok := toInt(start)
	if !ok {
		panic(Errorf(KindTypeMismatch, "SLICE start must be a whole NUMBER, got %s", TypeName(start)))
	}
	if st < 0 {
		st += n
	}
	if st < 0 {
		st = 0
	} else if st > n {
		st = n
	}
	en := n
	if len(end) > 0 && end[0] != nil {
		e, ok := toInt(end[0])
		if !ok {
			panic(Errorf(KindTypeMismatch, "SLICE end must be a whole NUMBER, got %s", TypeName(end[0])))
		}
		if e < 0 {
			e += n
		}
		if e < 0 {
			e = 0
		} else if e > n {
			e = n
		}
		en = e
	}
	if st >= en {
		return &Array{Items: []Value{}}
	}
	out := make([]Value, en-st)
	copy(out, a.Items[st:en])
	return &Array{Items: out}
}

// Sort returns a new sorted *Array copy without mutating arr. Numbers compare
// numerically, strings lexicographically, and other values by ToString.
func Sort(arr Value) *Array {
	a := arrayArg("SORT", arr)
	out := make([]Value, len(a.Items))
	copy(out, a.Items)
	sort.SliceStable(out, func(i, j int) bool {
		return sortLess(out[i], out[j])
	})
	return &Array{Items: out}
}

func sortLess(a, b Value) bool {
	if cmp, ok := decimalCompare(a, b); ok {
		return cmp < 0
	}
	ai, af, aInt, aok := num(a)
	bi, bf, bInt, bok := num(b)
	if aok && bok {
		if aInt && bInt {
			return ai < bi
		}
		return af < bf
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return as < bs
		}
	}
	return ToString(a) < ToString(b)
}

// RegexMatch reports whether s matches the regular expression pattern.
func RegexMatch(pattern string, s string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(Errorf(KindTypeMismatch, "REGEXMATCH invalid pattern %q: %v", pattern, err))
	}
	return re.MatchString(s)
}

// RegexReplace replaces matches of pattern in s with repl.
func RegexReplace(pattern string, s string, repl string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(Errorf(KindTypeMismatch, "REGEXREPLACE invalid pattern %q: %v", pattern, err))
	}
	return re.ReplaceAllString(s, repl)
}

func arrayArg(fn string, v Value) *Array {
	if a, ok := v.(*Array); ok && a != nil {
		return a
	}
	panic(Errorf(KindTypeMismatch, "%s needs an ARRAY, got %s", fn, TypeName(v)))
}

func mapArg(fn string, v Value) *Map {
	if m, ok := v.(*Map); ok && m != nil {
		return m
	}
	panic(Errorf(KindTypeMismatch, "%s needs a MAP, got %s", fn, TypeName(v)))
}
