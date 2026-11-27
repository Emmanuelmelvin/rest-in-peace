# RIP BASIC language reference

This is the reference for RIP BASIC as it is built today. The rules come from [`.kiro/steering/language.md`](../.kiro/steering/language.md) and [`requirements.md`](../.kiro/specs/rip-basic/requirements.md). When this page and the spec disagree, the spec wins, and the page is the bug.

Features marked **(coming in this release)** are specified but still being built this week: `TEST`/`EXPECT`/`REQUEST` and `rip test`, `SHARED DIM` and the shared-state warning, `rip dev` watch mode, and the hosted playground. Today, the compiler reports them as `?NOT IMPLEMENTED ERROR`.

Every example on this page passes `rip check`, except the ones marked as coming in this release and the ones that show an error or warning on purpose.

## Contents

1. [Source files and modes](#1-source-files-and-modes)
2. [Statements](#2-statements)
3. [Values, types and expressions](#3-values-types-and-expressions)
4. [Functions and subs](#4-functions-and-subs)
5. [Modules and Go interop](#5-modules-and-go-interop)
6. [Errors](#6-errors)
7. [HTTP](#7-http)
8. [Middleware](#8-middleware)
9. [Databases](#9-databases)
10. [Config, secrets and logging](#10-config-secrets-and-logging)
11. [Outbound HTTP: FETCH](#11-outbound-http-fetch)
12. [Testing](#12-testing)
13. [CLI](#13-cli)
14. [Diagnostics and warnings](#14-diagnostics-and-warnings)

---

## 1. Source files and modes

Source files use the `.bas` extension and UTF-8. Each file is in one of two modes:

- **Classic mode:** the first non-comment line starts with a line number.
- **Structured mode:** everything else.

A file can't mix the two. If it does, you get a `?SYNTAX ERROR` on the first line that doesn't fit.

```basic
10 REM CLASSIC MODE: EVERY LINE HAS A NUMBER
20 PRINT "HELLO FROM 1975"
30 GOTO 50
40 PRINT "SKIPPED"
50 END
```

```basic
' Structured mode: no line numbers.
PRINT "HELLO FROM TODAY"
```

### Lexical rules

- Keywords, variable names and function names are case-insensitive. `PRINT`, `print` and `Print` are the same, and so are `Total` and `TOTAL`.
- Member names after `.` and map keys are case-sensitive and kept exactly as written: `user.Name` and `user.name` are different keys.
- Comments start with `REM` or `'` and run to the end of the line.
- Strings use double quotes. `\"` is a quote and `\\` is a backslash.
- Numbers: `42`, `3.14`, `1e6`.
- Literals: `TRUE`, `FALSE`, `NIL`, maps `{"k": v}`, arrays `[a, b]`.

```basic
REM A REM COMMENT
' An apostrophe comment
let Total = 1
print TOTAL + total       ' prints 2
PRINT "SAY \"HI\""
PRINT 1e3 + 5
```

### Classic mode

- Lines run in line-number order, whatever order they appear in the file.
- A duplicate line number replaces the earlier line, as on a 1975 terminal.
- `:` separates statements on one line. In structured mode, put each statement on its own line.

```basic
10 PRINT "FIRST"
30 PRINT "THIRD"
20 PRINT "OLD LINE 20"
20 PRINT "NEW LINE 20"
40 A = 1 : B = 2 : PRINT A + B
```

Some features belong to one mode only:

| Classic mode only | Structured mode only |
|---|---|
| `GOTO`, `GOSUB`, `RETURN` from GOSUB, `ON … GOTO` | `FUNCTION`, `SUB`, `EXPORT`, `IMPORT` |
| `ROUTE … GOSUB n` | `ROUTE … CALL Fn`, `USE` |
| `PARAM()`, `QUERY()`, `HEADER()`, `BODY()` | `TRY … CATCH`, `TRANSACTION`, `TEST` |
| | `:` is a syntax error |

Everything else (`PRINT`, `IF`, loops, `SELECT CASE`, maps and arrays, built-ins, `THROW`, `DBCONNECT` and `SQL`, `RESPOND`, `SERVE`, `ROUTE GROUP`, `LOG`) works in both.

---

## 2. Statements

### Assignment and PRINT

`LET` is optional. `PRINT` writes its values and a newline to stdout. Separate several values with `;`; they are joined with no space between them.

```basic
LET a = 7
b = 3
PRINT a
PRINT "TOTAL: "; a + b
PRINT "TOTAL: " & a + b
```

### DIM and CONST

`DIM name AS TYPE [= value]` declares a typed variable (see [Types](#types)). Without a value, it starts at the type's zero value: `0`, `""`, `FALSE`. `CONST` declares a value that can't change.

```basic
DIM count AS INTEGER
DIM name AS STRING = "Ada"
CONST RATE = 3
count = count + 5
PRINT name; " "; count * RATE
```

`SHARED DIM` is described under [Concurrency and shared state](#concurrency-and-shared-state).

### IF

Block form, with optional `ELSEIF` and `ELSE`:

```basic
x = 85
IF x >= 90 THEN
  PRINT "A"
ELSEIF x >= 80 THEN
  PRINT "B"
ELSE
  PRINT "C"
END IF
```

Single-line form, with no `END IF` and no `ELSE`. In classic mode, every `:`-separated statement after `THEN` belongs to the `IF`. To jump, write `THEN GOTO 100`, not `THEN 100`.

```basic
x = 5
IF x > 3 THEN PRINT "BIG"
IF x = 5 THEN x = x + 1
```

### FOR and FOR EACH

`FOR i = a TO b [STEP s] … NEXT`. The loop runs zero times if the range is empty. `FOR EACH x IN array … NEXT` walks an array.

```basic
FOR i = 10 TO 0 STEP -5
  PRINT i
NEXT

FOR EACH name IN ["Ada", "Grace", "Linus"]
  PRINT "HELLO, " & name
NEXT
```

### WHILE and DO

`WHILE cond … WEND` checks first. `DO … LOOP [UNTIL cond]` runs the body at least once; without `UNTIL` it loops until `EXIT DO`. `DO WHILE` and `LOOP WHILE` don't exist; use `WHILE … WEND` or `LOOP UNTIL NOT (…)`.

```basic
n = 1
WHILE n < 100
  n = n * 3
WEND

DO
  n = n - 10
LOOP UNTIL n < 0
PRINT n
```

### SELECT CASE

Each `CASE` lists one or more values. `CASE ELSE` is optional. Cases don't fall through.

```basic
FOR EACH cmd IN ["start", "stop", "quit", "other"]
  SELECT CASE cmd
    CASE "start"
      PRINT "GO"
    CASE "stop", "quit"
      PRINT "HALT"
    CASE ELSE
      PRINT "WHAT?"
  END SELECT
NEXT
```

### EXIT and END

`EXIT FOR`, `EXIT WHILE` and `EXIT DO` leave the innermost loop of that kind. `END` stops the program's top-level run.

```basic
FOR i = 1 TO 10
  IF i = 4 THEN EXIT FOR
  PRINT i
NEXT
PRINT "STOPPED AT "; i
END
```

### GOTO, GOSUB, RETURN and ON … GOTO (classic mode)

They compile to real Go `goto`. `GOSUB n` jumps to line `n`, and `RETURN` comes back to the statement after the `GOSUB`. `ON x GOTO a, b, c` jumps to the x-th line in the list, and falls through when x is out of range.

```basic
10 X = 1
20 ON X GOTO 100, 200
30 PRINT "OUT OF RANGE"
40 END
100 GOSUB 300
110 X = 2
120 GOTO 20
200 PRINT "TWO"
210 X = 3
220 GOTO 20
300 PRINT "ONE, FROM A SUBROUTINE"
310 RETURN
```

A `RETURN` with no matching `GOSUB` raises `?RETURN WITHOUT GOSUB ERROR`. Using `GOTO` or `GOSUB` in a structured file is a compile error.

---

## 3. Values, types and expressions

### Maps and arrays

Values are JSON-friendly. Maps keep insertion order, so JSON output keeps the order you wrote. Arrays are zero-based.

```basic
user = {"name": "Ada", "age": 36, "langs": ["BASIC", "Go"]}
PRINT user.name
PRINT user["age"]
PRINT user.langs[1]

user.city = "London"
user["age"] = 37
PRINT user
```

`PRINT user` prints the map as JSON: `{"name":"Ada","age":37,"langs":["BASIC","Go"],"city":"London"}`.

### NIL and missing keys

- Reading a missing map key gives `NIL`.
- Reading a member of `NIL` raises `?NIL ACCESS ERROR` naming the member.
- `LEN(NIL)` is `0`, and `NIL` prints as an empty string.
- An array index out of range raises `?BAD SUBSCRIPT ERROR`.

```basic
m = {"a": 1}
PRINT m.b = NIL          ' TRUE
PRINT LEN(m.b)           ' 0
IF m.b = NIL THEN PRINT "NO B"
```

### Types

| Type | Values | Native Go type when declared |
|---|---|---|
| `NUMBER` | 64-bit float | `float64` |
| `INTEGER` | 64-bit integer | `int64` |
| `STRING` | text | `string` |
| `BOOLEAN` | `TRUE`, `FALSE` | `bool` |
| `DECIMAL` | exact decimal | `decimal.Decimal` |
| `MAP` | `{"k": v}` | dynamic |
| `ARRAY` | `[a, b]` | dynamic |
| `DATE` | reserved; `NOW()` and date values aren't built yet | |
| `NIL` | `NIL` | dynamic |
| `ANY` | anything (the default) | dynamic |

Variables are dynamic unless declared. A declared type is checked at runtime: assigning a value of the wrong type raises `?TYPE MISMATCH ERROR`. Declared `INTEGER`, `NUMBER`, `STRING`, `BOOLEAN` and `DECIMAL` variables, parameters and return values compile to native Go types, which is where much of the speed comes from.

```basic
DIM total AS INTEGER = 10
DIM items AS ARRAY = [1, 2, 3]
DIM price AS DECIMAL = DECIMAL("9.99")

TRY
  total = "ten"
CATCH e
  PRINT e.kind        ' TYPE MISMATCH
END TRY
```

### Operators

From lowest to highest precedence:

| Operators | Meaning |
|---|---|
| `OR` | logical or (short-circuits) |
| `AND` | logical and (short-circuits) |
| `NOT` | logical not |
| `=  <>  <  >  <=  >=` | comparison (`=` is also equality in conditions) |
| `&` | always concatenates |
| `+  -` | add, subtract; `+` concatenates when either side is a `STRING` |
| `*  /  \  MOD` | multiply, divide (always gives a `NUMBER`), integer divide, remainder |
| `^` | power |
| unary `-` | negate |
| `.x  [i]  (args)` | member, index, call |

```basic
PRINT 7 / 2            ' 3.5
PRINT 7 \ 2            ' 3
PRINT 17 MOD 5         ' 2
PRINT 2 ^ 10           ' 1024
PRINT "N=" + 5         ' N=5
PRINT 1 + 2 & 3        ' 33
PRINT 1 + 2 = 3 AND 4 * 2 = 8
```

Comparing values of different kinds (say, a `STRING` with a `NUMBER` using `<`) raises `?TYPE MISMATCH ERROR`. Dividing by zero raises `?DIVISION BY ZERO ERROR`.

### DECIMAL: money-safe arithmetic

`NUMBER` is a binary float, so `0.1 + 0.2` is `0.30000000000000004`. For money, use `DECIMAL`:

- `DECIMAL("19.99")` makes an exact decimal. Pass a string to keep it exact.
- `+ - * /` and all comparisons work on decimals.
- Mixing `DECIMAL` and `NUMBER` gives `DECIMAL`.
- `ROUND(x, places)` rounds half away from zero. `ROUND(x)` rounds to a whole number.
- In JSON, a `DECIMAL` is a string that keeps its scale: `"215.00"`.

```basic
LET price = DECIMAL("19.99")
PRINT price * 3                        ' 59.97
PRINT DECIMAL("0.1") + DECIMAL("0.2")  ' 0.3
PRINT ROUND(DECIMAL("2.345"), 2)       ' 2.35
PRINT {"total": price * 2}             ' {"total":"39.98"}
```

### Built-in functions

| Function | Returns |
|---|---|
| `LEN(x)` | length of a string, array or map; `0` for `NIL` |
| `UPPER(s)`, `LOWER(s)`, `TRIM(s)` | the string changed |
| `LEFT(s, n)`, `RIGHT(s, n)` | the first or last `n` characters |
| `MID(s, start [, n])` | `n` characters from `start` (1-based), or the rest |
| `STR(x)` | `x` as a string |
| `VAL(s)` | the number at the start of `s` |
| `INT(x)` | the largest whole number not greater than `x`, as an `INTEGER` (`INT(-2.5)` is `-3`) |
| `ABS(x)` | absolute value |
| `DECIMAL(x)`, `ROUND(x [, places])` | see [DECIMAL](#decimal-money-safe-arithmetic) |
| `ENV`, `ENVREQUIRE` | see [Config](#10-config-secrets-and-logging) |
| `SQL`, `SQLONE`, `SQLEXEC` | see [Databases](#9-databases) |
| `FETCH` | see [Outbound HTTP](#11-outbound-http-fetch) |
| `ERROR`, `HTTPERROR` | see [Errors](#6-errors) |
| `NEW(pkg.Type)` | see [Go interop](#go-interop) |
| `PARAM`, `QUERY`, `HEADER`, `BODY` | see [Classic routes](#classic-routes) |
| `REQUEST` | see [Testing](#12-testing) |
| `CORS`, `RATELIMIT`, `REQUESTLOG`, `APIKEY`, `BEARERJWT` | see [Middleware](#8-middleware) |
| `READFILE(path)`, `WRITEFILE(path, data)`, `APPENDFILE(path, data)` | read, write, or append UTF-8 files *(blocked in playground)* |
| `FILEEXISTS(path)`, `DELETEFILE(path)`, `MKDIR(path)`, `READDIR(path)`, `FILESTAT(path)` | file & directory helpers *(blocked in playground)* |
| `PATHJOIN(parts...)`, `DIRNAME(p)`, `BASENAME(p)`, `EXTNAME(p)` | cross-platform path manipulation |
| `CWD()`, `ARGS()`, `PID()`, `HOSTNAME()`, `PLATFORM()`, `CPUS()` | OS & process info *(only `PLATFORM`/`CPUS` allowed in playground)* |
| `EXEC(cmd [, args [, opts]])` | run subprocess (`{code, stdout, stderr}`; timeout & 1 MB output cap; *blocked in playground*) |
| `UUID()`, `RANDOMHEX(n)` | RFC 4122 v4 UUID and cryptographically random hex bytes |
| `HASH(algo, data)`, `HMAC(algo, key, data)` | hex digest (`sha256`, `sha512`, `sha1`, `md5`; HMAC: `sha256`, `sha512`) |
| `HASHPASSWORD(pw)`, `VERIFYPASSWORD(pw, hash)` | PBKDF2-SHA256 password hashing with random salt & constant-time verification |
| `BASE64ENCODE(s)`, `BASE64DECODE(s)` | standard Base64 encoding and decoding |
| `NOW()`, `UNIXTIME()`, `UNIXMS()` | UTC RFC3339 timestamp, Unix seconds, and Unix milliseconds |
| `FORMATDATE(ts, layout)`, `PARSEDATE(s [, layout])` | UTC date formatting (`"ISO"`, `"DATE"`, `"TIME"`, or Go layout) and parsing |
| `JSONPARSE(s)`, `JSONSTRINGIFY(v [, indent])` | order-preserving JSON parsing and serialization |
| `SPLIT(s, sep)`, `JOIN(arr, sep)`, `REPLACE(s, old, new)` | string splitting, joining, and replacement |
| `CONTAINS(x, y)`, `STARTSWITH(s, p)`, `ENDSWITH(s, p)`, `INDEXOF(x, y)` | substring / element / map-key search (0-based index, `-1` if absent) |
| `KEYS(m)`, `VALUES(m)`, `PUSH(arr, v)`, `POP(arr)`, `SLICE(arr, start [, end])`, `SORT(arr)` | map & array utilities |
| `REGEXMATCH(pat, s)`, `REGEXREPLACE(pat, s, repl)` | regular expression matching and replacement |
| `SLEEP(ms)` | pause the current goroutine for `ms` milliseconds (respects request cancellation) |

```basic
PRINT LEN("abc")
PRINT UPPER("abc") & LOWER("DEF")
PRINT "[" & TRIM("  hi  ") & "]"
PRINT LEFT("HELLO", 2); MID("HELLO", 2, 3); RIGHT("HELLO", 1)
PRINT VAL("42") + 1
PRINT INT(3.7); " "; ABS(-5)
PRINT STR(42) & "!"
```

---

## 4. Functions and subs

`FUNCTION Name(params) [AS TYPE] … END FUNCTION` returns a value with `RETURN value`. `SUB Name(params) … END SUB` returns nothing; `RETURN` with no value leaves it early. Calling a `SUB` is a statement on its own line.

- A parameter is `name [AS TYPE] [= default]`.
- Variables assigned inside a function or sub are local to it, even when a module-level variable has the same name.
- Functions can be called before they are defined in the file.
- Recursion works, up to 10,000 nested calls. Past that you get `?STACK OVERFLOW ERROR`.
- Functions and subs are declared at the top level of a file, not inside other blocks.

```basic
PRINT Greet("Ada")
PRINT Greet("Grace", "GOOD EVENING")
PRINT Fact(10)
Shout("boo")

FUNCTION Greet(name AS STRING, greeting = "HELLO") AS STRING
  RETURN greeting & ", " & name
END FUNCTION

FUNCTION Fact(n AS INTEGER) AS INTEGER
  IF n <= 1 THEN RETURN 1
  RETURN n * Fact(n - 1)
END FUNCTION

SUB Shout(s)
  IF s = "" THEN RETURN
  PRINT UPPER(s) & "!"
END SUB
```

A function can read module-level variables and constants. It can't reassign them (that makes a local), but it can change what's inside a module-level map or array:

```basic
CONST TAX = 10
LET stats = {"calls": 0}

FUNCTION WithTax(amount)
  stats.calls = stats.calls + 1
  RETURN amount + TAX
END FUNCTION

PRINT WithTax(100)
PRINT stats.calls
```

---

## 5. Modules and Go interop

### Modules

A program is all the `.bas` files reachable from its entry file, compiled together. Mark a declaration with `EXPORT` to make it importable. Paths are relative to the importing file.

```basic
' pricing.bas
EXPORT CONST TAX_RATE = DECIMAL("0.10")

EXPORT FUNCTION CalcTax(amount)
  RETURN ROUND(DECIMAL(amount) * TAX_RATE, 2)
END FUNCTION

EXPORT SUB Greet(name)
  PRINT "HELLO, " & name
END SUB

FUNCTION secretHelper(x)
  RETURN x * 2
END FUNCTION
```

```basic
' shop.bas
IMPORT { CalcTax, Greet } FROM "./pricing.bas"

Greet("ADA")
PRINT CalcTax("50.00")
```

```basic
' shop_all.bas
IMPORT "./pricing.bas"

PRINT TAX_RATE
PRINT CalcTax("200.00")
```

- `IMPORT "./x.bas"` imports every export. `IMPORT { A, B } FROM "./x.bas"` imports the named ones. Braces always mean named imports.
- Using a name that another file doesn't export is a compile error: `?UNDEFINED ERROR … SECRETHELPER is not exported by ./pricing.bas; add EXPORT before FUNCTION SECRETHELPER`.
- A circular `IMPORT` is a compile error; move the shared code into a third file.
- Each module's top-level statements run once, in import order, before the entry file's.

### Go interop

`IMPORT name FROM GO "path"` imports a Go package. `IMPORT { Fn, … } FROM GO "path"` imports named exported functions, which you then call without a prefix. Any Go package works from the CLI, standard library or third party.

Standard-library packages need nothing. For now, a third-party package must be resolvable from the project directory, so give the project a `go.mod` that requires it:

```sh
go mod init shop
go get github.com/google/uuid
```

```basic
IMPORT strings FROM GO "strings"
IMPORT url FROM GO "net/url"
IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT { NewString } FROM GO "github.com/google/uuid"

PRINT strings.ToUpper("rest in peace")
PRINT strings.Join(strings.Split("a,b,c", ","), "-")
PRINT LEN(NewString())

u = NEW(url.URL)
u.Scheme = "https"
u.Host = "rip.dev"
u.Path = "/v1/hello"
PRINT u.String()

TRY
  id = uuid.Parse("not-a-uuid")
CATCH e
  PRINT e.kind        ' GO
END TRY
```

How values cross the boundary:

- Arguments are converted to the Go parameter types, using the package's type information.
- A BASIC map converts to a Go struct by field name (case-insensitive) or by `json` tag.
- A `context.Context` parameter gets the current request's context automatically; you don't pass it.
- A Go function that returns `(value, error)` gives you the value. A non-nil error raises a catchable error with `e.kind = "GO"`.
- Go structs and pointers come back as values you can read fields from and call methods on. `NEW(pkg.Type)` makes a zero value.
- Signatures that can't be converted, such as callbacks, channels or generic functions, are compile errors with a hint: wrap them in a small Go helper.

Each request runs on its own goroutine, so a slow Go call blocks only its own request. BASIC code never manages concurrency.

In the hosted playground (coming in this release), `IMPORT … FROM GO` is limited to an allowlist such as `github.com/google/uuid` and `github.com/shopspring/decimal`.

---

## 6. Errors

### TRY, CATCH, FINALLY and THROW

```basic
TRY
  PRINT "OPENING THE CRYPT"
  THROW ERROR("THE CRYPT IS SEALED")
CATCH e
  PRINT e.kind; ": "; e.message; " AT "; e.file; ":"; e.line
FINALLY
  PRINT "CLOSING THE CRYPT"
END TRY
```

- `THROW ERROR("message")` raises an error with `e.kind = "USER"`.
- `THROW HTTPERROR(status, "message" [, details])` raises an error with `e.kind = "HTTP"`, which becomes an HTTP response if it escapes a handler.
- `THROW e` rethrows a caught error. You can change `e.message` first.
- `FINALLY` always runs, whether the `TRY` finished, threw, returned or exited a loop.
- `RETURN` and `EXIT` inside `TRY` and `CATCH` leave the enclosing function or loop, after `FINALLY` runs. `RETURN` inside `FINALLY` is a compile error.
- Runtime errors such as division by zero, a member of `NIL`, a bad subscript and Go panics are all caught the same way, with their own `e.kind`.

The caught value has these fields:

| Field | Value |
|---|---|
| `e.message` | the message |
| `e.kind` | `"USER"`, `"HTTP"`, `"GO"`, `"TYPE MISMATCH"`, `"NIL ACCESS"`, `"DIVISION BY ZERO"`, … |
| `e.file`, `e.line` | the BASIC source location |
| `e.status` | HTTP errors only; `NIL` otherwise |
| `e.details` | HTTP errors only, when given |

```basic
FUNCTION LoadInvoice(id)
  TRY
    THROW HTTPERROR(404, "INVOICE IS DEAD", {"id": id})
  CATCH e
    e.message = "LOADING: " & e.message
    THROW e
  END TRY
END FUNCTION

TRY
  LoadInvoice(7)
CATCH err
  PRINT err.status; " "; err.message; " "; err.details.id
END TRY
```

`TRY` is structured-mode only. In classic mode, `THROW` works, and an uncaught error stops the program.

### Errors in handlers

- An `HTTPERROR` that escapes a handler responds with its status and `{"error": message, "details": details}`. `"details"` is left out when there are none.
- Any other error, including a Go panic, responds `500 {"error": "INTERNAL ERROR", "requestId": "…"}`. The full error, with its BASIC file and line, goes to the log under that request ID, never to the client.
- `RESPOND` is not an error. `CATCH` never catches it, `FINALLY` still runs, and a surrounding `TRANSACTION` commits.

### Uncaught errors outside handlers

An error that escapes top-level code prints one line to stderr and the program exits with status 1. Never a Go stack trace:

```text
?NIL ACCESS ERROR IN main.bas:2 — cannot read "name" of NIL — check the value exists before using it
```

---

## 7. HTTP

### Routes and handlers

```basic
ROUTE GET "/hello" CALL Hello
ROUTE GET "/graves/:id" CALL ShowGrave
ROUTE POST "/graves" CALL Dig
ROUTE GET "/files/*" CALL ServeFile
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION

FUNCTION ShowGrave(req)
  RETURN {"id": req.params.id, "sort": req.query.sort}
END FUNCTION

FUNCTION Dig(req)
  IF req.body.name = NIL THEN THROW HTTPERROR(422, "A GRAVE NEEDS A NAME")
  RESPOND 201, {"name": req.body.name}, {"location": "/graves/1"}
END FUNCTION

FUNCTION ServeFile(req)
  RETURN {"path": req.path}
END FUNCTION
```

- `ROUTE <METHOD> "<path>" CALL Fn`. Methods: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`.
- Paths support `:name` parameters (one path segment each) and a trailing `*`, which matches the rest of the path.
- A handler is a `FUNCTION` that takes one argument, `req`. `RETURN value` responds `200` with the value as JSON.
- `RESPOND status, value [, headers]` sends the response and ends the handler. Code after it doesn't run.
- `SERVE [port]` starts the server after all top-level statements have run, so routes and middleware can be declared after it. The port defaults to `ENV("PORT")`, then `8080`.

The `req` map:

| Field | Value |
|---|---|
| `req.method`, `req.path` | `"GET"`, `"/graves/13"` (no query string) |
| `req.params` | path parameters, as strings |
| `req.query` | query-string values, as strings |
| `req.headers` | request headers, with lowercase keys |
| `req.body` | the parsed JSON body; `{}` when there is none |
| `req.ctx` | a map that middleware writes to (for example `req.ctx.user`) |
| `req.id` | the request ID, also used in logs and 500 responses |

### Route groups

`ROUTE GROUP "/prefix" … END GROUP` prefixes every route inside it.

```basic
ROUTE GROUP "/v1"
  ROUTE GET "/ping" CALL Ping
  ROUTE GET "/ghosts/:id" CALL GetGhost
END GROUP
SERVE

FUNCTION Ping(req)
  RETURN {"pong": TRUE}
END FUNCTION

FUNCTION GetGhost(req)
  RETURN {"id": req.params.id}
END FUNCTION
```

### Built-in responses

| Situation | Response |
|---|---|
| No route matches (including a known path with another method) | `404 {"error": "ROUTE NOT FOUND IN THIS REALM"}` |
| The body isn't valid JSON | `400 {"error": "BODY IS NOT VALID JSON"}` |
| The body is larger than `RIP_MAX_BODY_BYTES` (default 1 MB) | `413 {"error": "BODY IS TOO LARGE"}` |
| `GET /_health`, unless the app defines its own | `200 {"status": "ALIVE"}` |

On `SIGTERM` (or Ctrl-C), the server stops accepting connections, gives in-flight requests up to 10 seconds to finish, and closes database pools.

### Classic routes

In classic mode, a route jumps to a line with `GOSUB`. `PARAM("name")`, `QUERY("name")`, `HEADER("name")` and `BODY()` read the current request, and `RESPOND` ends the GOSUB. Top-level execution stops at `SERVE` (or `END`); the lines after it run only as route targets.

```basic
10 REM CLASSIC GOTO TODO
20 TODOS = {}
30 NEXTID = 1
40 ROUTE GET "/todos" GOSUB 100
50 ROUTE POST "/todos" GOSUB 200
60 ROUTE PATCH "/todos/:id" GOSUB 300
70 SERVE
100 RESPOND 200, TODOS
200 B = BODY()
210 IF B.title = NIL THEN RESPOND 400, {"error": "A TODO NEEDS A TITLE"}
220 ID = STR(NEXTID)
230 TODOS[ID] = {"id": ID, "title": B.title, "done": FALSE}
240 NEXTID = NEXTID + 1
250 RESPOND 201, TODOS[ID]
300 ID = PARAM("id")
310 IF TODOS[ID] = NIL THEN GOTO 400
320 TODOS[ID].done = TRUE
330 RESPOND 200, TODOS[ID]
400 RESPOND 404, {"error": "THAT TODO IS DEAD"}
```

Classic variables are global, so classic handlers run one at a time per program. Each request gets its own GOSUB stack.

### Concurrency and shared state

Structured handlers run concurrently, each request on its own goroutine. Module-level variables are shared by every request, so writing to them from a handler is a data race.

`SHARED DIM name AS TYPE [= value]`, at the top level of a file, declares a module-level variable that is safe for concurrent access. Changing a plain module-level variable inside a handler produces `?SHARED STATE WARNING IN file:line`.

```basic
SHARED DIM visits AS INTEGER = 0
ROUTE GET "/visit" CALL Visit
SERVE

FUNCTION Visit(req)
  visits = visits + 1
  RETURN {"visits": visits}
END FUNCTION
```

For explicit concurrency and background jobs:

- `SLEEP(ms)` pauses only the current goroutine/request for `ms` milliseconds (and respects request cancellation / playground deadlines).
- `SPAWN Fn(args...)` evaluates `args...` immediately and runs `Fn` in a background goroutine with a detached context and panic recovery *(CLI only — blocked in playground)*.
- `EVERY ms CALL Fn` runs `Fn()` on a recurring interval (`ms >= 10`) in the background and stops cleanly on graceful shutdown *(CLI only — blocked in playground)*.
- `PARALLEL ... END PARALLEL` runs every statement in the block concurrently, waits for all of them to finish, and re-raises the first error (catchable with `TRY`/`CATCH`).

```basic
DIM users, orders
PARALLEL
  users = FETCH("https://api.example.com/users")
  orders = FETCH("https://api.example.com/orders")
END PARALLEL
```

---

## 8. Middleware

`USE Fn` registers middleware that runs, in order, before every handler. `USE Fn ON "/admin/*"` scopes it to a path and everything under it. Middleware gets `req`, can write to `req.ctx`, and stops the request by calling `RESPOND` or throwing. Otherwise the next middleware, then the handler, runs.

```basic
USE Trace
USE AdminOnly ON "/admin/*"
ROUTE GET "/admin/vault" CALL Vault
SERVE

SUB Trace(req)
  req.ctx.seenAt = req.path
END SUB

SUB AdminOnly(req)
  IF req.headers["x-pass"] <> "open sesame" THEN
    RESPOND 403, {"error": "FORBIDDEN BY THE CRYPT"}
  END IF
  req.ctx.admin = TRUE
END SUB

FUNCTION Vault(req)
  RETURN {"admin": req.ctx.admin, "seenAt": req.ctx.seenAt}
END FUNCTION
```

`USE` goes in top-level code. It runs before the server starts.

### Built-in middleware

Built-ins are called with parentheses; your own middleware is named without them.

| Middleware | Does |
|---|---|
| `REQUESTLOG()` | logs one line per request: method, path, status, `durationMs`, request ID |
| `CORS([options])` | sets `Access-Control-Allow-*` headers and answers `OPTIONS` preflight with `204`. Options (all strings): `origins` (default `"*"`), `methods`, `headers`, `credentials`, `maxAge` |
| `RATELIMIT(perMinute)` | limits requests per client IP per minute; over the limit gets `429 {"error": "TOO MANY REQUESTS"}` |
| `APIKEY(envName)` | checks the `x-api-key` header against the secret in `ENV(envName)`; a wrong or missing key gets `401 {"error": "UNAUTHORIZED"}`. If the variable isn't set, every request is refused |
| `BEARERJWT(envSecret)` | verifies an HS256 JWT from `Authorization: Bearer …` using `ENV(envSecret)`, and sets `req.ctx.user` to its claims; failures get `401 {"error": "UNAUTHORIZED"}` |

```basic
USE REQUESTLOG()
USE CORS({"origins": "https://shop.example", "credentials": "true"})
USE RATELIMIT(120)
USE APIKEY("API_KEY") ON "/v1/*"
USE BEARERJWT("JWT_SECRET") ON "/me"

ROUTE GET "/me" CALL Me
SERVE

FUNCTION Me(req)
  RETURN {"user": req.ctx.user.sub}
END FUNCTION
```

Middleware is structured-mode only.

---

## 9. Databases

### Connecting and querying

`DBCONNECT url [AS name]` opens a connection pool. Supported URLs:

- `postgres://user:password@host:5432/db` (also `postgresql://`)
- `sqlite://:memory:` or a file, like `sqlite://./app.db`

Queries always use `$1, $2, …` placeholders, on every driver. Arguments are sent as parameters, never pasted into the SQL.

| Function | Returns |
|---|---|
| `SQL(query, args…)` | an array of row maps |
| `SQLONE(query, args…)` | the first row, or `NIL` |
| `SQLEXEC(query, args…)` | the number of affected rows |

```basic
DBCONNECT "sqlite://:memory:"

SQLEXEC("CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, price NUMERIC(12,2) NOT NULL)")
SQLEXEC("INSERT INTO items (name, price) VALUES ($1, $2)", "Coffin", DECIMAL("199.95"))
SQLEXEC("INSERT INTO items (name, price) VALUES ($1, $2)", "Candle", DECIMAL("15.00"))

FOR EACH row IN SQL("SELECT name, price FROM items ORDER BY id")
  PRINT row.name & ": " & row.price
NEXT

LET candle = SQLONE("SELECT price FROM items WHERE name = $1", "Candle")
IF candle <> NIL THEN PRINT candle.price
PRINT SQLEXEC("DELETE FROM items WHERE price < $1", DECIMAL("20"))
```

Columns come back keyed by their names in the result. `NUMERIC` and `DECIMAL` columns come back as `DECIMAL` values, not floats. A failed query raises an error you can catch.

`DBCONNECT` and the `SQL` functions also work in classic mode.

### Transactions

```basic
DBCONNECT "sqlite://:memory:"
SQLEXEC("CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance NUMERIC(12,2) NOT NULL)")
SQLEXEC("INSERT INTO accounts (id, balance) VALUES (1, 100), (2, 50)")

TRY
  TRANSACTION
    Transfer(1, 2, DECIMAL("40.00"))
    THROW ERROR("INSUFFICIENT SPIRIT")
  END TRANSACTION
CATCH e
  PRINT "ROLLED BACK: " & e.message
END TRY

SUB Transfer(fromId, toId, amount)
  SQLEXEC("UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromId)
  SQLEXEC("UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toId)
END SUB
```

- `TRANSACTION [ON name] … END TRANSACTION` commits when the block finishes normally, including through `RETURN` or `RESPOND`, and rolls back if any error escapes it.
- Every SQL call made while the block runs uses the transaction, including calls inside the functions it calls.
- A `TRANSACTION` inside another one becomes a savepoint.
- `TRANSACTION` is structured-mode only.

### Named connections

```basic
DBCONNECT "sqlite://:memory:"
DBCONNECT "sqlite://./audit.db" AS audit

TRANSACTION ON audit
  SQLEXEC("CREATE TABLE IF NOT EXISTS events (note TEXT NOT NULL)")
  SQLEXEC("INSERT INTO events (note) VALUES ($1)", "AUDIT OK")
END TRANSACTION
```

### Migrations

`rip migrate` applies `migrations/*.sql` in file-name order, each one once, and records them in a `_rip_migrations` table. It reads the database URL from `DATABASE_URL` (in the environment or `.env`).

```text
migrations/
  001_invoices.sql
  002_add_due_date.sql
```

### The injection warning

If a query string is built with `&` or `+` from a value that came from `req`, `rip check` (and every build) warns:

```basic
ROUTE GET "/search" CALL Search
SERVE

FUNCTION Search(req)
  RETURN SQL("SELECT * FROM graves WHERE name = '" & req.query.name & "'")
END FUNCTION
```

```text
?INJECTION RISK WARNING IN search.bas:5 — use $1 placeholders
```

The fix is `SQL("SELECT * FROM graves WHERE name = $1", req.query.name)`.

---

## 10. Config, secrets and logging

### ENV and ENVREQUIRE

- `ENV("NAME" [, default])` reads an environment variable. It gives `default` (or `NIL`) when the variable isn't set.
- `ENVREQUIRE("NAME")` stops startup with a clear error if the variable is missing or empty.
- `rip run` and `rip migrate` load a `.env` file from the project in development. Real environment variables win over `.env`, and nothing is loaded when `RIP_ENV=production`. A built binary never reads `.env`.

```basic
DBCONNECT ENVREQUIRE("DATABASE_URL")
CONST TAX_RATE = DECIMAL(ENV("TAX_RATE", "0.075"))
CONST REGION = ENV("REGION", "graveyard-1")
PRINT REGION
```

### LOG

`LOG level, message [, fields]` writes one JSON line to stdout with `time`, `level`, `msg`, `requestId` (inside a request) and the fields. Levels are the strings `"DEBUG"`, `"INFO"`, `"WARN"` and `"ERROR"`.

```basic
LOG "INFO", "INVOICE CREATED", {"invoiceId": 42, "total": DECIMAL("215.00")}
LOG "WARN", "LOGIN FAILED", {"user": "vlad", "password": "garlic"}
```

```text
{"time":"…","level":"INFO","msg":"INVOICE CREATED","invoiceId":42,"total":"215.00"}
{"time":"…","level":"WARN","msg":"LOGIN FAILED","user":"vlad","password":"[REDACTED]"}
```

Fields whose names look like secrets (containing `password`, `token`, `apiKey`, `authorization` or `secret`, in any case and at any depth) are written as `"[REDACTED]"`.

`PRINT` writes plain text to stdout. In the playground, both show in the live log panel.

---

## 11. Outbound HTTP: FETCH

`FETCH(url [, options])` calls another API and returns `{"status", "headers", "body"}`. A JSON response body is parsed into maps and arrays; anything else is a string. Response header names are lowercase.

| Option | Default |
|---|---|
| `method` | `"GET"` |
| `headers` | none |
| `body` | none; a string is sent as-is, anything else as JSON |
| `timeout` | `10` (seconds) |

```basic
LET r = FETCH("https://api.example.com/rates?base=EUR")
IF r.status = 200 THEN PRINT r.body.rates.USD

TRY
  LET created = FETCH("https://api.example.com/charges", {"method": "POST", "headers": {"authorization": "Bearer " & ENV("PAY_TOKEN", "")}, "body": {"amount": "215.00"}, "timeout": 5})
  PRINT created.status
CATCH e
  PRINT e.kind; ": "; e.message
END TRY
```

A non-2xx status is not an error; check `r.status`. A network failure or timeout raises a catchable error with `e.kind = "HTTP"`, which becomes a `500` if it escapes a handler. Responses are read up to 1 MB.

In the hosted playground (coming in this release), `FETCH` also blocks private, loopback, link-local and cloud-metadata addresses, including after DNS resolution and redirects.

---

## 12. Testing

**(coming in this release)** Tests are written in BASIC, in `*.test.bas` files under `tests/`.

- `TEST "name" … END TEST` declares a test.
- `EXPECT condition [, "message"]` fails the test with its BASIC source location if the condition is false: `?EXPECT FAILED ERROR IN file:line`.
- `REQUEST(method, path [, body, headers])` calls the app's routes in-process and returns `{status, headers, body}`. No network, no port. The app is the project's entry file (`main` in `rip.json`, default `main.bas`).

```basic
' tests/billing.test.bas
IMPORT { InvoiceTotals } FROM "../billing.bas"

TEST "ADDS TAX CORRECTLY"
  LET t = InvoiceTotals([{"unitPrice": "100.00", "quantity": 2}], DECIMAL("0.075"))
  EXPECT t.total = DECIMAL("215.00"), "200.00 PLUS 7.5% TAX"
END TEST

TEST "REJECTS MISSING API KEY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": []})
  EXPECT r.status = 401
END TEST

TEST "REJECTS NEGATIVE QUANTITY"
  LET item = {"description": "Coffin", "quantity": -1, "unitPrice": "50.00"}
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": [item]}, {"x-api-key": "test-key"})
  EXPECT r.status = 422
  EXPECT r.body.error = "QUANTITY MUST BE POSITIVE"
END TEST
```

`rip test`:

- compiles the tests with the app, whose `SERVE` doesn't bind a port;
- replaces the app's `DBCONNECT` URL with a fresh SQLite `:memory:` database, or with `TEST_DATABASE_URL` if set;
- loads `.env.test` if present;
- exits non-zero if any test fails.

---

## 13. CLI

| Command | Does |
|---|---|
| `rip new <name>` | scaffolds a project: `main.bas`, `rip.json`, `migrations/`, `tests/`, `.env.example`, `Dockerfile`, `.gitignore` and `.kiro/steering` |
| `rip run [file.bas]` | compiles and runs once |
| `rip dev [file.bas]` | compiles and runs; **(coming in this release)** watches files, rebuilds and restarts on change, and keeps the last good build running when a build fails. Today it runs once, like `rip run` |
| `rip test` | runs `tests/*.test.bas` **(coming in this release)** |
| `rip build [--os X] [--arch Y] [-o file]` | builds one static binary, `dist/<name>` by default |
| `rip migrate` | applies `migrations/*.sql` |
| `rip check [file.bas]` | compiles only, and prints errors and warnings |
| `rip version` | prints the version |

- Without a file argument, commands use the project in the current directory: `rip.json`'s `main` (default `main.bas`). With a file, they use that file and its directory.
- `rip.json` is `{"name": "shop", "main": "main.bas"}`. The name is used for the binary.
- `rip` writes Go code and calls the Go toolchain, so `rip` itself needs Go installed. The binary from `rip build` doesn't: it needs no Go, no `rip` and no source files at runtime.
- `rip build` sets `CGO_ENABLED=0`, `-trimpath` and `-ldflags "-s -w"`. Use `--os` and `--arch` to cross-compile, for example `rip build --os linux --arch arm64`.
- Generated Go goes to `.rip/build/` in the project. Add `.rip/` and `dist/` to `.gitignore` (`rip new` does).

Environment variables the runtime reads:

| Variable | Used for |
|---|---|
| `PORT` | the port when `SERVE` has none (default `8080`) |
| `RIP_MAX_BODY_BYTES` | the request body cap (default 1 MB) |
| `RIP_ENV` | `production` stops `rip` from loading `.env` |
| `DATABASE_URL` | the database for `rip migrate` |
| `TEST_DATABASE_URL` | the database for `rip test` (coming in this release) |

### The playground

**(coming in this release)** The hosted playground runs the same compiler in the browser's backend: write BASIC, click RUN, and get a live URL at `/p/<programId>/`, with a live log panel and a request tester. Each program runs as its own process with a 128 MB memory cap, a 5-second limit per request and compiled-in loop guards. A request that runs too long gets `508 {"error": "TRAPPED IN AN ETERNAL GOTO LOOP"}`. Programs expire after 7 days; production use is via the CLI.

---

## 14. Diagnostics and warnings

Every error, from the compiler or the runtime, has the same shape:

```text
?<KIND> ERROR IN <file>:<line> — <hint>
```

The hint says how to fix the problem. Compile errors stop the build. Warnings don't.

```basic
IF 1 = 1
  PRINT "NO THEN"
END IF
```

```text
?SYNTAX ERROR IN main.bas:1 — expected THEN after IF condition
```

More compile errors you may meet:

```text
?SYNTAX ERROR IN main.bas:2 — GOTO is only allowed in classic mode (files with line numbers); use IF, FOR, WHILE or FUNCTION instead
?SYNTAX ERROR IN main.bas:2 — this line has no line number, but the file started in classic mode; number every line, or remove all the numbers for structured mode
?SYNTAX ERROR IN main.bas:2 — NEXT without FOR; there is no open FOR for it to close
?UNDEFINED ERROR IN main.bas:2 — NOPE is not defined
?UNDEFINED ERROR IN main.bas:1 — SECRETHELPER is not exported by ./pricing.bas; add EXPORT before FUNCTION SECRETHELPER
```

And at runtime:

```text
?TYPE MISMATCH ERROR IN main.bas:2 — expected an INTEGER, got STRING two
?NIL ACCESS ERROR IN main.bas:2 — cannot read "name" of NIL — check the value exists before using it
?USER ERROR IN main.bas:2 — THE DEAD WALK
```

### Error kinds

| Kind | When |
|---|---|
| `SYNTAX` | the code can't be parsed, or a statement is in the wrong mode or place |
| `TYPE MISMATCH` | a value has the wrong type for a declaration, operator or call |
| `UNDEFINED` | an unknown variable, function, import, export or required env variable |
| `NIL ACCESS` | reading a member of `NIL` |
| `BAD SUBSCRIPT` | an array index out of range |
| `DIVISION BY ZERO` | `/`, `\` or `MOD` by zero |
| `STACK OVERFLOW` | more than 10,000 nested calls |
| `RETURN WITHOUT GOSUB` | classic `RETURN` with no `GOSUB` |
| `USER` | `THROW ERROR(…)` |
| `HTTP` | `THROW HTTPERROR(…)`, or a failed `FETCH` |
| `GO` | an error returned by a Go package |
| `DATABASE` | a failed connection, query or transaction |
| `ETERNAL LOOP` | playground loop guard or 5-second request limit exceeded (`508`; cannot be caught by `CATCH`) |
| `EXPECT FAILED` | a failed `EXPECT` (coming in this release) |
| `INTERNAL` | a bug in `rip` itself, or the server can't start (for example, the port is taken) |
| `NOT IMPLEMENTED` | a specified feature that isn't built yet |

### Warnings

| Warning | When |
|---|---|
| `?INJECTION RISK WARNING` | a SQL string is built with `&` or `+` from request data; see [the injection warning](#the-injection-warning) |
| `?SHARED STATE WARNING` | a handler changes a module-level variable not declared `SHARED` (coming in this release) |

Generated Go carries `//line` directives, so every runtime error, including a Go panic deep inside a library call, is reported at the BASIC line that caused it.
