# Implementation Plan — REST in Peace (RIP BASIC)

`P0` = must ship for the demo · `P1` = ship if time allows. Each task lists the requirements it covers.

**[Kiro]** = built in Kiro (see `docs/kiro.md`). Every other task is built with Claude Code. Claude Code sessions never edit the files a Kiro task owns (see `.kiro/steering/structure.md`).

Schedule: Wed 25 Nov → submit on Fri 4 Dec 2026, before 11:00 PM WAT (2:00 PM PT).

Stubs in the code are marked `TODO(task X.Y)`, so `grep -rn "TODO(task 1.2)"` finds the starting point for a task.

## Phase 0 — Scaffold (Wed 25 Nov)

- [x] 0.1 `P0` Repository scaffold: Go module, CLI skeleton, compiler/runtime/build/playground packages, examples, golden test layout, `/.kiro` specs, steering and hooks, MIT license, CI
  - _Requirements: 16.1_
- [x] 0.2 `P0` Set the module path to `github.com/Emmanuelmelvin/rest-in-peace` and push the public repo
  - _Requirements: 16.1_

## Phase 1 — Compiler core (Wed 25 – Fri 27 Nov)

- [x] 1.1 `P0` Lexer with file/line/col positions; keywords; numbers, strings, line numbers; `REM` and `'` comments; mode detection
  - `internal/compiler/lexer`, `internal/compiler/token`
  - _Requirements: 1.1–1.4, 1.6_
- [x] 1.2 `P0` Pratt expression parser: literals, JSON maps and arrays, member/index/call, operator precedence, short-circuit `AND`/`OR`
  - `internal/compiler/parser`, `internal/compiler/ast`
  - _Requirements: 3.5, 3.6, 1.5_
- [x] 1.3 `P0` Structured statements: IF/ELSEIF/ELSE, single-line IF, FOR/FOR EACH, WHILE, DO, SELECT CASE, EXIT
  - _Requirements: 2.1_
- [x] 1.4 `P0` FUNCTION/SUB, params with defaults, locals, CONST, DIM AS TYPE, forward references
  - _Requirements: 3.2, 3.4, 4.1–4.4_
- [x] 1.5 `P0` Go codegen for structured mode: `rt.Value` helpers, native types for typed declarations, `//line` directives, name mangling
  - `internal/compiler/codegen`
  - _Requirements: 3.3, 6.6; design §4.2–4.5_
- [x] 1.6 `P0` Build pipeline: write `.rip/build`, copy embedded `rt`, generate `go.mod` from build info, run `go build`
  - `internal/build`
  - _Requirements: 13; design §4.1_
- [x] 1.7 `P0` Classic mode: labels and native `goto`, GOSUB/RETURN stack, ON…GOTO, duplicate-line replacement
  - _Requirements: 1.6, 2.2, 2.3; design §4.7_
- [x] 1.8 `P0` Classic-style diagnostics with hints, e.g. `?SYNTAX ERROR IN x.bas:3 — expected THEN after IF condition`
  - `internal/compiler/diag`
  - _Requirements: 6.6_
- [x] 1.9 `P0` Golden test runner, and the first 30 golden programs
  - `internal/golden`, `testdata/golden`

## Phase 2 — Runtime (Sat 28 – Sun 29 Nov)

- [x] 2.1 `P0` HTTP server: ROUTE/CALL/GOSUB, `:params` and `*`, `req`, RESPOND, RETURN-as-response, 404 realm message, body cap, invalid-JSON 400
  - `rt/app.go`, `rt/request.go`
  - _Requirements: 7.1–7.6, 7.8, 7.9_
- [x] 2.2 `P0` TRY/CATCH/FINALLY, THROW, HTTPERROR, the RESPOND signal, panic-to-response mapping with request IDs
  - _Requirements: 6.1–6.7_
- [x] 2.3 `P0` USE middleware with path scoping; APIKEY, CORS, REQUESTLOG · **[Kiro: `rt/middleware.go`]**
  - _Requirements: 8.1–8.3_
- [x] 2.4 `P0` ENV/ENVREQUIRE, `.env` in dev, LOG via slog with redaction, PRINT · **[Kiro]**
  - _Requirements: 10.1–10.5_
- [x] 2.5 `P0` DECIMAL (shopspring/decimal): arithmetic, comparisons, ROUND, JSON · **[Kiro]**
  - _Requirements: 3.7_
- [x] 2.6 `P0` Classic per-request GOSUB stack and per-program mutex
  - _Requirements: 7.10_
- [x] 2.7 `P1` ROUTE GROUP; RATELIMIT, BEARERJWT · **[Kiro: `rt/middleware.go`]**
  - _Requirements: 7.7, 8.3_
- [x] 2.8 `P1` Runtime type checks for `AS <TYPE>` on dynamic boundaries
  - _Requirements: 3.2_
- [x] 2.9 `P1` Graceful shutdown, `/_health`
  - _Requirements: 14.5, 14.6_

## Phase 3 — Data & modules (Mon 30 Nov – Tue 1 Dec)

- [x] 3.1 `P0` DBCONNECT, SQL/SQLONE/SQLEXEC for pgx and modernc SQLite; `$n` placeholders on both; NUMERIC → DECIMAL
  - `rt/db.go`
  - _Requirements: 9.1–9.3, 9.7_
- [x] 3.2 `P0` TRANSACTION block bound to `Ctx`, with commit/rollback semantics
  - _Requirements: 9.5_
- [x] 3.3 `P0` `rip migrate` · **[Kiro]**
  - _Requirements: 9.6_
- [x] 3.4 `P0` IMPORT/EXPORT between `.bas` files, resolution, export checks
  - _Requirements: 5.1–5.3_
- [x] 3.5 `P0` IMPORT FROM GO: go/packages loading, typed call conversion, error results, `context.Context` injection, Host values, `NEW()`
  - _Requirements: 5.4–5.7; design §4.10_
- [x] 3.6 `P1` SQL injection compile warning · **[Kiro]**
  - _Requirements: 9.4_
- [x] 3.7 `P1` FETCH with timeout · **[Kiro]**
  - _Requirements: 11.1_

## Phase 4 — CLI & tests (Tue 1 Dec)

- [ ] 4.1 `P0` `rip new`, `rip dev` (watch, rebuild, restart), `rip run`, `rip build` (static binary, cross-compile)
  - _Requirements: 13_
- [ ] 4.2 `P0` TEST/EXPECT/REQUEST and `rip test` (SQLite `:memory:`, `.env.test`, non-zero exit on failure)
  - _Requirements: 12.1–12.4_
- [ ] 4.3 `P1` `rip check`; shared-state warning; `SHARED`
  - _Requirements: 13, 14.4_

## Phase 5 — Showcase & playground (Wed 2 – Thu 3 Dec)

- [ ] 5.1 `P0` Invoicing API example, migrations and tests; deploy the binary with Postgres
  - `examples/invoicing`
  - _Requirements: 16.2_
- [ ] 5.2 `P0` Playground: editor, RUN → live URL, SSE logs, request tester
  - `cmd/playground`, `internal/playground`, `playground/web`
  - _Requirements: 15.1, 15.3_
- [ ] 5.3 `P0` Process isolation, memory and time caps, watchdog, loop guards, interop allowlist, FETCH SSRF guard
  - _Requirements: 15.4, 15.5, 11.2_
- [ ] 5.4 `P0` CRT theme and samples · **[Kiro]**
  - _Requirements: 15.6, 15.7_
- [ ] 5.5 `P1` Secrets panel
  - _Requirements: 15.2_
- [ ] 5.6 `P1` Benchmark (req/s, idle RSS, binary size) and publish the numbers
  - _Requirements: 14.1, 14.2_

## Phase 6 — Polish & submit (Thu 3 – Fri 4 Dec)

- [ ] 6.1 `P0` README: pitch, quick start, language reference, invoicing walkthrough
- [ ] 6.2 `P0` Bug bash against the acceptance criteria; golden suite ≥ 80 programs
- [ ] 6.3 `P0` Kiro usage write-up (`docs/kiro.md`)
- [ ] 6.4 `P0` Demo video under 3 min on YouTube
- [ ] 6.5 `P0` Submit on Devpost **before Fri 4 Dec, 11:00 PM WAT**
- [ ] 6.6 `P1` VS Code / Kiro extension with a TextMate grammar for `.bas` highlighting
