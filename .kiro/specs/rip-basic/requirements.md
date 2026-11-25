# Requirements — REST in Peace (RIP BASIC)

> *A 1975 language, raised from the grave, rebuilt for production backends.*

**Hackathon:** Kiroween 2026 · **Category:** Resurrection (also targeting Best Startup Project)
**Deadline:** Fri 4 Dec 2026, 2:00 PM PT (11:00 PM WAT)

## Introduction

REST in Peace (RIP BASIC) is a modern BASIC for building real backend services: business logic, databases, auth, calls to other APIs, and tests. It **compiles to Go** and builds into **a single native binary**. That gives small memory use, millisecond startup, goroutine concurrency and the Go module ecosystem, while developers write readable BASIC.

It has two modes:
- **Classic mode:** line numbers, `GOTO`, `GOSUB`. The nostalgic 1975 feel, still fully working, and compiled to real Go `goto`.
- **Structured mode:** no line numbers, real functions, modules, `TRY/CATCH`. How you'd write production code, the way BASIC itself evolved into QBasic and Visual Basic.

It can be used three ways:
1. **CLI:** `rip new`, `rip dev`, `rip test`, `rip build`. `rip build` produces one static binary that you deploy anywhere.
2. **Playground:** write BASIC in the browser, click RUN, and get a live API URL.
3. **Go interop:** call Go packages (Stripe, AWS SDK, UUID…) from BASIC.

### Goals
- A language people can use for real business APIs, not just a joke.
- Production essentials: functions, modules, error handling, Postgres, transactions, middleware, secrets, tests, logging.
- Errors reported in BASIC terms (`?TYPE ERROR IN billing.bas:42`), never as raw Go panics or stack traces.
- Native-binary performance: low memory, fast startup, high throughput.
- Classic BASIC keeps working fully, so the Resurrection story stays intact.

### Non-goals (v1, on the roadmap)
- Static type checking. v1 checks declared types at runtime and uses them to emit native Go types.
- MySQL, MongoDB and Redis built-ins. In v1 these are reachable through Go interop.
- Background jobs and queues, WebSockets, a package registry for `.bas` modules.
- A Language Server (autocomplete, go-to-definition).

### Glossary
- **Program:** all `.bas` files of one project, compiled together into one Go binary.
- **Handler:** a function (structured mode) or GOSUB target (classic mode) bound to a route.
- **Runtime (`rt`):** the Go package that generated code calls for values, HTTP, SQL, errors and logs.

---

## Requirement 1 — Source files & modes

**User Story:** As a developer, I want my files parsed in the mode I wrote them in, so that both classic and structured BASIC work.

#### Acceptance Criteria
1. Source files SHALL use the `.bas` extension and UTF-8.
2. IF the first non-comment line of a file begins with a line number, THEN THE SYSTEM SHALL parse the file in classic mode. Otherwise it SHALL parse it in structured mode.
3. IF a file mixes line-numbered and unnumbered lines, THEN THE SYSTEM SHALL report a compile error.
4. Keywords, variable names and function names SHALL be case-insensitive. Comments are `REM …` or `' …`.
5. Member names after `.` and map keys SHALL be case-sensitive and kept exactly as written. This applies equally to JSON maps and Go values.
6. In classic mode, lines SHALL run in line-number order, a duplicate line number SHALL replace the earlier line, and `:` SHALL separate statements.

## Requirement 2 — Core statements

**User Story:** As a developer, I want the familiar BASIC control flow in both modes.

#### Acceptance Criteria
1. Both modes SHALL support:
   - `LET` (optional) and `PRINT`
   - `IF … THEN … [ELSEIF …] [ELSE …] END IF` and single-line `IF … THEN …`
   - `FOR … TO … [STEP] / NEXT` and `FOR EACH x IN arr / NEXT`
   - `WHILE … / WEND` and `DO … LOOP [UNTIL …]`
   - `SELECT CASE … END SELECT`
   - `EXIT FOR / EXIT WHILE / EXIT DO` and `END`
2. Classic mode SHALL also support `GOTO`, `GOSUB`, `RETURN` and `ON … GOTO`.
3. IF `GOTO` or `GOSUB` appears in a structured-mode file, THEN THE SYSTEM SHALL report a compile error.

## Requirement 3 — Values, types & expressions

**User Story:** As a developer, I want JSON-friendly values and exact money math.

#### Acceptance Criteria
1. Types: `NUMBER` (64-bit float), `INTEGER` (64-bit), `STRING`, `BOOLEAN`, `MAP`, `ARRAY`, `DECIMAL`, `DATE`, `NIL`, and `ANY` (the default).
2. Variables SHALL be dynamic by default. `DIM name AS <TYPE> [= value]` SHALL declare a typed variable. Assigning a value of the wrong type SHALL raise `?TYPE MISMATCH ERROR`.
3. WHEN a variable, parameter or return value is declared `INTEGER`, `NUMBER`, `STRING`, `BOOLEAN` or `DECIMAL`, THE SYSTEM SHALL compile it to a native Go type rather than a boxed dynamic value.
4. `CONST name = value` SHALL declare an unchangeable value.
5. JSON-style literals SHALL be supported: `{"name": "Ada"}`, `[1, 2, 3]`. Access: `u.name`, `u["name"]`, `a[0]` (zero-based). Maps SHALL keep insertion order.
6. Operators:
   - `+ - * / \ ^ MOD`, `= <> < > <= >=`, `AND OR NOT`.
   - `&` always concatenates. `+` concatenates when either operand is a `STRING`.
   - `AND` and `OR` SHALL short-circuit.
7. Money-safe arithmetic:
   1. `DECIMAL("19.99")` SHALL produce an exact decimal value supporting `+ - * /` and all comparison operators.
   2. `ROUND(x, places)` SHALL round it.
   3. Mixing `DECIMAL` and `NUMBER` SHALL convert to `DECIMAL`.
8. Reading a missing map key SHALL return `NIL`. Reading a member of `NIL` SHALL raise `?NIL ACCESS ERROR` naming the member.
9. `LEN(NIL)` SHALL return `0`.

## Requirement 4 — Functions & subroutines

**User Story:** As a developer, I want real functions so business logic is reusable and testable.

#### Acceptance Criteria
1. `FUNCTION Name(param [AS TYPE] [= default], …) [AS TYPE]` … `END FUNCTION`, with `RETURN value`.
2. `SUB Name(params)` … `END SUB` for procedures with no return value.
3. Variables assigned inside a function or sub SHALL be local to it.
4. Functions SHALL be callable before their definition in the same file.
5. Recursion SHALL be supported, with a depth limit (default 10,000) that raises `?STACK OVERFLOW ERROR`.

## Requirement 5 — Modules & Go interop

**User Story:** As a developer, I want to split code into files and use Go packages, so that I'm not limited to what the language ships with.

#### Acceptance Criteria
1. `EXPORT FUNCTION`, `EXPORT SUB` and `EXPORT CONST` SHALL make a declaration importable.
2. `IMPORT "./billing.bas"` SHALL import all exports. `IMPORT { CalcTax, CalcTotal } FROM "./billing.bas"` SHALL import named ones.
3. IF a name is used from another file without being exported and imported, THEN THE SYSTEM SHALL report a compile error.
4. `IMPORT uuid FROM GO "github.com/google/uuid"` SHALL import a Go package. `IMPORT { NewString } FROM GO "github.com/google/uuid"` SHALL import named exported functions.
5. WHEN BASIC calls a Go function, THE SYSTEM SHALL convert arguments to the Go parameter types using the package's type information:
   - BASIC maps SHALL convert to Go structs by field name, case-insensitively or by `json` tag.
   - A `context.Context` parameter SHALL receive the request's context automatically.
6. WHEN a Go function returns `(value, error)` and the error is non-nil, THE SYSTEM SHALL raise a catchable error with `e.kind = "GO"`.
7. Go structs and pointers SHALL be usable from BASIC through field access and method calls. `NEW(pkg.Type)` SHALL construct a zero value.
8. Each request SHALL run on its own goroutine, so blocking I/O blocks only that request. BASIC code never manages concurrency.

## Requirement 6 — Error handling

**User Story:** As a developer, I want structured errors and clean HTTP error responses.

#### Acceptance Criteria
1. `TRY` … `CATCH e` … `[FINALLY` …`] END TRY`.
2. `THROW ERROR("message")` and `THROW HTTPERROR(status, "message" [, details])`.
3. The caught value `e` SHALL expose `e.message`, `e.kind`, `e.file`, `e.line`, and for HTTP errors `e.status`.
4. IF an `HTTPERROR` escapes a handler, THEN the response SHALL use its status and `{"error": message, "details": …}`.
5. IF any other error, including a Go panic, escapes a handler, THEN the response SHALL be `500` with `{"error": "INTERNAL ERROR", "requestId": …}`. The full error SHALL be logged, and SHALL NOT be sent to the client in production.
6. All error messages SHALL use the classic form plus a hint and the BASIC source location: `?SYNTAX ERROR IN billing.bas:42 — expected THEN after IF condition`.
7. `RESPOND` SHALL NOT be catchable by `CATCH`. It SHALL run `FINALLY` blocks and count as a normal exit for `TRANSACTION`.

## Requirement 7 — HTTP server

**User Story:** As a backend developer, I want to declare routes in BASIC and serve them.

#### Acceptance Criteria
1. Routes:
   - Structured mode: `ROUTE <METHOD> "<path>" CALL <FunctionName>`.
   - Classic mode: `ROUTE <METHOD> "<path>" GOSUB <line>`.
   - Methods: `GET POST PUT PATCH DELETE`. Paths support `:params` and a trailing `*`.
2. Structured handlers SHALL receive one argument, `req`, with these fields:
   - `req.method`, `req.path`, `req.params`, `req.query`, `req.headers` (lowercase keys) and `req.id`.
   - `req.body`: parsed JSON, `{}` when there is no body.
   - `req.ctx`: a map that middleware can write to.
3. In classic mode, `PARAM()`, `QUERY()`, `HEADER()` and `BODY()` SHALL read the current request.
4. `RESPOND <status>, <value> [, <headersMap>]` SHALL send the response and end the handler. In classic mode, it ends the GOSUB.
5. A structured handler MAY instead `RETURN value`, which SHALL respond `200` with it.
6. `SERVE [port]` SHALL start the server after all top-level statements have run. The port defaults to `ENV("PORT")` or `8080`.
7. `ROUTE GROUP "/v1"` … `END GROUP` SHALL prefix the routes inside it.
8. Unmatched routes SHALL return `404 {"error": "ROUTE NOT FOUND IN THIS REALM"}`.
9. Request body size SHALL be capped (default 1 MB, configurable). IF a body is not valid JSON, THEN the response SHALL be `400 {"error": "BODY IS NOT VALID JSON"}`.
10. Classic mode and concurrency:
    - Each request SHALL get its own GOSUB stack.
    - Classic variables are global, so classic handlers SHALL run one at a time per program.

## Requirement 8 — Middleware

**User Story:** As a team lead, I want auth, CORS, logging and rate limits applied consistently.

#### Acceptance Criteria
1. `USE <FunctionName>` SHALL register middleware that runs, in order, before every handler. `USE <FunctionName> ON "/admin/*"` scopes it to a path.
2. Middleware SHALL receive `req`. IF it calls `RESPOND` or throws, THEN the handler SHALL NOT run. Otherwise the next middleware runs.
3. Built-in middleware:
   - `CORS(options)`, `RATELIMIT(perMinute)` and `REQUESTLOG()`.
   - `APIKEY(envName)`, which checks `x-api-key` against a secret.
   - `BEARERJWT(envSecret)`, which verifies an HS256 JWT and sets `req.ctx.user`.

## Requirement 9 — Databases

**User Story:** As a team lead, I want Postgres with transactions and parameterized queries, so that data stays safe and consistent.

#### Acceptance Criteria
1. `DBCONNECT <url> [AS <name>]` SHALL open a connection pool. Supported in v1: `postgres://…` and `sqlite://…` (a file or `:memory:`).
2. Query functions:
   - `SQL(query, args…)` SHALL return an array of row maps.
   - `SQLONE(query, args…)` SHALL return the first row or `NIL`.
   - `SQLEXEC(query, args…)` SHALL return the affected row count.
3. Queries SHALL use placeholders (`$1, $2…`) on every driver. Arguments SHALL be sent as parameters, never concatenated into SQL.
4. IF a query string is built with `+` or `&` from a value that came from `req`, THEN the compiler SHALL warn: `?INJECTION RISK WARNING IN <file>:<line> — use $1 placeholders`.
5. `TRANSACTION [ON <name>]` … `END TRANSACTION`:
   1. The block SHALL commit on normal exit, including `RESPOND`, and roll back if any error escapes it.
   2. Every SQL call made while the block runs SHALL use the transaction, including calls inside functions it calls.
6. `rip migrate` SHALL apply `migrations/*.sql` files in name order, once each, tracked in a `_rip_migrations` table.
7. Database `NUMERIC` and `DECIMAL` columns SHALL map to `DECIMAL` values, not floats.

## Requirement 10 — Config, secrets & logging

**User Story:** As a developer, I want config from the environment and structured logs that never leak secrets.

#### Acceptance Criteria
1. `ENV("NAME" [, default])` SHALL read an environment variable. `ENVREQUIRE("NAME")` SHALL stop startup with a clear error if it is missing.
2. The CLI SHALL load a `.env` file in development only.
3. `LOG <level>, <message> [, <map>]` SHALL write one JSON log line with `level`, `msg`, `requestId`, `time` and the map's fields. Levels: `DEBUG INFO WARN ERROR`.
4. `PRINT` SHALL write plain text to stdout and to the playground log.
5. Values named like secrets (`password`, `token`, `apiKey`, `authorization`) SHALL be redacted in logs.

## Requirement 11 — Outbound HTTP

**User Story:** As a developer, I want to call other APIs from BASIC.

#### Acceptance Criteria
1. `FETCH(url [, options])` SHALL return `{"status", "headers", "body"}`, with JSON bodies parsed. Options: `method`, `headers`, `body`, `timeout` (default 10 s).
2. In the hosted playground, `FETCH` SHALL block private, loopback, link-local and cloud-metadata addresses, including after DNS resolution and redirects, and SHALL cap responses at 1 MB.

## Requirement 12 — Testing

**User Story:** As a developer, I want tests in the same language, so that I can trust changes before deploying.

#### Acceptance Criteria
1. `TEST "name"` … `END TEST` blocks SHALL be allowed in `*.test.bas` files.
2. `EXPECT <condition> [, "message"]` SHALL fail the test with the BASIC source location if the condition is false.
3. `REQUEST(method, path [, body, headers])` SHALL call the app's routes in-process and return `{status, headers, body}`, with no network.
   - The app is the project's entry file (`main` in `rip.json`, default `main.bas`).
   - Its `SERVE` SHALL NOT bind a port during tests.
4. `rip test`:
   - SHALL run all test files against a fresh SQLite `:memory:` database by default, overridable with `TEST_DATABASE_URL`. The `DBCONNECT` URL is replaced in test runs.
   - SHALL load `.env.test` if present.
   - SHALL exit non-zero on failure.

## Requirement 13 — CLI

**User Story:** As a developer, I want one tool to create, run, test and ship services.

#### Acceptance Criteria
| Command | Does |
|---|---|
| `rip new <name>` | scaffold a project: `main.bas`, `rip.json`, `migrations/`, `tests/`, `.env.example`, `Dockerfile`, `/.kiro` steering |
| `rip dev` | compile and run, rebuilding and restarting on file change |
| `rip run <file>` | compile and run once |
| `rip test` | run tests (Requirement 12) |
| `rip build [--os X --arch Y]` | output one static native binary in `dist/` |
| `rip migrate` | apply migrations (9.6) |
| `rip check` | compile only; print errors and warnings |

1. IF the Go toolchain is missing, THEN `rip` SHALL say so and print how to install it.
2. `rip build` SHALL produce a binary that needs no Go toolchain, no `rip` and no source files at runtime.

## Requirement 14 — Performance & reliability

**User Story:** As a hackathon judge, I want proof that this is fast enough for real workloads.

#### Acceptance Criteria
1. Hello world SHALL serve at least 10,000 requests/s on 1 vCPU, measured with `oha` or `wrk`.
2. Idle memory (RSS) of a hello-world binary SHALL be at most 20 MB. Binary size SHALL be at most 30 MB.
3. A slow database call SHALL NOT block other requests.
4. Module-level variables are shared across requests. In structured mode, assigning one inside a handler SHALL produce a compile warning (`?SHARED STATE WARNING`) unless it is declared `SHARED`. `SHARED` variables SHALL be safe for concurrent access.
5. The server SHALL shut down gracefully on `SIGTERM`: stop accepting connections, finish in-flight requests (10 s max), and close DB pools.
6. `GET /_health` SHALL return `200 {"status": "ALIVE"}` unless the app defines its own.

## Requirement 15 — Hosted playground

**User Story:** As a judge or retro fan, I want to write BASIC in the browser and get a live URL.

#### Acceptance Criteria
1. A browser editor (CodeMirror) with BASIC highlighting. RUN compiles, builds and deploys to `https://<host>/p/<programId>/`.
2. Each program SHALL get its own SQLite database and a secrets panel, with values write-only after saving.
3. A live log panel (requests and `PRINT`/`LOG` output) over SSE, and a built-in request tester with copyable `curl`.
4. Program isolation:
   1. Each program SHALL run as a separate OS process with a memory cap (128 MB), a per-request time limit (5 s) and compiled-in loop guards.
   2. WHEN a limit is exceeded, the response SHALL be `508 {"error": "TRAPPED IN AN ETERNAL GOTO LOOP"}`.
   3. IF a process stops responding, THEN the host SHALL kill and restart it.
5. `IMPORT … FROM GO` in the playground SHALL be limited to an allowlist (for example `github.com/google/uuid` and `github.com/shopspring/decimal`). All Go packages are available via the CLI.
6. Samples: *Hello World*, *Classic GOTO Todo*, *Invoicing API*, and *GOTO Hell* (the loop guard demo).
7. A CRT green-phosphor theme with a `READY.` prompt that respects `prefers-reduced-motion`.
8. The playground is a demo environment. Programs expire after 7 days, and production use is via the CLI.

## Requirement 16 — Hackathon compliance

**User Story:** As the team, we want a valid, judge-friendly Kiroween submission.

#### Acceptance Criteria
1. Public GitHub repo, MIT license, `/.kiro` at the root and committed.
2. A deployed playground URL and a deployed Invoicing API URL.
3. README, a demo video under 3 minutes on YouTube, and a Kiro usage write-up.

---

## Decisions to confirm

These were not settled in the original spec. The proposed defaults are used in `design.md` until changed.

| Question | Proposed default |
|---|---|
| How does `DECIMAL` serialize to JSON? | As a string that keeps its scale: `"215.00"` |
| Which rounding mode does `ROUND` use? | Half away from zero |
| `rip test` uses SQLite but production uses Postgres. How do migrations work on both? | Write migrations in the portable SQL subset; `rip check` warns on Postgres-only syntax. Teams can set `TEST_DATABASE_URL` to a real Postgres. |
| What is the Go module path? | Decided: `github.com/Emmanuelmelvin/rest-in-peace` |
| What do classic handlers do under concurrency? | Run one at a time per program (7.10) |
| Should named `.bas` imports use braces, like Go imports? | Yes: `IMPORT { A, B } FROM "./x.bas"`, so braces always mean named imports |
| Where does a classic program's top-level run stop? | At `SERVE` or `END`; later lines run only as GOSUB targets (matches the classic Hello World) |
