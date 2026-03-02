---
name: samurai
description: "Samurai scoped testing framework for Go (github.com/zerosixty/samurai). MUST use when writing, modifying, reviewing, or debugging Go tests that import samurai, or reference samurai.Run, samurai.RunWith, samurai.Scope, samurai.TestScope, samurai.W, or samurai.BaseContext."
---

# Samurai Test Writing Rules

## API

```go
package feature_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/zerosixty/samurai"
)

func TestFeature(t *testing.T) {
    samurai.Run(t, func(s *samurai.Scope) {
        var db *DB                          // 1. DECLARE vars in builder body

        s.Test("with database", func(ctx context.Context, w samurai.W) {
            db = openDB(ctx)                // 2. ASSIGN inside Test callback
            w.Cleanup(func() { db.Close() }) // 3. ALWAYS register cleanup for resources
        }, func(s *samurai.Scope) {
            var user *User                  // 4. Nested var at next scope level

            s.Test("create user", func(_ context.Context, w samurai.W) {
                user = db.CreateUser()      // 5. db visible (parent assigned it)
            }, func(s *samurai.Scope) {
                s.Test("has email", func(_ context.Context, w samurai.W) {
                    assert.NotEmpty(w.Testing(), user.Email) // 6. user visible (parent assigned it)
                })
                s.Test("has role", func(_ context.Context, w samurai.W) {
                    assert.NotEmpty(w.Testing(), user.Role)  // 7. ISOLATED — own db, own user per leaf
                })
            })

            s.Test("list empty", func(ctx context.Context, w samurai.W) {
                users, err := db.ListUsers(ctx)  // 8. own db — isolated from "create user" paths
                assert.NoError(w.Testing(), err)
                assert.Empty(w.Testing(), users)
            })
        })

        s.Test("WIP feature", func(_ context.Context, w samurai.W) {}, func(s *samurai.Scope) {
            s.Skip()                        // 9. Skip entire scope (propagates to descendants)
            s.Test("not ready", func(_ context.Context, w samurai.W) {
                // never executes; appears as SKIP in output
            })
        })
    })
}
```

## Rules

- DECLARE variables with `var` in the builder body, ASSIGN inside `Test()` callbacks
- Each leaf path re-executes the builder from scratch — fresh variables, full isolation
- `w.Cleanup(fn)`: LIFO order, inner before outer, runs even on panic, thread-safe
- `s.Skip()`: affects entire scope regardless of call order, no callbacks/cleanups execute
- `context.Context`: first param, live during test, canceled after path completes
- Parallel by default; `samurai.Sequential()` forces order; `go test -parallel N` controls concurrency
- Assertion-agnostic: use `w.Testing()` with any library (testify, is, stdlib)

## RunWith (Custom Context)

Embed `*samurai.BaseContext` to satisfy `samurai.Context`. Use `Testing()` not `T()` (avoids testify conflict).

```go
type MyCtx struct {
    *samurai.BaseContext
    *assert.Assertions
}
type S = samurai.TestScope[*MyCtx]  // Go 1.24+ type alias

func TestWithAssertions(t *testing.T) {
    samurai.RunWith(t, func(w samurai.W) *MyCtx {
        return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
    }, func(s *S) {
        s.Test("check", func(_ context.Context, c *MyCtx) {
            c.Equal(42, value)  // assertion methods directly on context
        })
    })
}
```

Factory `func(W) V` is called once per scope level per path (not once globally).

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
```
