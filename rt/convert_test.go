package rt

import (
	"strings"
	"testing"
	"time"
)

func TestCheckDynamicTypes(t *testing.T) {
	m := MapOf("a", int64(1))
	a := ArrayOf(int64(1), int64(2))
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)

	if CheckMap(m) != m {
		t.Error("CheckMap did not return the map")
	}
	if CheckArray(a) != a {
		t.Error("CheckArray did not return the array")
	}
	if CheckDate(now) != now {
		t.Error("CheckDate did not return the time")
	}
	if CheckNil(nil) != nil {
		t.Error("CheckNil did not return nil")
	}

	var nilMap *Map
	var nilArr *Array
	bad := []struct {
		name string
		fn   func()
		want string
	}{
		{"CheckMap nil", func() { CheckMap(nil) }, "expected a MAP"},
		{"CheckMap typed nil", func() { CheckMap(nilMap) }, "expected a MAP"},
		{"CheckMap array", func() { CheckMap(a) }, "expected a MAP"},
		{"CheckMap string", func() { CheckMap("x") }, "expected a MAP"},
		{"CheckArray nil", func() { CheckArray(nil) }, "expected an ARRAY"},
		{"CheckArray typed nil", func() { CheckArray(nilArr) }, "expected an ARRAY"},
		{"CheckArray map", func() { CheckArray(m) }, "expected an ARRAY"},
		{"CheckArray int", func() { CheckArray(int64(5)) }, "expected an ARRAY"},
		{"CheckDate nil", func() { CheckDate(nil) }, "expected a DATE"},
		{"CheckDate string", func() { CheckDate("2026-11-26") }, "expected a DATE"},
		{"CheckNil int", func() { CheckNil(int64(0)) }, "expected NIL"},
		{"CheckNil map", func() { CheckNil(m) }, "expected NIL"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			r := escaped(tc.fn)
			e, ok := r.(*Error)
			if !ok || e.Kind != KindTypeMismatch || !strings.Contains(e.Message, tc.want) {
				t.Errorf("%s: raised %v, want TYPE MISMATCH containing %q", tc.name, r, tc.want)
			}
		})
	}
}
