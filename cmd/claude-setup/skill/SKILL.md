---
name: samurai
description: "Samurai scoped testing framework for Go (github.com/zerosixty/samurai). MUST use when writing, modifying, reviewing, or debugging Go tests that import samurai, or reference samurai.Run, samurai.RunWith, samurai.Scope, samurai.TestScope, samurai.W, or samurai.BaseContext."
---

# Samurai Test Writing Rules

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

## When to Use

USE samurai when ALL of these hold:
1. **Branching action tree**: a chain of business actions (API calls, state transitions, DB writes) forks into 2+ mutually exclusive paths
2. **Mutually exclusive siblings**: running one path's actions would corrupt the starting state for another (e.g., "accept" vs "reject" from "requested" state)
3. **>= 3 leaf paths**: below this, duplicated `t.Run` blocks are simpler

Detect candidates at two levels:
- **Within one function** — non-samurai test with nested `t.Run` where parent mutates state and 2+ children branch -> candidate for samurai
- **Across functions** — group `TestFunc_*` by feature; if later functions repeat ALL actions of earlier ones plus add diverging steps (hidden tree with duplicated action chains) -> candidate for samurai consolidation. Do NOT flag when tests call the same endpoint with different configurations (variants, not a tree)

DO NOT use samurai when ANY of these hold:
1. **Read-only subtests**: branches only query shared state without mutating it
2. **Sequential accumulation**: each step depends on cumulative effects of ALL prior steps (linear chain, not branching)
3. **Same action, different inputs**: multiple tests call the same endpoint with different configurations — no test repeats another's actions as a prefix
4. **Flat/independent**: each subtest creates its own complete setup
5. **Existing isolation**: test already uses Ginkgo `BeforeEach` or similar

Cost: samurai re-executes the full path per leaf — same cost as manually duplicating setup, just automated. Parallel execution offsets wall-clock time.

## Additional resources

- For the full API example (samurai.Run) and RunWith (custom context), see [api.md](api.md)
- For validation rules (panic conditions) and wrong patterns, see [pitfalls.md](pitfalls.md)
