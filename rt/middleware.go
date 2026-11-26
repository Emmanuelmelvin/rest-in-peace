package rt

import (
	_ "github.com/golang-jwt/jwt/v5" // pinned for BearerJWT (task 2.7)
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
	// TODO(task 2.3): validate x-api-key header against Env(ToString(envName))
	// using crypto/subtle.ConstantTimeCompare; return 401 when missing or wrong.
	panic(notBuilt("APIKEY", "2.3"))
}

// CORS implements CORS([options]) (task 2.3, Requirement 8.3).
// Sets Access-Control-Allow-* headers via c.SetResponseHeader and answers
// OPTIONS preflight requests with 204.
// Called by generated code for: USE CORS([options]) [ON "/path/*"].
func CORS(options ...Value) Handler {
	// TODO(task 2.3): parse optional options map (origins, methods, headers)
	// and set Access-Control-Allow-* headers; handle OPTIONS preflight.
	panic(notBuilt("CORS", "2.3"))
}

// RequestLog implements REQUESTLOG() (task 2.3, Requirement 8.3).
// Logs each completed request (method, path, status, durationMs) via Log.
// Called by generated code for: USE REQUESTLOG() [ON "/path/*"].
func RequestLog() Handler {
	// TODO(task 2.3): record start time and register c.OnFinish to call Log
	// with method, path, status and durationMs.
	panic(notBuilt("REQUESTLOG", "2.3"))
}

// RateLimit implements RATELIMIT(perMinute) (task 2.7, Requirement 8.3).
// Caps requests per client IP per minute and responds 429 when exceeded.
// Called by generated code for: USE RATELIMIT(perMinute) [ON "/path/*"].
func RateLimit(perMinute Value) Handler {
	// TODO(task 2.7): thread-safe per-IP rate limiter using c.Req.RemoteAddr;
	// respond 429 when a client exceeds perMinute requests per minute.
	panic(notBuilt("RATELIMIT", "2.7"))
}

// BearerJWT implements BEARERJWT(envSecret) (task 2.7, Requirement 8.3).
// Verifies an HS256 JWT from the Authorization: Bearer <token> header using
// the secret in ENV(envSecret), and sets req.ctx.user to the claims map.
// Called by generated code for: USE BEARERJWT("JWT_SECRET") [ON "/path/*"].
func BearerJWT(envSecret Value) Handler {
	// TODO(task 2.7): verify HS256 JWT with github.com/golang-jwt/jwt/v5,
	// reject non-HS256 algorithms, set c.Req.Ctx.Set("user", claimsMap), or 401.
	panic(notBuilt("BEARERJWT", "2.7"))
}
