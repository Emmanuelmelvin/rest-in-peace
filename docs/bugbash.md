# Bug Bash Report — RIP BASIC (Task 6.2)

Acceptance-criteria bug bash covering Requirements 1–11 and 14.5–14.6 (all features built through Phase 3), tracked on branch `kiro/task-6.2-bugbash` (Phase 4 Track U).

## Golden Suite Count

| Metric | Count |
|---|---:|
| Pre-existing golden `.bas` programs | 133 |
| New `bash_*` golden `.bas` programs (2 started by Kiro + 23 added/completed) | 25 |
| **Total golden `.bas` programs** | **158** |
| Task 6.2 target | ≥ 80 |

All 25 `bash_*` goldens and `TestGoldenLayout` pass:

```text
go test -v ./internal/golden -run 'TestGolden/.*/bash_|TestGoldenLayout'
```

## Acceptance Criteria Coverage Matrix

| Requirement | Acceptance Criteria Tested | New `bash_*` Goldens | Existing Goldens / Unit Tests |
|---|---|---|---|
| **Req 1** Source files & modes | UTF-8 strings & keys (1.1), mode detection & mixed-mode errors (1.2–1.3), case-insensitive keywords/vars/funcs (1.4), case-sensitive map keys & member access (1.5), classic ordering/duplicates/`:` (1.6) | `structured/bash_1_utf8_and_case.bas` | `classic/leading_comments`, `out_of_order_lines`, `duplicate_line`, `colon_separator`; `structured/case_insensitive`, `map_keys_case_sensitive`; `errors/mixed_modes*` |
| **Req 2** Core statements | `FOR EACH`, `SELECT CASE`, `WHILE`/`EXIT WHILE`, `DO ... LOOP UNTIL`, `IF/ELSEIF/ELSE/END IF` in **classic mode** (2.1), `GOSUB`/`RETURN` (2.2), `GOTO`/`GOSUB` rejected in structured mode (2.3) | `classic/bash_2_classic_blocks.bas` | `classic/count`, `gosub*`, `goto_jump`, `on_goto*`, `end_stops_early`; `structured/if_*`, `for_*`, `while_wend`, `do_loop_*`, `select_case*`, `exit_*`; `errors/goto_in_structured`, `gosub_in_structured` |
| **Req 3** Values, types & expressions | Typed `DIM` runtime `?TYPE MISMATCH ERROR` (3.2–3.3), `CONST` immutability (3.4), map insertion order in JSON (3.5), short-circuit `AND`/`OR` (3.6), `DECIMAL` arithmetic/comparisons/`NUMBER` mixing/`ROUND` half-away-from-zero/scale-preserving JSON string (3.7 & Decision), missing map key → `NIL` (3.8), `LEN(NIL) = 0` (3.9) | `structured/bash_3_decimal_json.bas`, `structured/bash_3_decimal_ops.bas`, `http/bash_3_decimal_http.bas`, `structured/bash_3_nil_and_len.bas`, `errors/bash_3_const_reassign.bas`, `errors/bash_3_dim_type_mismatch.bas` | `structured/dim_integer`, `dim_types`, `typed_boundaries`, `const*`, `map_*`, `array_index`, `missing_key_nil`, `and_or_short_circuit`, `string_plus_vs_amp`, `builtins_*`; `errors/nil_member_access`, `type_mismatch_*` |
| **Req 4** Functions & subroutines | Typed params with defaults (4.1), `SUB` (4.2), local scope isolation (4.3), forward references & mutual recursion (4.4), recursion depth limit → `?STACK OVERFLOW ERROR` (4.5) | `structured/bash_4_mutual_recursion.bas`, `errors/bash_4_stack_overflow.bas` | `structured/functions`, `function_locals`, `default_params`, `forward_call`, `recursion_factorial`, `sub_*`, `typed_functions` |
| **Req 5** Modules & Go interop | `EXPORT`/`IMPORT` whole & named (5.1–5.3), named Go imports `IMPORT { ... } FROM GO` (5.4), typed arg conversion (5.5), Go `(val, error)` → catchable `e.kind = "GO"` (5.6), `NEW(pkg.Type)` & method calls (5.7), concurrent goroutines per request (5.8) | `structured/bash_5_gointerop_named.bas`, `http/bash_11_fetch.bas` (concurrent self-call) | `structured/mod_*`, `gointerop_stdlib`, `gointerop_uuid`; `errors/mod_*`, `gointerop_*` |
| **Req 6** Error handling | `TRY`/`CATCH`/`FINALLY` (6.1), `THROW ERROR` & `THROW HTTPERROR` (6.2), `e.message`/`e.kind`/`e.file`/`e.line`/`e.status` (6.3), `HTTPERROR` with & without `details` (6.4 & Decision), `500` internal error with `requestId` (6.5), classic error format (6.6), `RESPOND` uncatchable by `CATCH` and runs `FINALLY` (6.7) | `structured/bash_6_error_fields.bas`, `http/bash_6_httperror_nodetails.bas` | `structured/try_*`; `http/http_error`, `internal_error`, `respond_finally`; `errors/throw_*`, `try_*` |
| **Req 7** HTTP server | All 5 methods `GET`/`POST`/`PUT`/`PATCH`/`DELETE` + `:params` + `*` (7.1), `req.method`/`params`/`query`/`headers`/`body`/`ctx`/`id` (7.2), classic `PARAM`/`QUERY`/`HEADER`/`BODY` (7.3), `RESPOND` with custom headers (7.4), `RETURN` → `200` (7.5), `SERVE` (7.6), `ROUTE GROUP` (7.7), unmatched path & unmatched method → `404 {"error": "ROUTE NOT FOUND IN THIS REALM"}` (7.8 & Decision), invalid JSON → `400` & body cap `413` (7.9), classic serialization (7.10) | `http/bash_7_methods_and_headers.bas` | `http/hello*`, `params`, `query`, `headers`, `json_body`, `empty_body`, `invalid_json`, `not_found`, `respond*`, `return_200`, `route_group`, `wildcard`, `classic_request`, `classic_todo`; `rt/app_test.go` (body cap `413`) |
| **Req 8** Middleware | `USE` ordering & `ON "/path/*"` scoping (8.1), `RESPOND`/`THROW` short-circuiting (8.2), built-in `CORS`, `APIKEY`, `RATELIMIT`, `REQUESTLOG`, `BEARERJWT` (8.3) | `http/bash_8_cors.bas`, `http/bash_8_apikey_ratelimit.bas`, `http/bash_8_bearerjwt.bas`, `http/bash_14_health_middleware.bas` | `http/middleware`; `rt/middleware_test.go` |
| **Req 9** Databases | `DBCONNECT "sqlite://:memory:"` (9.1), `SQL`/`SQLONE`/`SQLEXEC` (9.2), `$n` placeholders (9.3), `?INJECTION RISK WARNING` in structured & classic modes (9.4), `TRANSACTION` commit on `RESPOND` & rollback on error (9.5), `NUMERIC` → `DECIMAL` in JSON (9.7) | `errors/bash_9_sql_injection_warn.bas`, `errors/bash_9_sql_injection_classic.bas`, `http/bash_9_db_http_tx.bas` | `structured/db_sqlite`, `tx_commit_rollback`; `http/tx_respond`; `internal/cli/migrate_test.go` (9.6) |
| **Req 10** Config, secrets & logging | `ENV(name [, default])` & `ENVREQUIRE(name)` missing → `?UNDEFINED ERROR` (10.1), `LOG` JSON line & invalid level → `?TYPE MISMATCH ERROR` (10.3), `PRINT` (10.4), secret key redaction (10.5) | `structured/bash_10_env.bas`, `errors/bash_10_envrequire_missing.bas`, `errors/bash_10_log_bad_level.bas`, `http/bash_14_health_middleware.bas` | `rt/env_test.go`, `rt/log_test.go` |
| **Req 11** Outbound HTTP | `FETCH(url [, options])` with `method`, `headers`, JSON `body`, `timeout`, returning `{status, headers, body}` with parsed JSON & lowercase headers (11.1) | `http/bash_11_fetch.bas` | `rt/fetch_test.go` |
| **Req 14.5–14.6** Reliability & health | Graceful shutdown on `SIGTERM` (14.5), default `GET /_health` → `200 {"status": "ALIVE"}` with global middleware registered & custom override (14.6) | `http/bash_14_health_middleware.bas` | `http/health`, `health_classic`, `health_override`; `rt/app_test.go` (graceful shutdown) |

## Bugs, Rough Edges & Spec Notes Discovered

Because Track U owns only `testdata/golden/*/bash_*` and `docs/bugbash.md`, compiler/runtime/harness files were not modified. The findings below are logged for follow-up fixes at merge/closeout:

### 1. Bug: `?INJECTION RISK WARNING` misses tainted intermediate variables (`internal/compiler/check/injection.go`)

- **Requirement:** 9.4 — *"IF a query string is built with `+` or `&` from a value that came from `req`, THEN the compiler SHALL warn: `?INJECTION RISK WARNING IN <file>:<line> — use $1 placeholders`."*
- **Reproduction (classic or structured mode):**
  ```basic
  10 DBCONNECT "sqlite://:memory:"
  20 ROUTE GET "/search" GOSUB 100
  30 SERVE
  100 LET Q = "SELECT * FROM items WHERE name = " + QUERY("q")
  110 LET R = SQL(Q)
  120 RESPOND 200, R
  ```
- **Actual behavior:** Compiles with **no warning**.
- **Expected behavior:** Emits `?INJECTION RISK WARNING` at `SQL(Q)` (line 6).
- **Root cause:** `injectionAnalyzer.scanStmt` (`internal/compiler/check/injection.go:111-114`) marks `Q` as tainted in `taintEnv`, **but** `isConcatWithTaint` (`injection.go:260-271`) checks `bin, ok := e.(*ast.BinaryExpr)` on `call.Args[0]` directly. Because `Q` is an `*ast.Ident`, `isConcatWithTaint` immediately returns `false`. Inline concatenation (`SQL("..." + QUERY("q"))` or `SQL("..." & req.params.id)`) warns as expected.
- **Suggested fix:** Track a separate `concatTainted` set in `taintEnv` (set when an assignment's RHS is a `+`/`&` expression containing taint) and check `env.isConcatTainted(id.Name)` when `queryArg` is an `*ast.Ident`.

### 2. Rough Edge: `checkScriptLayout` rejects empty-body `204 No Content` responses (`internal/golden/http_golden_test.go`)

- **Location:** `internal/golden/http_golden_test.go:78-82`
- **Observed:** When testing `OPTIONS /ping` against `USE CORS(...)`, the server correctly returned `204 No Content` with an empty body, and `TestGolden/http/bash_8_cors` **passed** at runtime. However, `TestGoldenLayout` failed with:
  ```text
  http/bash_8_cors.http:9: expected body is not JSON (EOF):
  ```
- **Cause:** `checkScriptLayout` unconditionally calls `ParseJSON(ex.Want.Body)` on every exchange, even when `ex.Want.Status == 204` (empty body) or `ex.Want.Body == "<any>"`.
- **Suggested fix:** Skip `ParseJSON` in `checkScriptLayout` when `strings.TrimSpace(ex.Want.Body) == ""` (for `204`/`304`) or `strings.TrimSpace(ex.Want.Body) == AnyValue`.

### 3. Rough Edge: `RIP_MAX_BODY_BYTES` is read at `rt.NewApp()` before top-level statements (`rt/app.go`)

- **Location:** `rt/app.go:67-75`
- **Observed:** `NewApp` reads `RIP_MAX_BODY_BYTES` once when constructing the app (before any top-level BASIC code executes). Setting `RIP_MAX_BODY_BYTES` in the process environment before starting the binary works as specified (`rt/app_test.go`), whereas mutating it via `os.Setenv` in top-level BASIC code does not affect `App.MaxBody` (unlike `APIKEY`/`BEARERJWT` which read `ENV(...)` on each request).
- **Impact:** Low — process-level env configuration works, and `.http` golden tests currently do not have a mechanism to pass custom per-test environment variables.

### 4. Note: `?STACK OVERFLOW ERROR` reports the `FUNCTION` line

- **Observed:** In `errors/bash_4_stack_overflow.bas`, infinite recursion reports `?STACK OVERFLOW ERROR IN bash_4_stack_overflow.bas:2` (the `FUNCTION Infinite(n)` entry line where `rt.EnterCall` runs) rather than the recursive call site on line 3. Kiro's initial `.stderr` expected line 3; updated to line 2 to match compiler codegen.

### 5. Note: `GET /_health` bypasses global `USE` middleware

- **Location:** `rt/app.go:186-189`
- **Observed:** When no custom `/_health` route is registered, `App.Handler()` responds `200 {"status": "ALIVE"}` before dispatching through `a.mux` and global `USE` middleware. Verified by `http/bash_14_health_middleware.bas`. This is desirable for unauthenticated container/load-balancer health checks.
