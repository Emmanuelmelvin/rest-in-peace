# Design — REST in Peace (RIP BASIC)

## 1. Architecture

```
          .bas source files
                 │
   ┌─────────────▼──────────────┐
   │ Compiler (internal/compiler)│
   │ lexer → parser → AST        │
   │ → resolve & check, warnings │
   │ → Go codegen (//line marks) │
   └─────────────┬──────────────┘
                 │ Go source (package main)
   ┌─────────────▼──────────────┐
   │ Build (internal/build)      │
   │ .rip/build/: go.mod, rt/,   │
   │ generated *.go → go build   │
   └─────────────┬──────────────┘
                 │ native binary
   ┌─────────────▼──────────────┐      ┌───────────────────┐
   │ Runtime (rt, linked in)     │◀────▶│ Postgres (pgx) /  │
   │ net/http server, router,    │      │ SQLite (modernc)  │
   │ middleware, errors, SQL,    │      └───────────────────┘
   │ FETCH, LOG, DECIMAL, ENV    │◀────▶ Go packages (interop)
   └─────────────┬──────────────┘
                 │
      ┌──────────┴───────────┐
      │                      │
   rip CLI              Playground host
   (cmd/rip)            (cmd/playground: builds programs,
                         one OS process each, SSE logs)
```

**Key idea:** `rip` is a Go binary that turns BASIC into Go and then calls the Go toolchain. A program is all of a project's `.bas` files compiled together into **one Go `package main`**, plus the runtime package `rt`, linked into one static binary.

## 2. Tech choices

| Area | Choice | Why |
|---|---|---|
| Compiler & CLI | Go, stdlib only, plus `golang.org/x/tools/go/packages` for interop | one language end to end |
| Parser | hand-written lexer and Pratt parser | friendly, precise errors |
| HTTP | `net/http` with Go 1.22+ `ServeMux` patterns | fast, zero dependencies |
| Postgres | `github.com/jackc/pgx/v5` | fastest Go Postgres driver; native `NUMERIC` |
| SQLite | `modernc.org/sqlite` | pure Go, so `CGO_ENABLED=0` static builds still work |
| DECIMAL | `github.com/shopspring/decimal` | de facto standard |
| JWT | `github.com/golang-jwt/jwt/v5` | HS256 verification |
| Logs | `log/slog` JSON handler | stdlib, structured |
| Playground web | Vite, React, CodeMirror 6 | fast to build |
| Deploy | Fly.io or Render; Neon or Supabase Postgres for the demo API | free tiers |

**Rule:** the runtime package `rt` is a single flat package and SHALL NOT import any other package from this repo. It gets embedded into the CLI and copied into each build (§4.1).

## 3. Repository layout

```
/.kiro/
  specs/rip-basic/{requirements,design,tasks}.md
  steering/{product,tech,structure,language,style,security}.md
  hooks/*.kiro.hook
/cmd/
  rip/                 CLI entry point
  playground/          playground host entry point
/internal/
  cli/                 one file per subcommand
  compiler/
    token/ lexer/ ast/ parser/ check/ codegen/   pipeline stages
    diag/              BASIC-style errors and warnings
    compiler.go        Compile(project) → Go files + diagnostics
  build/               build dir, embedded runtime extraction, go build
  scaffold/            `rip new` templates (embedded)
  playground/          host: programs, sandbox processes, SSE, proxy
  golden/              golden test runner
/rt/                   runtime package linked into every program
embed.go               embeds rt/*.go into the CLI (package rip)
/playground/web/       Vite + React + CodeMirror front end
/examples/             hello, classic-todo, goto-hell, invoicing/
/testdata/golden/      *.bas + expected output / errors
```

## 4. Compilation & build

### 4.1 Build directory
`rip run`, `rip dev`, `rip test` and `rip build` all write `.rip/build/` in the project:

```
.rip/build/
  go.mod      module ripapp; requires rt's dependencies + Go interop packages
  go.sum
  rt/         runtime source, copied from the CLI's embedded copy
  main.go     generated: top-level statements, routes, Run()
  m_billing.go, m_main.go, …   one generated file per .bas module
```

- **The runtime is embedded in the CLI** (`embed.go`, `//go:embed rt/*.go`), so a build always uses the runtime that matches the compiler. `_test.go` files are skipped when copying.
- **Dependency versions come from the CLI's own build info** (`runtime/debug.ReadBuildInfo`). The generated `go.mod` pins exactly the pgx, SQLite and decimal versions the CLI was built with. Interop packages are added with `go get`.
- **`go build` does the rest.** Go's build cache makes rebuilds incremental, which keeps `rip dev` fast after the first run. The first build needs the module cache or network access.
- `rip build` sets `CGO_ENABLED=0`, `-trimpath` and `-ldflags "-s -w"`, and supports `--os` and `--arch` for cross-compilation.

### 4.2 Value model (`rt`)

```go
type Value = any // nil | bool | int64 | float64 | string | decimal.Decimal
                 // | time.Time | *rt.Map | *rt.Array | *rt.Func | rt.Host
```

- `*rt.Map` is an insertion-ordered map with string keys, so JSON output keeps the order fields were written in.
- `*rt.Array` wraps `[]Value`.
- `rt.Host` wraps any Go value from interop and gives reflective field access and method calls.
- **Typed declarations compile to native Go types** (Requirement 3.3). `DIM n AS INTEGER` becomes `var n int64`, and arithmetic on two native ints is a plain `+`. Untyped values go through helpers: `rt.Add(a, b)`, `rt.Lt(a, b)`, `rt.Eq(a, b)`, `rt.Concat(a, b)`. This is where the speed comes from, and why types matter.
- `AND` and `OR` compile to `rt.Truthy(a) && rt.Truthy(b)`.

### 4.3 Identifiers & case
- BASIC names are normalized to a canonical case and mangled, so they can't collide with Go keywords or across modules: `billing.bas` + `LineTotal` → `b_billing_linetotal`.
- Member names and map keys are emitted verbatim, as `rt.Get(v, "customerId")`.

### 4.4 Functions and the request context
Every generated function takes `c *rt.Ctx` as its first parameter. `Ctx` carries:
- the `context.Context`
- the current request
- the active transaction
- the call depth, for the stack overflow limit
- the request deadline, for loop guards
- the request-scoped logger

```basic
FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
```
```go
//line main.bas:4
func b_main_hello(c *rt.Ctx, req rt.Value) rt.Value {
	defer c.Enter()()
//line main.bas:5
	return rt.MapOf("message", "HELLO WORLD")
}
```

### 4.5 Error locations: `//line` directives
Before every statement, codegen emits a `//line file.bas:N` directive. The Go compiler then records BASIC positions in the binary. When `rt` recovers a panic, `runtime.Callers` frames already report `billing.bas:42`. No source maps and no runtime line tracking are needed.

### 4.6 Errors, TRY and RESPOND
- `THROW` → `panic(*rt.Error)`. Go runtime panics (nil map, index out of range) are converted to `*rt.Error` with the right kind when recovered.
- `TRY/CATCH/FINALLY` → an immediately invoked closure with `defer`/`recover`. It returns `(ret rt.Value, done bool)` so that `RETURN` and `EXIT` inside a `TRY` reach the enclosing function.
- `RESPOND` → `panic(*rt.Respond)`. Every generated `CATCH` re-panics `*rt.Respond` before user code runs (Requirement 6.7). The handler wrapper turns it into the HTTP response.

### 4.7 Classic mode → native Go `goto`
A classic program compiles to one Go function. All variables are package-level globals, and every BASIC line becomes a label:

```go
func classicMain(c *rt.Ctx, entry int) {
	gosub := c.GosubStack()        // per request (Requirement 7.10)
	switch entry { case 100: goto L100 /* … */ }
L10:
	app.Route("GET", "/hello", 100)
L20:
	app.Serve(nil)
	return
L100:
	rt.RespondNow(c, 200, rt.MapOf("message", "HELLO WORLD"))
}
```

- `GOTO n` → `goto Ln`.
- `GOSUB n` → push a return ID, then `goto Ln`. `RETURN` → pop the ID, then `switch` to its label.
- `ON x GOTO` → a `switch` of `goto`s.
- Go allows jumping *out of* blocks but not into them. That's fine here, because all labels sit at the function's top level and all variables are globals, so no `goto` jumps over a declaration.
- Classic handlers are serialized with a per-program mutex (Requirement 7.10).

### 4.8 Program startup
`main.go` runs each module's top-level statements in import order (`DBCONNECT`, `USE`, `ROUTE`, …). `SERVE` only records the port, and `app.Run()` starts listening after all top-level code has run. Tests call `app.Handler()` instead of `Run()`, so no port is bound (Requirement 12.3).

### 4.9 Databases & transactions
- `rt.DB` is an interface with two drivers: pgx pools and `database/sql` with modernc SQLite. For SQLite, placeholders `$n` are rewritten to `?n`.
- `TRANSACTION` → `rt.Tx(c, func() { … })`. The transaction is stored on `c`, so every `SQL*` call during the block uses it, including calls inside functions it calls (Requirement 9.5). It commits on normal return or `*rt.Respond`, and rolls back on any other panic, then re-panics.
- Postgres `NUMERIC` → `decimal.Decimal` via pgx type registration. SQLite stores decimals as TEXT, and declared `NUMERIC` columns are parsed back into decimals.

### 4.10 Go interop
- `IMPORT x FROM GO "path"` makes the compiler load the package with `go/packages` and read its exported API with `go/types`.
- Each call site gets typed conversion code: `rt.ToString(arg)`, `rt.ToStruct[T](arg)`, and so on.
- Trailing `error` results are checked and raised as `kind = "GO"`.
- `context.Context` parameters receive `c.Context()`.
- Unsupported signatures, such as channels or generic functions, are compile errors with a hint.

### 4.11 Concurrency & shared state
Each request runs on its own goroutine (from `net/http`). Plain module-level variables aren't safe for concurrent writes, so assigning one in a handler triggers the shared-state warning. `SHARED` variables compile to an `rt.Shared` value guarded by a mutex.

### 4.12 Loop guards (playground builds)
With `--guard`, codegen inserts `c.Guard()` at every loop back-edge and `GOTO`. `Guard` checks the request deadline and the iteration budget, and panics with `rt.ErrEternalLoop`, which becomes `508`.

## 5. Playground

```
browser ──HTTP──▶ playground host (cmd/playground)
                  ├─ POST /api/programs        compile + build (warm Go build cache)
                  ├─ GET  /api/programs/:id/logs  SSE
                  └─ /p/:id/*  reverse proxy ──▶ program process (own port)
```

- Programs are built with `--guard` and the interop allowlist, then run as **separate OS processes**:
  - `GOMEMLIMIT` plus an address-space rlimit on Linux, for the 128 MB cap.
  - The 5 s per-request deadline is enforced in the program by `rt` and backed by a host watchdog that kills the process.
  - Each program gets its own SQLite file in a temporary directory.
- Processes are stronger isolation than threads, but still not a hard security boundary. The host holds no secrets of its own and runs in its own container with only `/tmp` writable.
- The `FETCH` SSRF guard runs inside `rt` when `RIP_PLAYGROUND=1`. It checks the resolved IP of every connection, including redirects, through a custom `net.Dialer.Control`.
- Programs expire 7 days after their last deploy.

## 6. Grammar (EBNF, abridged)

```
file        = classicFile | structuredFile ;
structured  = { import | decl | stmt } ;
import      = "IMPORT" ( string
                       | "{" names "}" "FROM" string
                       | ident "FROM" "GO" string
                       | "{" names "}" "FROM" "GO" string ) ;
decl        = [ "EXPORT" ] ( function | sub | const ) | "SHARED" dim ;
function    = "FUNCTION" ident "(" [ params ] ")" [ "AS" type ] block "END" "FUNCTION" ;
params      = param { "," param } ;  param = ident [ "AS" type ] [ "=" expr ] ;
stmt        = dim | let | print | if | for | foreach | while | do | select | try
            | throw | return | respond | route | group | use | serve
            | transaction | dbconnect | log | call | exit ;
try         = "TRY" block "CATCH" ident block [ "FINALLY" block ] "END" "TRY" ;
transaction = "TRANSACTION" [ "ON" ident ] block "END" "TRANSACTION" ;
route       = "ROUTE" method string ( "CALL" ident | "GOSUB" lineno ) ;
group       = "ROUTE" "GROUP" string { route } "END" "GROUP" ;
respond     = "RESPOND" expr "," expr [ "," expr ] ;
expr        = Pratt: OR < AND < NOT < compare < & < + - < * / \ MOD < ^ < unary < postfix ;
postfix     = primary { "." name | "[" expr "]" | "(" [ args ] ")" } ;
```

## 7. Showcase — Invoicing API
The sources live in `examples/invoicing/` (`billing.bas`, `main.bas`, `migrations/`, `tests/`). The demo covers:
- DECIMAL money math in `billing.bas`, tested in isolation
- API-key middleware on `/v1/*`
- a transaction that writes an invoice and its items
- `422` for a non-positive quantity and `404 INVOICE IS DEAD` for a missing invoice

## 8. Security design
- Parameterized SQL only (Requirement 9.3), plus a compile-time injection warning (9.4).
- Secrets come from `ENV` and are redacted in logs (10.5). Internal error details are never sent to clients (6.5).
- A body-size cap (7.9) and built-in rate limiting (8.3).
- Playground: a process per program with memory and time caps, a watchdog, loop guards, an interop allowlist, the SSRF guard for `FETCH`, SQLite per program, and no host env or files. The host runs in its own container with no secrets.
