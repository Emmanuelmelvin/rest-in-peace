package rt

import (
	"fmt"
	"runtime"
	"strings"
)

// Kind is the classic error word: ?<KIND> ERROR IN file:line — hint.
type Kind string

const (
	KindTypeMismatch       Kind = "TYPE MISMATCH"
	KindNilAccess          Kind = "NIL ACCESS"
	KindBadSubscript       Kind = "BAD SUBSCRIPT"
	KindDivisionByZero     Kind = "DIVISION BY ZERO"
	KindStackOverflow      Kind = "STACK OVERFLOW"
	KindReturnWithoutGosub Kind = "RETURN WITHOUT GOSUB"
	KindUndefined          Kind = "UNDEFINED"
	KindUser               Kind = "USER" // THROW ERROR("…")
	KindHTTP               Kind = "HTTP" // THROW HTTPERROR(…)
	KindGo                 Kind = "GO"   // error returned by a Go package
	KindEternalLoop        Kind = "ETERNAL LOOP"
	KindExpectFailed       Kind = "EXPECT FAILED"
	KindNotAllowed         Kind = "NOT ALLOWED"
	KindInternal           Kind = "INTERNAL"
)

// Error is a BASIC runtime error. Generated code raises it with panic, and
// TRY/CATCH recovers it (design §4.6). File and Line are filled in from the
// //line directives when the panic is recovered.
type Error struct {
	Kind    Kind
	Message string
	File    string
	Line    int
	Status  int   // HTTP errors only
	Details Value // HTTP errors only
	Cause   error // wrapped Go error, if any
}

// Errorf builds an Error with a formatted message.
func Errorf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Error formats the classic way: ?KIND ERROR IN file:line — message.
func (e *Error) Error() string {
	loc := ""
	if e.File != "" {
		loc = fmt.Sprintf(" IN %s:%d", e.File, e.Line)
	}
	return fmt.Sprintf("?%s ERROR%s — %s", e.Kind, loc, e.Message)
}

// Unwrap exposes the wrapped Go error.
func (e *Error) Unwrap() error { return e.Cause }

// HTTPError builds the value for THROW HTTPERROR(status, message [, details]).
func HTTPError(status int, message string, details Value) *Error {
	return &Error{Kind: KindHTTP, Status: status, Message: message, Details: details}
}

// NewError implements ERROR("message"), the value of THROW ERROR(…).
// Called by generated code.
//
// Error values are ordinary maps of the same shape as the e that CATCH binds
// (see Value), so e.message, PRINT and JSON work on them like on any other
// value. THROW turns them back into an *Error.
func NewError(message Value) Value {
	return (&Error{Kind: KindUser, Message: ToString(message)}).Value()
}

// NewHTTPError implements HTTPERROR(status, "message" [, details]).
// Called by generated code.
func NewHTTPError(status, message Value, details ...Value) Value {
	n, ok := toInt(status)
	if !ok || n < 100 || n > 599 {
		panic(Errorf(KindTypeMismatch, `HTTPERROR needs a status code from 100 to 599, like HTTPERROR(404, "NOT FOUND"), got %s %s`, TypeName(status), ToString(status)))
	}
	var d Value
	if len(details) > 0 {
		d = details[0]
	}
	return HTTPError(int(n), ToString(message), d).Value()
}

// Value returns the error as the BASIC value that CATCH e binds
// (Requirement 6.3): a map with message, kind, file and line, plus status
// and details when the error has them. An unknown location is NIL.
func (e *Error) Value() *Map {
	m := MapOf("message", e.Message, "kind", string(e.Kind))
	if e.File != "" {
		m.Set("file", e.File)
		m.Set("line", int64(e.Line))
	} else {
		m.Set("file", nil)
		m.Set("line", nil)
	}
	if e.Status != 0 {
		m.Set("status", int64(e.Status))
	}
	if e.Details != nil {
		m.Set("details", e.Details)
	}
	return m
}

// Throw implements THROW v. v is an error value: one made by ERROR or
// HTTPERROR, or the e of a CATCH, which rethrows it with its kind and its
// original location. Anything else is a TYPE MISMATCH.
func Throw(v Value) {
	panic(errorFromValue(v))
}

// errorFromValue turns the value of THROW back into an *Error.
func errorFromValue(v Value) *Error {
	switch x := v.(type) {
	case *Error:
		return x
	case *Map:
		if kind, ok := mapGet(x, "kind").(string); ok && kind != "" {
			e := &Error{Kind: Kind(kind), Message: ToString(mapGet(x, "message")), Details: mapGet(x, "details")}
			if file, ok := mapGet(x, "file").(string); ok {
				e.File = file
				if n, ok := toInt(mapGet(x, "line")); ok {
					e.Line = int(n)
				}
			}
			if n, ok := toInt(mapGet(x, "status")); ok {
				e.Status = int(n)
			}
			return e
		}
	}
	return Errorf(KindTypeMismatch, `THROW needs an error, like THROW ERROR("message") or THROW HTTPERROR(404, "NOT FOUND"), got %s`, TypeName(v))
}

func mapGet(m *Map, key string) Value {
	if m == nil {
		return nil
	}
	v, _ := m.Get(key)
	return v
}

// Recover is deferred by every generated TRY body (design §4.6):
//
//	defer rt.Recover(&e)
//
// It stops a BASIC error or Go panic and stores it in *e for CATCH, with the
// BASIC file and line where it was raised. RESPOND, END and ETERNAL LOOP are
// not errors that CATCH may stop, so they keep unwinding (Requirement 6.7)
// and the FINALLY around the TRY still runs. It must be deferred directly,
// so that recover works and the panicking frames are still on the stack.
func Recover(e **Error) {
	r := recover()
	if r == nil {
		return
	}
	switch x := r.(type) {
	case *Respond, endSignal:
		panic(r)
	case *Error:
		if x.Kind == KindEternalLoop {
			panic(r)
		}
	}
	err := FromPanic(r)
	if err.File == "" {
		err.File, err.Line = basLocation()
	}
	*e = err
}

// ErrorResponse maps an error that escaped a handler or middleware to the
// HTTP response (Requirements 6.4 and 6.5). The HTTP server (app.go) calls it
// for every *Error it recovers; c.Req is the current request.
//
//   - HTTPERROR: its status and {"error": message, "details": …}, without
//     "details" when there are none.
//   - ETERNAL LOOP (the playground loop guard): 508 and its message.
//   - Anything else, including Go panics: 500 and only
//     {"error": "INTERNAL ERROR", "requestId": …}. The full error goes to the
//     log with the request ID, never to the client.
func ErrorResponse(c *Ctx, e *Error) (status int, body Value) {
	if e.Kind == KindHTTP && e.Status != 0 {
		m := MapOf("error", e.Message)
		if e.Details != nil {
			m.Set("details", e.Details)
		}
		return e.Status, m
	}
	logError(c, e)
	if e.Kind == KindEternalLoop {
		return 508, MapOf("error", e.Message)
	}
	id := ""
	if c != nil && c.Req != nil {
		id = c.Req.ID
	}
	return 500, MapOf("error", "INTERNAL ERROR", "requestId", id)
}

// logError logs an error that became a 5xx response. Log adds the request
// ID. It is a variable so that tests can watch it.
var logError = func(c *Ctx, e *Error) {
	fields := MapOf("kind", string(e.Kind))
	if e.File != "" {
		fields.Set("file", e.File)
		fields.Set("line", int64(e.Line))
	}
	Log(c, "ERROR", e.Error(), fields)
}

// Respond is the control-flow signal raised by RESPOND. It is not an error:
// CATCH re-panics it, FINALLY still runs, and TRANSACTION commits
// (Requirement 6.7).
type Respond struct {
	Status  int
	Body    Value
	Headers *Map
}

// RespondNow implements RESPOND status, value [, headers].
func RespondNow(c *Ctx, status int, body Value, headers ...*Map) {
	r := &Respond{Status: status, Body: body}
	if len(headers) > 0 {
		r.Headers = headers[0]
	}
	panic(r)
}

// FromPanic turns a recovered panic value into an *Error. Go runtime panics
// get the closest BASIC kind (design §4.6).
func FromPanic(r any) *Error {
	switch x := r.(type) {
	case *Error:
		return x
	case *Respond:
		return Errorf(KindInternal, "RESPOND only works inside a ROUTE handler")
	case runtime.Error:
		msg := x.Error()
		switch {
		case strings.Contains(msg, "divide by zero"):
			return &Error{Kind: KindDivisionByZero, Message: "the right side of a division is zero", Cause: x}
		case strings.Contains(msg, "index out of range"):
			return &Error{Kind: KindBadSubscript, Message: "an index is outside the array — arrays start at 0", Cause: x}
		case strings.Contains(msg, "nil map"), strings.Contains(msg, "nil pointer"):
			return &Error{Kind: KindNilAccess, Message: "a value is NIL — check it exists before using it", Cause: x}
		}
		return &Error{Kind: KindInternal, Message: msg, Cause: x}
	case error:
		return &Error{Kind: KindInternal, Message: x.Error(), Cause: x}
	default:
		return Errorf(KindInternal, "%v", r)
	}
}

// notBuilt marks runtime features that are still stubs.
func notBuilt(feature, task string) *Error {
	return Errorf(KindInternal, "%s is not built yet (task %s in .kiro/specs/rip-basic/tasks.md)", feature, task)
}
