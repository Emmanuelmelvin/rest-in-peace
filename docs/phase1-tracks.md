# Phase 1: three tracks in parallel

Phase 1 (the compiler core, tasks 1.1–1.9 in `.kiro/specs/rip-basic/tasks.md`) runs as three tracks at the same time. Each track has its own git worktree and branch, and only touches the files it owns. The AST (`internal/compiler/ast/ast.go`) and the tokens (`internal/compiler/token/token.go`) are the contract between them.

| Track | Built with | Branch | Tasks | Owns |
|---|---|---|---|---|
| **A: front end** | Claude Code | its own session branch | 1.1 lexer, 1.2 expressions, 1.3 statements, 1.4 functions, 1.7a classic-mode parsing, 1.8 error hints | `internal/compiler/lexer/`, `internal/compiler/parser/`, `internal/compiler/check/` |
| **B: back end** | Claude Code | its own session branch | 1.5 Go codegen, 1.7b classic `goto` codegen, plus the `rt` operators that are still stubs | `internal/compiler/codegen/`, `rt/` |
| **C: test harness** | Claude Code | its own session branch | 1.9 golden runner, finishing 1.6, and 25+ new golden programs | `internal/golden/`, `internal/build/`, `testdata/golden/` |

## Rules for every track
1. **Don't edit files you don't own.** If you need a change somewhere else, say so in your summary and it gets made once, at merge time.
2. **`ast.go` and `token.go` are frozen.** If the parser or codegen needs a new node or field, stop and report it; don't edit them on your branch.
3. **Follow `.kiro/steering/`**, especially `language.md`. Never invent syntax.
4. **Commit to your own branch.** Don't merge into `main` and don't push; merging happens centrally.
5. **Before every commit:** `gofmt -l .` prints nothing, and `go vet ./...` and `go test ./...` pass.
6. **Don't tick boxes in `tasks.md`**; they're ticked at merge time.

## Merge order
C (merged) → B → A into `main`, then the full golden suite runs. That's the first time the compiler works end to end: `.bas` → tokens → AST → Go → binary → output compared with the `.out` files.
