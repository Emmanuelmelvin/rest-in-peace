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

// Classic-mode request helpers (Requirement 7.3). Generated code passes the
// request's Ctx; outside a request they raise an error.

// Param implements PARAM("name"): a :name path parameter.
func Param(c *Ctx, name Value) Value {
	// TODO(task 2.1)
	panic(notBuilt("PARAM", "2.1"))
}

// Query implements QUERY("name"): a query-string value.
func Query(c *Ctx, name Value) Value {
	// TODO(task 2.1)
	panic(notBuilt("QUERY", "2.1"))
}

// Header implements HEADER("name"): a request header, matched case-insensitively.
func Header(c *Ctx, name Value) Value {
	// TODO(task 2.1)
	panic(notBuilt("HEADER", "2.1"))
}

// Body implements BODY(): the parsed JSON body, {} when there is none.
func Body(c *Ctx) Value {
	// TODO(task 2.1)
	panic(notBuilt("BODY", "2.1"))
}
