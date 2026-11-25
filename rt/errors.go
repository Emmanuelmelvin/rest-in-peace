package rt

import "fmt"

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

// notBuilt marks runtime features that are still stubs.
func notBuilt(feature, task string) *Error {
	return Errorf(KindInternal, "%s is not built yet (task %s in .kiro/specs/rip-basic/tasks.md)", feature, task)
}
