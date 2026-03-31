# Samurai Pitfalls & Validation

## Validation (all panic)

| Condition | Error |
|-----------|-------|
| Empty name `""` | `Test called with empty name` |
| Name contains `/` | `must not contain '/'` |
| `nil` fn | `Test called with nil function` |
| >1 builder args | `at most one builder function` |
| `nil` builder arg | `Test called with nil builder` |
| Duplicate sibling names | `duplicate test name` |
| Builder with no `Test()` calls | `no tests defined` / `builder contains no tests` |
| `Test()`/`Skip()` after builder returned | `sealed scope` |
| `nil` builder/factory to `Run`/`RunWith` | panic |
| Builder structure changes between runs | `not found in scope` |

## Wrong Patterns (NEVER do these)

```go
// WRONG 1: Side effects inline in builder — runs during BOTH discovery AND execution
s.Test("parent", fn, func(s *samurai.Scope) {
    user := db.CreateUser("test") // BUG: executes twice!
    s.Test("check", func(_ context.Context, w samurai.W) { ... })
})
// FIX: move assignment into a Test() callback (see correct usage above)

// WRONG 2: Missing cleanup — resource leak
s.Test("open db", func(ctx context.Context, w samurai.W) {
    db := openDB(ctx) // never closed!
})
// FIX: w.Cleanup(func() { db.Close() })

// WRONG 3: Shared mutable state between sibling leaves
var counter int // package-level or captured from outside Run
s.Test("A", func(_ context.Context, _ samurai.W) { counter++ }) // race!
s.Test("B", func(_ context.Context, _ samurai.W) { counter++ }) // race!
// FIX: declare var inside builder body — each path gets its own copy

// WRONG 4: Non-deterministic builder structure
if someCondition {
    s.Test("A", fn) // discovery sees different structure than execution → panic
}
// FIX: builder must produce the same Test() calls every time

// WRONG 5: Nil cleanup
w.Cleanup(nil) // panics

// WRONG 6: Assertion-only child Tests — unnecessary nesting and extra re-execution
s.Test("create user", func(_ context.Context, w samurai.W) {
    user = db.CreateUser("test")
}, func(s *samurai.Scope) {
    s.Test("has email", func(_ context.Context, w samurai.W) {
        assert.NotEmpty(w.Testing(), user.Email) // just reading, no new action
    })
    s.Test("has role", func(_ context.Context, w samurai.W) {
        assert.NotEmpty(w.Testing(), user.Role)  // just reading, no new action
    })
})
// FIX: assert in the same Test that performs the action
s.Test("create user", func(_ context.Context, w samurai.W) {
    user := db.CreateUser("test")
    assert.NotEmpty(w.Testing(), user.Email)
    assert.NotEmpty(w.Testing(), user.Role)
})
```
