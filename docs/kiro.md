# How Kiro built REST in Peace

> Draft for the Kiroween write-up (task 6.3). Fill it in as you go: notes written at the time beat notes reconstructed on deadline day.

## Spec-driven development
- `.kiro/specs/rip-basic/requirements.md`: 16 requirements in EARS form.
- `design.md`: the compile-to-Go architecture, the value model, `//line` error mapping, and classic mode as native `goto`.
- `tasks.md`: the dated plan. Every stub in the code is tagged `TODO(task X.Y)`.
- *Notes to add:* which tasks Kiro implemented straight from the spec, and where the spec changed during the build and why.

## Steering
- `language.md` stops Kiro from inventing BASIC syntax.
- `security.md` enforces parameterized SQL, log redaction and playground isolation.
- `style.md` sets the spooky-but-helpful error tone.
- `product.md`, `tech.md` and `structure.md` give the always-on context.
- *Notes to add:* examples where steering changed Kiro's output.

## Hooks
- `golden-tests-on-save`: compiler or runtime edits run `go vet` and the golden suite.
- `rip-check-on-bas-save`: every `.bas` save is compile-checked, including the injection lint.
- `example-tests-on-save`: keeps the Invoicing demo green.
- *Notes to add:* regressions the hooks caught.

## Vibe coding vs. spec coding
- *Notes to add:* which parts were exploratory chats (for example the CRT theme), and which were spec tasks.

## MCP
- *Optional:* a Postgres MCP server for inspecting the Neon demo database.
