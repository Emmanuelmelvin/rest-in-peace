package rt

// Request implements REQUEST(method, path [, body, headers]) for rip test. It
// calls the app's routes in-process and returns {status, headers, body}
// (Requirement 12.3).
func (a *App) Request(c *Ctx, method, path string, body Value, headers *Map) *Map {
	// TODO(task 4.2): build an *http.Request, run it through a.Handler() with
	// httptest.NewRecorder, and parse the JSON response body.
	panic(notBuilt("REQUEST", "4.2"))
}

// Expect implements EXPECT cond [, "message"] (Requirement 12.2).
func Expect(c *Ctx, cond bool, msg string) {
	if cond {
		return
	}
	if msg == "" {
		msg = "the condition was FALSE"
	}
	panic(Errorf(KindExpectFailed, "%s", msg))
}
