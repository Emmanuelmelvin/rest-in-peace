package rt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// catchFetch calls Fetch and returns the result, or the *Error it panicked
// with. Any other panic escapes normally (test infrastructure failure).
func catchFetch(c *Ctx, url string, opts ...Value) (result *Map, err *Error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	result = Fetch(c, url, opts...)
	return
}

func newCtx() *Ctx { return NewCtx(context.Background()) }

// --------------------------------------------------------------------------
// GET JSON
// --------------------------------------------------------------------------

func TestFetchGETJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":1,"name":"Ada"}`))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status, _ := res.Get("status"); status != int64(200) {
		t.Errorf("status = %v, want 200", status)
	}
	body, _ := res.Get("body")
	m, ok := body.(*Map)
	if !ok {
		t.Fatalf("body is %T, want *Map", body)
	}
	if v, _ := m.Get("name"); v != "Ada" {
		t.Errorf("body.name = %v, want Ada", v)
	}
	if v, _ := m.Get("id"); v != int64(1) {
		t.Errorf("body.id = %v, want 1", v)
	}
}

// --------------------------------------------------------------------------
// GET plain text
// --------------------------------------------------------------------------

func TestFetchGETPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := res.Get("body")
	if body != "hello world" {
		t.Errorf("body = %q, want %q", body, "hello world")
	}
}

// --------------------------------------------------------------------------
// POST with map body — JSON-encoded, Content-Type set automatically
// --------------------------------------------------------------------------

func TestFetchPOSTMapBody(t *testing.T) {
	var gotMethod, gotContentType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(`{"created":true}`))
	}))
	defer srv.Close()

	payload := MapOf("email", "ada@example.com", "amount", int64(100))
	opts := MapOf("method", "POST", "body", payload)
	res, err := catchFetch(newCtx(), srv.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("server got method %s, want POST", gotMethod)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", gotContentType)
	}

	var decoded map[string]any
	if err2 := json.Unmarshal([]byte(gotBody), &decoded); err2 != nil {
		t.Fatalf("body is not JSON: %s — %v", gotBody, err2)
	}
	if decoded["email"] != "ada@example.com" {
		t.Errorf("decoded.email = %v", decoded["email"])
	}

	if status, _ := res.Get("status"); status != int64(201) {
		t.Errorf("status = %v, want 201", status)
	}
}

// --------------------------------------------------------------------------
// POST with string body — sent as-is, Content-Type not auto-set
// --------------------------------------------------------------------------

func TestFetchPOSTStringBody(t *testing.T) {
	var gotContentType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	opts := MapOf(
		"method", "POST",
		"body", `raw payload`,
		"headers", MapOf("Content-Type", "text/plain"),
	)
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody != "raw payload" {
		t.Errorf("body = %q, want %q", gotBody, "raw payload")
	}
	if gotContentType != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", gotContentType)
	}
}

// --------------------------------------------------------------------------
// Custom response headers — stored with lowercase keys
// --------------------------------------------------------------------------

func TestFetchResponseHeadersAreLowercased(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "abc-123")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	headers, _ := res.Get("headers")
	hm, ok := headers.(*Map)
	if !ok {
		t.Fatalf("headers is %T, want *Map", headers)
	}
	v, exists := hm.Get("x-request-id")
	if !exists || v != "abc-123" {
		t.Errorf("headers[x-request-id] = %v (exists=%v), want abc-123", v, exists)
	}
	if _, exists := hm.Get("X-Request-ID"); exists {
		t.Errorf("headers should only have lowercase keys")
	}
}

// --------------------------------------------------------------------------
// Custom request headers forwarded to server
// --------------------------------------------------------------------------

func TestFetchCustomRequestHeaders(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	opts := MapOf("headers", MapOf("Authorization", "Bearer tok"))
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok")
	}
}

// --------------------------------------------------------------------------
// Non-200 status codes are returned, not treated as errors
// --------------------------------------------------------------------------

func TestFetchNon200StatusReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"NOT FOUND"}`))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status, _ := res.Get("status"); status != int64(404) {
		t.Errorf("status = %v, want 404", status)
	}
}

// --------------------------------------------------------------------------
// Timeout fires as a catchable *Error with KindHTTP
// --------------------------------------------------------------------------

func TestFetchTimeoutFiresAsCatchableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the FETCH timeout.
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	opts := MapOf("timeout", 0.05) // 50 ms
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if err.Kind != KindHTTP {
		t.Errorf("error kind = %q, want HTTP", err.Kind)
	}
	if !strings.Contains(strings.ToUpper(err.Message), "TIMED OUT") {
		t.Errorf("error message %q should mention timeout", err.Message)
	}
}

// --------------------------------------------------------------------------
// Timeout <= 0 raises ?TYPE MISMATCH ERROR at call time (not network error)
// --------------------------------------------------------------------------

func TestFetchZeroTimeoutIsTypeMismatch(t *testing.T) {
	for _, bad := range []Value{float64(0), float64(-1), int64(0), int64(-5)} {
		opts := MapOf("timeout", bad)
		_, err := catchFetch(newCtx(), "http://127.0.0.1:0", opts)
		if err == nil || err.Kind != KindTypeMismatch {
			t.Errorf("timeout=%v: got kind=%q, want TYPE MISMATCH", bad, err.Kind)
		}
	}
}

// --------------------------------------------------------------------------
// Invalid timeout type raises ?TYPE MISMATCH ERROR
// --------------------------------------------------------------------------

func TestFetchInvalidTimeoutType(t *testing.T) {
	for _, bad := range []Value{"10s", true, MapOf()} {
		opts := MapOf("timeout", bad)
		_, err := catchFetch(newCtx(), "http://127.0.0.1:0", opts)
		if err == nil || err.Kind != KindTypeMismatch {
			t.Errorf("timeout=%v (%T): got kind=%q, want TYPE MISMATCH", bad, bad, err.Kind)
		}
	}
}

// --------------------------------------------------------------------------
// Invalid URL raises ?HTTP ERROR
// --------------------------------------------------------------------------

func TestFetchInvalidURL(t *testing.T) {
	_, err := catchFetch(newCtx(), "not-a-url://%%%")
	if err == nil {
		t.Fatal("expected an error for invalid URL, got nil")
	}
	if err.Kind != KindHTTP {
		t.Errorf("error kind = %q, want HTTP", err.Kind)
	}
}

// --------------------------------------------------------------------------
// Unreachable host raises ?HTTP ERROR (not a panic string)
// --------------------------------------------------------------------------

func TestFetchUnreachableHostRaisesHTTPError(t *testing.T) {
	// Port 0 on 127.0.0.1 — nothing listens there.
	_, err := catchFetch(newCtx(), "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected an error for unreachable host, got nil")
	}
	if err.Kind != KindHTTP {
		t.Errorf("error kind = %q, want HTTP", err.Kind)
	}
}

// --------------------------------------------------------------------------
// Invalid opts type (non-*Map) raises ?TYPE MISMATCH ERROR
// --------------------------------------------------------------------------

func TestFetchInvalidOptsType(t *testing.T) {
	for _, bad := range []Value{"options", int64(1), true, ArrayOf()} {
		_, err := catchFetch(newCtx(), "http://127.0.0.1:0", bad)
		if err == nil || err.Kind != KindTypeMismatch {
			t.Errorf("opts=%v (%T): got kind=%q, want TYPE MISMATCH", bad, bad, err.Kind)
		}
	}
}

// --------------------------------------------------------------------------
// Context cancellation propagates as a catchable *Error
// --------------------------------------------------------------------------

func TestFetchContextCancellationRaisesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := NewCtx(ctx)

	// Cancel the context immediately in a goroutine after the request starts.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := catchFetch(c, srv.URL)
	if err == nil {
		t.Fatal("expected an error after context cancellation, got nil")
	}
	if err.Kind != KindHTTP {
		t.Errorf("error kind = %q, want HTTP", err.Kind)
	}
}

// --------------------------------------------------------------------------
// nil opts is allowed (treated as no options)
// --------------------------------------------------------------------------

func TestFetchNilOptsIsAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL, nil)
	if err != nil {
		t.Fatalf("nil opts raised error: %v", err)
	}
	if status, _ := res.Get("status"); status != int64(200) {
		t.Errorf("status = %v, want 200", status)
	}
}

// --------------------------------------------------------------------------
// PUT / PATCH / DELETE methods are forwarded correctly
// --------------------------------------------------------------------------

func TestFetchHTTPMethods(t *testing.T) {
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		method := method
		t.Run(method, func(t *testing.T) {
			var gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				w.WriteHeader(200)
			}))
			defer srv.Close()

			opts := MapOf("method", method)
			_, err := catchFetch(newCtx(), srv.URL, opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != method {
				t.Errorf("server got %s, want %s", gotMethod, method)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Method is case-insensitive (lowercase "post" → "POST")
// --------------------------------------------------------------------------

func TestFetchMethodCaseInsensitive(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(200)
	}))
	defer srv.Close()

	opts := MapOf("method", "post")
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("server got %q, want POST", gotMethod)
	}
}

// --------------------------------------------------------------------------
// JSON body auto Content-Type can be overridden by an explicit header
// --------------------------------------------------------------------------

func TestFetchBodyContentTypeOverride(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	opts := MapOf(
		"method", "POST",
		"body", MapOf("x", int64(1)),
		"headers", MapOf("content-type", "application/json; charset=utf-8"),
	)
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q, want application/json; charset=utf-8", gotContentType)
	}
}

// --------------------------------------------------------------------------
// Empty response body → empty string (not nil, not a panic)
// --------------------------------------------------------------------------

func TestFetchEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := res.Get("body")
	if body != "" {
		t.Errorf("body = %v (%T), want empty string", body, body)
	}
}

// --------------------------------------------------------------------------
// JSON array body is parsed into *Array
// --------------------------------------------------------------------------

func TestFetchJSONArrayBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`[1,2,3]`))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := res.Get("body")
	arr, ok := body.(*Array)
	if !ok {
		t.Fatalf("body is %T, want *Array", body)
	}
	if arr.Len() != 3 {
		t.Errorf("array len = %d, want 3", arr.Len())
	}
	if arr.Items[0] != int64(1) {
		t.Errorf("arr[0] = %v, want 1", arr.Items[0])
	}
}

// --------------------------------------------------------------------------
// Timeout value given as int64 (not just float64) works correctly
// --------------------------------------------------------------------------

func TestFetchTimeoutAsInt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// timeout = 0 (integer) should give TYPE MISMATCH
	opts := MapOf("timeout", int64(0))
	_, err := catchFetch(newCtx(), srv.URL, opts)
	if err == nil || err.Kind != KindTypeMismatch {
		t.Errorf("timeout=0 (int64): got kind=%q, want TYPE MISMATCH", err.Kind)
	}
}

// --------------------------------------------------------------------------
// Result map has exactly three keys: status, headers, body
// --------------------------------------------------------------------------

func TestFetchResultShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hi"))
	}))
	defer srv.Close()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"status", "headers", "body"} {
		if _, ok := res.Get(key); !ok {
			t.Errorf("result missing key %q", key)
		}
	}
}

// --------------------------------------------------------------------------
// nil Ctx is safe (falls back to context.Background)
// --------------------------------------------------------------------------

func TestFetchNilCtxIsAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := catchFetch(nil, srv.URL)
	if err != nil {
		t.Fatalf("nil ctx raised error: %v", err)
	}
	if status, _ := res.Get("status"); status != int64(200) {
		t.Errorf("status = %v, want 200", status)
	}
}
