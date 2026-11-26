package rt_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/rt"
)

type hostEmbed struct {
	TenantID string `json:"tenant_id"`
}

type hostUser struct {
	hostEmbed
	Name    string            `json:"full_name"`
	Age     int               `json:"age"`
	Active  bool              `json:"is_active"`
	Score   float64           `json:"score"`
	Tags    []string          `json:"tags"`
	Meta    map[string]int    `json:"meta"`
	Note    *string           `json:"note,omitempty"`
	ignored string            // unexported
	Skipped string            `json:"-"`
	Sub     *hostSub          `json:"sub,omitempty"`
	raw     map[string]string // unexported
}

type hostSub struct {
	Code string `json:"code"`
}

type ctxKey struct{}

func (u *hostUser) Greet(prefix string) string {
	return prefix + " " + u.Name
}

func (u *hostUser) WithCtx(ctx context.Context, suffix string) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v + ":" + u.Name + ":" + suffix
}

func (u *hostUser) Join(sep string, parts ...string) string {
	return u.Name + ":" + strings.Join(parts, sep)
}

func (u *hostUser) FailIf(bad bool) (string, error) {
	if bad {
		return "", errors.New("boom from go")
	}
	return "ok:" + u.Name, nil
}

func (u *hostUser) ErrOnly(bad bool) error {
	if bad {
		return errors.New("only error")
	}
	return nil
}

func (u *hostUser) AcceptStruct(sub *hostSub) string {
	if sub == nil {
		return "nil"
	}
	return sub.Code
}

func (u hostUser) ValueMethod() string {
	return "val:" + u.Name
}

func TestHostWrapAndJSON(t *testing.T) {
	if got := rt.WrapHost(nil); got != nil {
		t.Errorf("WrapHost(nil) = %v, want nil", got)
	}
	var nilPtr *url.URL
	if got := rt.WrapHost(nilPtr); got != nil {
		t.Errorf("WrapHost((*url.URL)(nil)) = %v, want nil", got)
	}

	u := &hostSub{Code: "ABC"}
	h := rt.WrapHost(u)
	if rt.WrapHost(h) != h {
		t.Errorf("WrapHost should not double-wrap Host")
	}
	b, err := json.Marshal(rt.MapOf("sub", h))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"sub":{"code":"ABC"}}`; got != want {
		t.Errorf("MarshalJSON = %s, want %s", got, want)
	}
}

func TestHostGetAndSet(t *testing.T) {
	u := &url.URL{}
	h := rt.WrapHost(u)

	// Set by exact name and case-insensitive name.
	rt.SetMember(h, "Scheme", "https")
	rt.SetMember(h, "host", "example.com")
	rt.SetMember(h, "PATH", "/hello")
	rt.SetMember(h, "rawquery", "a=1")

	if got := rt.Get(h, "Scheme"); got != "https" {
		t.Errorf("Scheme = %v, want https", got)
	}
	if got := rt.Get(h, "host"); got != "example.com" {
		t.Errorf("host = %v, want example.com", got)
	}
	if got := rt.HostCall(rt.NewCtx(nil), h, "String"); got != "https://example.com/hello?a=1" {
		t.Errorf("String() = %v", got)
	}

	// JSON tag and embedded field read/write.
	usr := rt.WrapHost(&hostUser{})
	rt.SetMember(usr, "full_name", "Ada")
	rt.SetMember(usr, "age", int64(37))
	rt.SetMember(usr, "is_active", true)
	rt.SetMember(usr, "tenant_id", "t-1")
	rt.SetMember(usr, "note", "hello")
	rt.SetMember(usr, "sub", rt.MapOf("code", "X9"))

	if got := rt.Get(usr, "Name"); got != "Ada" {
		t.Errorf("Name = %v, want Ada", got)
	}
	if got := rt.Get(usr, "full_name"); got != "Ada" {
		t.Errorf("full_name = %v, want Ada", got)
	}
	if got := rt.Get(usr, "age"); got != int64(37) {
		t.Errorf("age = %v, want int64(37)", got)
	}
	if got := rt.Get(usr, "tenant_id"); got != "t-1" {
		t.Errorf("tenant_id = %v, want t-1", got)
	}
	if got := rt.Get(usr, "note"); got != "hello" {
		t.Errorf("note = %v, want hello", got)
	}
	if got := rt.Get(rt.Get(usr, "sub"), "code"); got != "X9" {
		t.Errorf("sub.code = %v, want X9", got)
	}

	// Value struct read works; writing a non-pointer struct panics TYPE MISMATCH.
	valHost := rt.WrapHost(hostUser{Name: "Grace"})
	if got := rt.Get(valHost, "name"); got != "Grace" {
		t.Errorf("value struct Get = %v, want Grace", got)
	}
	expectHostPanic(t, rt.KindTypeMismatch, func() {
		rt.SetMember(valHost, "Name", "Other")
	})

	// Missing / unexported field panics UNDEFINED.
	expectHostPanic(t, rt.KindUndefined, func() {
		rt.Get(usr, "DoesNotExist")
	})
	expectHostPanic(t, rt.KindUndefined, func() {
		rt.Get(usr, "ignored")
	})
}

func TestHostCall(t *testing.T) {
	u := rt.WrapHost(&hostUser{Name: "Ada"})
	ctx := context.WithValue(context.Background(), ctxKey{}, "req-42")
	c := rt.NewCtx(ctx)

	if got := rt.HostCall(c, u, "Greet", "Hello"); got != "Hello Ada" {
		t.Errorf("Greet = %v", got)
	}
	// Case-insensitive method match.
	if got := rt.HostCall(c, u, "greet", "Hi"); got != "Hi Ada" {
		t.Errorf("greet = %v", got)
	}
	// context.Context auto-injection.
	if got := rt.HostCall(c, u, "WithCtx", "end"); got != "req-42:Ada:end" {
		t.Errorf("WithCtx = %v, want req-42:Ada:end", got)
	}
	// Variadic method.
	if got := rt.HostCall(c, u, "Join", "-", "a", "b", "c"); got != "Ada:a-b-c" {
		t.Errorf("Join = %v", got)
	}
	if got := rt.HostCall(c, u, "Join", "-"); got != "Ada:" {
		t.Errorf("Join(0 variadic) = %v", got)
	}
	// Map → struct pointer conversion inside reflective method call.
	if got := rt.HostCall(c, u, "AcceptStruct", rt.MapOf("code", "Z1")); got != "Z1" {
		t.Errorf("AcceptStruct(map) = %v", got)
	}
	if got := rt.HostCall(c, u, "AcceptStruct", nil); got != "nil" {
		t.Errorf("AcceptStruct(nil) = %v", got)
	}
	// (T, error) success and failure → KindGo.
	if got := rt.HostCall(c, u, "FailIf", false); got != "ok:Ada" {
		t.Errorf("FailIf(false) = %v", got)
	}
	err := expectHostPanic(t, rt.KindGo, func() {
		rt.HostCall(c, u, "FailIf", true)
	})
	if err.Message != "boom from go" {
		t.Errorf("error message = %q, want boom from go", err.Message)
	}
	// (error) success and failure.
	if got := rt.HostCall(c, u, "ErrOnly", false); got != nil {
		t.Errorf("ErrOnly(false) = %v, want nil", got)
	}
	expectHostPanic(t, rt.KindGo, func() {
		rt.HostCall(c, u, "ErrOnly", true)
	})
	// Pointer-receiver method called on value struct copy.
	valHost := rt.WrapHost(hostUser{Name: "Bob"})
	if got := rt.HostCall(c, valHost, "Greet", "Yo"); got != "Yo Bob" {
		t.Errorf("value struct pointer method = %v", got)
	}
	if got := rt.HostCall(c, valHost, "ValueMethod"); got != "val:Bob" {
		t.Errorf("ValueMethod = %v", got)
	}
	// Method on time.Time works.
	tm := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	if got := rt.HostCall(c, tm, "Year"); got != int64(2026) {
		t.Errorf("time.Time.Year() = %v", got)
	}
	// Calling method on non-Host BASIC values fails cleanly.
	expectHostPanic(t, rt.KindTypeMismatch, func() {
		rt.HostCall(c, rt.MapOf("a", int64(1)), "Get", "a")
	})
	expectHostPanic(t, rt.KindNilAccess, func() {
		rt.HostCall(c, nil, "String")
	})
	expectHostPanic(t, rt.KindUndefined, func() {
		rt.HostCall(c, u, "NoSuchMethod")
	})
}

func TestToStructAndConversions(t *testing.T) {
	m := rt.MapOf(
		"full_name", "Grace Hopper",
		"AGE", int64(85),
		"is_active", true,
		"score", float64(99.5),
		"tags", rt.ArrayOf("cobol", "compiler"),
		"meta", rt.MapOf("rank", int64(1)),
		"note", "admiral",
		"tenant_id", "navy",
		"sub", rt.MapOf("CODE", "S1"),
		"unknownField", "ignored",
	)

	u := rt.ToStruct[hostUser](m)
	if u.Name != "Grace Hopper" || u.Age != 85 || !u.Active || u.Score != 99.5 {
		t.Fatalf("ToStruct primitive fields mismatch: %+v", u)
	}
	if u.TenantID != "navy" {
		t.Errorf("embedded TenantID = %q, want navy", u.TenantID)
	}
	if len(u.Tags) != 2 || u.Tags[0] != "cobol" || u.Tags[1] != "compiler" {
		t.Errorf("Tags = %v", u.Tags)
	}
	if u.Meta["rank"] != 1 {
		t.Errorf("Meta = %v", u.Meta)
	}
	if u.Note == nil || *u.Note != "admiral" {
		t.Errorf("Note = %v", u.Note)
	}
	if u.Sub == nil || u.Sub.Code != "S1" {
		t.Errorf("Sub = %+v", u.Sub)
	}

	ptr := rt.ToStructPtr[hostUser](m)
	if ptr == nil || ptr.Name != "Grace Hopper" {
		t.Fatalf("ToStructPtr failed: %+v", ptr)
	}
	if got := rt.ToStructPtr[hostUser](nil); got != nil {
		t.Errorf("ToStructPtr(nil) = %v, want nil", got)
	}
	if got := rt.ToGo[io.Reader](nil); got != nil {
		t.Errorf("ToGo[io.Reader](nil) = %v, want nil", got)
	}
	if got := rt.ToGo[io.Reader](rt.WrapHost(strings.NewReader("ok"))); got == nil {
		t.Errorf("ToGo[io.Reader](*strings.Reader) = nil, want reader")
	}

	// Slices and maps.
	sl := rt.ToGoSlice[string](rt.ArrayOf("a", "b"))
	if len(sl) != 2 || sl[0] != "a" || sl[1] != "b" {
		t.Errorf("ToGoSlice = %v", sl)
	}
	gm := rt.ToGoMap[int](rt.MapOf("x", int64(10), "y", float64(20)))
	if gm["x"] != 10 || gm["y"] != 20 {
		t.Errorf("ToGoMap = %v", gm)
	}
	if b := rt.ToGoBytes("hi"); string(b) != "hi" {
		t.Errorf("ToGoBytes(string) = %q", b)
	}
	if b := rt.ToGoBytes(rt.ArrayOf(int64(65), int64(66))); string(b) != "AB" {
		t.Errorf("ToGoBytes(array) = %q", b)
	}

	// Numeric range errors.
	expectHostPanic(t, rt.KindTypeMismatch, func() { rt.ToGoInt8(int64(200)) })
	expectHostPanic(t, rt.KindTypeMismatch, func() { rt.ToGoUint(int64(-1)) })
	expectHostPanic(t, rt.KindTypeMismatch, func() { rt.ToGoInt(float64(3.5)) })
	expectHostPanic(t, rt.KindTypeMismatch, func() { rt.ToGoString(int64(5)) })
}

func TestFromGoAndGoResult(t *testing.T) {
	if got := rt.FromGo(42); got != int64(42) {
		t.Errorf("FromGo(int) = %T(%v)", got, got)
	}
	if got := rt.FromGo(float32(2.5)); got != float64(2.5) {
		t.Errorf("FromGo(float32) = %T(%v)", got, got)
	}
	if got := rt.FromGo([]byte("abc")); got != "abc" {
		t.Errorf("FromGo([]byte) = %T(%v)", got, got)
	}
	arr, ok := rt.FromGo([]string{"x", "y"}).(*rt.Array)
	if !ok || arr.Len() != 2 || arr.Items[0] != "x" {
		t.Errorf("FromGo([]string) = %#v", arr)
	}
	mp, ok := rt.FromGo(map[string]int{"b": 2, "a": 1}).(*rt.Map)
	if !ok || mp.Len() != 2 || strings.Join(mp.Keys(), ",") != "a,b" {
		t.Errorf("FromGo(map[string]int) = %#v", mp)
	}
	if got := rt.GoResult("ok", nil); got != "ok" {
		t.Errorf("GoResult(ok, nil) = %v", got)
	}
	err := expectHostPanic(t, rt.KindGo, func() {
		rt.GoResult("ignored", errors.New("bad input"))
	})
	if err.Message != "bad input" {
		t.Errorf("GoResult error = %q", err.Message)
	}
}

func expectHostPanic(t *testing.T, kind rt.Kind, fn func()) (caught *rt.Error) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic with kind %s, got none", kind)
		}
		e, ok := r.(*rt.Error)
		if !ok {
			t.Fatalf("expected *rt.Error, got %T (%v)", r, r)
		}
		if e.Kind != kind {
			t.Fatalf("error kind = %s, want %s (message: %s)", e.Kind, kind, e.Message)
		}
		caught = e
	}()
	fn()
	return nil
}
