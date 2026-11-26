---
inclusion: always
---

# Structure

```
cmd/rip/                  CLI entry point (thin: calls internal/cli)
cmd/playground/           playground host entry point
internal/cli/             one file per subcommand: new, dev, run, test, build, migrate, check
internal/compiler/        compiler.go = public entry point (Compile)
  token/ lexer/ ast/ parser/ check/ codegen/ diag/
internal/build/           .rip/build writer, embedded runtime extraction, go toolchain calls
internal/scaffold/        `rip new` templates (embedded with go:embed)
internal/playground/      host: program store, sandbox processes, SSE logs, reverse proxy
internal/golden/          golden test runner over testdata/golden
rt/                       runtime package linked into every compiled program
embed.go                  package rip: embeds rt/*.go for the build step
playground/web/           front end
examples/                 hello.bas, classic-todo.bas, goto-hell.bas, invoicing/
testdata/golden/          <case>.bas + <case>.out (expected stdout) or <case>.err (expected diagnostics)
```

## Who owns what: Kiro tasks vs Claude Code

The tasks marked **[Kiro]** in `tasks.md` are built in Kiro, and everything else with Claude Code. Each side stays out of the other's files, so the history shows clearly which tool built what.

| Kiro task | Files Kiro owns |
|---|---|
| 2.3b / 2.7b Built-in middleware (`APIKEY`, `CORS`, `REQUESTLOG`, `RATELIMIT`, `BEARERJWT`) | `rt/middleware.go`, `rt/middleware_test.go` |
| 2.4 ENV, LOG, redaction | `rt/env.go`, `rt/log.go`, `rt/log_test.go`, `rt/env_test.go` |
| 2.5 DECIMAL and ROUND | `rt/decimal.go`, `rt/decimal_test.go` |
| 3.3 `rip migrate` & embedded migrations | `internal/cli/migrate.go`, `internal/cli/migrate_test.go`, `internal/cli/build.go`, `internal/build/migrations.go` |
| 3.6 SQL injection warning | `internal/compiler/check/injection.go`, `internal/compiler/check/injection_test.go` |
| 3.7 Outbound HTTP (`FETCH`) | `rt/fetch.go`, `rt/fetch_test.go`, `testdata/golden/*/fetch_*` |
| 5.4 CRT theme and samples | `playground/web/src/styles.css`, `playground/web/src/samples.ts` |

- **Claude Code sessions never edit these files**, and never implement these features anywhere else (for example, no DECIMAL arithmetic in `rt/ops.go`).
- **Seams:** where shared code needs to call into a Kiro file, Claude Code adds a one-line call to a function the Kiro file defines (for example, `rt/ops.go` calls `decimalArith` from `rt/decimal.go`). It ships with a stub body, which Kiro then replaces. The seams for tasks 2.3b/2.7b, 2.4 and 2.5 are in place:
  - **2.3b / 2.7b:** `rt/middleware.go` defines `APIKey`, `CORS`, `RequestLog`, `RateLimit` and `BearerJWT`, each returning `rt.Handler`. Codegen compiles `USE APIKEY(…)`, `USE CORS([…])`, `USE REQUESTLOG()`, `USE RATELIMIT(…)` and `USE BEARERJWT(…)` to them via `App.Use(mw, scope)`. `Ctx` exposes `SetResponseHeader(name, value)` (for `CORS`) and `OnFinish(func(status int))` (for `REQUESTLOG`), `Request` exposes `RemoteAddr` (for `RATELIMIT`), and `github.com/golang-jwt/jwt/v5` is pinned in `go.mod` and `internal/build/deps.go` (for `BEARERJWT`).
  - **2.5:** `rt/decimal.go` defines `Decimal` (= `shopspring/decimal.Decimal`) and stubs for `DecimalOf`, `AsDecimal`, `Round`, `decimalArith`, `decimalCompare`, `decimalNeg`, `decimalString` and `decimalJSON`. Each stub's comment names its callers. Codegen compiles `DECIMAL(…)`, `ROUND(…)` and `AS DECIMAL` (a native `rt.Decimal`) to them, and `ops.go` and `value.go` call the hooks for arithmetic, comparisons, `PRINT` and JSON.
  - **2.4:** codegen compiles `LOG` to `rt.Log(c, level, msg, fields)` (fields is a `*Map` or nil) and `ENV`/`ENVREQUIRE` to `rt.Env`/`rt.EnvRequire` in their current signatures. `PRINT` writes to stdout, which the playground host captures as the program's log. The CLI already loads `.env` for `rip run`.
  - **3.6:** `internal/compiler/check/injection.go` defines `checkSQLInjection(p *ast.Program) diag.List`, called from `Check` in `internal/compiler/check/check.go`. Kiro replaces the stub body and adds tests in `internal/compiler/check/injection_test.go`.
- **Kiro stays inside its own files too.** If a Kiro task needs a change elsewhere, write it down in `docs/kiro.md` and leave it to be done on the Claude Code side.

## Rules
- `rt/` is one flat package. It must not import anything from this repo, because it gets copied into each program's build directory as `ripapp/rt`.
- `internal/compiler/*` must not import `rt`. The compiler emits calls to `rt`; it never runs them.
- Each pipeline stage depends only on earlier stages: token ← lexer ← parser(ast) ← check ← codegen.
- Every new language feature needs at least one golden test in `testdata/golden/`.
