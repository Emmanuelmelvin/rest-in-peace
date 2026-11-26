package rt

import (
	"errors"
	"strings"
	"testing"
)

// try runs body the way a generated TRY does (codegen/try.go): FINALLY in a
// defer around everything, the body under defer Recover, then CATCH.
func try(body func(), catch func(e *Error), finally func()) {
	defer finally()
	var e *Error
	func() {
		defer Recover(&e)
		body()
	}()
	if e != nil {
		catch(e)
	}
}

// escaped runs f and returns what panicked out of it.
func escaped(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

func TestErrorValue(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"ERROR", NewError("boom"), `{"message":"boom","kind":"USER","file":null,"line":null}`},
		{"ERROR of a number", NewError(int64(42)), `{"message":"42","kind":"USER","file":null,"line":null}`},
		{"HTTPERROR", NewHTTPError(int64(404), "INVOICE IS DEAD"), `{"message":"INVOICE IS DEAD","kind":"HTTP","file":null,"line":null,"status":404}`},
		{"HTTPERROR with details", NewHTTPError(float64(422), "BAD", MapOf("field", "email")), `{"message":"BAD","kind":"HTTP","file":null,"line":null,"status":422,"details":{"field":"email"}}`},
		{"HTTPERROR with NIL details", NewHTTPError(int64(400), "BAD", nil), `{"message":"BAD","kind":"HTTP","file":null,"line":null,"status":400}`},
		{"located error", (&Error{Kind: KindDivisionByZero, Message: "zero", File: "a.bas", Line: 3}).Value(), `{"message":"zero","kind":"DIVISION BY ZERO","file":"a.bas","line":3}`},
	}
	for _, c := range cases {
		if got := ToString(c.v); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	// e.message and friends are plain member access.
	if got := Get(NewHTTPError(int64(404), "GONE"), "status"); got != int64(404) {
		t.Errorf("e.status = %v", got)
	}
}

func TestHTTPErrorNeedsAStatus(t *testing.T) {
	for _, status := range []Value{int64(99), int64(600), float64(404.5), "404", nil} {
		e, ok := escaped(func() { NewHTTPError(status, "X") }).(*Error)
		if !ok || e.Kind != KindTypeMismatch || !strings.Contains(e.Message, "status code from 100 to 599") {
			t.Errorf("HTTPERROR(%v, …) raised %v", status, e)
		}
	}
}

func TestThrow(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want Error
	}{
		{"ERROR", NewError("boom"), Error{Kind: KindUser, Message: "boom"}},
		{"HTTPERROR", NewHTTPError(int64(409), "TAKEN", "dup"), Error{Kind: KindHTTP, Message: "TAKEN", Status: 409, Details: "dup"}},
		// Rethrowing a caught e keeps its kind and where it was first raised.
		{"caught e", (&Error{Kind: KindNilAccess, Message: "nil", File: "x.bas", Line: 9}).Value(), Error{Kind: KindNilAccess, Message: "nil", File: "x.bas", Line: 9}},
		{"changed e", MapOf("message", "more context", "kind", "USER", "line", nil), Error{Kind: KindUser, Message: "more context"}},
		{"*Error", &Error{Kind: KindGo, Message: "io"}, Error{Kind: KindGo, Message: "io"}},
		{"STRING", "oops", Error{Kind: KindTypeMismatch}},
		{"MAP without kind", MapOf("message", "x"), Error{Kind: KindTypeMismatch}},
		{"NIL", nil, Error{Kind: KindTypeMismatch}},
	}
	for _, c := range cases {
		e, ok := escaped(func() { Throw(c.v) }).(*Error)
		switch {
		case !ok:
			t.Errorf("%s: THROW did not raise an *Error", c.name)
		case c.want.Kind == KindTypeMismatch:
			if e.Kind != KindTypeMismatch || !strings.Contains(e.Message, `THROW needs an error, like THROW ERROR("message")`) {
				t.Errorf("%s: got %v", c.name, e)
			}
		case e.Kind != c.want.Kind || e.Message != c.want.Message || e.File != c.want.File ||
			e.Line != c.want.Line || e.Status != c.want.Status || e.Details != c.want.Details:
			t.Errorf("%s: got %+v, want %+v", c.name, *e, c.want)
		}
	}
}

func TestRecoverCatchesErrorsAndPanics(t *testing.T) {
	var zero int
	cases := []struct {
		name string
		body func()
		kind Kind
	}{
		{"THROW", func() { Throw(NewError("boom")) }, KindUser},
		{"rt error", func() { Get(nil, "x") }, KindNilAccess},
		{"Go divide by zero", func() { _ = 1 / zero }, KindDivisionByZero},
		{"Go index out of range", func() { _ = []int{}[zero] }, KindBadSubscript},
		{"Go nil map", func() { var m map[string]int; m["x"] = 1 }, KindNilAccess},
		{"Go error value", func() { panic(errors.New("disk on fire")) }, KindInternal},
		{"anything else", func() { panic(42) }, KindInternal},
	}
	for _, c := range cases {
		var caught *Error
		finally := false
		try(c.body, func(e *Error) { caught = e }, func() { finally = true })
		if caught == nil || caught.Kind != c.kind {
			t.Errorf("%s: caught %v, want kind %s", c.name, caught, c.kind)
		}
		if !finally {
			t.Errorf("%s: FINALLY did not run", c.name)
		}
	}
}

// RESPOND is not an error: CATCH never sees it, FINALLY still runs, and it
// keeps going to the handler wrapper (Requirement 6.7). END and the loop
// guard's ETERNAL LOOP pass through the same way.
func TestRecoverLetsSignalsThrough(t *testing.T) {
	cases := []struct {
		name  string
		body  func()
		check func(r any) bool
	}{
		{"RESPOND", func() { RespondNow(nil, 201, "made") }, func(r any) bool {
			x, ok := r.(*Respond)
			return ok && x.Status == 201 && x.Body == "made"
		}},
		{"END", End, func(r any) bool { _, ok := r.(endSignal); return ok }},
		{"ETERNAL LOOP", func() { panic(&Error{Kind: KindEternalLoop, Status: 508, Message: "LOOP"}) }, func(r any) bool {
			e, ok := r.(*Error)
			return ok && e.Kind == KindEternalLoop
		}},
	}
	for _, c := range cases {
		caught, finally := false, false
		r := escaped(func() {
			try(c.body, func(*Error) { caught = true }, func() { finally = true })
		})
		if caught {
			t.Errorf("%s: CATCH ran", c.name)
		}
		if !finally {
			t.Errorf("%s: FINALLY did not run", c.name)
		}
		if !c.check(r) {
			t.Errorf("%s: %#v escaped", c.name, r)
		}
	}

	// A RESPOND inside nested TRYs runs every FINALLY, innermost first.
	var order []string
	r := escaped(func() {
		try(func() {
			try(func() { RespondNow(nil, 200, "ok") },
				func(*Error) { order = append(order, "inner catch") },
				func() { order = append(order, "inner finally") })
		}, func(*Error) { order = append(order, "outer catch") },
			func() { order = append(order, "outer finally") })
	})
	if _, ok := r.(*Respond); !ok || strings.Join(order, ", ") != "inner finally, outer finally" {
		t.Errorf("escaped %#v, ran %v", r, order)
	}
}

func TestErrorResponse(t *testing.T) {
	var logged []*Error
	saved := logError
	logError = func(c *Ctx, e *Error) { logged = append(logged, e) }
	defer func() { logError = saved }()

	c := NewCtx(nil)
	c.Req = &Request{ID: "req-7"}
	internal := &Error{Kind: KindNilAccess, Message: `cannot read "id" of NIL`, File: "api.bas", Line: 12}
	cases := []struct {
		name   string
		c      *Ctx
		e      *Error
		status int
		body   string
		logged bool
	}{
		{"HTTPERROR", c, HTTPError(404, "INVOICE IS DEAD", nil), 404, `{"error":"INVOICE IS DEAD"}`, false},
		{"HTTPERROR with details", c, HTTPError(422, "BAD INPUT", MapOf("field", "email")), 422, `{"error":"BAD INPUT","details":{"field":"email"}}`, false},
		{"rethrown HTTPERROR", c, errorFromValue(NewHTTPError(int64(403), "NO")), 403, `{"error":"NO"}`, false},
		{"runtime error", c, internal, 500, `{"error":"INTERNAL ERROR","requestId":"req-7"}`, true},
		{"Go panic", c, FromPanic(errors.New("pq: password authentication failed")), 500, `{"error":"INTERNAL ERROR","requestId":"req-7"}`, true},
		{"THROW ERROR", c, errorFromValue(NewError("secret detail")), 500, `{"error":"INTERNAL ERROR","requestId":"req-7"}`, true},
		{"HTTP kind without a status", c, &Error{Kind: KindHTTP, Message: "?"}, 500, `{"error":"INTERNAL ERROR","requestId":"req-7"}`, true},
		{"no request", nil, internal, 500, `{"error":"INTERNAL ERROR","requestId":""}`, true},
		{"ETERNAL LOOP", c, &Error{Kind: KindEternalLoop, Status: 508, Message: "TRAPPED IN AN ETERNAL GOTO LOOP"}, 508, `{"error":"TRAPPED IN AN ETERNAL GOTO LOOP"}`, true},
	}
	for _, tc := range cases {
		logged = nil
		status, body := ErrorResponse(tc.c, tc.e)
		if status != tc.status || ToString(body) != tc.body {
			t.Errorf("%s: %d %s, want %d %s", tc.name, status, ToString(body), tc.status, tc.body)
		}
		if got := len(logged) == 1 && logged[0] == tc.e; got != tc.logged {
			t.Errorf("%s: logged %v, want logged=%v", tc.name, logged, tc.logged)
		}
		if tc.status == 500 && (strings.Contains(ToString(body), tc.e.Message) || strings.Contains(ToString(body), "api.bas")) {
			t.Errorf("%s: the 500 body leaks the error: %s", tc.name, ToString(body))
		}
	}
}
