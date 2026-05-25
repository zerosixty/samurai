# Samurai API Reference

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
            s.Test("create user", func(_ context.Context, w samurai.W) {
                user := db.CreateUser()             // 4. db visible (parent assigned it)
                assert.NotEmpty(w.Testing(), user.Email) // 5. assert in same Test as the action
                assert.NotEmpty(w.Testing(), user.Role)  // 6. one action + assertions = one Test
            })

            s.Test("lists no users initially", func(ctx context.Context, w samurai.W) {
                users, err := db.ListUsers(ctx)  // 7. own db — isolated from "create user" paths
                assert.NoError(w.Testing(), err)
                assert.Empty(w.Testing(), users)
            })
        })

        s.Test("WIP feature", func(_ context.Context, w samurai.W) {}, func(s *samurai.Scope) {
            s.Skip()                        // 8. Skip entire scope (propagates to descendants)
            s.Test("not ready", func(_ context.Context, w samurai.W) {
                // never executes; appears as SKIP in output
            })
        })
    })
}

func openDB(ctx context.Context) *DB { /* ... */ } // 9. helpers BELOW TestFeature, never above
```

## RunWith (Custom Context)

Embed `*samurai.BaseContext` to satisfy `samurai.Context`. Use `Testing()` not `T()` (avoids testify conflict). Use `w.Context()` in the factory for initialization that needs a `context.Context`.

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
        s.Test("the answer matches the spec", func(_ context.Context, c *MyCtx) {
            c.Equal(42, value)  // assertion methods directly on context
        })
    })
}

// methods on *MyCtx and other helpers go HERE, below the Test function
```

Factory `func(W) V` is called once per test path. The same value is reused across all scope levels in that path.
