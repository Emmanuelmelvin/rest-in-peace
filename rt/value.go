// Package rt is the RIP BASIC runtime. Generated programs import it as
// "ripapp/rt", and it is linked into every compiled binary.
//
// Rules (see .kiro/steering/structure.md): rt is one flat package, and it never
// imports anything else from the rip repository.
package rt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Value is any dynamic BASIC value. Allowed dynamic types (design §4.2):
// nil, bool, int64, float64, string, Decimal (decimal.go), time.Time, *Map,
// *Array, and later Host values from Go interop (task 3.5).
// Typed declarations (DIM x AS INTEGER) compile to native Go types instead.
type Value = any

// Map is an insertion-ordered map with string keys, so JSON output keeps the
// order the fields were written in.
type Map struct {
	keys []string
	vals map[string]Value
}

// NewMap returns an empty map.
func NewMap() *Map { return &Map{vals: map[string]Value{}} }

// MapOf builds a map from alternating key/value pairs: MapOf("a", 1, "b", 2).
func MapOf(kv ...Value) *Map {
	m := NewMap()
	for i := 0; i+1 < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			panic(Errorf(KindInternal, "map key must be a STRING, got %s", TypeName(kv[i])))
		}
		m.Set(k, kv[i+1])
	}
	return m
}

// Get returns the value for key and whether it was present.
func (m *Map) Get(key string) (Value, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[key]
	return v, ok
}

// Set stores a value, keeping the key's original position if it already exists.
func (m *Map) Set(key string, v Value) {
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = v
}

// Delete removes a key.
func (m *Map) Delete(key string) {
	if _, ok := m.vals[key]; !ok {
		return
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

// Len returns the number of keys.
func (m *Map) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// Keys returns the keys in insertion order.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// MarshalJSON writes the map with its keys in insertion order.
func (m *Map) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := marshalValue(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Array is a zero-based BASIC array.
type Array struct {
	Items []Value
}

// ArrayOf builds an array literal.
func ArrayOf(items ...Value) *Array { return &Array{Items: items} }

// Len returns the number of items.
func (a *Array) Len() int {
	if a == nil {
		return 0
	}
	return len(a.Items)
}

// MarshalJSON writes the array, using [] rather than null for an empty array.
func (a *Array) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, v := range a.Items {
		if i > 0 {
			buf.WriteByte(',')
		}
		vb, err := marshalValue(v)
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// marshalValue encodes one map or array element as JSON. DECIMALs keep their
// scale (decimalJSON in decimal.go); everything else uses encoding/json.
func marshalValue(v Value) ([]byte, error) {
	if b, ok := decimalJSON(v); ok {
		return b, nil
	}
	return json.Marshal(v)
}

// Get implements member access v.name (Requirements 1.5 and 3.8). A missing
// key returns NIL; a member of NIL raises ?NIL ACCESS ERROR.
func Get(v Value, key string) Value {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "cannot read %q of NIL — check the value exists before using it", key))
	case *Map:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot read %q of NIL — check the value exists before using it", key))
		}
		val, _ := x.Get(key)
		return val
	default:
		// TODO(task 3.5): Host values from Go interop use reflective field access.
		panic(Errorf(KindTypeMismatch, "cannot read %q of a %s", key, TypeName(v)))
	}
}

// Index implements v[i] for maps (string keys) and arrays (zero-based).
func Index(v Value, i Value) Value {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "cannot index NIL"))
	case *Map:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot index NIL"))
		}
		k, ok := i.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "map keys are STRINGs, got %s", TypeName(i)))
		}
		val, _ := x.Get(k)
		return val
	case *Array:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot index NIL"))
		}
		n, ok := toInt(i)
		if !ok {
			panic(Errorf(KindTypeMismatch, "array index must be a whole NUMBER, got %s", TypeName(i)))
		}
		if n < 0 || n >= int64(len(x.Items)) {
			panic(badSubscript(n, len(x.Items)))
		}
		return x.Items[n]
	default:
		panic(Errorf(KindTypeMismatch, "cannot index a %s", TypeName(v)))
	}
}

// SetMember implements v.name = val.
func SetMember(v Value, key string, val Value) {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "cannot set %q on NIL — create the map first, e.g. x = {}", key))
	case *Map:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot set %q on NIL — create the map first, e.g. x = {}", key))
		}
		x.Set(key, val)
	default:
		panic(Errorf(KindTypeMismatch, "cannot set %q on a %s", key, TypeName(v)))
	}
}

// SetIndex implements v[i] = val.
func SetIndex(v Value, i Value, val Value) {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "cannot index NIL"))
	case *Map:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot index NIL"))
		}
		k, ok := i.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "map keys are STRINGs, got %s", TypeName(i)))
		}
		x.Set(k, val)
	case *Array:
		if x == nil {
			panic(Errorf(KindNilAccess, "cannot index NIL"))
		}
		n, ok := toInt(i)
		if !ok {
			panic(Errorf(KindTypeMismatch, "array index must be a whole NUMBER, got %s", TypeName(i)))
		}
		if n < 0 || n >= int64(len(x.Items)) {
			panic(badSubscript(n, len(x.Items)))
		}
		x.Items[n] = val
	default:
		panic(Errorf(KindTypeMismatch, "cannot index a %s", TypeName(v)))
	}
}

func badSubscript(n int64, length int) *Error {
	if length == 0 {
		return Errorf(KindBadSubscript, "index %d is outside an empty array — arrays start at 0", n)
	}
	return Errorf(KindBadSubscript, "index %d is outside 0..%d — arrays start at 0", n, length-1)
}

// Truthy decides IF, WHILE, UNTIL, NOT, AND and OR for dynamic values.
// Non-BOOLEAN conditions are allowed, as in classic BASIC (IF X THEN …):
// NIL, FALSE, 0 and "" are false, and everything else is true. That also
// makes the common IF B.title THEN … work without a comparison.
func Truthy(v Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	default:
		return true
	}
}

// TypeName returns the BASIC type name used in error messages.
func TypeName(v Value) string {
	switch v.(type) {
	case nil:
		return "NIL"
	case bool:
		return "BOOLEAN"
	case int64:
		return "INTEGER"
	case float64:
		return "NUMBER"
	case string:
		return "STRING"
	case *Map:
		return "MAP"
	case *Array:
		return "ARRAY"
	case time.Time:
		return "DATE"
	case Decimal:
		return "DECIMAL"
	default:
		return fmt.Sprintf("GO %T", v)
	}
}

func toInt(v Value) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x == math.Trunc(x) && x >= math.MinInt64 && x < math.MaxInt64 {
			return int64(x), true
		}
	}
	return 0, false
}
