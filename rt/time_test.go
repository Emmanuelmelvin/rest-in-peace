package rt

import (
	"math"
	"testing"
	"time"
)

func TestTimeNowAndUnix(t *testing.T) {
	before := time.Now().UTC().Truncate(time.Second)
	nowStr := Now()
	after := time.Now().UTC().Add(time.Second)

	parsed, err := time.Parse(time.RFC3339, nowStr)
	if err != nil {
		t.Fatalf("Now() = %q is not valid RFC3339: %v", nowStr, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("Now() = %v outside [%v, %v]", parsed, before, after)
	}

	sec := UnixTime()
	ms := UnixMs()
	if sec < before.Unix() || sec > after.Unix() {
		t.Errorf("UnixTime() = %d outside [%d, %d]", sec, before.Unix(), after.Unix())
	}
	if ms < sec*1000-1000 || ms > (sec+2)*1000 {
		t.Errorf("UnixMs() = %d inconsistent with UnixTime() = %d", ms, sec)
	}
}

func TestDateFormat(t *testing.T) {
	const ts int64 = 1700000000 // 2023-11-14T22:13:20Z

	tests := []struct {
		name   string
		ts     Value
		layout string
		want   string
	}{
		{"int64 ISO", ts, "ISO", "2023-11-14T22:13:20Z"},
		{"int64 iso lower", ts, "iso", "2023-11-14T22:13:20Z"},
		{"int64 DATE", ts, "DATE", "2023-11-14"},
		{"int64 TIME", ts, "TIME", "22:13:20"},
		{"int64 custom", ts, "02 Jan 2006 15:04", "14 Nov 2023 22:13"},
		{"float64 DATE", float64(ts), "DATE", "2023-11-14"},
		{"RFC3339 UTC string", "2023-11-14T22:13:20Z", "DATE", "2023-11-14"},
		{"RFC3339 offset string converted to UTC", "2023-11-15T00:13:20+02:00", "ISO", "2023-11-14T22:13:20Z"},
		{"time.Time value", time.Unix(ts, 0), "TIME", "22:13:20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDate(tt.ts, tt.layout)
			if got != tt.want {
				t.Errorf("FormatDate(%v, %q) = %q, want %q", tt.ts, tt.layout, got, tt.want)
			}
		})
	}

	errCases := map[string]func(){
		"bad string": func() { FormatDate("not-a-date", "ISO") },
		"bool":       func() { FormatDate(true, "ISO") },
		"nil":        func() { FormatDate(nil, "ISO") },
		"NaN":        func() { FormatDate(math.NaN(), "ISO") },
		"Inf":        func() { FormatDate(math.Inf(1), "ISO") },
	}
	for name, fn := range errCases {
		if k := panicKind(fn); k != KindTypeMismatch {
			t.Errorf("FormatDate %s: got %q, want %q", name, k, KindTypeMismatch)
		}
	}
}

func TestDateParse(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		layout []Value
		want   int64
	}{
		{"RFC3339", "2023-11-14T22:13:20Z", nil, 1700000000},
		{"RFC3339Nano", "2023-11-14T22:13:20.123456789Z", nil, 1700000000},
		{"RFC3339 with offset", "2023-11-15T00:13:20+02:00", nil, 1700000000},
		{"space datetime", "2023-11-14 22:13:20", nil, 1700000000},
		{"date only", "2023-11-14", nil, time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC).Unix()},
		{"explicit layout", "14/11/2023", []Value{"02/01/2006"}, time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC).Unix()},
		{"preset DATE", "2023-11-14", []Value{"DATE"}, time.Date(2023, 11, 14, 0, 0, 0, 0, time.UTC).Unix()},
		{"nil layout falls back", "2023-11-14T22:13:20Z", []Value{nil}, 1700000000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDate(tt.s, tt.layout...)
			if got != tt.want {
				t.Errorf("ParseDate(%q, %v) = %d, want %d", tt.s, tt.layout, got, tt.want)
			}
		})
	}

	errCases := map[string]func(){
		"unparseable default": func() { ParseDate("not-a-date") },
		"mismatch layout":     func() { ParseDate("2023-11-14", "02/01/2006") },
		"non-string layout":   func() { ParseDate("2023-11-14", int64(123)) },
	}
	for name, fn := range errCases {
		if k := panicKind(fn); k != KindTypeMismatch {
			t.Errorf("ParseDate %s: got %q, want %q", name, k, KindTypeMismatch)
		}
	}
}
