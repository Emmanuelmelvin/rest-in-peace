# Phase 3: six tracks in parallel (3 Antigravity sub-agents + 3 Kiro)

Phase 3 (Data & modules, tasks `3.1`–`3.7` in `.kiro/specs/rip-basic/tasks.md`) runs across six isolated git worktrees branched from `phase3-seams`, each touching only the files it owns: **3 Antigravity sub-agents** (`3.1`+`3.2`, `3.4`, `3.5`) and **3 Kiro headless sessions** (`3.3`, `3.6`, `3.7`; see `.kiro/steering/structure.md`).

Every track starts from the branch `phase3-seams`, which added:
- **Pinned Phase 3 modules** in `go.mod` and `internal/build/deps.go`: `github.com/jackc/pgx/v5` (`pgxpool`), `modernc.org/sqlite`, and `golang.org/x/tools` (`go/packages`, compiler-only);
- **Kiro seam for task 3.6:** `internal/compiler/check/injection.go` defines `checkSQLInjection(p *ast.Program) diag.List`, called from `Check` in `check.go`;
- **DB & Transaction seams (tasks 3.1, 3.2):** `laterStmt` in `structured.go` dispatches `*ast.DBConnectStmt` and `*ast.TransactionStmt` to `dbStmt` in `codegen/db.go`; `classicScan` scans them; `SQL`, `SQLONE` and `SQLEXEC` are registered as variadic built-ins in `codegen/expr.go` calling `rt.SQL`, `rt.SQLOne` and `rt.SQLExec` with `c`;
- **`.bas` module seams (task 3.4):** `check.go` calls `checkModules(p)` in `check/modules.go`; `codegen.go` passes `*ast.Program` on `gen`, dispatches non-Go imports to `basImport` in `codegen/modules.go`, and `structuredFile` calls `bindBasImports()` after collecting local declarations;
- **Go interop seams (task 3.5):** `check.go` calls `checkGoImports(p)` in `check/gointerop.go`; `internal/compiler/gointerop/loader.go` provides the `go/packages` loader package; `codegen.go` and `expr.go` dispatch Go imports, extra import lines, package member access, `NEW`/named Go calls, and method calls to `codegen/gointerop.go`; `rt/value.go` calls `hostGet` and `hostSet` in `rt/host.go`; `internal/build/build.go` calls `extraGoModRequires(files)` in `internal/build/gointerop.go`;
- **`FETCH` seam (task 3.7):** `rt/fetch.go` defines `rt.Fetch(c *Ctx, url string, opts ...Value) *Map`, wired in `codegen/expr.go`.

## Tracks (all runnable in parallel)

| Track | Runner | Tasks | Owns |
|---|---|---|---|
| **G: DB & transactions** | Antigravity sub-agent (`phase3/track-g-db`) | `3.1` `DBCONNECT`, `SQL`/`SQLONE`/`SQLEXEC` (pgx + modernc SQLite, `$n` → `?n`, `NUMERIC` → `DECIMAL`, pool shutdown) · `3.2` `TRANSACTION` bound to `Ctx` | `rt/db.go`, `rt/ctx.go`, `rt/app.go` (only if needed — `OnShutdown` already exists), new `rt/db_test.go`, `internal/compiler/codegen/db.go`, new `internal/compiler/codegen/db_test.go`, new goldens `testdata/golden/*/db_*` and `tx_*` |
| **I: BASIC modules** | Antigravity sub-agent (`phase3/track-i-modules`) | `3.4` `IMPORT`/`EXPORT` between `.bas` files, recursive loading, cycle & export visibility checks | `internal/compiler/compiler.go`, new `internal/compiler/compiler_test.go`, `internal/compiler/check/modules.go`, new `internal/compiler/check/modules_test.go`, `internal/compiler/codegen/modules.go`, new `internal/compiler/codegen/modules_test.go`, new goldens `testdata/golden/*/mod_*` |
| **J: Go interop** | Antigravity sub-agent (`phase3/track-j-gointerop`) | `3.5` `IMPORT … FROM GO`: `go/packages` + `go/types`, typed arg conversion, `(val, error)` → `GO` error, `context.Context` injection, `rt.Host` reflection, `NEW()`, extra `go.mod` requires | `internal/compiler/gointerop/`, `internal/compiler/check/gointerop.go`, new `internal/compiler/check/gointerop_test.go`, `internal/compiler/codegen/gointerop.go`, new `internal/compiler/codegen/gointerop_test.go`, `rt/host.go`, new `rt/host_test.go`, `internal/build/gointerop.go`, new `internal/build/gointerop_test.go`, new goldens `testdata/golden/*/gointerop_*` |
| **H: Migrations** | **Kiro** (`kiro/task-3.3-migrate`) | `3.3` `rip migrate` (`_rip_migrations`, `migrations/*.sql` in order, per-file tx) + embedded migrations for `rip build` | `internal/cli/migrate.go`, new `internal/cli/migrate_test.go`, `internal/cli/build.go`, new `internal/build/migrations.go` (if needed for embedding) |
| **L: SQL injection warning** | **Kiro** (`kiro/task-3.6-injection`) | `3.6` `?INJECTION RISK WARNING` for SQL strings built from `req` | `internal/compiler/check/injection.go`, `internal/compiler/check/injection_test.go` |
| **K: Outbound HTTP** | **Kiro** (`kiro/task-3.7-fetch`) | `3.7` `FETCH(url [, options])` with timeout, JSON parsing, response map | `rt/fetch.go`, new `rt/fetch_test.go`, new goldens `testdata/golden/*/fetch_*` |

## Rules for every track
1. **Don't edit files you don't own.** If you need a change in a frozen file, note it in your summary so it can be landed cleanly at merge time.
2. **Frozen during the wave:** `ast.go`, `token.go`, `go.mod`, `go.sum`, `internal/build/deps.go`, `internal/build/build.go`, `rt/value.go`, `internal/compiler/check/check.go`, `internal/compiler/codegen/{codegen,structured,classic,expr,http,try}.go`, and the seam signatures in `phase3-seams`.
3. **Never touch Kiro's files:** `internal/compiler/check/injection.go`, `internal/compiler/check/injection_test.go`, `internal/cli/migrate.go`, `internal/cli/migrate_test.go`, `internal/cli/build.go`, `rt/fetch.go`, `rt/fetch_test.go`, `rt/env.go`, `rt/log.go`, `rt/decimal.go`, `rt/middleware.go` (and their tests).
4. **Follow `.kiro/steering/`**, especially `language.md` and `security.md`. Never invent syntax.
5. **Commit to your own branch.** Don't merge into `main` and don't push; merging happens centrally.
6. **Before every commit:** `gofmt -l .` prints nothing, and `go vet ./...` plus your track's `go test` packages pass.
7. **Don't tick boxes in `tasks.md`**; they're ticked at Phase 3 closeout.

## Merge order
`phase3-seams` (merged, #20) → Kiro **L** (`3.6`, merged, #21) → Kiro **K** (`3.7`, merged, #22) → Kiro **H** (`3.3`, merged, #23) → Track **G** (`3.1`+`3.2`, merged, #24) → Track **I** (`3.4`, merged, #25) → Track **J** (`3.5`, merged, #26) → `phase3-closeout` (#27) into `main`, with all Phase 3 tasks (`3.1`–`3.7`) complete.
