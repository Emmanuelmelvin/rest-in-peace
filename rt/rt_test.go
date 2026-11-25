package rt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMapKeepsInsertionOrder(t *testing.T) {
	m := MapOf("b", int64(1), "a", int64(2))
	if got, want := ToString(m), `{"b":1,"a":2}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestEmptyArrayIsJSONArray(t *testing.T) {
	if got := ToString(ArrayOf()); got != "[]" {
		t.Fatalf("got %s, want []", got)
	}
}

func TestMissingKeyIsNil(t *testing.T) {
	if v := Get(MapOf("a", int64(1)), "missing"); v != nil {
		t.Fatalf("got %v, want NIL", v)
	}
}

func TestMemberKeysAreCaseSensitive(t *testing.T) {
	m := MapOf("customerId", int64(7))
	if v := Get(m, "CUSTOMERID"); v != nil {
		t.Fatalf("got %v, want NIL for a differently-cased key", v)
	}
}

func TestMemberOfNilRaisesNilAccess(t *testing.T) {
	defer func() {
		e, ok := recover().(*Error)
		if !ok || e.Kind != KindNilAccess {
			t.Fatalf("want a NIL ACCESS error, got %v", e)
		}
	}()
	Get(nil, "name")
}

func TestArithmetic(t *testing.T) {
	if got := Add(int64(2), int64(3)); got != int64(5) {
		t.Errorf("2 + 3 = %v", got)
	}
	if got := Add(int64(2), 0.5); got != 2.5 {
		t.Errorf("2 + 0.5 = %v", got)
	}
	if got := Add("A", int64(1)); got != "A1" {
		t.Errorf(`"A" + 1 = %v`, got)
	}
	if !Eq(int64(1), 1.0) {
		t.Errorf("1 = 1.0 should be TRUE")
	}
}

func TestErrorFormat(t *testing.T) {
	e := &Error{Kind: KindNilAccess, Message: "boom", File: "billing.bas", Line: 42}
	if got, want := e.Error(), "?NIL ACCESS ERROR IN billing.bas:42 — boom"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStackOverflow(t *testing.T) {
	c := NewCtx(context.Background())
	c.maxDepth = 3
	var f func()
	f = func() {
		defer c.Enter()()
		f()
	}
	defer func() {
		e, ok := recover().(*Error)
		if !ok || e.Kind != KindStackOverflow {
			t.Fatalf("want a STACK OVERFLOW error, got %v", e)
		}
		if c.depth != 0 {
			t.Fatalf("call depth leaked: %d", c.depth)
		}
	}()
	f()
}

func TestUnknownRouteIsNotFoundInThisRealm(t *testing.T) {
	app := NewApp("main.bas")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nowhere", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ROUTE NOT FOUND IN THIS REALM") {
		t.Fatalf("body %q", rec.Body.String())
	}
}
