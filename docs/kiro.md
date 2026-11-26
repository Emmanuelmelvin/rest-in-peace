# How we used Kiro

> Draft for the Kiroween write-up (task 6.3). Fill it in as each Kiro task happens: notes written at the time beat notes reconstructed on deadline day.

## What was built with what

REST in Peace was built with two tools, and this document is upfront about which did what.

| Built with | What |
|---|---|
| **Kiro** | Tasks 2.3b/2.7b (built-in middleware: APIKEY, CORS, REQUESTLOG, RATELIMIT, BEARERJWT), 2.4 (ENV, LOG, redaction), 2.5 (DECIMAL), 3.3 (`rip migrate`), 3.6 (SQL injection warning), 3.7 (`FETCH`), and 5.4 (playground CRT theme and samples), plus the spec-driven workflow: the `.kiro/` specs, steering and hooks that every task follows |
| **Claude Code** | Everything else: the compiler (lexer, parser, Go codegen), runtime, HTTP, database, CLI, playground backend, and the scaffold and specs themselves |

The two never edit the same files. Ownership is listed in `.kiro/steering/structure.md`, and the git history shows it.

## The spec-driven setup
- `.kiro/specs/rip-basic/requirements.md`: 16 requirements in EARS form.
- `design.md`: compiling to Go, the value model, `//line` error mapping, and classic mode as native `goto`.
- `tasks.md`: the dated plan, with the Kiro tasks marked **[Kiro]**.
- `steering/`: `language.md` (never invent syntax), `security.md`, `style.md`, `product.md`, `tech.md`, `structure.md`.
- `hooks/`: golden tests on compiler/runtime save, `rip check` on `.bas` save, and example tests on save.

## Kiro task log

Fill in one section per task as you do it.

### 2.5 DECIMAL and ROUND
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) chat session on branch `kiro/task-2.5-decimal` (branched from `phase2-seams`), pointed at task 2.5 in `.kiro/specs/rip-basic/tasks.md` and `.kiro/steering/`.
- **What the spec and steering contributed:** `requirements.md` (§3.2, §3.3, §3.7, and "Decisions to confirm") locked in `shopspring/decimal` arithmetic, `DECIMAL` + `INTEGER`/`NUMBER` promotion, half-away-from-zero `ROUND`, and scale-preserving JSON strings (`"215.00"`). `structure.md` kept edits strictly inside `rt/decimal.go` and `rt/decimal_test.go` while matching the existing seam signatures called by `rt/ops.go` and `rt/value.go`. `style.md` guided table-driven tests and `?TYPE MISMATCH ERROR` / `?DIVISION BY ZERO ERROR` messages with hints.
- **What the hooks / tests caught:** Running `go test ./rt/...` caught that `shopspring/decimal`'s `String()` strips trailing zeros (`"215.00"` → `"215"`). Kiro probed `d.Exponent()` and added `formatDecimal` using `StringFixed(-exp)` for user-meaningful scales (`<= 10`), while falling back to `String()` for high-precision division (`10.00 / 4.00` → `"2.5"` rather than 16 trailing decimals). All 55 `rt` tests, `gofmt`, and `go vet` passed.
- **What I changed by hand, and why:** Nothing in Kiro's files — committed verbatim as `e30e572`. Merge-time follow-up (outside Kiro's ownership): update `TestDecimalSeams` in `internal/compiler/codegen/seams_test.go`, which still asserts the old `"is not built yet (task 2.5)"` stub panic and now exits `0` because `DECIMAL()` and `ROUND()` work end-to-end.
- **Screenshot or clip for the video:** *To capture*

### 2.4 ENV, LOG and redaction
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) chat session on branch `kiro/task-2.4-env-log`, pointed at task 2.4 in `.kiro/specs/rip-basic/tasks.md` and `.kiro/steering/`.
- **What `security.md` changed in the output** (e.g. redaction): Drove recursive redaction for all five secret key patterns (`password`, `token`, `apiKey`, `authorization`, `secret`, case-insensitive) across top-level fields, nested `*rt.Map`, `*rt.Array`, `map[string]any`, and `[]any` at arbitrary depth, while rendering `*Map`, `*Array`, and `Decimal` cleanly as JSON via `json.RawMessage` and adding thread-safe `SetLogOutput(w io.Writer)` for testing and log redirection.
- **What the hooks / tests caught:** `gofmt` flagged `rt/log_test.go` and `go vet ./...` caught a missing `fmt` import in `rt/log.go`; Kiro fixed both and verified all 86 `rt` tests passing (`go test -count=1 ./rt/...`).
- **What I changed by hand, and why:** Nothing in Kiro's files — committed verbatim as `81728f7`. Follow-ups for non-Kiro files: wire `rt.SetLogOutput` (or stdout capture) in `internal/playground` for SSE log streaming (Req 10.4), and ensure `.env` dev loading is wired in `internal/cli/` for `rip dev` / `rip run` (Req 10.2).

### 2.3b / 2.7b Built-in middleware (APIKEY, CORS, REQUESTLOG, RATELIMIT, BEARERJWT)
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) headless session on branch `kiro/task-2.3-2.7-middleware` in a dedicated worktree off `main` after PR #13 added the `rt/middleware.go` seams and `Ctx.SetResponseHeader` / `Ctx.OnFinish` / `Request.RemoteAddr` hooks.
- **What the spec and steering contributed:** `requirements.md` (Requirement 8.1–8.3) and `security.md` drove constant-time API key comparison (without leaking key length), explicit HS256-only JWT verification with `WithExpirationRequired()` and `*jwt.SigningMethodHMAC` assertion, per-IP 60s sliding-window rate limiting with fail-fast startup validation, CORS header injection across error responses + `OPTIONS` 204 preflight, and post-response `REQUESTLOG` via `OnFinish`. `structure.md` kept edits strictly inside `rt/middleware.go` and `rt/middleware_test.go`.
- **What the hooks / tests caught:** `go vet` caught an unused test variable and `gofmt` caught struct-field alignment in `rt/middleware.go`; Kiro fixed both and verified all 25+ new middleware tests passing in `go test -count=1 ./rt/...`.
- **What I changed by hand, and why:** Nothing in Kiro's files — committed verbatim as `16a7540` and merged in PR #17.

### 3.6 SQL injection warning
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) headless session on branch `kiro/task-3.6-injection` in a dedicated worktree off `phase3-seams` (PR #20 wired `checkSQLInjection(p)` in `check.go`).
- **What the spec and steering contributed:** `requirements.md` (Requirement 9.4) and `security.md` drove per-file taint propagation from `req.*` (structured mode) and `PARAM`/`QUERY`/`HEADER`/`BODY` (classic mode) across transitive `LET`/`DIM` assignments and string built-ins (`TRIM`, `UPPER`, `LOWER`, `LEFT`, `RIGHT`, `MID`, `STR`), warning with `?INJECTION RISK WARNING IN <file>:<line> — use $1 placeholders` only on `+`/`&` concatenation while leaving safe parameterized queries and literal concatenations clean and keeping `HasErrors() == false`.
- **What the hooks / tests caught:** Initial test run caught `classic_transitive_param` because each classic line got a fresh `taintEnv`; Kiro switched classic mode to a single file-wide `taintEnv` (matching classic BASIC's global variable scope) and verified all 43 new injection tests plus the full `check` suite, `gofmt`, and `go vet`.
- **What I changed by hand, and why:** Nothing in Kiro's files — merged verbatim in PR #21.
- **The `rip check` hook in action** (a warning firing on save): *To capture*

### 3.7 Outbound HTTP (`FETCH`)
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) headless session on branch `kiro/task-3.7-fetch` in a dedicated worktree off `phase3-seams` (which wired `FETCH` in `codegen/expr.go` to `rt.Fetch`).
- **What the spec and steering contributed:** `requirements.md` (Requirement 11.1–11.2) and `security.md` guided options validation (`method`, `headers`, `body` with JSON auto-encoding, `timeout` default 10s layered over `c.Context()`), 1 MB response cap via `io.LimitReader`, a `fetchDialer` hook ready for task 5.3's playground SSRF guard, lowercase response headers, JSON auto-decoding, and catchable `*rt.Error` (`KindHTTP`) on timeouts/cancellations/network errors.
- **What the hooks / tests caught:** All 23 `httptest.NewServer` tests in `rt/fetch_test.go`, the full `rt` suite, `gofmt`, and `go vet` passed on the first run.
- **What I changed by hand, and why:** Nothing in Kiro's files — merged verbatim in PR #22.

### 3.3 `rip migrate`
- **Date:** 2026-11-26
- **How it started:** `kiro-cli` (v3.6.2) headless session on branch `kiro/task-3.3-migrate` in a dedicated worktree off `phase3-seams` (with `pgx/v5` and `modernc.org/sqlite` already pinned in `go.mod`).
- **What the spec and steering contributed:** `requirements.md` (Requirement 9.6 & 13) drove `.env` + `DATABASE_URL` resolution with classic `?CONFIG ERROR` when unset, Postgres (`pgx.Connect`) and SQLite (`modernc.org/sqlite` with `splitSQL` for multi-statement files) drivers, `_rip_migrations` tracking, lexicographical ordering of `migrations/*.sql`, per-file transactions with rollback on failure, and `internal/build/migrations.go` wired from `internal/cli/build.go`.
- **What the hooks / tests caught:** `go vet` flagged an undefined helper (`splitSQLStatements`) in `migrate_test.go`. The real cause was an orchestration mistake, not Kiro: two `kiro-cli` sessions were accidentally launched on the same worktree for about 13 minutes, and the second one rewrote `migrate.go` (renaming the helper to `splitSQL`) under the first. The duplicate session was killed. The remaining session then aligned the test, rebuilt, and verified all 13 tests in `internal/cli/migrate_test.go`, plus `gofmt` and `go vet`, before committing.
- **What I changed by hand, and why:** Nothing in Kiro's files — merged verbatim in PR #23.

### 5.4 Playground CRT theme and samples
- **Date:**
- **How it started:** (vibe-coding the look, then spec for the samples?)
- **Before/after screenshots:**

## Spec coding vs. vibe coding
- *To fill in:* where the spec drove the work, where chat was faster, and what you'd do differently.

## MCP
- *Optional:* a Postgres MCP server for inspecting the demo database from Kiro.
