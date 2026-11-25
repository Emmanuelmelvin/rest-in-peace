---
inclusion: always
---

# Product — REST in Peace (RIP BASIC)

- **What:** a modern BASIC for production backend services that compiles to Go and ships as one native binary.
- **Pitch:** "BASIC died in the '90s. We brought it back — for production."
- **Hackathon:** Kiroween 2026, Resurrection category (also Best Startup Project). Deadline: Fri 4 Dec 2026, 2:00 PM PT / 11:00 PM WAT.
- **Two modes:** classic (line numbers, GOTO, GOSUB) for the resurrection story; structured (functions, modules, TRY/CATCH) for real work.
- **Demo centrepiece:** the Invoicing API in `examples/invoicing`, with DECIMAL money math, a Postgres transaction, API-key middleware and tests.
- **Tone:** spooky but helpful. Errors read like 1975 (`?SYNTAX ERROR IN main.bas:12`), always with a hint that tells you how to fix the problem.
- **Source of truth:** `.kiro/specs/rip-basic/{requirements,design,tasks}.md`. When the code and the spec disagree, fix one of them; don't leave them drifting.
- **Scope discipline:** P0 tasks first. P1 tasks only when every P0 task in the current phase is done.
