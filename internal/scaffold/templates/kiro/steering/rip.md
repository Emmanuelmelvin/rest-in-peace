---
inclusion: always
---

# This project is written in RIP BASIC

- Source files are `.bas`. The entry point is set in `rip.json` (default `main.bas`).
- Use structured mode: `FUNCTION`/`SUB`, `IMPORT`/`EXPORT`, `TRY`/`CATCH`. No line numbers or `GOTO`.
- Routes: `ROUTE GET "/path/:id" CALL Handler`. Handlers take `req` and `RETURN` a value or `RESPOND status, value`.
- Errors: `THROW HTTPERROR(404, "NOT FOUND")`.
- SQL: always use `$1, $2…` placeholders and pass values as arguments. Never build SQL with `+` or `&`.
- Money: use `DECIMAL("19.99")` and `ROUND(x, 2)`, never floats.
- Secrets: `ENV("NAME")` or `ENVREQUIRE("NAME")`. Never hard-code them.
- Tests live in `tests/*.test.bas`, using `TEST "…" … END TEST`, `EXPECT` and `REQUEST(method, path, body, headers)`.
- Commands: `rip dev`, `rip test`, `rip check`, `rip migrate`, `rip build`.
