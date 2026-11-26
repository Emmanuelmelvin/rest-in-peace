# Phase 2: three tracks in parallel

Phase 2 (the runtime, tasks 2.1–2.9 in `.kiro/specs/rip-basic/tasks.md`) runs like Phase 1: parallel tracks, each in its own git worktree and branch, each touching only the files it owns. Tasks 2.4 (ENV, LOG) and 2.5 (DECIMAL) are built in Kiro at the same time (see `.kiro/steering/structure.md`).

Every track starts from the branch `phase2-seams`, which added:
- the Kiro seams: `rt/decimal.go` (stubs), native `AS DECIMAL`, and codegen for `LOG`, `ENV`, `ENVREQUIRE`, `DECIMAL()` and `ROUND()`;
- the dispatch the tracks fill in: `laterStmt` in `structured.go` sends `ROUTE`, `ROUTE GROUP`, `USE`, `RESPOND` and `SERVE` to `httpStmt` in `codegen/http.go`, and `TRY` and `THROW` to `errorStmt` in `codegen/try.go`. `classicScan` already scans these statements, and marks `ROUTE … GOSUB` lines as jump targets;
- the built-ins, already registered in `codegen/expr.go`: `PARAM`, `QUERY`, `HEADER` and `BODY` (passed `c`) call stubs in `rt/request.go`; `ERROR` and `HTTPERROR` call stubs in `rt/errors.go`;
- `rt.ErrorResponse(c, e) (status, body)` in `rt/errors.go`: the one call the HTTP server makes to turn an escaped error into a response. It has a minimal working body, so the server works before task 2.2 finishes it;
- `FromPanic`, moved from `rt/main.go` to `rt/errors.go`.

## Wave 1

| Track | Tasks | Owns |
|---|---|---|
| **D: HTTP core** | 2.1 server, routes, `req`, `RESPOND`, RETURN-as-response, 404, body cap, invalid JSON · 2.6 classic per-request GOSUB stack and per-program mutex · 2.9 graceful shutdown, `/_health` | `rt/app.go`, `rt/request.go`, `rt/main.go`, `rt/ctx.go`, new `rt/*_test.go` files for these, `codegen/http.go`, `codegen/classic.go`, new `codegen/http_test.go`, `internal/compiler/check/` |
| **E: errors** | 2.2 `TRY/CATCH/FINALLY`, `THROW`, `ERROR`, `HTTPERROR`, the `RESPOND` signal through `CATCH`/`FINALLY`, panic-to-response mapping with request IDs | `rt/errors.go`, new `rt/errors_test.go`, `codegen/try.go`, new `codegen/try_test.go`, new goldens named `testdata/golden/{structured,errors}/try_*` and `throw_*` |
| **F: HTTP golden harness** | An HTTP mode for the golden runner, plus golden programs for Requirements 6–8 and 14.6 | `internal/golden/`, new `testdata/golden/http/` |

## Rules for every track
1. **Don't edit files you don't own.** If you need a change somewhere else, say so in your summary and it gets made once, at merge time.
2. **Frozen during the wave:** `ast.go` and `token.go`; the builtin table in `codegen/expr.go`; the dispatch in `structured.go` (`laterStmt`, `collectDecls`, `simpleStmt`); the signatures of `httpStmt`, `errorStmt`, `collectTry` and `ErrorResponse`.
3. **Never touch the Kiro files**: `rt/env.go`, `rt/log.go`, `rt/decimal.go` and their tests. Call their functions; don't change them, and don't implement DECIMAL, ENV or LOG anywhere else.
4. **Follow `.kiro/steering/`**, especially `language.md`. Never invent syntax. Where the spec leaves a choice open (for example the status for a too-large body), pick the simplest option and add it to the "Decisions to confirm" table in `requirements.md`, in your summary rather than in the file.
5. **Commit to your own branch.** Don't merge into `main` and don't push; merging happens centrally.
6. **Before every commit:** `gofmt -l .` prints nothing, and `go vet ./...` and `go test ./...` pass.
7. **Don't tick boxes in `tasks.md`**; they're ticked at merge time.

## Merge order
`phase2-seams` (merged, #8) → E (merged, #9) → D (merged, #10) → F (merged, #11) into `main`, then all 20 Wave 1 HTTP golden programs passed end to end (with the 2 Wave 2 goldens `middleware` and `route_group` compiling cleanly and skipping on `NOT IMPLEMENTED`).

## Wave 2 (after the merge)
- **USE middleware plumbing + ROUTE GROUP** (done): `rt/app.go`, `rt/ctx.go`, `rt/request.go`, `internal/compiler/codegen/{http,expr,classic}.go`, `internal/compiler/check/http.go`, plus Kiro seams in `rt/middleware.go` and `github.com/golang-jwt/jwt/v5` in `go.mod` / `internal/build/deps.go`.
- **Built-in middleware (Kiro)**: `APIKEY`, `CORS`, `REQUESTLOG` (task 2.3) and `RATELIMIT`, `BEARERJWT` (task 2.7) in `rt/middleware.go` and `rt/middleware_test.go`.
- **Runtime type checks (task 2.8)**: `AS <TYPE>` checks on dynamic boundaries (`rt/convert.go`, `internal/compiler/codegen/`).
