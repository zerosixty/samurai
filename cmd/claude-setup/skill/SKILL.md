---
name: samurai
description: "Samurai scoped testing framework for Go (github.com/zerosixty/samurai). MUST use when writing, modifying, reviewing, or debugging Go tests that import samurai, or reference samurai.Run, samurai.RunWith, samurai.Scope, samurai.TestScope, samurai.W, or samurai.BaseContext."
---

# Samurai Test Writing Rules

**Good For:** Go tests with a branching state-mutating tree (≥3 leaf paths, mutually exclusive siblings) using `samurai.Run` / `samurai.RunWith`.
**Bad For:** read-only chains, sequential accumulation, same-action-different-input variants, flat independent tests (see "When to Use" below for full criteria).

## Rules

- One action + its assertions = one `Test()`. Don't split assertions into child Tests — assert the result in the same callback that produced it. Child Tests are for new actions (setup, mutations, queries), not for checking fields of a parent's result
- DECLARE variables with `var` in the builder body, ASSIGN inside `Test()` callbacks
- Each leaf path re-executes the builder from scratch — fresh variables, full isolation
- `w.Cleanup(fn)`: LIFO order, inner before outer, runs even on panic, thread-safe
- `s.Skip()`: affects entire scope regardless of call order, no callbacks/cleanups execute
- `w.Context()`: returns the scope's `context.Context` — use in factories for initialization needing a context
- `context.Context`: first param in `Test` callbacks, live during test, canceled after path completes
- Parallel by default; `samurai.Sequential()` forces order; `go test -parallel N` controls concurrency
- Assertion-agnostic: use `w.Testing()` with any library (testify, is, stdlib)

## Naming

Every `s.Test("name", ...)` — at every tree level — names a business action or outcome from the actor's POV. Not the function under test, the assertion, or the input shape.

Forbidden patterns:

- **Identifier leak** — CamelCase tokens matching Go identifiers in the Test body. `"UpdateQuote calls..."` leaks `UpdateQuote`.
- **HTTP code leak** — bare 3-digit codes (`\b[1-5]\d{2}\b`) or `4xx/5xx` shorthand. `"returns 409"` → `"rejects duplicate id"`.
- **Assertion phrasing** — `is / equals / returns / not nil / has length / contains`. `"err is not nil"` → `"rejects invalid signature"`.
- **Structure phrasing** — `empty / nil / with N / map / slice / struct`. `"with empty slice"` → `"borrower has no past loans"`.

Risky-looking words used in a domain-natural sense (`borrower returns the equipment`, `has insufficient collateral`, `no past loans`) are clean — only flag when the word describes the assertion, the data shape, or the identifier. Flag matches, read the body, propose 1–2 replacements. See [naming.md](naming.md) for the protocol.

## File layout

For samurai test files, top-to-bottom:

1. Context type (`type fooCtx struct {...}`) + `samurai.TestScope` alias — RunWith only
2. `func Test*` — body contains the `samurai.Run`/`RunWith` call and the `s.Test(...)` tree. **Prefer inlining** the factory closure (`func(w samurai.W) *fooCtx { ... }`) inside `RunWith`. **Extract** to a private function in the same file (placed with the helpers, below the Tests) only when 2+ Test functions need identical factory logic — at that point duplication outweighs locality. Helper calls *inside* an inlined factory body (e.g. `newBaseCtx(w)`, fixture builders) are fine either way.
3. Methods on `*fooCtx` and other private helpers **defined in this file** — always **below** the Test function, never prepended (shared cross-file helpers in the same package are out of scope)

Rationale: tests on top so the file's contract is visible immediately; setup stays local when it has a single caller, and graduates to a helper when it doesn't; helpers are appendix.

## When to Use

USE samurai when ALL of these hold:
1. **Branching action tree**: a chain of business actions (API calls, state transitions, DB writes) forks into 2+ mutually exclusive paths
2. **Mutually exclusive siblings**: running one path's actions would corrupt the starting state for another (e.g., "accept" vs "reject" from "requested" state)
3. **>= 3 leaf paths**: below this, duplicated `t.Run` blocks are simpler

Detect candidates at two levels:
- **Within one function** — non-samurai test with nested `t.Run` where parent mutates state and 2+ children branch -> candidate for samurai
- **Across functions** — group `TestFunc_*` by feature; if later functions repeat earlier ones' **state-mutating** actions (writes/RPC mutations/DB inserts) plus add diverging steps -> candidate. DO NOT flag: repeated read chains (data dup, not state tree); same endpoint with different inputs (variants, not branches)

DO NOT use samurai when ANY of these hold:
1. **Read-only subtests**: branches only query shared state without mutating it. `Get → extract id → Get` chains are data extraction, not state trees — chain length is irrelevant
2. **Sequential accumulation**: each step depends on cumulative effects of ALL prior steps (linear chain, not branching)
3. **Same action, different inputs**: multiple tests call the same endpoint with different configurations — no test repeats another's actions as a prefix
4. **Flat/independent**: each subtest creates its own complete setup

Common false positives: `_HappyPath` / `_AccessDenied` siblings (access-denied uses fresh setup + bad input — rule #3); long read-only flow chains (rule #1); N tests sharing a `createApp()`/`insertFixtures()` helper (rule #4).

### Detection protocol

1. **Read bodies, not names.** `Test_X_HappyPath` is not evidence.
2. **Swap-order test.** If two tests can be reordered with no effect, they are flat — not a tree.
3. **Find the writes.** No mutating step in any sibling → samurai adds nothing.

Cost: samurai re-executes the full path per leaf — same cost as manually duplicating setup, just automated. Parallel execution offsets wall-clock time.

## Additional resources (lazy-load — read only when triggered)

- **Read [api.md](api.md) only when** you need the worked-out `samurai.Run` / `RunWith` API example (e.g. wiring a new test scope, writing a custom context type for `RunWith`). The "When to Use" / "Detection protocol" sections above are sufficient for review and detection work.
- **Read [naming.md](naming.md) only when** authoring a new `s.Test(...)` name and the right phrasing isn't obvious, or reviewing a samurai test where at least one name matches a forbidden pattern above. The inline "Naming" section is enough for clearly-good or clearly-bad names; load this when you need the actor/verb/outcome replacement protocol.
- **Read [pitfalls.md](pitfalls.md) only when** debugging a panic, a validation error, or a wrong pattern (e.g. asserting on a parent's result inside a child Test, declaring vars where they should be assigned). Skip otherwise — the rules above already cover the common authoring decisions.
