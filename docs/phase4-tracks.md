# Phase 4 wave: nine tracks in parallel (3 Claude Code + 3 Antigravity + 3 Kiro)

Nine of the remaining tasks run at once, each in its own git worktree and branch cut from `phase4-seams`, each touching only the files it owns. The rest (5.5, 6.3–6.6) come later.

`phase4-seams` added:
- **Kiro seams for task 4.3:** `internal/compiler/check/shared.go` defines `checkSharedState(p *ast.Program) diag.List`, called from `Check`; `internal/compiler/codegen/shared.go` defines `(g *gen) sharedDim(o *out, s *ast.DimStmt, sc *scope)`, called from `simpleStmt` in `structured.go` for `SHARED DIM` (see `.kiro/steering/structure.md`);
- Kiro ownership rows for 4.3 and 6.2 in `.kiro/steering/structure.md` and `tasks.md`;
- this file.

## Tracks

| Track | Runner | Branch | Task | Owns |
|---|---|---|---|---|
| **M: Tests in BASIC** | Claude Code | `phase4/track-m-test` | 4.2 `TEST`/`EXPECT`/`REQUEST` and `rip test` | `internal/cli/test.go`, new `internal/cli/test_test.go`, `rt/expect.go`, new `rt/expect_test.go`, new `internal/compiler/codegen/testing.go` (+ `_test.go`), the `*ast.TestDecl`/`*ast.ExpectStmt` branch of `simpleStmt` in `structured.go`, test-mode hooks in `rt/app.go`/`rt/main.go` (`SERVE` must not bind a port under `rip test`), test-build options in `internal/build/` (new file preferred), new goldens `testdata/golden/*/test_*` |
| **N: Playground host** | Claude Code | `phase4/track-n-playground` | 5.2 editor → RUN → live URL, SSE logs, request tester · 5.3 process isolation, memory/time caps, watchdog, loop guards, interop allowlist, FETCH SSRF guard | `internal/playground/**`, `cmd/playground/**`, `deploy/playground.Dockerfile`, `playground/web/src/App.tsx`, `main.tsx`, `basic.ts`, new files in `playground/web/src/`, `playground/web/package.json`, new `rt/playground.go` (+ `_test.go`) for the SSRF dialer that sets the `fetchDialer` hook from `rt/fetch.go` |
| **O: README & language reference** | Claude Code | `phase4/track-o-docs` | 6.1 README: pitch, quick start, language reference, invoicing walkthrough | `README.md`, new `docs/language.md` |
| **P: `rip dev` watch** | Antigravity | `phase4/track-p-dev` | 4.1 `rip dev` (watch, rebuild, restart, keep last good build), check `rip new`/`rip run`/`rip build` against Requirement 13 | `internal/cli/run.go`, new `internal/cli/watch.go`, new `internal/cli/watch_test.go`, `internal/cli/new.go`, `internal/scaffold/**` |
| **Q: Invoicing showcase** | Antigravity | `phase4/track-q-invoicing` | 5.1 Invoicing API example, migrations, tests, deploy config with Postgres | `examples/invoicing/**`, new `deploy/invoicing/**` |
| **R: Benchmark** | Antigravity | `phase4/track-r-bench` | 5.6 req/s, idle RSS, binary size; publish the numbers | new `bench/**` (its own `go.mod`), new `docs/benchmarks.md` |
| **S: CRT theme & samples** | **Kiro** | `kiro/task-5.4-crt` | 5.4 CRT theme and samples | `playground/web/src/styles.css`, `playground/web/src/samples.ts` |
| **T: `SHARED`** | **Kiro** | `kiro/task-4.3-shared` | 4.3 `?SHARED STATE WARNING`, `SHARED DIM` safe for concurrent access | `internal/compiler/check/shared.go` (+ `_test.go`), `internal/compiler/codegen/shared.go` (+ `_test.go`), new `rt/shared.go` (+ `_test.go`), one-line hooks in `structured.go`/`expr.go` only, new goldens `testdata/golden/*/shared_*` |
| **U: Bug bash goldens** | **Kiro** | `kiro/task-6.2-bugbash` | 6.2 golden programs for acceptance criteria of features that are already built (Requirements 1–11, 14.5–14.6) | new goldens `testdata/golden/*/bash_*`, new `docs/bugbash.md` |

## Rules for every track
1. **Don't edit files you don't own.** If you need a change in someone else's file, write it in your final summary so it can be landed at merge time.
2. **Frozen during the wave:** `go.mod`, `go.sum`, `internal/build/deps.go`, `internal/build/build.go`, `ast.go`, `token.go`, `internal/compiler/check/check.go`, `internal/compiler/codegen/{codegen,expr,classic,http,try,db,modules,gointerop}.go`, `structured.go` (except the parts named above), `rt/value.go`, `rt/fetch.go`, `.kiro/specs/**`, `.kiro/steering/**`, `docs/kiro.md`.
3. **Never touch Kiro's files** (see `.kiro/steering/structure.md`): `rt/env.go`, `rt/log.go`, `rt/decimal.go`, `rt/middleware.go`, `rt/fetch.go`, `internal/cli/migrate.go`, `internal/cli/build.go`, `internal/build/migrations.go`, `internal/compiler/check/injection.go`, their tests, plus the files of tracks S, T and U above.
4. **Follow `.kiro/steering/`**, especially `language.md` and `security.md`. Never invent syntax. Where the spec leaves a choice open, pick the simplest option and list it under "Decisions to confirm" in your summary.
5. **Commit to your own branch** in your own worktree. Don't merge, don't push, don't open PRs; merging happens centrally.
6. **Before every commit:** `gofmt -l .` prints nothing; `go vet ./...` passes; your own packages' tests pass. The full `go test ./...` takes ~25 min on this Windows machine, so run only your packages (and `go test ./internal/golden -run '<your prefix>'` for goldens).
7. **Don't tick boxes in `tasks.md`.** They're ticked at closeout.
8. **Kiro tracks:** don't edit `docs/kiro.md`. Put your task-log entry (date, how it started, what the spec and steering contributed, what the hooks/tests caught, what was changed by hand) at the end of your final commit message; it is copied into `docs/kiro.md` at closeout.

## Merge order (planned)
`phase4-seams` → T (4.3) → M (4.2) → U (6.2) → P (4.1) → N (5.2/5.3) → S (5.4) → Q (5.1, its tests run once M lands) → R (5.6) → O (6.1, refreshed last) → `phase4-closeout`.
