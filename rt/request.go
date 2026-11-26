package rt

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Request is the parsed HTTP request (Requirement 7.2).
type Request struct {
	ID         string
	Method     string
	Path       string
	RemoteAddr string // client host:port from net/http, used by RATELIMIT
	Params     *Map
	Query      *Map
	Headers    *Map  // lowercase keys
	Body       Value // parsed JSON; an empty map when there is no body
	Ctx        *Map  // written by middleware
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
	return current(c, "PARAM").Params.getOrNil(ToString(name))
}

// Query implements QUERY("name"): a query-string value.
func Query(c *Ctx, name Value) Value {
	return current(c, "QUERY").Query.getOrNil(ToString(name))
}

// Header implements HEADER("name"): a request header, matched case-insensitively.
func Header(c *Ctx, name Value) Value {
	return current(c, "HEADER").Headers.getOrNil(strings.ToLower(ToString(name)))
}

// Body implements BODY(): the parsed JSON body, {} when there is none.
func Body(c *Ctx) Value {
	return current(c, "BODY").Body
}

func current(c *Ctx, fn string) *Request {
	if c == nil || c.Req == nil {
		panic(Errorf(KindUndefined, "%s() only works while a ROUTE … GOSUB handler is answering a request", fn))
	}
	return c.Req
}

func (m *Map) getOrNil(key string) Value {
	v, _ := m.Get(key)
	return v
}

// parseQuery parses a query string into a map in the order the keys appear.
// When a key repeats, the first value wins.
func parseQuery(raw string) *Map {
	m := NewMap()
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		key, err1 := url.QueryUnescape(k)
		val, err2 := url.QueryUnescape(v)
		if err1 != nil || err2 != nil {
			continue
		}
		if _, ok := m.Get(key); !ok {
			m.Set(key, val)
		}
	}
	return m
}

func sortedHeaderKeys(h http.Header) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// decodeJSON parses one JSON value into BASIC values: objects become *Map
// (keeping key order), arrays *Array, whole numbers int64, other numbers
// float64.
func decodeJSON(data []byte) (Value, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := decodeValue(d)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON value")
	}
	return v, nil
}

func decodeValue(d *json.Decoder) (Value, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			m := NewMap()
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, errors.New("object key is not a string")
				}
				v, err := decodeValue(d)
				if err != nil {
					return nil, err
				}
				m.Set(k, v)
			}
			if _, err := d.Token(); err != nil { // '}'
				return nil, err
			}
			return m, nil
		case '[':
			a := ArrayOf()
			for d.More() {
				v, err := decodeValue(d)
				if err != nil {
					return nil, err
				}
				a.Items = append(a.Items, v)
			}
			if _, err := d.Token(); err != nil { // ']'
				return nil, err
			}
			return a, nil
		}
		return nil, errors.New("unexpected delimiter")
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return n, nil
		}
		return t.Float64()
	default: // string, bool, nil
		return t, nil
	}
}
