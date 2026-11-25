# REST in Peace

> *A 1975 language, raised from the grave, rebuilt for production backends.*

**RIP BASIC** is a modern BASIC for real backend services. It **compiles to Go** and ships as **one native binary**: small memory use, millisecond startup, goroutines under the hood. You write readable BASIC.

```basic
ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
```

Line-numbered classic BASIC still works too. It compiles to real Go `goto`:

```basic
10 ROUTE GET "/hello" GOSUB 100
20 SERVE
100 RESPOND 200, {"message": "HELLO WORLD"}
```

Built for Kiroween 2026 (Resurrection category).

## Status

🚧 **Scaffold.** The repository structure, CLI, runtime skeleton, build pipeline and specs are in place. The compiler stages are stubs tagged `TODO(task X.Y)`, matching [`.kiro/specs/rip-basic/tasks.md`](.kiro/specs/rip-basic/tasks.md).

## Quick start (once the compiler lands)

Requires **Go 1.24+**.

```sh
go install github.com/Emmanuelmelvin/rest-in-peace/cmd/rip@latest
rip new shop && cd shop
rip dev            # http://localhost:8080/hello
rip test
rip build          # dist/shop: one static binary
```

From a clone:

```sh
go run ./cmd/rip help
go test ./...
```

## What's in the language

- **Two modes:** classic (`GOTO`, `GOSUB`) and structured (`FUNCTION`, `IMPORT`/`EXPORT`, `TRY`/`CATCH`).
- **HTTP built in:** `ROUTE`, `USE` middleware (`APIKEY`, `CORS`, `RATELIMIT`, `BEARERJWT`), `RESPOND`.
- **Data:** `DBCONNECT` to Postgres or SQLite, `SQL`/`SQLONE`/`SQLEXEC` with `$1` placeholders, `TRANSACTION` blocks, `rip migrate`.
- **Money-safe:** `DECIMAL("19.99")` and `ROUND(x, 2)`.
- **Go interop:** `IMPORT uuid FROM GO "github.com/google/uuid"`.
- **Tests in BASIC:** `TEST "…"`, `EXPECT`, `REQUEST(...)`.
- **Errors in BASIC terms:** `?TYPE MISMATCH ERROR IN billing.bas:42 — …`. Never a Go stack trace.

See [`examples/invoicing`](examples/invoicing) for a complete API with auth, a Postgres transaction and tests.

## How it works

```
.bas → lexer → parser → checks → Go codegen (//line directives) → go build → native binary
```

The runtime (`rt/`) is embedded in the `rip` CLI and copied into every build, so a program always links the runtime that matches its compiler. Details are in [`design.md`](.kiro/specs/rip-basic/design.md).

## Repository layout

| Path | What |
|---|---|
| `.kiro/` | Kiro specs, steering and hooks |
| `cmd/rip`, `internal/cli` | the `rip` CLI |
| `internal/compiler` | lexer, parser, AST, checks, Go codegen, diagnostics |
| `internal/build` | build directory, embedded runtime, `go build` |
| `rt/` | runtime linked into every program |
| `cmd/playground`, `internal/playground`, `playground/web` | hosted playground |
| `examples/` | Hello World, Classic GOTO Todo, GOTO Hell, Invoicing API |
| `testdata/golden` | golden test programs |

## Built with Kiro

The spec-driven plan, the steering files and the hooks are all under [`.kiro/`](.kiro). The write-up is in [`docs/kiro.md`](docs/kiro.md).

## License

[MIT](LICENSE)
