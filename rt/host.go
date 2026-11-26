package rt

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Go interop Host wrapper and conversion helpers (Requirements 5.4–5.7,
// design §4.2 and §4.10, task 3.5).

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// Host wraps an arbitrary Go value (struct, pointer, interface, etc.) returned
// by or constructed for IMPORT … FROM GO, exposing reflective field access and
// method calls to BASIC code.
type Host struct {
	V any
}

// MarshalJSON encodes the wrapped Go value directly instead of {"V": …}.
func (h Host) MarshalJSON() ([]byte, error) {
	if h.V == nil {
		return []byte("null"), nil
	}
	return json.Marshal(h.V)
}

// String formats the wrapped Go value using fmt.Stringer when available.
func (h Host) String() string {
	if h.V == nil {
		return ""
	}
	if s, ok := h.V.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprint(h.V)
}

// WrapHost wraps a Go value in a Host value (returning nil if v is nil or a
// typed nil pointer/interface/slice/map/func/chan).
func WrapHost(v any) Value {
	if v == nil {
		return nil
	}
	if h, ok := v.(Host); ok {
		if h.V == nil {
			return nil
		}
		return h
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return nil
		}
	}
	return Host{V: v}
}

// GoError wraps a non-nil Go error as a catchable BASIC error with kind "GO"
// (Requirement 5.6).
func GoError(err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{Kind: KindGo, Message: err.Error(), Cause: err}
}

// CheckGoErr panics with a catchable GO error when err is non-nil.
func CheckGoErr(err error) {
	if err != nil {
		panic(GoError(err))
	}
}

// GoResult checks a trailing Go error (panicking with GoError if non-nil) and
// converts the primary return value to a BASIC Value.
func GoResult[T any](v T, err error) Value {
	if err != nil {
		panic(GoError(err))
	}
	return FromGo(v)
}

// GoResultStr checks err and returns the native string result.
func GoResultStr(v string, err error) string {
	if err != nil {
		panic(GoError(err))
	}
	return v
}

// GoResultBool checks err and returns the native bool result.
func GoResultBool(v bool, err error) bool {
	if err != nil {
		panic(GoError(err))
	}
	return v
}

// GoResultInt64 checks err and returns the native int64 result.
func GoResultInt64(v int64, err error) int64 {
	if err != nil {
		panic(GoError(err))
	}
	return v
}

// GoResultFloat64 checks err and returns the native float64 result.
func GoResultFloat64(v float64, err error) float64 {
	if err != nil {
		panic(GoError(err))
	}
	return v
}

// FromGo converts a Go value returned from interop into a BASIC Value:
// native BASIC primitives stay native, unnamed Go slices/maps become *Array/*Map,
// []byte becomes string, and structs/pointers/named types/interfaces wrap in Host.
func FromGo(v any) Value {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case Host:
		if x.V == nil {
			return nil
		}
		return x
	case bool, int64, float64, string, Decimal, time.Time:
		return x
	case *Map:
		if x == nil {
			return nil
		}
		return x
	case *Array:
		if x == nil {
			return nil
		}
		return x
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case uint:
		return uintToBasic(uint64(x))
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return uintToBasic(x)
	case uintptr:
		return uintToBasic(uint64(x))
	case float32:
		return float64(x)
	case []byte:
		return string(x)
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		// Dereference pointers to built-in primitives (*string, *int64, *bool, …),
		// common in Go cloud/payment SDK structs, while keeping *struct / named types as Host.
		elem := rv.Elem()
		if elem.Type().PkgPath() == "" {
			switch elem.Kind() {
			case reflect.Bool, reflect.String,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
				reflect.Float32, reflect.Float64:
				return FromGo(elem.Interface())
			}
		}
	case reflect.Interface, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return nil
		}
	case reflect.Slice:
		if rv.Type().Name() == "" {
			n := rv.Len()
			items := make([]Value, n)
			for i := 0; i < n; i++ {
				items[i] = FromGo(rv.Index(i).Interface())
			}
			return &Array{Items: items}
		}
	case reflect.Map:
		if rv.Type().Name() == "" && rv.Type().Key().Kind() == reflect.String {
			m := NewMap()
			if !rv.IsNil() {
				keys := rv.MapKeys()
				sort.Slice(keys, func(i, j int) bool {
					return keys[i].String() < keys[j].String()
				})
				for _, rk := range keys {
					m.Set(rk.String(), FromGo(rv.MapIndex(rk).Interface()))
				}
			}
			return m
		}
	}
	return WrapHost(v)
}

func uintToBasic(u uint64) Value {
	if u <= math.MaxInt64 {
		return int64(u)
	}
	return float64(u)
}

// ---- Reflective field access and method calls on Host ----

// hostGet implements reflective field access on a Host value, called from
// Get in rt/value.go. Returns (val, true) when v is a Host value.
func hostGet(v Value, key string) (Value, bool) {
	h, ok := v.(Host)
	if !ok {
		return nil, false
	}
	if h.V == nil {
		panic(Errorf(KindNilAccess, "cannot read %q of NIL — check the value exists before using it", key))
	}
	rv := reflect.ValueOf(h.V)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			panic(Errorf(KindNilAccess, "cannot read %q of NIL — check the value exists before using it", key))
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		idx := findStructField(rv.Type(), key)
		if idx == nil {
			panic(Errorf(KindUndefined, "Go type %T has no exported field %q", h.V, key))
		}
		fv, ok := fieldByIndexRead(rv, idx)
		if !ok {
			return nil, true
		}
		return FromGo(fv.Interface()), true
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			if rv.IsNil() {
				return nil, true
			}
			rk := reflect.ValueOf(key).Convert(rv.Type().Key())
			mv := rv.MapIndex(rk)
			if !mv.IsValid() {
				return nil, true
			}
			return FromGo(mv.Interface()), true
		}
	}
	panic(Errorf(KindTypeMismatch, "cannot read %q of Go type %T", key, h.V))
}

// hostSet implements reflective field assignment on a Host value, called from
// SetMember in rt/value.go. Returns true when v is a Host value.
func hostSet(v Value, key string, val Value) bool {
	h, ok := v.(Host)
	if !ok {
		return false
	}
	if h.V == nil {
		panic(Errorf(KindNilAccess, "cannot set %q on NIL", key))
	}
	rv := reflect.ValueOf(h.V)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			panic(Errorf(KindNilAccess, "cannot set %q on NIL", key))
		}
		elem := rv.Elem()
		if elem.Kind() == reflect.Struct {
			idx := findStructField(elem.Type(), key)
			if idx == nil {
				panic(Errorf(KindUndefined, "Go type %T has no exported field %q", h.V, key))
			}
			fv := fieldByIndexWrite(elem, idx)
			if !fv.CanSet() {
				panic(Errorf(KindTypeMismatch, "cannot set field %q on Go type %T", key, h.V))
			}
			fv.Set(convertValueToReflect(val, fv.Type()))
			return true
		}
	}
	if rv.Kind() == reflect.Struct {
		panic(Errorf(KindTypeMismatch, "cannot set field %q on non-pointer Go struct %T — use NEW() to get a mutable pointer", key, h.V))
	}
	if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		if rv.IsNil() {
			panic(Errorf(KindNilAccess, "cannot set %q on NIL Go map %T", key, h.V))
		}
		rk := reflect.ValueOf(key).Convert(rv.Type().Key())
		rv.SetMapIndex(rk, convertValueToReflect(val, rv.Type().Elem()))
		return true
	}
	panic(Errorf(KindTypeMismatch, "cannot set %q on Go type %T", key, h.V))
}

// HostCall invokes an exported Go method on a Host value (or time.Time),
// matching method names case-insensitively, auto-injecting c.Context() for
// context.Context parameters, converting arguments, and panicking with
// GoError on a non-nil trailing error (Requirements 5.5–5.7).
func HostCall(c *Ctx, v Value, method string, args ...Value) Value {
	if v == nil {
		panic(Errorf(KindNilAccess, "cannot call method %q on NIL", method))
	}
	var raw any
	switch x := v.(type) {
	case Host:
		raw = x.V
	case time.Time:
		raw = x
	default:
		panic(Errorf(KindTypeMismatch, "cannot call method %q on a %s", method, TypeName(v)))
	}
	if raw == nil {
		panic(Errorf(KindNilAccess, "cannot call method %q on NIL", method))
	}

	rv := reflect.ValueOf(raw)
	mv := findMethod(rv, method)
	if !mv.IsValid() && rv.Kind() == reflect.Struct {
		// Allow calling pointer-receiver methods on a value struct copy.
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		mv = findMethod(ptr, method)
	}
	if !mv.IsValid() {
		panic(Errorf(KindUndefined, "Go type %T has no exported method %q", raw, method))
	}

	mt := mv.Type()
	numIn := mt.NumIn()
	variadic := mt.IsVariadic()
	fixedCount := numIn
	if variadic {
		fixedCount = numIn - 1
	}

	// Validate signature does not use unsupported channel/func params.
	for i := 0; i < numIn; i++ {
		pt := mt.In(i)
		if variadic && i == numIn-1 {
			pt = pt.Elem()
		}
		if pt.Kind() == reflect.Chan || pt.Kind() == reflect.Func {
			panic(Errorf(KindTypeMismatch, "Go method %T.%s takes unsupported parameter type %s", raw, method, pt))
		}
	}

	// Count required BASIC args (excluding context.Context).
	wantBasicFixed := 0
	for i := 0; i < fixedCount; i++ {
		if mt.In(i) != contextType {
			wantBasicFixed++
		}
	}
	if variadic {
		if len(args) < wantBasicFixed {
			panic(Errorf(KindTypeMismatch, "Go method %T.%s expects at least %d argument(s), got %d", raw, method, wantBasicFixed, len(args)))
		}
	} else if len(args) != wantBasicFixed {
		panic(Errorf(KindTypeMismatch, "Go method %T.%s expects %d argument(s), got %d", raw, method, wantBasicFixed, len(args)))
	}

	in := make([]reflect.Value, 0, len(args)+1)
	argIdx := 0
	for i := 0; i < fixedCount; i++ {
		pt := mt.In(i)
		if pt == contextType {
			var gctx context.Context
			if c != nil {
				gctx = c.Context()
			}
			if gctx == nil {
				gctx = context.Background()
			}
			in = append(in, reflect.ValueOf(gctx))
			continue
		}
		in = append(in, convertValueToReflect(args[argIdx], pt))
		argIdx++
	}
	if variadic {
		elemType := mt.In(numIn - 1).Elem()
		for ; argIdx < len(args); argIdx++ {
			in = append(in, convertValueToReflect(args[argIdx], elemType))
		}
	}

	out := mv.Call(in)
	switch len(out) {
	case 0:
		return nil
	case 1:
		if mt.Out(0).Implements(errorType) {
			if !out[0].IsNil() {
				panic(GoError(out[0].Interface().(error)))
			}
			return nil
		}
		return FromGo(out[0].Interface())
	case 2:
		if mt.Out(1).Implements(errorType) {
			if !out[1].IsNil() {
				panic(GoError(out[1].Interface().(error)))
			}
			return FromGo(out[0].Interface())
		}
	}
	panic(Errorf(KindTypeMismatch, "Go method %T.%s returns %d values; RIP BASIC supports at most one value plus an optional trailing error", raw, method, len(out)))
}

func findMethod(rv reflect.Value, name string) reflect.Value {
	if mv := rv.MethodByName(name); mv.IsValid() {
		return mv
	}
	rt := rv.Type()
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		if m.IsExported() && strings.EqualFold(m.Name, name) {
			return rv.Method(i)
		}
	}
	return reflect.Value{}
}

// ---- Generic & typed conversions from BASIC Value to Go types ----

// ToGo converts a BASIC Value to any target Go type T (structs, pointers,
// slices, maps, interfaces, defined types, primitives).
func ToGo[T any](v Value) T {
	var zero T
	target := reflect.TypeOf(&zero).Elem()
	rv := convertValueToReflect(v, target)
	if !rv.IsValid() {
		return zero
	}
	iface := rv.Interface()
	if iface == nil {
		return zero
	}
	return iface.(T)
}

// ToStruct converts a BASIC Map or Host struct/*struct to Go struct type T,
// matching map keys to exported struct fields case-insensitively or by json tag
// (Requirement 5.5).
func ToStruct[T any](v Value) T {
	return ToGo[T](v)
}

// ToStructPtr converts NIL, a BASIC Map, or a Host struct/*struct to *T.
func ToStructPtr[T any](v Value) *T {
	return ToGo[*T](v)
}

// ToGoSlice converts NIL, a BASIC Array, or a Host slice to []T.
func ToGoSlice[T any](v Value) []T {
	return ToGo[[]T](v)
}

// ToGoMap converts NIL, a BASIC Map, or a Host map to map[string]T.
func ToGoMap[T any](v Value) map[string]T {
	return ToGo[map[string]T](v)
}

// ToGoAny unwraps Host values when passing a BASIC Value to a Go any parameter.
func ToGoAny(v Value) any {
	if h, ok := v.(Host); ok {
		return h.V
	}
	return v
}

// ToGoString converts a BASIC Value to a Go string.
func ToGoString(v Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	if h, ok := v.(Host); ok && h.V != nil {
		rv := reflect.ValueOf(h.V)
		if rv.Kind() == reflect.String {
			return rv.String()
		}
	}
	panic(Errorf(KindTypeMismatch, "Go expects string, got %s", TypeName(v)))
}

// ToGoBool converts a BASIC Value to a Go bool.
func ToGoBool(v Value) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if h, ok := v.(Host); ok && h.V != nil {
		rv := reflect.ValueOf(h.V)
		if rv.Kind() == reflect.Bool {
			return rv.Bool()
		}
	}
	panic(Errorf(KindTypeMismatch, "Go expects bool, got %s", TypeName(v)))
}

// ToGoBytes converts NIL, a BASIC STRING, an ARRAY of byte integers, or a Host
// []byte into []byte.
func ToGoBytes(v Value) []byte {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		return []byte(x)
	case Host:
		if x.V == nil {
			return nil
		}
		if b, ok := x.V.([]byte); ok {
			return b
		}
		if s, ok := x.V.(string); ok {
			return []byte(s)
		}
	case *Array:
		out := make([]byte, len(x.Items))
		for i, it := range x.Items {
			out[i] = ToGoUint8(it)
		}
		return out
	}
	panic(Errorf(KindTypeMismatch, "Go expects []byte, got %s", TypeName(v)))
}

func ToGoInt(v Value) int       { return int(toSignedInt(v, "int", math.MinInt, math.MaxInt)) }
func ToGoInt8(v Value) int8     { return int8(toSignedInt(v, "int8", math.MinInt8, math.MaxInt8)) }
func ToGoInt16(v Value) int16   { return int16(toSignedInt(v, "int16", math.MinInt16, math.MaxInt16)) }
func ToGoInt32(v Value) int32   { return int32(toSignedInt(v, "int32", math.MinInt32, math.MaxInt32)) }
func ToGoInt64(v Value) int64   { return toSignedInt(v, "int64", math.MinInt64, math.MaxInt64) }
func ToGoUint(v Value) uint     { return uint(toUnsignedInt(v, "uint", math.MaxUint)) }
func ToGoUint8(v Value) uint8   { return uint8(toUnsignedInt(v, "uint8", math.MaxUint8)) }
func ToGoUint16(v Value) uint16 { return uint16(toUnsignedInt(v, "uint16", math.MaxUint16)) }
func ToGoUint32(v Value) uint32 { return uint32(toUnsignedInt(v, "uint32", math.MaxUint32)) }
func ToGoUint64(v Value) uint64 { return toUnsignedInt(v, "uint64", math.MaxUint64) }
func ToGoUintptr(v Value) uintptr {
	return uintptr(toUnsignedInt(v, "uintptr", uint64(^uintptr(0))))
}
func ToGoFloat32(v Value) float32 { return float32(ToGoFloat64(v)) }

func ToGoFloat64(v Value) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case Host:
		if x.V != nil {
			rv := reflect.ValueOf(x.V)
			switch rv.Kind() {
			case reflect.Float32, reflect.Float64:
				return rv.Float()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return float64(rv.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				return float64(rv.Uint())
			}
		}
	}
	panic(Errorf(KindTypeMismatch, "Go expects float64, got %s", TypeName(v)))
}

func toSignedInt(v Value, name string, minVal, maxVal int64) int64 {
	if n, ok := toInt(v); ok {
		if n < minVal || n > maxVal {
			panic(Errorf(KindTypeMismatch, "value %d overflows Go %s", n, name))
		}
		return n
	}
	if h, ok := v.(Host); ok && h.V != nil {
		rv := reflect.ValueOf(h.V)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := rv.Int()
			if n < minVal || n > maxVal {
				panic(Errorf(KindTypeMismatch, "value %d overflows Go %s", n, name))
			}
			return n
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u := rv.Uint()
			if u > uint64(maxVal) {
				panic(Errorf(KindTypeMismatch, "value %d overflows Go %s", u, name))
			}
			return int64(u)
		}
	}
	panic(Errorf(KindTypeMismatch, "Go expects %s, got %s", name, TypeName(v)))
}

func toUnsignedInt(v Value, name string, maxVal uint64) uint64 {
	if n, ok := toInt(v); ok {
		if n < 0 || uint64(n) > maxVal {
			panic(Errorf(KindTypeMismatch, "value %d is out of range for Go %s", n, name))
		}
		return uint64(n)
	}
	if h, ok := v.(Host); ok && h.V != nil {
		rv := reflect.ValueOf(h.V)
		switch rv.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			u := rv.Uint()
			if u > maxVal {
				panic(Errorf(KindTypeMismatch, "value %d overflows Go %s", u, name))
			}
			return u
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := rv.Int()
			if n < 0 || uint64(n) > maxVal {
				panic(Errorf(KindTypeMismatch, "value %d is out of range for Go %s", n, name))
			}
			return uint64(n)
		}
	}
	panic(Errorf(KindTypeMismatch, "Go expects %s, got %s", name, TypeName(v)))
}

// ---- Core reflective value conversion ----

func convertValueToReflect(v Value, target reflect.Type) reflect.Value {
	orig := v
	if h, ok := v.(Host); ok {
		v = h.V
	}
	if v == nil {
		switch target.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
			return reflect.Zero(target)
		default:
			panic(Errorf(KindTypeMismatch, "cannot pass NIL where Go expects %s", target))
		}
	}

	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(target) {
		return rv
	}

	// Host *T passed where Go expects T (value struct).
	if target.Kind() == reflect.Struct && rv.Kind() == reflect.Ptr && !rv.IsNil() && rv.Elem().Type().AssignableTo(target) {
		return rv.Elem()
	}
	// Host T passed where Go expects *T.
	if target.Kind() == reflect.Ptr && rv.Type().AssignableTo(target.Elem()) {
		p := reflect.New(target.Elem())
		p.Elem().Set(rv)
		return p
	}

	switch target.Kind() {
	case reflect.String:
		return reflect.ValueOf(ToGoString(orig)).Convert(target)

	case reflect.Bool:
		return reflect.ValueOf(ToGoBool(orig)).Convert(target)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := target.Bits()
		minVal := int64(-1) << (bits - 1)
		maxVal := int64(1)<<(bits-1) - 1
		n := toSignedInt(orig, target.String(), minVal, maxVal)
		out := reflect.New(target).Elem()
		out.SetInt(n)
		return out

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		bits := target.Bits()
		var maxVal uint64
		if bits == 64 {
			maxVal = math.MaxUint64
		} else {
			maxVal = uint64(1)<<bits - 1
		}
		u := toUnsignedInt(orig, target.String(), maxVal)
		out := reflect.New(target).Elem()
		out.SetUint(u)
		return out

	case reflect.Float32, reflect.Float64:
		f := ToGoFloat64(orig)
		out := reflect.New(target).Elem()
		out.SetFloat(f)
		return out

	case reflect.Slice:
		if target.Elem().Kind() == reflect.Uint8 {
			if s, ok := v.(string); ok {
				return reflect.ValueOf([]byte(s)).Convert(target)
			}
		}
		if arr, ok := v.(*Array); ok {
			if arr == nil {
				return reflect.Zero(target)
			}
			sl := reflect.MakeSlice(target, len(arr.Items), len(arr.Items))
			for i, item := range arr.Items {
				sl.Index(i).Set(convertValueToReflect(item, target.Elem()))
			}
			return sl
		}

	case reflect.Array:
		if target.Elem().Kind() == reflect.Uint8 {
			if s, ok := v.(string); ok && len(s) == target.Len() {
				out := reflect.New(target).Elem()
				for i := 0; i < len(s); i++ {
					out.Index(i).SetUint(uint64(s[i]))
				}
				return out
			}
		}
		if arr, ok := v.(*Array); ok && arr != nil {
			if len(arr.Items) != target.Len() {
				panic(Errorf(KindTypeMismatch, "Go expects %s (length %d), got ARRAY of length %d", target, target.Len(), len(arr.Items)))
			}
			out := reflect.New(target).Elem()
			for i, item := range arr.Items {
				out.Index(i).Set(convertValueToReflect(item, target.Elem()))
			}
			return out
		}

	case reflect.Map:
		if target.Key().Kind() == reflect.String {
			if m, ok := v.(*Map); ok {
				if m == nil {
					return reflect.Zero(target)
				}
				gm := reflect.MakeMapWithSize(target, m.Len())
				for _, k := range m.Keys() {
					val, _ := m.Get(k)
					rk := reflect.ValueOf(k).Convert(target.Key())
					gm.SetMapIndex(rk, convertValueToReflect(val, target.Elem()))
				}
				return gm
			}
		}

	case reflect.Struct:
		if m, ok := v.(*Map); ok {
			if m == nil {
				panic(Errorf(KindTypeMismatch, "cannot pass NIL MAP where Go expects %s", target))
			}
			dst := reflect.New(target).Elem()
			populateStruct(dst, m)
			return dst
		}

	case reflect.Ptr:
		if target.Elem().Kind() == reflect.Struct {
			if m, ok := v.(*Map); ok {
				if m == nil {
					return reflect.Zero(target)
				}
				ptr := reflect.New(target.Elem())
				populateStruct(ptr.Elem(), m)
				return ptr
			}
		}
		// Pointer to non-struct (e.g. *string, *int64 in Stripe/AWS SDKs).
		elem := convertValueToReflect(orig, target.Elem())
		ptr := reflect.New(target.Elem())
		ptr.Elem().Set(elem)
		return ptr

	case reflect.Interface:
		if target.NumMethod() == 0 {
			return rv
		}
	}

	panic(Errorf(KindTypeMismatch, "Go expects %s, got %s", target, TypeName(orig)))
}

func populateStruct(dst reflect.Value, m *Map) {
	st := dst.Type()
	for _, k := range m.Keys() {
		idx := findStructField(st, k)
		if idx == nil {
			continue
		}
		fv := fieldByIndexWrite(dst, idx)
		if !fv.CanSet() {
			continue
		}
		val, _ := m.Get(k)
		fv.Set(convertValueToReflect(val, fv.Type()))
	}
}

// ---- Struct field lookup (exact, json tag, case-insensitive, embedded) ----

type fieldEntry struct {
	name    string
	jsonTag string
	index   []int
}

func findStructField(t reflect.Type, key string) []int {
	entries := collectStructFields(t)
	// Priority 1: exact exported field name.
	for _, e := range entries {
		if e.name == key {
			return e.index
		}
	}
	// Priority 2: exact json tag name.
	for _, e := range entries {
		if e.jsonTag != "" && e.jsonTag == key {
			return e.index
		}
	}
	// Priority 3: case-insensitive exported field name.
	for _, e := range entries {
		if strings.EqualFold(e.name, key) {
			return e.index
		}
	}
	// Priority 4: case-insensitive json tag name.
	for _, e := range entries {
		if e.jsonTag != "" && strings.EqualFold(e.jsonTag, key) {
			return e.index
		}
	}
	return nil
}

// collectStructFields returns exported fields in breadth-first order so
// direct fields take precedence over embedded struct fields.
func collectStructFields(root reflect.Type) []fieldEntry {
	type queueItem struct {
		t     reflect.Type
		index []int
	}
	var out []fieldEntry
	visited := map[reflect.Type]bool{}
	queue := []queueItem{{t: root}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.t] {
			continue
		}
		visited[cur.t] = true

		for i := 0; i < cur.t.NumField(); i++ {
			f := cur.t.Field(i)
			idx := append(append([]int(nil), cur.index...), i)
			if f.IsExported() {
				out = append(out, fieldEntry{
					name:    f.Name,
					jsonTag: parseJSONTagName(f.Tag.Get("json")),
					index:   idx,
				})
			}
			if f.Anonymous {
				ft := f.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					queue = append(queue, queueItem{t: ft, index: idx})
				}
			}
		}
	}
	return out
}

func parseJSONTagName(tag string) string {
	if tag == "" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" || name == "" {
		return ""
	}
	return name
}

func fieldByIndexRead(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

func fieldByIndexWrite(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}
