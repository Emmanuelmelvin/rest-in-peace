package rt

import (
	"math"
	"strings"
	"time"
)

var defaultDateLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Now returns the current UTC timestamp formatted as RFC3339.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// UnixTime returns the current Unix timestamp in seconds.
func UnixTime() int64 {
	return time.Now().Unix()
}

// UnixMs returns the current Unix timestamp in milliseconds.
func UnixMs() int64 {
	return time.Now().UnixMilli()
}

// FormatDate formats a Unix timestamp (int64/float64 seconds), RFC3339 string,
// or time.Time in UTC using layout. Preset names "ISO", "DATE", and "TIME" map
// to RFC3339, "2006-01-02", and "15:04:05" respectively; any other string is
// used directly as a Go time layout.
func FormatDate(ts Value, layout string) string {
	var t time.Time
	switch x := ts.(type) {
	case int64:
		t = time.Unix(x, 0).UTC()
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			panic(Errorf(KindTypeMismatch, "FORMATDATE needs a finite NUMBER, got %v", x))
		}
		sec, frac := math.Modf(x)
		t = time.Unix(int64(sec), int64(math.Round(frac*1e9))).UTC()
	case string:
		parsed, err := time.Parse(time.RFC3339, x)
		if err != nil {
			panic(Errorf(KindTypeMismatch, "FORMATDATE could not parse %q as RFC3339: %v", x, err))
		}
		t = parsed.UTC()
	case time.Time:
		t = x.UTC()
	default:
		panic(Errorf(KindTypeMismatch, "FORMATDATE needs a NUMBER or RFC3339 STRING, got %s", TypeName(ts)))
	}
	return t.Format(resolveDateLayout(layout))
}

// ParseDate parses a date string into Unix seconds (int64). When layout is
// omitted, RFC3339, RFC3339Nano, "2006-01-02 15:04:05", and "2006-01-02" are
// tried in order. Preset names ("ISO", "DATE", "TIME") are also accepted.
func ParseDate(s string, layout ...Value) int64 {
	if len(layout) > 0 && layout[0] != nil {
		ls, ok := layout[0].(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "PARSEDATE layout must be a STRING, got %s", TypeName(layout[0])))
		}
		t, err := time.Parse(resolveDateLayout(ls), s)
		if err != nil {
			panic(Errorf(KindTypeMismatch, "PARSEDATE could not parse %q with layout %q: %v", s, ls, err))
		}
		return t.Unix()
	}
	for _, l := range defaultDateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Unix()
		}
	}
	panic(Errorf(KindTypeMismatch, "PARSEDATE could not parse %q", s))
}

func resolveDateLayout(layout string) string {
	switch strings.ToUpper(layout) {
	case "ISO":
		return time.RFC3339
	case "DATE":
		return "2006-01-02"
	case "TIME":
		return "15:04:05"
	default:
		return layout
	}
}
