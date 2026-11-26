package rt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// fetchMaxBody is the response-body read limit (1 MB, Requirement 11.2).
const fetchMaxBody = 1 << 20

// fetchDefaultTimeout is used when the caller does not supply one (Requirement 11.1).
const fetchDefaultTimeout = 10 * time.Second

// fetchDialer is the hook point for the playground SSRF guard (task 5.3).
// In a plain binary it is nil and the default transport is used.
// When RIP_PLAYGROUND=1 the playground host replaces it with a dialer that
// blocks private, loopback, link-local and cloud-metadata addresses.
var fetchDialer func() http.RoundTripper

// Fetch implements FETCH(url [, options]) (Requirement 11.1).
//
// Returns a *Map with keys:
//   - "status"  — int64 HTTP status code
//   - "headers" — *Map of lowercase header names to string values
//   - "body"    — parsed JSON (*Map/*Array/primitive) when the response
//     Content-Type is application/json or the body is valid JSON;
//     otherwise the raw response string
//
// opts, if provided, must be a *Map (otherwise ?TYPE MISMATCH ERROR):
//   - "method"  — string, uppercased (default "GET")
//   - "headers" — *Map of header names to values
//   - "body"    — string sent as-is, or any other non-nil value JSON-encoded;
//     Content-Type defaults to application/json when body is non-string
//   - "timeout" — NUMBER of seconds (default 10; <= 0 or wrong type raises ?TYPE MISMATCH ERROR)
func Fetch(c *Ctx, url string, opts ...Value) *Map {
	method := "GET"
	var reqBody io.Reader
	var reqHeaders *Map
	timeout := fetchDefaultTimeout

	// --- parse opts ---------------------------------------------------------
	if len(opts) > 0 && opts[0] != nil {
		m, ok := opts[0].(*Map)
		if !ok {
			panic(Errorf(KindTypeMismatch,
				"FETCH options must be a MAP like {\"method\": \"POST\", \"body\": data}, got %s",
				TypeName(opts[0])))
		}

		// method
		if v, ok := m.Get("method"); ok && v != nil {
			s, ok := v.(string)
			if !ok {
				panic(Errorf(KindTypeMismatch,
					"FETCH options.method must be a STRING like \"POST\", got %s", TypeName(v)))
			}
			method = strings.ToUpper(s)
		}

		// headers
		if v, ok := m.Get("headers"); ok && v != nil {
			hm, ok := v.(*Map)
			if !ok {
				panic(Errorf(KindTypeMismatch,
					"FETCH options.headers must be a MAP, got %s", TypeName(v)))
			}
			reqHeaders = hm
		}

		// body
		if v, ok := m.Get("body"); ok && v != nil {
			switch b := v.(type) {
			case string:
				reqBody = strings.NewReader(b)
			default:
				encoded, err := marshalValue(v)
				if err != nil {
					panic(Errorf(KindInternal,
						"FETCH could not encode body as JSON — %s", err.Error()))
				}
				reqBody = strings.NewReader(string(encoded))
				// mark that we want to default Content-Type to application/json
				// (overridable by an explicit header; we set it after building the request)
				if reqHeaders == nil {
					reqHeaders = NewMap()
				}
				if _, hasct := headerGet(reqHeaders, "content-type"); !hasct {
					reqHeaders.Set("content-type", "application/json")
				}
			}
		}

		// timeout
		if v, ok := m.Get("timeout"); ok && v != nil {
			secs, _, _, numOk := numFloat(v)
			if !numOk {
				panic(Errorf(KindTypeMismatch,
					"FETCH options.timeout must be a NUMBER of seconds like 5 or 0.5, got %s",
					TypeName(v)))
			}
			if secs <= 0 {
				panic(Errorf(KindTypeMismatch,
					"FETCH options.timeout must be greater than zero, got %g", secs))
			}
			timeout = time.Duration(secs * float64(time.Second))
		}
	}

	// --- build context with timeout -----------------------------------------
	parent := context.Background()
	if c != nil {
		parent = c.Context()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// --- build request -------------------------------------------------------
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		panic(Errorf(KindHTTP,
			"FETCH could not create request for %q — %s", url, err.Error()))
	}

	for _, k := range reqHeaders.Keys() {
		v, _ := reqHeaders.Get(k)
		req.Header.Set(k, ToString(v))
	}

	// --- choose transport ----------------------------------------------------
	transport := http.DefaultTransport
	if fetchDialer != nil {
		transport = fetchDialer()
	} else if isPlayground() {
		// RIP_PLAYGROUND=1 but no dialer installed yet (pre-5.3): allow, the
		// host container holds no secrets (security.md).
		transport = http.DefaultTransport
	}

	client := &http.Client{Transport: transport}

	// --- execute -------------------------------------------------------------
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			panic(Errorf(KindHTTP,
				"FETCH timed out after %s — increase options.timeout or check the remote URL", timeout))
		}
		if ctx.Err() == context.Canceled {
			panic(Errorf(KindHTTP,
				"FETCH was cancelled — the request context was cancelled before the response arrived"))
		}
		panic(Errorf(KindHTTP,
			"FETCH failed for %q — %s", url, err.Error()))
	}
	defer resp.Body.Close()

	// --- read body (capped at fetchMaxBody) ----------------------------------
	raw, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBody))
	if err != nil {
		panic(Errorf(KindHTTP,
			"FETCH could not read response body from %q — %s", url, err.Error()))
	}

	// --- build response headers map (lowercase keys) -------------------------
	headers := NewMap()
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			headers.Set(strings.ToLower(k), vs[0])
		}
	}

	// --- parse body ----------------------------------------------------------
	body := parseResponseBody(raw, resp.Header.Get("Content-Type"))

	return MapOf(
		"status", int64(resp.StatusCode),
		"headers", headers,
		"body", body,
	)
}

// parseResponseBody returns parsed JSON when the Content-Type is
// application/json or when the body is valid JSON, and the raw string
// otherwise.
func parseResponseBody(raw []byte, contentType string) Value {
	if len(raw) == 0 {
		return ""
	}
	ct := strings.ToLower(contentType)
	isJSON := strings.Contains(ct, "application/json") || strings.Contains(ct, "+json")
	if isJSON {
		if v, err := decodeJSON(raw); err == nil {
			return v
		}
	}
	// Try JSON parse even without the content type (best-effort).
	if !isJSON && len(raw) > 0 && (raw[0] == '{' || raw[0] == '[' || raw[0] == '"' ||
		raw[0] == 't' || raw[0] == 'f' || raw[0] == 'n' || (raw[0] >= '0' && raw[0] <= '9') || raw[0] == '-') {
		if v, err := decodeJSON(raw); err == nil {
			return v
		}
	}
	return string(raw)
}

// headerGet looks up a header key case-insensitively in a *Map.
func headerGet(m *Map, key string) (Value, bool) {
	if m == nil {
		return nil, false
	}
	lower := strings.ToLower(key)
	for _, k := range m.Keys() {
		if strings.ToLower(k) == lower {
			v, _ := m.Get(k)
			return v, true
		}
	}
	return nil, false
}

// numFloat extracts a float64 from a Value, accepting int64, float64 and
// Decimal (via AsDecimal). Returns (float64, intValue, isInt, ok).
func numFloat(v Value) (f float64, i int64, isInt bool, ok bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), x, true, true
	case float64:
		return x, 0, false, true
	}
	// Try decimal (task 2.5 — may be a stub if 2.5 is not yet merged).
	if d, dOk := asDecimalFloat(v); dOk {
		return d, 0, false, true
	}
	return 0, 0, false, false
}

// asDecimalFloat tries to read a Decimal value as float64 without importing
// shopspring directly (the type is a type alias defined in decimal.go).
// If the value is not a Decimal this returns (0, false).
func asDecimalFloat(v Value) (float64, bool) {
	// Decimal = decimal.Decimal; it has a InexactFloat64() method.
	type floater interface{ InexactFloat64() float64 }
	if d, ok := v.(floater); ok {
		return d.InexactFloat64(), true
	}
	return 0, false
}

// isPlayground reports whether the program is running inside the hosted
// playground (security.md, Requirement 11.2).
func isPlayground() bool {
	return os.Getenv("RIP_PLAYGROUND") == "1"
}

// marshalFetchBody is used by tests to verify JSON encoding of request bodies.
// It delegates to the package-level marshalValue.
var marshalFetchBody = func(v Value) ([]byte, error) {
	raw, err := marshalValue(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}
