# How we used Kiro

> Draft for the Kiroween write-up (task 6.3). Fill it in as each Kiro task happens: notes written at the time beat notes reconstructed on deadline day.

## What was built with what

REST in Peace was built with two tools, and this document is upfront about which did what.

| Built with | What |
|---|---|
| **Kiro** | Tasks 2.4 (ENV, LOG, redaction), 2.5 (DECIMAL), 3.6 (SQL injection warning) and 5.4 (playground CRT theme and samples), plus the spec-driven workflow: the `.kiro/` specs, steering and hooks that every task follows |
| **Claude Code** | Everything else: the compiler (lexer, parser, Go codegen), runtime, HTTP, database, CLI, playground backend, and the scaffold and specs themselves |

The two never edit the same files. Ownership is listed in `.kiro/steering/structure.md`, and the git history shows it.

## The spec-driven setup
- `.kiro/specs/rip-basic/requirements.md`: 16 requirements in EARS form.
- `design.md`: compiling to Go, the value model, `//line` error mapping, and classic mode as native `goto`.
- `tasks.md`: the dated plan, with the Kiro tasks marked **[Kiro]**.
- `steering/`: `language.md` (never invent syntax), `security.md`, `style.md`, `product.md`, `tech.md`, `structure.md`.
- `hooks/`: golden tests on compiler/runtime save, `rip check` on `.bas` save, and example tests on save.

## Kiro task log

Fill in one section per task as you do it.

### 2.5 DECIMAL and ROUND
- **Date:**
- **How it started:** (spec task button / chat / vibe)
- **What the spec and steering contributed:**
- **What the hooks caught:**
- **What I changed by hand, and why:**
- **Screenshot or clip for the video:**

### 2.4 ENV, LOG and redaction
- **Date:**
- **How it started:**
- **What `security.md` changed in the output** (e.g. redaction):
- **What the hooks caught:**
- **What I changed by hand, and why:**

### 3.6 SQL injection warning
- **Date:**
- **How it started:**
- **The `rip check` hook in action** (a warning firing on save):
- **What I changed by hand, and why:**

### 5.4 Playground CRT theme and samples
- **Date:**
- **How it started:** (vibe-coding the look, then spec for the samples?)
- **Before/after screenshots:**

## Spec coding vs. vibe coding
- *To fill in:* where the spec drove the work, where chat was faster, and what you'd do differently.

## MCP
- *Optional:* a Postgres MCP server for inspecting the demo database from Kiro.
