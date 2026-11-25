---
inclusion: always
---

# Security rules

## SQL
- Queries always use `$1, $2…` placeholders, and arguments are always passed as parameters. Never build SQL by concatenating values, not in `rt` and not in examples.
- The compiler warns (`?INJECTION RISK WARNING`) when a query string is built with `+` or `&` from request data. Never silence this warning in examples.

## Secrets & errors
- Secrets come only from `ENV`/`ENVREQUIRE`. Never hard-code them, including in examples. Use `.env.example` with placeholder values.
- Log redaction covers keys matching `password`, `token`, `apiKey`, `authorization` and `secret`, case-insensitively.
- In production, a 500 response contains only `{"error": "INTERNAL ERROR", "requestId": …}`. Details go to the log.

## HTTP
- Request bodies are capped (default 1 MB).
- `APIKEY` and `BEARERJWT` compare secrets in constant time (`crypto/subtle`).

## Playground isolation
- One OS process per program, with `GOMEMLIMIT` and rlimits, a 5 s request deadline, a host watchdog and compiled-in loop guards.
- Go interop is limited to the allowlist in `internal/playground/allowlist.go`.
- `FETCH` runs with the SSRF guard: block private, loopback, link-local and metadata IPs after DNS resolution, re-check on every redirect, and cap responses at 1 MB.
- Programs get their own SQLite file and an empty environment, apart from `PORT` and `RIP_PLAYGROUND=1`. They never see the host's environment or files.
- The host container holds no secrets. Treat process isolation as defence in depth, not as a hard boundary.
