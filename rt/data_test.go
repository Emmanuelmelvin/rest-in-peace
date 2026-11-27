package rt

import (
	"testing"
)

func TestJSONParseAndStringify(t *testing.T) {
	raw := `{"z":1,"a":[true,null,2.5,"hi"],"m":{"second":2,"first":1}}`
	v := JSONParse(raw)
	m, ok := v.(*Map)
	if !ok {
		t.Fatalf("JSONParse returned %T, want *Map", v)
	}
	// Verify insertion order preserved.
	keys := m.Keys()
	wantKeys := []string{"z", "a", "m"}
	if len(keys) != len(wantKeys) {
		t.Fatalf("keys = %v, want %v", keys, wantKeys)
	}
	for i := range wantKeys {
		if keys[i] != wantKeys[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, keys[i], wantKeys[i])
		}
	}

	// Compact round-trip preserves insertion order.
	if got := JSONStringify(v); got != raw {
		t.Errorf("JSONStringify compact = %q, want %q", got, raw)
	}

	// Pretty print with bool true (2 spaces).
	small := MapOf("b", int64(1), "a", int64(2))
	wantPretty2 := "{\n  \"b\": 1,\n  \"a\": 2\n}"
	if got := JSONStringify(small, true); got != wantPretty2 {
		t.Errorf("JSONStringify(small, true) =\n%s\nwant:\n%s", got, wantPretty2)
	}
	// Pretty print with bool false -> compact.
	if got := JSONStringify(small, false); got != `{"b":1,"a":2}` {
		t.Errorf("JSONStringify(small, false) = %q, want compact", got)
	}
	// Pretty print with int 4 spaces.
	wantPretty4 := "{\n    \"b\": 1,\n    \"a\": 2\n}"
	if got := JSONStringify(small, int64(4)); got != wantPretty4 {
		t.Errorf("JSONStringify(small, 4) =\n%s\nwant:\n%s", got, wantPretty4)
	}
	// Pretty print with 0 spaces -> compact.
	if got := JSONStringify(small, int64(0)); got != `{"b":1,"a":2}` {
		t.Errorf("JSONStringify(small, 0) = %q, want compact", got)
	}

	// Error cases.
	if k := panicKind(func() { JSONParse("{bad json") }); k != KindTypeMismatch {
		t.Errorf("JSONParse invalid: got %q, want %q", k, KindTypeMismatch)
	}
	if k := panicKind(func() { JSONParse(`{"a":1} trailing`) }); k != KindTypeMismatch {
		t.Errorf("JSONParse trailing: got %q, want %q", k, KindTypeMismatch)
	}
	if k := panicKind(func() { JSONStringify(small, 1.5) }); k != KindTypeMismatch {
		t.Errorf("JSONStringify bad indent: got %q, want %q", k, KindTypeMismatch)
	}
}

func TestSplitJoinReplace(t *testing.T) {
	arr := Split("a,b,c", ",")
	if arr.Len() != 3 || arr.Items[0] != "a" || arr.Items[1] != "b" || arr.Items[2] != "c" {
		t.Fatalf("Split = %v, want [a b c]", arr.Items)
	}

	joined := Join(ArrayOf("a", int64(2), true), "-")
	if joined != "a-2-TRUE" {
		t.Errorf("Join = %q, want %q", joined, "a-2-TRUE")
	}
	if k := panicKind(func() { Join("not-array", ",") }); k != KindTypeMismatch {
		t.Errorf("Join non-array: got %q, want %q", k, KindTypeMismatch)
	}

	if got := Replace("foo bar foo", "foo", "baz"); got != "baz bar baz" {
		t.Errorf("Replace = %q, want %q", got, "baz bar baz")
	}
	if !StartsWith("hello world", "hello") || StartsWith("hello", "world") {
		t.Errorf("StartsWith unexpected result")
	}
	if !EndsWith("hello world", "world") || EndsWith("hello", "world") {
		t.Errorf("EndsWith unexpected result")
	}
}

func TestDataContainsAndIndexOf(t *testing.T) {
	arr := ArrayOf(int64(10), "hi", true, 3.5)
	m := MapOf("x", int64(1), "y", nil)

	tests := []struct {
		name     string
		haystack Value
		needle   Value
		want     bool
	}{
		{"string true", "hello world", "world", true},
		{"string false", "hello world", "xyz", false},
		{"array int", arr, int64(10), true},
		{"array float==int", arr, 10.0, true},
		{"array string", arr, "hi", true},
		{"array missing", arr, "nope", false},
		{"map present", m, "x", true},
		{"map nil value still present", m, "y", true},
		{"map missing", m, "z", false},
	}
	for _, tt := range tests {
		if got := Contains(tt.haystack, tt.needle); got != tt.want {
			t.Errorf("Contains(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}

	// IndexOf with unicode rune indexing.
	idxTests := []struct {
		name     string
		haystack Value
		needle   Value
		want     int64
	}{
		{"ascii", "hello world", "world", 6},
		{"unicode rune index", "héllo 🌍!", "🌍", 6},
		{"unicode not found", "héllo", "z", -1},
		{"empty needle", "abc", "", 0},
		{"array found", arr, "hi", 1},
		{"array int==float", arr, 10.0, 0},
		{"array not found", arr, "missing", -1},
	}
	for _, tt := range idxTests {
		if got := IndexOf(tt.haystack, tt.needle); got != tt.want {
			t.Errorf("IndexOf(%s) = %d, want %d", tt.name, got, tt.want)
		}
	}

	errCases := map[string]func(){
		"Contains bad haystack": func() { Contains(int64(1), int64(1)) },
		"Contains str bad ndl":  func() { Contains("abc", int64(1)) },
		"Contains map bad key":  func() { Contains(m, int64(1)) },
		"IndexOf bad haystack":  func() { IndexOf(m, "x") },
		"IndexOf str bad ndl":   func() { IndexOf("abc", int64(1)) },
	}
	for name, fn := range errCases {
		if k := panicKind(fn); k != KindTypeMismatch {
			t.Errorf("%s: got %q, want %q", name, k, KindTypeMismatch)
		}
	}
}

func TestArrayAndMapOps(t *testing.T) {
	// Keys & Values preserve insertion order.
	m := MapOf("c", int64(3), "a", int64(1), "b", int64(2))
	ks := Keys(m)
	vs := Values(m)
	if ks.Len() != 3 || ks.Items[0] != "c" || ks.Items[1] != "a" || ks.Items[2] != "b" {
		t.Fatalf("Keys = %v, want [c a b]", ks.Items)
	}
	if vs.Len() != 3 || vs.Items[0] != int64(3) || vs.Items[1] != int64(1) || vs.Items[2] != int64(2) {
		t.Fatalf("Values = %v, want [3 1 2]", vs.Items)
	}

	// Push & Pop.
	a := ArrayOf(int64(1))
	if n := Push(a, int64(2)); n != 2 {
		t.Errorf("Push returned %d, want 2", n)
	}
	if n := Push(a, int64(3)); n != 3 {
		t.Errorf("Push returned %d, want 3", n)
	}
	if v := Pop(a); v != int64(3) {
		t.Errorf("Pop returned %v, want 3", v)
	}
	if v := Pop(a); v != int64(2) {
		t.Errorf("Pop returned %v, want 2", v)
	}
	if v := Pop(a); v != int64(1) {
		t.Errorf("Pop returned %v, want 1", v)
	}
	if v := Pop(a); v != nil {
		t.Errorf("Pop on empty returned %v, want nil", v)
	}

	// Slice: positive, negative, clamped, and non-mutating.
	src := ArrayOf(int64(10), int64(20), int64(30), int64(40), int64(50))
	sliceTests := []struct {
		name  string
		start Value
		end   []Value
		want  []Value
	}{
		{"mid", int64(1), []Value{int64(4)}, []Value{int64(20), int64(30), int64(40)}},
		{"to end", int64(3), nil, []Value{int64(40), int64(50)}},
		{"negative start", int64(-2), nil, []Value{int64(40), int64(50)}},
		{"negative both", int64(-4), []Value{int64(-1)}, []Value{int64(20), int64(30), int64(40)}},
		{"clamped high", int64(2), []Value{int64(99)}, []Value{int64(30), int64(40), int64(50)}},
		{"clamped low", int64(-99), []Value{int64(2)}, []Value{int64(10), int64(20)}},
		{"start >= end", int64(4), []Value{int64(2)}, []Value{}},
	}
	for _, tt := range sliceTests {
		got := Slice(src, tt.start, tt.end...)
		if len(got.Items) != len(tt.want) {
			t.Errorf("Slice(%s) = %v, want %v", tt.name, got.Items, tt.want)
			continue
		}
		for i := range tt.want {
			if !Eq(got.Items[i], tt.want[i]) {
				t.Errorf("Slice(%s)[%d] = %v, want %v", tt.name, i, got.Items[i], tt.want[i])
			}
		}
	}
	// Mutating slice must not mutate original.
	s := Slice(src, int64(0), int64(2))
	s.Items[0] = int64(999)
	if src.Items[0] != int64(10) {
		t.Errorf("Slice shared backing array with original")
	}

	// Sort: numbers, strings, non-mutating.
	nums := ArrayOf(int64(10), 2.5, int64(2), int64(-5))
	sortedNums := Sort(nums)
	wantNums := []Value{int64(-5), int64(2), 2.5, int64(10)}
	for i := range wantNums {
		if !Eq(sortedNums.Items[i], wantNums[i]) {
			t.Errorf("Sort(nums)[%d] = %v, want %v", i, sortedNums.Items[i], wantNums[i])
		}
	}
	if nums.Items[0] != int64(10) {
		t.Errorf("Sort mutated input array")
	}

	strs := ArrayOf("banana", "apple", "cherry")
	sortedStrs := Sort(strs)
	wantStrs := []Value{"apple", "banana", "cherry"}
	for i := range wantStrs {
		if sortedStrs.Items[i] != wantStrs[i] {
			t.Errorf("Sort(strs)[%d] = %v, want %v", i, sortedStrs.Items[i], wantStrs[i])
		}
	}
}

func TestRegex(t *testing.T) {
	if !RegexMatch(`^\d{3}-\d{4}$`, "555-1234") {
		t.Errorf("RegexMatch expected true")
	}
	if RegexMatch(`^\d{3}-\d{4}$`, "abc-1234") {
		t.Errorf("RegexMatch expected false")
	}
	if got := RegexReplace(`\d+`, "item 12 and 34", "#"); got != "item # and #" {
		t.Errorf("RegexReplace = %q, want %q", got, "item # and #")
	}
	if got := RegexReplace(`(\w+)@(\w+)`, "a@b", "$2:$1"); got != "b:a" {
		t.Errorf("RegexReplace capture = %q, want %q", got, "b:a")
	}

	if k := panicKind(func() { RegexMatch("[invalid", "abc") }); k != KindTypeMismatch {
		t.Errorf("RegexMatch bad pattern: got %q, want %q", k, KindTypeMismatch)
	}
	if k := panicKind(func() { RegexReplace("[invalid", "abc", "x") }); k != KindTypeMismatch {
		t.Errorf("RegexReplace bad pattern: got %q, want %q", k, KindTypeMismatch)
	}
}
