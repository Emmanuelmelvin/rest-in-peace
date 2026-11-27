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

**Phases 0–3 are done.** The compiler, the runtime and the CLI work end to end. You can build and deploy a real API today:

- the compiler: lexer, parser, checks, Go codegen with `//line` directives, classic and structured modes;
- the HTTP runtime: routes, route groups, `RESPOND`, `/_health`, body limits, graceful shutdown;
- middleware: your own, plus `APIKEY`, `CORS`, `RATELIMIT`, `BEARERJWT` and `REQUESTLOG`;
- errors: `TRY`/`CATCH`/`FINALLY`, `THROW`, `HTTPERROR`, BASIC-style messages, no Go stack traces;
- exact `DECIMAL` money math;
- databases: Postgres and SQLite, `$1` placeholders, `TRANSACTION`, `rip migrate`, an SQL injection warning;
- `IMPORT`/`EXPORT` between `.bas` files, and `IMPORT … FROM GO` for any Go package;
- `ENV`, `LOG` with secret redaction, and `FETCH` for outbound HTTP;
- `rip new`, `rip run`, `rip build`, `rip migrate` and `rip check`.

**In progress this week:** `rip dev` watch mode, tests in BASIC (`TEST`, `EXPECT`, `REQUEST`) and `rip test`, `SHARED` variables, the hosted playground, and the benchmark. The plan is in [`tasks.md`](.kiro/specs/rip-basic/tasks.md).

## Quick start

You need [Go 1.26+](https://go.dev/dl/). `rip` turns BASIC into Go and calls the Go toolchain to build it. The binaries it builds don't need Go.

```sh
go install github.com/Emmanuelmelvin/rest-in-peace/cmd/rip@latest
rip new shop && cd shop
rip run                       # READY. listening on :8080
curl localhost:8080/hello     # {"message":"HELLO WORLD"}
rip build                     # dist/shop: one static binary
```

`rip dev` (rebuild and restart on every save) and `rip test` land this week. Until then, `rip dev` runs once, like `rip run`.

| Command | Does |
|---|---|
| `rip new <name>` | scaffold a project (`main.bas`, `rip.json`, `migrations/`, `tests/`, `.env.example`, `Dockerfile`, `.kiro/`) |
| `rip dev [file.bas]` | compile and run, rebuilding and restarting on file change |
| `rip run [file.bas]` | compile and run once |
| `rip test` | run `tests/*.test.bas` against SQLite `:memory:` (or `TEST_DATABASE_URL`) |
| `rip build [--os X] [--arch Y] [-o file]` | build one static binary in `dist/` |
| `rip migrate` | apply `migrations/*.sql` |
| `rip check [file.bas]` | compile only; print errors and warnings |

From a clone:

```sh
go run ./cmd/rip help
go run ./cmd/rip run examples/hello.bas
go test ./...
```

## A short tour

The full reference is [`docs/language.md`](docs/language.md). Here are the highlights.

**Routes, JSON and errors.** Handlers get `req` and return a value, which is sent as JSON. `RESPOND` sets the status and headers. An `HTTPERROR` that escapes a handler becomes the response.

```basic
ROUTE GET "/graves/:id" CALL GetGrave
ROUTE POST "/graves" CALL Dig
SERVE

FUNCTION GetGrave(req)
  IF req.params.id <> "13" THEN THROW HTTPERROR(404, "GRAVE IS EMPTY")
  RETURN {"id": req.params.id, "name": "VLAD"}
END FUNCTION

FUNCTION Dig(req)
  IF req.body.name = NIL THEN THROW HTTPERROR(422, "A GRAVE NEEDS A NAME")
  RESPOND 201, {"name": req.body.name}, {"location": "/graves/13"}
END FUNCTION
```

**Money that adds up.** `NUMBER` is a float, so `0.1 + 0.2` isn't `0.3`. `DECIMAL` is exact, and it serializes to JSON as a string that keeps its scale (`"215.00"`).

```basic
LET subtotal = DECIMAL("19.99") * 3
LET tax = ROUND(subtotal * DECIMAL("0.075"), 2)
PRINT subtotal + tax            ' 64.47
```

**Databases and transactions.** Placeholders are always `$1, $2…`, on Postgres and SQLite. A `TRANSACTION` commits when the block finishes and rolls back if an error escapes it, including SQL run by functions it calls.

```basic
DBCONNECT ENV("DATABASE_URL", "sqlite://:memory:")
SQLEXEC("CREATE TABLE IF NOT EXISTS souls (name TEXT NOT NULL)")

TRANSACTION
  SQLEXEC("INSERT INTO souls (name) VALUES ($1)", "Casper")
  SQLEXEC("INSERT INTO souls (name) VALUES ($1)", "Vlad")
END TRANSACTION

FOR EACH soul IN SQL("SELECT name FROM souls")
  PRINT soul.name
NEXT
```

**Middleware.** Built-ins or your own, scoped to a path.

```basic
USE REQUESTLOG()
USE RATELIMIT(120)
USE APIKEY("API_KEY") ON "/admin/*"
USE Haunt
ROUTE GET "/admin/stats" CALL Stats
SERVE

SUB Haunt(req)
  req.ctx.ghost = "BOO"
END SUB

FUNCTION Stats(req)
  RETURN {"ghost": req.ctx.ghost}
END FUNCTION
```

**Any Go package.** Arguments and results are converted using the package's types, and Go errors become BASIC errors with `e.kind = "GO"`. For a third-party package, the project needs a `go.mod` that requires it (`go mod init shop && go get github.com/google/uuid`).

```basic
IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT strings FROM GO "strings"

PRINT strings.ToUpper("rest in peace")
PRINT uuid.NewString()
```

**Errors in BASIC terms.** At compile time and at runtime, with the BASIC file and line and a hint. Never a Go stack trace.

```text
?SYNTAX ERROR IN main.bas:1 — expected THEN after IF condition
?NIL ACCESS ERROR IN main.bas:2 — cannot read "name" of NIL — check the value exists before using it
?INJECTION RISK WARNING IN main.bas:5 — use $1 placeholders
```

## Walkthrough: the Invoicing API

[`examples/invoicing`](examples/invoicing) is a complete service: API-key auth, exact money math, a Postgres transaction, migrations and tests.

```
examples/invoicing/
  rip.json              {"name": "invoicing", "main": "main.bas"}
  main.bas              routes, middleware, database
  billing.bas           pure business logic
  migrations/001_invoices.sql
  tests/billing.test.bas
  .env.example  .env.test  Dockerfile
```

### 1. Business logic in its own module

`billing.bas` has no HTTP and no SQL, so it's easy to test. It exports two functions. Totals are `DECIMAL`, and a bad quantity throws a `422` that will reach the client as is.

```basic
' billing.bas
EXPORT FUNCTION LineTotal(item) AS DECIMAL
  RETURN ROUND(DECIMAL(item.unitPrice) * item.quantity, 2)
END FUNCTION

EXPORT FUNCTION InvoiceTotals(items AS ARRAY, taxRate AS DECIMAL) AS MAP
  DIM subtotal AS DECIMAL = DECIMAL("0")
  FOR EACH item IN items
    IF item.quantity = NIL OR item.quantity <= 0 THEN THROW HTTPERROR(422, "QUANTITY MUST BE POSITIVE")
    subtotal = subtotal + LineTotal(item)
  NEXT
  LET tax = ROUND(subtotal * taxRate, 2)
  RETURN {"subtotal": subtotal, "tax": tax, "total": subtotal + tax}
END FUNCTION
```

### 2. Wiring: config, middleware, routes

`main.bas` imports `InvoiceTotals`, connects to the database named by `DATABASE_URL` (startup stops with a clear error if it's missing), and reads the tax rate from the environment. Every request is logged, CORS headers are added, and everything under `/v1/` needs an `x-api-key` header that matches `API_KEY`.

```basic
' main.bas
IMPORT { InvoiceTotals } FROM "./billing.bas"

DBCONNECT ENVREQUIRE("DATABASE_URL")
CONST TAX_RATE = DECIMAL(ENV("TAX_RATE", "0.075"))

USE REQUESTLOG()
USE CORS({"origins": ENV("CORS_ORIGIN", "*")})
USE APIKEY("API_KEY") ON "/v1/*"

ROUTE GROUP "/v1"
  ROUTE POST "/invoices"     CALL CreateInvoice
  ROUTE GET  "/invoices/:id" CALL GetInvoice
END GROUP

SERVE
```

### 3. Handlers: validation, a transaction, a 404

`CreateInvoice` validates the body, computes the totals, then writes the invoice and its items in one transaction. If any insert fails, nothing is written. `RESPOND 201` sends the new row. `GetInvoice` returns `404 INVOICE IS DEAD` for a missing invoice.

```basic
FUNCTION CreateInvoice(req)
  LET b = req.body
  IF b.customerId = NIL OR LEN(b.items) = 0 THEN
    THROW HTTPERROR(400, "CUSTOMER AND ITEMS REQUIRED")
  END IF

  LET totals = InvoiceTotals(b.items, TAX_RATE)

  TRANSACTION
    LET inv = SQLONE("INSERT INTO invoices (customer_id, subtotal, tax, total) VALUES ($1, $2, $3, $4) RETURNING *", b.customerId, totals.subtotal, totals.tax, totals.total)
    FOR EACH item IN b.items
      SQLEXEC("INSERT INTO invoice_items (invoice_id, description, quantity, unit_price) VALUES ($1, $2, $3, $4)", inv.id, item.description, item.quantity, item.unitPrice)
    NEXT
  END TRANSACTION

  LOG "INFO", "INVOICE CREATED", {"invoiceId": inv.id, "total": totals.total}
  RESPOND 201, inv
END FUNCTION

FUNCTION GetInvoice(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  inv.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1", inv.id)
  RETURN inv
END FUNCTION
```

The money columns are `NUMERIC(12,2)` in [the migration](examples/invoicing/migrations/001_invoices.sql), so they come back from the database as `DECIMAL`, not floats.

### 4. Run it

```sh
cd examples/invoicing
cp .env.example .env          # set DATABASE_URL to your Postgres, and API_KEY
rip migrate                   # APPLY 001_invoices.sql
rip run                       # READY. listening on :8080
```

```sh
curl -X POST localhost:8080/v1/invoices -H "x-api-key: change-me" \
  -d '{"customerId": 42, "items": [
        {"description": "Oak coffin", "quantity": 1, "unitPrice": "150.00"},
        {"description": "Candles",    "quantity": 4, "unitPrice": "12.50"}]}'
```

```text
201 {"id": 1, "customer_id": 42, "subtotal": "200.00", "tax": "15.00", "total": "215.00", "created_at": "…"}
```

| Request | Response |
|---|---|
| `GET /v1/invoices/1` | `200`, the invoice with its `items` |
| `GET /v1/invoices/999` | `404 {"error": "INVOICE IS DEAD"}` |
| a `quantity` of `-1` | `422 {"error": "QUANTITY MUST BE POSITIVE"}` |
| no `customerId` | `400 {"error": "CUSTOMER AND ITEMS REQUIRED"}` |
| no `x-api-key` | `401 {"error": "UNAUTHORIZED"}` |
| a crash anywhere | `500 {"error": "INTERNAL ERROR", "requestId": "…"}`, details in the log only |

`REQUESTLOG()` writes one JSON line per request, and `LOG` adds its own, tagged with the same request ID: `{"level":"INFO","msg":"INVOICE CREATED","requestId":"…","invoiceId":1,"total":"215.00"}`.

### 5. Test it (coming in this release)

[`tests/billing.test.bas`](examples/invoicing/tests/billing.test.bas) tests the money math directly, and the HTTP layer in-process with `REQUEST`, no network:

```basic
' tests/billing.test.bas
IMPORT { InvoiceTotals } FROM "../billing.bas"

TEST "ADDS TAX CORRECTLY"
  LET t = InvoiceTotals([{"unitPrice": "100.00", "quantity": 2}], DECIMAL("0.075"))
  EXPECT t.total = DECIMAL("215.00")
END TEST

TEST "REJECTS MISSING API KEY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": []})
  EXPECT r.status = 401
END TEST
```

`rip test` runs them against a fresh SQLite `:memory:` database (or `TEST_DATABASE_URL`) with the values in `.env.test`, and exits non-zero on failure.

### 6. Ship it

```sh
rip build --os linux --arch amd64 -o dist/invoicing
```

The result is one static binary with no runtime dependencies. The [`Dockerfile`](examples/invoicing/Dockerfile) builds it and copies it into a distroless image.

## How it works

```
.bas → lexer → parser → checks → Go codegen (//line directives) → go build → native binary
```

- A program is all of a project's `.bas` files, compiled into one Go `package main` plus the runtime package `rt`.
- Typed declarations (`DIM n AS INTEGER`) compile to native Go types. Untyped values go through small runtime helpers.
- Every BASIC statement gets a `//line file.bas:N` directive, so the Go compiler records BASIC positions in the binary. When something goes wrong, the error names the BASIC line, with no source maps.
- Classic programs compile to one Go function where every BASIC line is a label, so `GOTO` really is `goto`.
- Each request runs on its own goroutine. Classic handlers, which share global variables, run one at a time.
- The runtime (`rt/`) is embedded in the `rip` CLI and copied into every build, so a program always links the runtime that matches its compiler. Builds go to `.rip/build/` and use Go's build cache.

Details are in [`design.md`](.kiro/specs/rip-basic/design.md).

## Playground & benchmarks

- **Hosted playground (coming in this release):** write BASIC in a CRT green-phosphor browser editor, click **RUN**, and get a live API URL at `/p/<programId>/` with live SSE logs and a built-in request tester. Each program runs in its own OS process with a 128 MB memory cap, a 5-second request timeout, and compiled-in loop guards (`508 {"error": "TRAPPED IN AN ETERNAL GOTO LOOP"}`).
- **Benchmarks:** throughput (`req/s`), idle memory (RSS), and static binary size numbers will be published in [`docs/benchmarks.md`](docs/benchmarks.md).

## Repository layout

| Path | What |
|---|---|
| `.kiro/` | Kiro specs, steering and hooks |
| `cmd/rip`, `internal/cli` | the `rip` CLI |
| `internal/compiler` | lexer, parser, AST, checks, Go codegen, diagnostics |
| `internal/build` | build directory, embedded runtime, `go build` |
| `internal/scaffold` | `rip new` templates |
| `rt/` | runtime linked into every program |
| `cmd/playground`, `internal/playground`, `playground/web` | hosted playground |
| `examples/` | [`hello.bas`](examples/hello.bas), [`hello-classic.bas`](examples/hello-classic.bas), [`classic-todo.bas`](examples/classic-todo.bas), [`goto-hell.bas`](examples/goto-hell.bas), [`invoicing/`](examples/invoicing) |
| `testdata/golden` | golden test programs |
| `docs/` | [`language.md`](docs/language.md), [`kiro.md`](docs/kiro.md), track plans |

## Built with Kiro

The project is spec-driven. [`requirements.md`](.kiro/specs/rip-basic/requirements.md), [`design.md`](.kiro/specs/rip-basic/design.md) and [`tasks.md`](.kiro/specs/rip-basic/tasks.md) are the source of truth, the steering files in [`.kiro/steering`](.kiro/steering) keep every change on the same language rules, and the hooks run checks as files change. Several runtime and compiler pieces (middleware, ENV and LOG, DECIMAL, `rip migrate`, the injection warning, `FETCH`) were built in Kiro. The write-up is in [`docs/kiro.md`](docs/kiro.md).

## License

[MIT](LICENSE)
