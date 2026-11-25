---
inclusion: always
---

# Tech

- **Language:** Go (see `go.mod` for the minimum version). The compiler, CLI, runtime and playground host are all Go. The playground front end is TypeScript (Vite + React + CodeMirror 6).
- **Pipeline:** `.bas` → lexer → parser → AST → check → Go codegen → `.rip/build/` → `go build` → native binary.
- **Runtime dependencies:** `jackc/pgx/v5` (Postgres), `modernc.org/sqlite` (pure Go, no cgo), `shopspring/decimal`, `golang-jwt/jwt/v5`. Everything else comes from the stdlib: `net/http`, `log/slog`, `encoding/json`.
- **Compiler dependencies:** stdlib, plus `golang.org/x/tools/go/packages` for Go interop.
- **Static builds:** `CGO_ENABLED=0` must always work. Don't add a dependency that needs cgo.
- **Add dependencies deliberately.** Each new module is a line in every generated program's `go.mod`.

## Commands
```
go build ./...            # build everything
go vet ./...              # lint
go test ./...             # unit + golden tests
go run ./cmd/rip <cmd>    # run the CLI from source
cd playground/web && npm install && npm run dev   # front end
```
