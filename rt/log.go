package rt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

// secretKeyRe matches map keys that name secrets (Requirement 10.5, security.md).
// Case-insensitive; a key matches when any of these words appear in it.
var secretKeyRe = regexp.MustCompile(`(?i)(password|token|apikey|authorization|secret)`)

// logMu guards logOutput so SetLogOutput is safe for concurrent tests.
var logMu sync.RWMutex

// logOutput is the writer used by the JSON handler. Defaults to os.Stdout.
// Swap it in tests via SetLogOutput; restore with SetLogOutput(os.Stdout).
var logOutput io.Writer = os.Stdout

// SetLogOutput redirects all subsequent Log calls to w. Pass os.Stdout to
// restore the default. This is safe for concurrent use.
func SetLogOutput(w io.Writer) {
	logMu.Lock()
	logOutput = w
	logMu.Unlock()
}

// newHandler builds a JSON slog.Handler from the current logOutput.
func newHandler() slog.Handler {
	logMu.RLock()
	w := logOutput
	logMu.RUnlock()
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
}

// Log implements LOG level, message [, fields] (Requirement 10.3).
//
// Writes one JSON line with keys: time, level, msg, requestId (when available)
// and any extra fields from the fields map. Secret-looking keys (see
// secretKeyRe) are replaced with "[REDACTED]" at every nesting depth
// (Requirement 10.5). *Map, *Array and Decimal values render as clean JSON.
func Log(c *Ctx, level Value, msg Value, fields *Map) {
	lvl, ok := parseLevel(level)
	if !ok {
		panic(Errorf(KindTypeMismatch,
			"LOG level must be DEBUG, INFO, WARN or ERROR, got %q", ToString(level)))
	}

	ctx := context.Background()
	attrs := make([]slog.Attr, 0, 8)

	if c != nil {
		ctx = c.Context()
		if c.Req != nil && c.Req.ID != "" {
			attrs = append(attrs, slog.String("requestId", c.Req.ID))
		}
	}

	for _, k := range fields.Keys() {
		v, _ := fields.Get(k)
		attrs = append(attrs, slog.Any(k, redactAndWrap(k, v)))
	}

	slog.New(newHandler()).LogAttrs(ctx, lvl, ToString(msg), attrs...)
}

// parseLevel converts a BASIC log level string to slog.Level.
// Returns ok=false for unknown values so the caller can panic with a typed error.
func parseLevel(level Value) (slog.Level, bool) {
	switch strings.ToUpper(ToString(level)) {
	case "DEBUG":
		return slog.LevelDebug, true
	case "INFO":
		return slog.LevelInfo, true
	case "WARN":
		return slog.LevelWarn, true
	case "ERROR":
		return slog.LevelError, true
	}
	return 0, false
}

// redactAndWrap redacts v when k looks like a secret key, and wraps *Map,
// *Array and Decimal values in json.RawMessage so slog emits them as JSON
// objects/arrays/strings rather than Go struct internals.
func redactAndWrap(k string, v Value) any {
	if secretKeyRe.MatchString(k) {
		return "[REDACTED]"
	}
	return wrapForLog(v)
}

// wrapForLog converts a BASIC value to something slog will render cleanly.
//
//   - *Map   → json.RawMessage (preserves insertion order and applies recursive redaction)
//   - *Array → json.RawMessage (applies recursive redaction to each element)
//   - Decimal → json.RawMessage (quoted scale-preserving string, e.g. "19.99")
//   - map[string]any → json.RawMessage (recursive redaction)
//   - []any / []Value → json.RawMessage (recursive redaction)
//   - everything else → passes through to slog unchanged
func wrapForLog(v Value) any {
	switch x := v.(type) {
	case *Map:
		return json.RawMessage(marshalMapRedacted(x))
	case *Array:
		return json.RawMessage(marshalArrayRedacted(x))
	case map[string]any:
		return json.RawMessage(marshalGoMapRedacted(x))
	case []any:
		return json.RawMessage(marshalSliceRedacted(x))
	default:
		if b, ok := decimalJSON(v); ok {
			return json.RawMessage(b)
		}
		return x
	}
}

// marshalMapRedacted encodes a *Map as JSON, redacting secret keys and
// recursively wrapping nested values.
func marshalMapRedacted(m *Map) []byte {
	if m == nil {
		return []byte("null")
	}
	var buf []byte
	buf = append(buf, '{')
	for i, k := range m.Keys() {
		if i > 0 {
			buf = append(buf, ',')
		}
		kb, _ := json.Marshal(k)
		buf = append(buf, kb...)
		buf = append(buf, ':')
		raw := marshalValueRedacted(k, func() Value { v, _ := m.Get(k); return v }())
		buf = append(buf, raw...)
	}
	buf = append(buf, '}')
	return buf
}

// marshalArrayRedacted encodes a *Array as JSON with recursive redaction.
// Keys are unavailable at this level, so values are only redacted when they
// are themselves maps or arrays whose keys match.
func marshalArrayRedacted(a *Array) []byte {
	if a == nil {
		return []byte("null")
	}
	buf := []byte{'['}
	for i, item := range a.Items {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, marshalValueRedacted("", item)...)
	}
	buf = append(buf, ']')
	return buf
}

// marshalGoMapRedacted encodes a plain Go map[string]any with redaction.
func marshalGoMapRedacted(m map[string]any) []byte {
	if m == nil {
		return []byte("null")
	}
	buf := []byte{'{'}
	first := true
	for k, v := range m {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf = append(buf, kb...)
		buf = append(buf, ':')
		buf = append(buf, marshalValueRedacted(k, v)...)
	}
	buf = append(buf, '}')
	return buf
}

// marshalSliceRedacted encodes a []any with recursive redaction.
func marshalSliceRedacted(s []any) []byte {
	buf := []byte{'['}
	for i, item := range s {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, marshalValueRedacted("", item)...)
	}
	buf = append(buf, ']')
	return buf
}

// marshalValueRedacted encodes a single value as JSON, redacting if the
// associated key is secret-looking, and recursing into maps/arrays.
func marshalValueRedacted(k string, v Value) []byte {
	if k != "" && secretKeyRe.MatchString(k) {
		b, _ := json.Marshal("[REDACTED]")
		return b
	}
	switch x := v.(type) {
	case *Map:
		return marshalMapRedacted(x)
	case *Array:
		return marshalArrayRedacted(x)
	case map[string]any:
		return marshalGoMapRedacted(x)
	case []any:
		return marshalSliceRedacted(x)
	default:
		if b, ok := decimalJSON(v); ok {
			return b
		}
		b, err := json.Marshal(v)
		if err != nil {
			b, _ = json.Marshal(fmt.Sprintf("%v", v))
		}
		return b
	}
}
