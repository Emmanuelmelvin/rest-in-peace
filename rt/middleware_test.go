package rt

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- helpers ----------------------------------------------------------------

// newTestCtx builds a minimal *Ctx wired to a fake request with the given
// method, path and headers. remoteAddr defaults to "1.2.3.4:9999".
func newTestCtx(method, path string, headers map[string]string, remoteAddr string) *Ctx {
	if remoteAddr == "" {
		remoteAddr = "1.2.3.4:9999"
	}
	req := &Request{
		ID:         "test-req",
		Method:     method,
		Path:       path,
		RemoteAddr: remoteAddr,
		Params:     NewMap(),
		Query:      NewMap(),
		Headers:    NewMap(),
		Body:       NewMap(),
		Ctx:        NewMap(),
	}
	for k, v := range headers {
		req.Headers.Set(strings.ToLower(k), v)
	}
	c := NewCtx(nil)
	c.Req = req
	return c
}

// runMiddleware calls mw and returns any *Respond it panics, plus a flag for
// whether it panicked. It does NOT swallow non-Respond panics.
func runMiddleware(mw Handler, c *Ctx) (resp *Respond, panicked bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if rsp, ok := r.(*Respond); ok {
			resp = rsp
			panicked = true
			return
		}
		panic(r) // re-raise unexpected panics
	}()
	mw(c, nil)
	return nil, false
}

// makeJWT signs an HS256 token with the given secret and claims. ttl<=0 omits exp.
func makeJWT(secret string, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		panic(fmt.Sprintf("makeJWT: %v", err))
	}
	return signed
}

// withEnv sets an environment variable for the duration of f and restores it.
func withEnv(t *testing.T, key, value string) func() {
	t.Helper()
	t.Setenv(key, value)
	return func() {} // t.Setenv already registers cleanup
}

// --- APIKey -----------------------------------------------------------------

func TestAPIKey(t *testing.T) {
	t.Setenv("TEST_API_KEY", "s3cr3t")

	mw := APIKey("TEST_API_KEY")

	tests := []struct {
		name       string
		header     string // value of x-api-key
		wantStatus int    // 0 = no respond (pass-through)
	}{
		{"correct key passes", "s3cr3t", 0},
		{"wrong key rejected", "wrong", 401},
		{"missing key rejected", "", 401},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			if tc.header != "" {
				headers["x-api-key"] = tc.header
			}
			c := newTestCtx("GET", "/", headers, "")
			resp, panicked := runMiddleware(mw, c)
			if tc.wantStatus == 0 {
				if panicked {
					t.Fatalf("expected pass-through, got RESPOND %d", resp.Status)
				}
				return
			}
			if !panicked {
				t.Fatal("expected RESPOND, got pass-through")
			}
			if resp.Status != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", resp.Status, tc.wantStatus)
			}
			if body, _ := resp.Body.(*Map); body != nil {
				v, _ := body.Get("error")
				if ToString(v) != "UNAUTHORIZED" {
					t.Fatalf("body.error: got %q, want UNAUTHORIZED", ToString(v))
				}
			}
		})
	}
}

func TestAPIKey_MissingEnvSecret(t *testing.T) {
	// Don't set the env var — middleware should lock the route.
	t.Setenv("MISSING_API_KEY_ENV", "")

	mw := APIKey("MISSING_API_KEY_ENV")
	c := newTestCtx("GET", "/", map[string]string{"x-api-key": "anything"}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 when secret env is empty, got panicked=%v status=%v", panicked, func() int {
			if resp != nil {
				return resp.Status
			}
			return 0
		}())
	}
}

func TestAPIKey_ConstantTime(t *testing.T) {
	// Different-length keys must still return 401, not panic on subtle.ConstantTimeCompare.
	t.Setenv("CT_KEY", "short")
	mw := APIKey("CT_KEY")
	c := newTestCtx("GET", "/", map[string]string{"x-api-key": "this-is-a-much-longer-key"}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for wrong-length key, got panicked=%v", panicked)
	}
}

// --- CORS -------------------------------------------------------------------

func TestCORS_DefaultHeaders(t *testing.T) {
	mw := CORS()
	c := newTestCtx("GET", "/api", nil, "")
	resp, panicked := runMiddleware(mw, c)
	if panicked {
		t.Fatalf("unexpected RESPOND %d on non-OPTIONS", resp.Status)
	}
	check := func(name, want string) {
		t.Helper()
		got, ok := c.respHeaders[name]
		if !ok || got != want {
			t.Errorf("header %s: got %q, want %q", name, got, want)
		}
	}
	check("Access-Control-Allow-Origin", "*")
	check("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	check("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
}

func TestCORS_OptionsPreflightResponds204(t *testing.T) {
	mw := CORS()
	c := newTestCtx("OPTIONS", "/api", nil, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked {
		t.Fatal("expected RESPOND 204 for OPTIONS, got pass-through")
	}
	if resp.Status != 204 {
		t.Fatalf("status: got %d, want 204", resp.Status)
	}
	// CORS headers must still be set before the respond.
	if c.respHeaders["Access-Control-Allow-Origin"] != "*" {
		t.Error("CORS headers not set before OPTIONS 204")
	}
}

func TestCORS_CustomOptions(t *testing.T) {
	opts := MapOf(
		"origins", "https://example.com",
		"methods", "GET, POST",
		"headers", "Content-Type",
		"credentials", "true",
		"maxAge", "3600",
	)
	mw := CORS(opts)
	c := newTestCtx("GET", "/", nil, "")
	runMiddleware(mw, c)

	want := map[string]string{
		"Access-Control-Allow-Origin":      "https://example.com",
		"Access-Control-Allow-Methods":     "GET, POST",
		"Access-Control-Allow-Headers":     "Content-Type",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Max-Age":           "3600",
	}
	for k, v := range want {
		if got := c.respHeaders[k]; got != v {
			t.Errorf("header %s: got %q, want %q", k, got, v)
		}
	}
}

func TestCORS_CredentialsOmittedWhenEmpty(t *testing.T) {
	mw := CORS()
	c := newTestCtx("GET", "/", nil, "")
	runMiddleware(mw, c)
	if _, ok := c.respHeaders["Access-Control-Allow-Credentials"]; ok {
		t.Error("credentials header should be absent when not configured")
	}
}

func TestCORS_MaxAgeOmittedWhenEmpty(t *testing.T) {
	mw := CORS()
	c := newTestCtx("GET", "/", nil, "")
	runMiddleware(mw, c)
	if _, ok := c.respHeaders["Access-Control-Max-Age"]; ok {
		t.Error("max-age header should be absent when not configured")
	}
}

// --- RequestLog -------------------------------------------------------------

func TestRequestLog_OnFinishCalledWithStatus(t *testing.T) {
	// Wire an App and check the OnFinish fires by counting calls.

	mw := RequestLog()
	c := newTestCtx("POST", "/invoices", nil, "")

	var captured struct {
		status int
		called bool
	}
	// Inject a secondary OnFinish so we can observe what REQUESTLOG registered.
	c.OnFinish(func(status int) {
		captured.status = status
		captured.called = true
	})

	runMiddleware(mw, c)

	// Simulate the server calling runOnFinish after the response.
	c.runOnFinish(201)

	if !captured.called {
		t.Fatal("OnFinish was not called")
	}
	if captured.status != 201 {
		t.Fatalf("status: got %d, want 201", captured.status)
	}
}

func TestRequestLog_ViaHTTP(t *testing.T) {
	// Confirm RequestLog integrates with the full App pipeline without panicking.
	a := NewApp("test.bas")
	a.Use(RequestLog(), "")
	a.Route("GET", "/ping", func(c *Ctx, req Value) Value {
		return MapOf("ok", true)
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

// --- RateLimit --------------------------------------------------------------

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	mw := RateLimit(int64(5))
	for i := 0; i < 5; i++ {
		c := newTestCtx("GET", "/", nil, "10.0.0.1:1234")
		resp, panicked := runMiddleware(mw, c)
		if panicked {
			t.Fatalf("request %d should be allowed, got RESPOND %d", i+1, resp.Status)
		}
	}
}

func TestRateLimit_Blocks429WhenExceeded(t *testing.T) {
	mw := RateLimit(int64(2))
	ip := "10.0.0.2:5678"
	for i := 0; i < 2; i++ {
		c := newTestCtx("GET", "/", nil, ip)
		resp, panicked := runMiddleware(mw, c)
		if panicked {
			t.Fatalf("request %d should pass, got RESPOND %d", i+1, resp.Status)
		}
	}
	// Third request must be rejected.
	c := newTestCtx("GET", "/", nil, ip)
	resp, panicked := runMiddleware(mw, c)
	if !panicked {
		t.Fatal("expected 429, got pass-through")
	}
	if resp.Status != 429 {
		t.Fatalf("status: got %d, want 429", resp.Status)
	}
	v, _ := resp.Body.(*Map)
	if v == nil {
		t.Fatal("body is nil")
	}
	msg, _ := v.Get("error")
	if ToString(msg) != "TOO MANY REQUESTS" {
		t.Fatalf("body.error: got %q, want TOO MANY REQUESTS", ToString(msg))
	}
}

func TestRateLimit_SeparateBucketsPerIP(t *testing.T) {
	mw := RateLimit(int64(1))
	for _, ip := range []string{"192.168.1.1:80", "192.168.1.2:80", "192.168.1.3:80"} {
		c := newTestCtx("GET", "/", nil, ip)
		resp, panicked := runMiddleware(mw, c)
		if panicked {
			t.Fatalf("IP %s first request should pass, got RESPOND %d", ip, resp.Status)
		}
	}
}

func TestRateLimit_InvalidPerMinutePanics(t *testing.T) {
	tests := []struct {
		name  string
		value Value
	}{
		{"zero", int64(0)},
		{"negative", int64(-1)},
		{"string", "fast"},
		{"nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic, got none")
				}
				e, ok := r.(*Error)
				if !ok {
					t.Fatalf("expected *Error, got %T", r)
				}
				if e.Kind != KindTypeMismatch {
					t.Fatalf("kind: got %q, want TYPE MISMATCH", e.Kind)
				}
			}()
			RateLimit(tc.value)
		})
	}
}

func TestRateLimit_AddrWithoutPort(t *testing.T) {
	// RemoteAddr with no port must not panic.
	mw := RateLimit(int64(10))
	c := newTestCtx("GET", "/", nil, "")
	c.Req.RemoteAddr = "1.2.3.4" // no port
	resp, panicked := runMiddleware(mw, c)
	if panicked {
		t.Fatalf("unexpected RESPOND %d for addr-without-port", resp.Status)
	}
}

// --- BearerJWT --------------------------------------------------------------

func TestBearerJWT_ValidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "supersecret")
	now := time.Now()
	tokenStr := makeJWT("supersecret", jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"exp":  now.Add(time.Hour).Unix(),
	})

	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/admin", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")

	resp, panicked := runMiddleware(mw, c)
	if panicked {
		t.Fatalf("expected pass-through, got RESPOND %d", resp.Status)
	}

	user, ok := c.Req.Ctx.Get("user")
	if !ok || user == nil {
		t.Fatal("ctx.user not set")
	}
	m, ok := user.(*Map)
	if !ok {
		t.Fatalf("ctx.user: got %T, want *Map", user)
	}
	sub, _ := m.Get("sub")
	if ToString(sub) != "user-123" {
		t.Fatalf("ctx.user.sub: got %q, want user-123", ToString(sub))
	}
	role, _ := m.Get("role")
	if ToString(role) != "admin" {
		t.Fatalf("ctx.user.role: got %q, want admin", ToString(role))
	}
}

func TestBearerJWT_ExpiredToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "supersecret")
	tokenStr := makeJWT("supersecret", jwt.MapClaims{
		"sub": "user-999",
		"exp": time.Now().Add(-time.Hour).Unix(), // already expired
	})
	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for expired token, got panicked=%v", panicked)
	}
}

func TestBearerJWT_WrongSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "correct")
	tokenStr := makeJWT("wrong-secret", jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for wrong secret, got panicked=%v", panicked)
	}
}

func TestBearerJWT_NonHS256Algorithm(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cr3t")
	// Sign with HS384 instead of HS256.
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte("s3cr3t"))

	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for non-HS256 algorithm, got panicked=%v", panicked)
	}
}

func TestBearerJWT_MissingAuthHeader(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cr3t")
	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", nil, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for missing header, got panicked=%v", panicked)
	}
}

func TestBearerJWT_MalformedToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cr3t")
	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer this.is.not.a.jwt",
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for malformed token, got panicked=%v", panicked)
	}
}

func TestBearerJWT_MissingBearerPrefix(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cr3t")
	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Token somevalue",
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 when header lacks Bearer prefix, got panicked=%v", panicked)
	}
}

func TestBearerJWT_MissingEnvSecret(t *testing.T) {
	t.Setenv("JWT_SECRET_MISSING", "")
	mw := BearerJWT("JWT_SECRET_MISSING")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer any.token.here",
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 when secret env is empty, got panicked=%v", panicked)
	}
}

func TestBearerJWT_ClaimsMapContainsAllFields(t *testing.T) {
	t.Setenv("JWT_SECRET", "signingkey")
	now := time.Now()
	tokenStr := makeJWT("signingkey", jwt.MapClaims{
		"sub":   "user-42",
		"email": "ada@example.com",
		"admin": true,
		"score": float64(99),
		"exp":   now.Add(time.Hour).Unix(),
	})

	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")
	runMiddleware(mw, c)

	user, _ := c.Req.Ctx.Get("user")
	m := user.(*Map)

	email, _ := m.Get("email")
	if ToString(email) != "ada@example.com" {
		t.Errorf("email: got %q", ToString(email))
	}
	admin, _ := m.Get("admin")
	if admin != true {
		t.Errorf("admin: got %v, want true", admin)
	}
	score, _ := m.Get("score")
	if score.(int64) != 99 {
		t.Errorf("score: got %v, want int64(99)", score)
	}
}

func TestBearerJWT_TokenWithoutExpRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "s3cr3t")
	// Token deliberately has no exp claim.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "u",
	})
	tokenStr, _ := token.SignedString([]byte("s3cr3t"))

	mw := BearerJWT("JWT_SECRET")
	c := newTestCtx("GET", "/", map[string]string{
		"Authorization": "Bearer " + tokenStr,
	}, "")
	resp, panicked := runMiddleware(mw, c)
	if !panicked || resp.Status != 401 {
		t.Fatalf("expected 401 for token without exp, got panicked=%v", panicked)
	}
}
