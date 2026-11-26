package rt

// Go interop Host wrapper and conversion helpers (Requirements 5.4–5.7,
// design §4.2 and §4.10, task 3.5).

// Host wraps an arbitrary Go value (struct, pointer, interface, etc.) returned
// by or constructed for IMPORT … FROM GO, exposing reflective field access and
// method calls to BASIC code.
type Host struct {
	V any
}

// WrapHost wraps a Go value in a Host value (returning nil if v is nil).
func WrapHost(v any) Value {
	if v == nil {
		return nil
	}
	return Host{V: v}
}

// hostGet implements reflective field access on a Host value, called from
// Get in rt/value.go. Returns (val, true) when v is a Host value.
func hostGet(v Value, key string) (Value, bool) {
	if _, ok := v.(Host); ok {
		// TODO(task 3.5): reflectively look up exported field key (case-insensitively).
		panic(notBuilt("Host field read", "3.5"))
	}
	return nil, false
}

// hostSet implements reflective field assignment on a Host value, called from
// SetMember in rt/value.go. Returns true when v is a Host value.
func hostSet(v Value, key string, val Value) bool {
	if _, ok := v.(Host); ok {
		// TODO(task 3.5): reflectively set exported field key on pointer-backed Host struct.
		panic(notBuilt("Host field write", "3.5"))
	}
	return false
}
