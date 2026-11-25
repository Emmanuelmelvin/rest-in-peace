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
	"time"
)

// Value is any dynamic BASIC value. Allowed dynamic types (design §4.2):
// nil, bool, int64, float64, string, time.Time, *Map, *Array, and later
// decimal (task 2.5) and Host values from Go interop (task 3.5).
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
		vb, err := json.Marshal(m.vals[k])
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
	if a.Items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(a.Items)
}

// Get implements member access v.name (Requirements 1.5 and 3.8). A missing
// key returns NIL; a member of NIL raises ?NIL ACCESS ERROR.
func Get(v Value, key string) Value {
	switch x := v.(type) {
	case nil:
		panic(Errorf(KindNilAccess, "cannot read %q of NIL — check the value exists before using it", key))
	case *Map:
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
			panic(Errorf(KindBadSubscript, "index %d is outside 0..%d — arrays start at 0", n, len(x.Items)-1))
		}
		return x.Items[n]
	default:
		panic(Errorf(KindTypeMismatch, "cannot index a %s", TypeName(v)))
	}
}

// SetMember implements v.name = val.
func SetMember(v Value, key string, val Value) {
	m, ok := v.(*Map)
	if !ok || m == nil {
		panic(Errorf(KindTypeMismatch, "cannot set %q on a %s", key, TypeName(v)))
	}
	m.Set(key, val)
}

// SetIndex implements v[i] = val.
func SetIndex(v Value, i Value, val Value) {
	switch x := v.(type) {
	case *Map:
		k, ok := i.(string)
		if !ok || x == nil {
			panic(Errorf(KindTypeMismatch, "map keys are STRINGs, got %s", TypeName(i)))
		}
		x.Set(k, val)
	case *Array:
		n, ok := toInt(i)
		if !ok || x == nil || n < 0 || n >= int64(len(x.Items)) {
			panic(Errorf(KindBadSubscript, "cannot set index %v", i))
		}
		x.Items[n] = val
	default:
		panic(Errorf(KindTypeMismatch, "cannot index a %s", TypeName(v)))
	}
}

// Truthy decides IF/WHILE conditions for dynamic values.
// TODO(task 1.5): confirm in the spec whether non-BOOLEAN conditions should be a TYPE MISMATCH instead.
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
	default:
		return fmt.Sprintf("GO %T", v)
	}
}

func toInt(v Value) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x == float64(int64(x)) {
			return int64(x), true
		}
	}
	return 0, false
}
