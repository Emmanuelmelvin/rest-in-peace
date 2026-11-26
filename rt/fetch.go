package rt

// Outbound HTTP (Requirement 11, task 3.7).

// Fetch implements FETCH(url [, options]), returning a *Map with keys
// "status", "headers" and "body" (with JSON bodies parsed).
// Options may include "method", "headers", "body" and "timeout" (default 10s).
func Fetch(c *Ctx, url string, opts ...Value) *Map {
	// TODO(task 3.7): perform outbound HTTP request using c.Context() with
	// timeout, JSON response parsing, and optional playground SSRF hook.
	panic(notBuilt("FETCH", "3.7"))
}
