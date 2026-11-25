---
inclusion: always
---

# RIP BASIC language rules

Use this file whenever you write `.bas` code, examples, docs or compiler features. **Never invent syntax.** If a feature isn't listed here or in `requirements.md`, ask before adding it, then update both files.

## Modes
- **Classic mode:** the first non-comment line starts with a line number. Lines run in number order, a duplicate line number replaces the earlier line, and `:` separates statements. Supports `GOTO`, `GOSUB`, `RETURN`, `ON x GOTO a, b, c`.
- **Structured mode:** everything else. `GOTO` and `GOSUB` are compile errors.
- Mixing the two modes in one file is a compile error.

## Lexical
- Keywords, variable names and function names are case-insensitive. Member names after `.` and map keys are case-sensitive.
- Comments: `REM …` or `' …`.
- Strings use double quotes. Numbers: `42`, `3.14`, `1e6`.
- Literals: `TRUE`, `FALSE`, `NIL`, maps `{"k": v}`, arrays `[a, b]`.

## Statements
`LET` (optional) · `PRINT` · `DIM x AS T [= v]` · `SHARED DIM x AS T` · `CONST` ·
`IF … THEN … [ELSEIF …] [ELSE …] END IF`, single-line `IF c THEN stmt` ·
`FOR i = a TO b [STEP s] … NEXT` · `FOR EACH x IN arr … NEXT` · `WHILE … WEND` ·
`DO … LOOP [UNTIL c]` · `SELECT CASE x … CASE 1, 2 … CASE ELSE … END SELECT` ·
`EXIT FOR|WHILE|DO` · `END` ·
`FUNCTION` / `SUB` / `EXPORT` · `IMPORT` · `TRY … CATCH e … [FINALLY …] END TRY` · `THROW` ·
`ROUTE <METHOD> "<path>" CALL Fn | GOSUB n` · `ROUTE GROUP "/p" … END GROUP` ·
`USE Fn [ON "/path/*"]` · `RESPOND status, value [, headers]` · `SERVE [port]` ·
`DBCONNECT url [AS name]` · `TRANSACTION [ON name] … END TRANSACTION` ·
`LOG level, msg [, map]` · `TEST "name" … END TEST` · `EXPECT cond [, "msg"]`

## Imports
- `IMPORT "./billing.bas"`: all exports.
- `IMPORT { A, B } FROM "./billing.bas"`: named exports.
- `IMPORT uuid FROM GO "github.com/google/uuid"`: a Go package.
- `IMPORT { NewString } FROM GO "github.com/google/uuid"`: named Go functions.
- Braces always mean named imports.

## Types
`NUMBER` `INTEGER` `STRING` `BOOLEAN` `MAP` `ARRAY` `DECIMAL` `DATE` `NIL` `ANY`.
Typed declarations compile to native Go types. Untyped values are dynamic.

## Operators (lowest → highest)
`OR` < `AND` < `NOT` < `= <> < > <= >=` < `&` < `+ -` < `* / \ MOD` < `^` < unary `-` < postfix `.x [i] (args)`.
`&` always concatenates. `+` concatenates when either side is a STRING. `AND`/`OR` short-circuit.

## Semantics to remember
- A missing map key returns `NIL`. A member of `NIL` raises `?NIL ACCESS ERROR`. `LEN(NIL) = 0`.
- Arrays are zero-based.
- `DECIMAL` is exact. Mixing it with `NUMBER` gives `DECIMAL`.
- `RESPOND` ends the handler. It is never caught by `CATCH`, it runs `FINALLY`, and it commits `TRANSACTION`.
- A structured handler's `RETURN v` responds `200` with `v`.
- `SERVE` starts listening only after all top-level statements have run. In classic mode, the top-level run stops at `SERVE` (or `END`), and the lines after it run only as GOSUB targets.
- Classic handlers run one at a time per program, and each request has its own GOSUB stack.

## Built-in functions (v1)
`LEN` `UPPER` `LOWER` `TRIM` `LEFT` `RIGHT` `MID` `STR` `VAL` `INT` `ABS` `ROUND` `DECIMAL` `NOW`
`ENV` `ENVREQUIRE` `SQL` `SQLONE` `SQLEXEC` `FETCH` `REQUEST` `ERROR` `HTTPERROR` `NEW`
`PARAM` `QUERY` `HEADER` `BODY` (classic mode) · middleware: `CORS` `RATELIMIT` `REQUESTLOG` `APIKEY` `BEARERJWT`

## Error message format
`?<KIND> ERROR IN <file>:<line> — <hint>`

Kinds: `SYNTAX`, `TYPE MISMATCH`, `NIL ACCESS`, `BAD SUBSCRIPT`, `DIVISION BY ZERO`, `STACK OVERFLOW`, `RETURN WITHOUT GOSUB`, `UNDEFINED`, `USER`, `GO`, `HTTP`, `EXPECT FAILED`, `INTERNAL`. The runtime list lives in `rt/errors.go`; the compiler list in `internal/compiler/diag`.
Warnings: `?INJECTION RISK WARNING`, `?SHARED STATE WARNING`.
