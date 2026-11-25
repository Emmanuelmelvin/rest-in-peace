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
| 2.4 ENV, LOG, redaction | `rt/env.go`, `rt/log.go`, `rt/log_test.go`, `rt/env_test.go` |
| 2.5 DECIMAL and ROUND | `rt/decimal.go`, `rt/decimal_test.go` |
| 3.6 SQL injection warning | `internal/compiler/check/injection.go`, `internal/compiler/check/injection_test.go` |
| 5.4 CRT theme and samples | `playground/web/src/styles.css`, `playground/web/src/samples.ts` |

- **Claude Code sessions never edit these files**, and never implement these features anywhere else (for example, no DECIMAL arithmetic in `rt/ops.go`).
- **Seams:** where shared code needs to call into a Kiro file, Claude Code adds a one-line call to a function the Kiro file defines (for example, `rt/ops.go` calls `decimalArith` from `rt/decimal.go`). It ships with a stub body, which Kiro then replaces. The seams are added when Phase 1 is merged, before the Kiro tasks start.
- **Kiro stays inside its own files too.** If a Kiro task needs a change elsewhere, write it down in `docs/kiro.md` and leave it to be done on the Claude Code side.

## Rules
- `rt/` is one flat package. It must not import anything from this repo, because it gets copied into each program's build directory as `ripapp/rt`.
- `internal/compiler/*` must not import `rt`. The compiler emits calls to `rt`; it never runs them.
- Each pipeline stage depends only on earlier stages: token ← lexer ← parser(ast) ← check ← codegen.
- Every new language feature needs at least one golden test in `testdata/golden/`.
