package rt

import (
	"crypto/subtle"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Built-in HTTP middleware factories (Requirement 8.3; tasks 2.3 and 2.7).
//
// Owned by Kiro (see .kiro/steering/structure.md). Generated code compiles
// USE APIKEY(…), USE CORS(…), USE REQUESTLOG(), USE RATELIMIT(…) and
// USE BEARERJWT(…) to calls to these five functions, each returning a Handler
// registered via App.Use.
//
// Each factory runs once at startup (when USE executes) and returns a Handler
// invoked per request before the route handler:
//   - Call RespondNow(c, status, body) or Throw(HTTPError(status, msg, nil))
//     to stop the request before the route handler runs (Requirement 8.2).
//   - Call c.SetResponseHeader(name, value) to add headers to the response
//     (used by CORS).
//   - Call c.OnFinish(func(status int) { … }) to run after the response is
//     written, receiving the final HTTP status code (used by REQUESTLOG).
//   - Read c.Req.RemoteAddr for the client address (used by RATELIMIT).
//   - Write c.Req.Ctx.Set("user", claimsMap) to pass verified claims to
//     handlers (used by BEARERJWT).

// APIKey implements APIKEY(envName) (task 2.3, Requirement 8.3).
// Checks the x-api-key request header against ENV(envName).
// Called by generated code for: USE APIKEY("API_KEY") [ON "/path/*"].
func APIKey(envName Value) Handler {
	name := ToString(envName)
	return func(c *Ctx, req Value) Value {
		secret := Env(name)
		if secret == nil || ToString(secret) == "" {
			// No secret configured — treat the route as locked.
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		key := ""
		if c.Req != nil {
			v, _ := c.Req.Headers.Get("x-api-key")
			key = ToString(v)
		}
		expected := []byte(ToString(secret))
		got := []byte(key)
		// ConstantTimeCompare requires equal-length slices; pad shorter one.
		if len(got) != len(expected) {
			// Use a dummy compare so timing is consistent, then reject.
			subtle.ConstantTimeCompare(expected, expected)
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		if subtle.ConstantTimeCompare(expected, got) != 1 {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		return nil
	}
}

// corsDefaults holds the defaults used when CORS is called with no options or
// with an options map that omits a field.
var corsDefaults = struct {
	origins     string
	methods     string
	headers     string
	credentials string
	maxAge      string
}{
	origins:     "*",
	methods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	headers:     "Content-Type, Authorization, X-Api-Key",
	credentials: "",
	maxAge:      "",
}

// CORS implements CORS([options]) (task 2.3, Requirement 8.3).
// Sets Access-Control-Allow-* headers via c.SetResponseHeader and answers
// OPTIONS preflight requests with 204.
// Called by generated code for: USE CORS([options]) [ON "/path/*"].
func CORS(options ...Value) Handler {
	origins := corsDefaults.origins
	methods := corsDefaults.methods
	headers := corsDefaults.headers
	credentials := corsDefaults.credentials
	maxAge := corsDefaults.maxAge

	if len(options) > 0 {
		if m, ok := options[0].(*Map); ok && m != nil {
			if v, ok := m.Get("origins"); ok {
				origins = ToString(v)
			}
			if v, ok := m.Get("methods"); ok {
				methods = ToString(v)
			}
			if v, ok := m.Get("headers"); ok {
				headers = ToString(v)
			}
			if v, ok := m.Get("credentials"); ok {
				credentials = ToString(v)
			}
			if v, ok := m.Get("maxAge"); ok {
				maxAge = ToString(v)
			}
		}
	}

	return func(c *Ctx, req Value) Value {
		c.SetResponseHeader("Access-Control-Allow-Origin", origins)
		c.SetResponseHeader("Access-Control-Allow-Methods", methods)
		c.SetResponseHeader("Access-Control-Allow-Headers", headers)
		if credentials != "" {
			c.SetResponseHeader("Access-Control-Allow-Credentials", credentials)
		}
		if maxAge != "" {
			c.SetResponseHeader("Access-Control-Max-Age", maxAge)
		}
		if c.Req != nil && c.Req.Method == "OPTIONS" {
			RespondNow(c, 204, nil)
		}
		return nil
	}
}

// RequestLog implements REQUESTLOG() (task 2.3, Requirement 8.3).
// Logs each completed request (method, path, status, durationMs) via Log.
// Called by generated code for: USE REQUESTLOG() [ON "/path/*"].
func RequestLog() Handler {
	return func(c *Ctx, req Value) Value {
		start := time.Now()
		method := ""
		path := ""
		if c.Req != nil {
			method = c.Req.Method
			path = c.Req.Path
		}
		c.OnFinish(func(status int) {
			ms := time.Since(start).Milliseconds()
			Log(c, "INFO", "REQUEST", MapOf(
				"method", method,
				"path", path,
				"status", int64(status),
				"durationMs", ms,
			))
		})
		return nil
	}
}

// bucket holds one client's sliding-window state for RATELIMIT.
type bucket struct {
	mu          sync.Mutex
	times       []time.Time // ring of request timestamps within the current window
	windowStart time.Time
}

// allow reports whether a new request is within the per-minute limit.
func (b *bucket) allow(limit int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	// Prune timestamps older than 60 s.
	cutoff := now.Add(-time.Minute)
	i := 0
	for i < len(b.times) && b.times[i].Before(cutoff) {
		i++
	}
	b.times = b.times[i:]
	if len(b.times) >= limit {
		return false
	}
	b.times = append(b.times, now)
	return true
}

// RateLimit implements RATELIMIT(perMinute) (task 2.7, Requirement 8.3).
// Caps requests per client IP per minute and responds 429 when exceeded.
// Called by generated code for: USE RATELIMIT(perMinute) [ON "/path/*"].
func RateLimit(perMinute Value) Handler {
	n, ok := toInt(perMinute)
	if !ok || n <= 0 {
		panic(Errorf(KindTypeMismatch, "RATELIMIT needs a positive INTEGER, got %s %s — e.g. USE RATELIMIT(60)", TypeName(perMinute), ToString(perMinute)))
	}
	limit := int(n)

	var mu sync.Mutex
	clients := map[string]*bucket{}

	getBucket := func(ip string) *bucket {
		mu.Lock()
		defer mu.Unlock()
		b, ok := clients[ip]
		if !ok {
			b = &bucket{}
			clients[ip] = b
		}
		return b
	}

	return func(c *Ctx, req Value) Value {
		addr := ""
		if c.Req != nil {
			addr = c.Req.RemoteAddr
		}
		ip, _, err := net.SplitHostPort(addr)
		if err != nil {
			// If the address has no port (unusual), use it as-is.
			ip = addr
		}
		b := getBucket(ip)
		if !b.allow(limit) {
			RespondNow(c, 429, MapOf("error", "TOO MANY REQUESTS"))
		}
		return nil
	}
}

// BearerJWT implements BEARERJWT(envSecret) (task 2.7, Requirement 8.3).
// Verifies an HS256 JWT from the Authorization: Bearer <token> header using
// the secret in ENV(envSecret), and sets req.ctx.user to the claims map.
// Called by generated code for: USE BEARERJWT("JWT_SECRET") [ON "/path/*"].
func BearerJWT(envSecret Value) Handler {
	secretEnvName := ToString(envSecret)
	return func(c *Ctx, req Value) Value {
		secret := Env(secretEnvName)
		if secret == nil || ToString(secret) == "" {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		secretBytes := []byte(ToString(secret))

		authHeader := ""
		if c.Req != nil {
			v, _ := c.Req.Headers.Get("authorization")
			authHeader = ToString(v)
		}
		if authHeader == "" {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// Parse with explicit HS256-only algorithm acceptance.
		token, err := jwt.Parse(tokenStr,
			func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, fmt.Errorf("only HS256 is accepted, got %v", t.Method.Alg())
				}
				return secretBytes, nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !token.Valid {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			RespondNow(c, 401, MapOf("error", "UNAUTHORIZED"))
		}

		claimsMap := NewMap()
		for k, v := range claims {
			claimsMap.Set(k, jwtClaimValue(v))
		}

		if c.Req != nil {
			c.Req.Ctx.Set("user", claimsMap)
		}
		return nil
	}
}

// jwtClaimValue converts a JWT claim value (from jwt.MapClaims) into a BASIC Value.
// Numbers become int64 when they are whole, float64 otherwise. Nested objects
// become *Map. Arrays become *Array. Everything else keeps its Go type.
func jwtClaimValue(v any) Value {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case map[string]any:
		m := NewMap()
		for k, val := range x {
			m.Set(k, jwtClaimValue(val))
		}
		return m
	case []any:
		a := ArrayOf()
		for _, item := range x {
			a.Items = append(a.Items, jwtClaimValue(item))
		}
		return a
	default:
		return x
	}
}
