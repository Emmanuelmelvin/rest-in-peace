package rt

// Request is the parsed HTTP request (Requirement 7.2).
type Request struct {
	ID      string
	Method  string
	Path    string
	Params  *Map
	Query   *Map
	Headers *Map  // lowercase keys
	Body    Value // parsed JSON; an empty map when there is no body
	Ctx     *Map  // written by middleware
}

// Value returns the request as the BASIC map passed to structured handlers.
func (r *Request) Value() *Map {
	return MapOf(
		"method", r.Method,
		"path", r.Path,
		"params", r.Params,
		"query", r.Query,
		"headers", r.Headers,
		"body", r.Body,
		"ctx", r.Ctx,
		"id", r.ID,
	)
}
