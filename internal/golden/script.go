package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Wildcards for expected values in a .http script. In an expected JSON body
// they stand in for a whole value; as an expected header value, for the
// whole header value.
const (
	AnyValue  = "<any>"        // any value, as long as the key or header is present
	AnyString = "<any string>" // any non-empty string
)

// Header is one header line in a .http script.
type Header struct {
	Name, Value string
}

// Request is the request half of an Exchange.
type Request struct {
	Method  string
	Target  string // path and query, sent as written: "/search?q=ghost"
	Headers []Header
	Body    string // sent as written; empty means no body
}

// Response is the expected half of an Exchange.
type Response struct {
	Status  int
	Headers []Header // only these headers are checked
	Body    string   // compared as JSON when it parses as JSON, else as text
}

// Exchange is one request in a .http script and the response it expects.
type Exchange struct {
	Name string // the text after "###", may be empty
	Line int    // line of the request line, for messages
	Req  Request
	Want Response
}

// ParseScript parses a .http script (the format is described in the package
// comment). name is used in error messages.
func ParseScript(name, src string) ([]Exchange, error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	p := &scriptParser{name: name, lines: lines}
	var exs []Exchange
	// Everything before the first ### line is a comment.
	for p.i < len(lines) && !isExchangeStart(lines[p.i]) {
		p.i++
	}
	for p.i < len(lines) {
		ex, err := p.exchange()
		if err != nil {
			return nil, err
		}
		exs = append(exs, ex)
	}
	if len(exs) == 0 {
		return nil, fmt.Errorf("%s: no exchanges; each one starts with a ### line", name)
	}
	return exs, nil
}

type scriptParser struct {
	name  string
	lines []string
	i     int // next line, 0-based
}

func (p *scriptParser) errorf(line int, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", p.name, line, fmt.Sprintf(format, args...))
}

func isExchangeStart(l string) bool { return strings.HasPrefix(l, "###") }
func isStatusLine(l string) bool    { return strings.HasPrefix(l, "=>") }

func (p *scriptParser) exchange() (Exchange, error) {
	ex := Exchange{Name: strings.TrimSpace(strings.TrimPrefix(p.lines[p.i], "###"))}
	p.i++

	// Skip blank lines and comments up to the request line.
	for p.i < len(p.lines) {
		l := strings.TrimSpace(p.lines[p.i])
		if isExchangeStart(p.lines[p.i]) || l != "" && !strings.HasPrefix(l, "#") {
			break
		}
		p.i++
	}
	if p.i >= len(p.lines) || isExchangeStart(p.lines[p.i]) {
		return ex, p.errorf(p.i, "exchange %q has no request line like GET /path", ex.Name)
	}
	ex.Line = p.i + 1
	fields := strings.Fields(p.lines[p.i])
	if len(fields) != 2 || !isToken(fields[0]) || fields[0] != strings.ToUpper(fields[0]) || !strings.HasPrefix(fields[1], "/") {
		return ex, p.errorf(ex.Line, "want a request line like GET /path, got %q", p.lines[p.i])
	}
	ex.Req.Method, ex.Req.Target = fields[0], fields[1]
	p.i++

	var err error
	if ex.Req.Headers, err = p.headers(isStatusLine); err != nil {
		return ex, err
	}
	ex.Req.Body = p.body(isStatusLine)
	if p.i >= len(p.lines) || !isStatusLine(p.lines[p.i]) {
		return ex, p.errorf(ex.Line, "%s %s has no expected response; add a line like => 200", ex.Req.Method, ex.Req.Target)
	}

	statusText := strings.TrimSpace(strings.TrimPrefix(p.lines[p.i], "=>"))
	status, err := strconv.Atoi(statusText)
	if err != nil || status < 100 || status > 599 {
		return ex, p.errorf(p.i+1, "want => followed by a status code like 200, got %q", p.lines[p.i])
	}
	ex.Want.Status = status
	p.i++
	if ex.Want.Headers, err = p.headers(isExchangeStart); err != nil {
		return ex, err
	}
	ex.Want.Body = p.body(isExchangeStart)
	return ex, nil
}

// headers reads Name: value lines up to a blank line (which it consumes), a
// line for which stop is true, or the end of the script.
func (p *scriptParser) headers(stop func(string) bool) ([]Header, error) {
	var hs []Header
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if stop(l) {
			break
		}
		p.i++
		if strings.TrimSpace(l) == "" {
			break
		}
		name, value, ok := strings.Cut(l, ":")
		if !ok || !isToken(name) {
			return nil, p.errorf(p.i, "want a header line like Name: value, or a blank line before the body, got %q", l)
		}
		hs = append(hs, Header{Name: name, Value: strings.TrimSpace(value)})
	}
	return hs, nil
}

// body reads lines up to one for which stop is true, or the end of the
// script, and trims blank lines from both ends.
func (p *scriptParser) body(stop func(string) bool) string {
	start := p.i
	for p.i < len(p.lines) && !stop(p.lines[p.i]) {
		p.i++
	}
	return strings.Trim(strings.Join(p.lines[start:p.i], "\n"), "\n \t")
}

// isToken reports whether s is a valid HTTP method or header name.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r > 0x7e || r <= ' ' || strings.ContainsRune("\"(),/:;<=>?@[\\]{}", r) {
			return false
		}
	}
	return true
}

// Got is the response a program actually sent.
type Got struct {
	Status int
	Header http.Header
	Body   string
}

// MatchResponse compares got with want: the status, each listed header
// (case-insensitively by name, exactly by value unless it is a wildcard), and
// the body. A want body that parses as JSON is compared structurally with
// MatchJSON; otherwise the bodies are compared as text after Normalize.
func MatchResponse(got Got, want Response) error {
	var problems []string
	if got.Status != want.Status {
		problems = append(problems, fmt.Sprintf("status is %d, want %d", got.Status, want.Status))
	}
	for _, h := range want.Headers {
		vals, present := got.Header[http.CanonicalHeaderKey(h.Name)]
		v := strings.Join(vals, ", ")
		if !present {
			problems = append(problems, fmt.Sprintf("header %s is missing", h.Name))
		} else if !matchString(v, h.Value) {
			problems = append(problems, fmt.Sprintf("header %s is %q, want %q", h.Name, v, h.Value))
		}
	}
	if err := matchBody(got.Body, want.Body); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func matchString(got, want string) bool {
	switch want {
	case AnyValue:
		return true
	case AnyString:
		return got != ""
	}
	return got == want
}

func matchBody(got, want string) error {
	if strings.TrimSpace(want) == AnyValue {
		return nil
	}
	w, err := ParseJSON(want)
	if err != nil || strings.TrimSpace(want) == "" {
		if Normalize(got) != Normalize(want) {
			return fmt.Errorf("body differs")
		}
		return nil
	}
	g, err := ParseJSON(got)
	if err != nil {
		return fmt.Errorf("body is not JSON (%v)", err)
	}
	if err := MatchJSON(g, w); err != nil {
		return fmt.Errorf("body: %v", err)
	}
	return nil
}

// Object is a JSON object with its keys in document order.
type Object []Member

// Member is one key of an Object.
type Member struct {
	Key   string
	Value any
}

// ParseJSON parses exactly one JSON value, keeping object keys in order.
// Objects become Object, arrays []any, numbers json.Number, and strings,
// booleans and null their usual Go values.
func ParseJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("more than one JSON value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		obj := Object{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			obj = append(obj, Member{Key: kt.(string), Value: v})
		}
		_, err := dec.Token() // }
		return obj, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token() // ]
		return arr, err
	}
	return tok, nil
}

// MatchJSON compares two values from ParseJSON. Object keys must match in
// order, because maps keep insertion order (Requirement 3.5). Numbers are
// compared by value, so 1.0 matches 1. A want string AnyValue matches any
// value, and AnyString any non-empty string.
func MatchJSON(got, want any) error {
	return matchJSON(got, want, "$")
}

func matchJSON(got, want any, path string) error {
	if s, ok := want.(string); ok {
		switch s {
		case AnyValue:
			return nil
		case AnyString:
			if g, ok := got.(string); ok && g != "" {
				return nil
			}
			return fmt.Errorf("%s is %s, want a non-empty string", path, compactJSON(got))
		}
	}
	mismatch := func() error {
		return fmt.Errorf("%s is %s, want %s", path, compactJSON(got), compactJSON(want))
	}
	switch w := want.(type) {
	case Object:
		g, ok := got.(Object)
		if !ok {
			return mismatch()
		}
		if gk, wk := keys(g), keys(w); strings.Join(gk, "\x00") != strings.Join(wk, "\x00") {
			if sorted(gk) == sorted(wk) {
				return fmt.Errorf("%s has keys in order %q, want %q", path, gk, wk)
			}
			return fmt.Errorf("%s has keys %q, want %q", path, gk, wk)
		}
		for i := range w {
			if err := matchJSON(g[i].Value, w[i].Value, path+"."+w[i].Key); err != nil {
				return err
			}
		}
		return nil
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return mismatch()
		}
		for i := range w {
			if err := matchJSON(g[i], w[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case json.Number:
		g, ok := got.(json.Number)
		if !ok {
			return mismatch()
		}
		gr, gok := new(big.Rat).SetString(string(g))
		wr, wok := new(big.Rat).SetString(string(w))
		if !gok || !wok || gr.Cmp(wr) != 0 {
			return mismatch()
		}
		return nil
	}
	if got != want {
		return mismatch()
	}
	return nil
}

func keys(o Object) []string {
	ks := make([]string, len(o))
	for i, m := range o {
		ks[i] = m.Key
	}
	return ks
}

func sorted(ks []string) string {
	c := append([]string(nil), ks...)
	sort.Strings(c)
	return strings.Join(c, "\x00")
}

// compactJSON formats a ParseJSON value back as compact JSON, for messages.
func compactJSON(v any) string {
	var b bytes.Buffer
	writeJSON(&b, v)
	return b.String()
}

func writeJSON(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case Object:
		b.WriteByte('{')
		for i, m := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			k, _ := json.Marshal(m.Key)
			b.Write(k)
			b.WriteByte(':')
			writeJSON(b, m.Value)
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSON(b, e)
		}
		b.WriteByte(']')
	case json.Number:
		b.WriteString(string(x))
	default:
		j, _ := json.Marshal(x)
		b.Write(j)
	}
}
