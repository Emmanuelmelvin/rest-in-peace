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

## Rules
- `rt/` is one flat package. It must not import anything from this repo, because it gets copied into each program's build directory as `ripapp/rt`.
- `internal/compiler/*` must not import `rt`. The compiler emits calls to `rt`; it never runs them.
- Each pipeline stage depends only on earlier stages: token ← lexer ← parser(ast) ← check ← codegen.
- Every new language feature needs at least one golden test in `testdata/golden/`.
