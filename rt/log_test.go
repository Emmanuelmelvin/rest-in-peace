package rt

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLog(t *testing.T) {
	tests := []struct {
		name       string
		ctx        *Ctx
		level      Value
		msg        Value
		fields     *Map
		wantFields map[string]any // fields that must appear in the JSON (checked in addition to level & msg)
	}{
		{
			name:   "DEBUG level",
			ctx:    NewCtx(nil),
			level:  "DEBUG",
			msg:    "debug message",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "DEBUG",
				"msg":   "debug message",
			},
		},
		{
			name:   "INFO level",
			ctx:    NewCtx(nil),
			level:  "info", // case-insensitive
			msg:    "info message",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "info message",
			},
		},
		{
			name:   "WARN level",
			ctx:    NewCtx(nil),
			level:  "WaRn", // mixed case
			msg:    "warning message",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "WARN",
				"msg":   "warning message",
			},
		},
		{
			name:   "ERROR level",
			ctx:    NewCtx(nil),
			level:  "ERROR",
			msg:    "error message",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "ERROR",
				"msg":   "error message",
			},
		},
		{
			name: "includes requestId when present",
			ctx: &Ctx{
				Req: &Request{ID: "req-123"},
			},
			level:  "INFO",
			msg:    "request log",
			fields: NewMap(),
			wantFields: map[string]any{
				"level":     "INFO",
				"msg":       "request log",
				"requestId": "req-123",
			},
		},
		{
			name:   "no requestId when ctx.Req is nil",
			ctx:    NewCtx(nil),
			level:  "INFO",
			msg:    "no request",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "no request",
			},
		},
		{
			name: "no requestId when ctx.Req.ID is empty",
			ctx: &Ctx{
				Req: &Request{ID: ""},
			},
			level:  "INFO",
			msg:    "empty request ID",
			fields: NewMap(),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "empty request ID",
			},
		},
		{
			name:  "includes additional fields",
			ctx:   NewCtx(nil),
			level: "INFO",
			msg:   "user action",
			fields: MapOf(
				"userId", int64(42),
				"action", "login",
			),
			wantFields: map[string]any{
				"level":  "INFO",
				"msg":    "user action",
				"userId": float64(42),
				"action": "login",
			},
		},
		{
			name:  "redacts top-level password",
			ctx:   NewCtx(nil),
			level: "INFO",
			msg:   "auth",
			fields: MapOf(
				"username", "alice",
				"password", "secret123",
			),
			wantFields: map[string]any{
				"level":    "INFO",
				"msg":      "auth",
				"username": "alice",
				"password": "[REDACTED]",
			},
		},
		{
			name:  "redacts various secret-looking keys (case-insensitive)",
			ctx:   NewCtx(nil),
			level: "INFO",
			msg:   "secrets",
			fields: MapOf(
				"apiKey", "sk_live_123",
				"TOKEN", "bearer xyz",
				"Authorization", "Basic abc",
				"client_secret", "s3cr3t",
			),
			wantFields: map[string]any{
				"level":         "INFO",
				"msg":           "secrets",
				"apiKey":        "[REDACTED]",
				"TOKEN":         "[REDACTED]",
				"Authorization": "[REDACTED]",
				"client_secret": "[REDACTED]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetLogOutput(&buf)
			defer SetLogOutput(os.Stdout)

			Log(tt.ctx, tt.level, tt.msg, tt.fields)

			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("failed to parse log JSON: %v\nOutput: %s", err, buf.String())
			}

			for k, want := range tt.wantFields {
				if gotV, ok := got[k]; !ok {
					t.Errorf("missing field %q in log output", k)
				} else if gotV != want {
					t.Errorf("field %q = %v, want %v", k, gotV, want)
				}
			}

			// time should always be present
			if _, ok := got["time"]; !ok {
				t.Error("missing 'time' field in log output")
			}

			// requestId should NOT be present when it's not in wantFields
			if _, wantReqID := tt.wantFields["requestId"]; !wantReqID {
				if _, gotReqID := got["requestId"]; gotReqID {
					t.Error("unexpected 'requestId' field in log output")
				}
			}
		})
	}
}

func TestLogInvalidLevel(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Log() with invalid level did not panic")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("panic was not an *Error: %T", r)
		}
		if e.Kind != KindTypeMismatch {
			t.Errorf("panic Kind = %v, want %v", e.Kind, KindTypeMismatch)
		}
		if !strings.Contains(e.Message, "DEBUG, INFO, WARN or ERROR") {
			t.Errorf("panic Message = %q, want mention of valid levels", e.Message)
		}
	}()

	var buf bytes.Buffer
	SetLogOutput(&buf)
	defer SetLogOutput(os.Stdout)

	Log(NewCtx(nil), "TRACE", "invalid level", NewMap())
}

func TestLogRedactionNested(t *testing.T) {
	tests := []struct {
		name       string
		fields     *Map
		wantFields map[string]any
	}{
		{
			name: "redacts nested Map with password",
			fields: MapOf(
				"user", MapOf(
					"name", "alice",
					"password", "secret123",
				),
			),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "nested",
				"user": map[string]any{
					"name":     "alice",
					"password": "[REDACTED]",
				},
			},
		},
		{
			name: "redacts deeply nested token",
			fields: MapOf(
				"auth", MapOf(
					"jwt", MapOf(
						"token", "xyz",
						"exp", int64(1234),
					),
				),
			),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "nested",
				"auth": map[string]any{
					"jwt": map[string]any{
						"token": "[REDACTED]",
						"exp":   float64(1234),
					},
				},
			},
		},
		{
			name: "redacts in Array of Maps",
			fields: MapOf(
				"users", ArrayOf(
					MapOf("name", "alice", "apiKey", "key1"),
					MapOf("name", "bob", "apiKey", "key2"),
				),
			),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "nested",
				"users": []any{
					map[string]any{"name": "alice", "apiKey": "[REDACTED]"},
					map[string]any{"name": "bob", "apiKey": "[REDACTED]"},
				},
			},
		},
		{
			name: "redacts Go map[string]any",
			fields: MapOf(
				"meta", map[string]any{
					"client_secret": "s3cr3t",
					"env":           "production",
				},
			),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "nested",
				"meta": map[string]any{
					"client_secret": "[REDACTED]",
					"env":           "production",
				},
			},
		},
		{
			name: "redacts in Go []any slice",
			fields: MapOf(
				"items", []any{
					map[string]any{"id": float64(1), "password": "pw1"},
					map[string]any{"id": float64(2), "password": "pw2"},
				},
			),
			wantFields: map[string]any{
				"level": "INFO",
				"msg":   "nested",
				"items": []any{
					map[string]any{"id": float64(1), "password": "[REDACTED]"},
					map[string]any{"id": float64(2), "password": "[REDACTED]"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetLogOutput(&buf)
			defer SetLogOutput(os.Stdout)

			Log(NewCtx(nil), "INFO", "nested", tt.fields)

			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("failed to parse log JSON: %v\nOutput: %s", err, buf.String())
			}

			for k, want := range tt.wantFields {
				gotJSON, _ := json.Marshal(got[k])
				wantJSON, _ := json.Marshal(want)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("field %q mismatch:\ngot:  %s\nwant: %s", k, gotJSON, wantJSON)
				}
			}
		})
	}
}

func TestLogDecimalRendering(t *testing.T) {
	var buf bytes.Buffer
	SetLogOutput(&buf)
	defer SetLogOutput(os.Stdout)

	fields := MapOf(
		"price", DecimalOf("19.99"),
		"total", DecimalOf("215.00"),
	)

	Log(NewCtx(nil), "INFO", "pricing", fields)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	// DECIMAL values should be quoted strings preserving scale
	if got["price"] != "19.99" {
		t.Errorf("price = %v, want \"19.99\"", got["price"])
	}
	if got["total"] != "215.00" {
		t.Errorf("total = %v, want \"215.00\"", got["total"])
	}
}

func TestLogArrayAndMapRendering(t *testing.T) {
	var buf bytes.Buffer
	SetLogOutput(&buf)
	defer SetLogOutput(os.Stdout)

	fields := MapOf(
		"tags", ArrayOf("foo", "bar"),
		"meta", MapOf("key", "value", "count", int64(3)),
	)

	Log(NewCtx(nil), "INFO", "structured", fields)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	// tags should be a JSON array
	tags, ok := got["tags"].([]any)
	if !ok {
		t.Fatalf("tags is not a []any: %T", got["tags"])
	}
	if len(tags) != 2 || tags[0] != "foo" || tags[1] != "bar" {
		t.Errorf("tags = %v, want [foo bar]", tags)
	}

	// meta should be a JSON object
	meta, ok := got["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta is not a map[string]any: %T", got["meta"])
	}
	if meta["key"] != "value" || meta["count"] != float64(3) {
		t.Errorf("meta = %v, want {key:value count:3}", meta)
	}
}

func TestLogNilFields(t *testing.T) {
	var buf bytes.Buffer
	SetLogOutput(&buf)
	defer SetLogOutput(os.Stdout)

	// Log should not panic with nil fields (empty map case)
	Log(NewCtx(nil), "INFO", "empty fields", NewMap())

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	if got["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got["level"])
	}
	if got["msg"] != "empty fields" {
		t.Errorf("msg = %v, want \"empty fields\"", got["msg"])
	}
}

func TestLogConcurrentSetOutput(t *testing.T) {
	// Smoke test: SetLogOutput is safe for concurrent use
	done := make(chan bool, 2)
	go func() {
		for i := 0; i < 100; i++ {
			SetLogOutput(os.Stdout)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			var buf bytes.Buffer
			SetLogOutput(&buf)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()
	<-done
	<-done
	SetLogOutput(os.Stdout) // restore
}
