<p align="center">
  <img src="docs/assets/banner.jpg" alt="samurai — scoped testing for Go" width="100%">
</p>

# Samurai 侍

Scoped testing for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/zerosixty/samurai.svg)](https://pkg.go.dev/github.com/zerosixty/samurai)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)
![Zero Dependencies](https://img.shields.io/badge/dependencies-0-brightgreen)
[![GoLand Plugin](https://img.shields.io/jetbrains/plugin/v/30391-samurai-test-runner?label=GoLand%20Plugin&logo=jetbrains&color=orange)](https://plugins.jetbrains.com/plugin/30391-samurai-test-runner)

A scoped testing framework for Go with path isolation. You define a test tree using a single `Test()` method, the framework discovers all leaf paths, then runs each one independently with fresh local variables. Parallel by default via `t.Parallel()`. Zero dependencies, bring your own assertion library.

## Why "samurai"? *(as Yoda would say)*

No goal, a samurai has. Only the path. Each test -- a samurai it is, following its own path from root to leaf. Cross paths, two samurai never do. Walk in parallel they will, each with its own state, its own setup, its own cleanup. Shared mutable ground? Exists not. The path, there is only.

## What it does

You write a builder function that describes a tree of tests. Samurai runs the builder once in discovery mode to collect all the paths from root to leaf. Then, for each path, it runs the builder again from scratch in execution mode. Because the builder re-runs per path, local variables (`var db *DB`, `var user *User`, etc.) are fresh allocations every time. Paths can't interfere with each other.

All paths call `t.Parallel()` by default, so they run concurrently. There's no goroutine-local storage or global state involved. You bring your own assertion library ([testify](https://github.com/stretchr/testify), [is](https://github.com/matryer/is), plain `t.Error`, whatever). Cleanups registered via `w.Cleanup()` run in LIFO order even if the test panics.

## Install

```bash
go get github.com/zerosixty/samurai
```

Requires Go 1.24+.

## Quick Start

`samurai.Run` takes a builder function. Variables declared in the builder are fresh per path because the builder re-runs for each leaf:

```go
package db_test

import (
    "context"
    "database/sql"
    "testing"

    "github.com/zerosixty/samurai"
)

func TestDatabase(t *testing.T) {
    samurai.Run(t, func(s *samurai.Scope) {
        var db *sql.DB  // fresh allocation for every path

        s.Test("with database", func(ctx context.Context, w samurai.W) {
            db = openTestDB(ctx)
            w.Cleanup(func() { db.Close() })
        }, func(s *samurai.Scope) {
            s.Test("can ping", func(ctx context.Context, w samurai.W) {
                // this db belongs only to this path
                if err := db.PingContext(ctx); err != nil {
                    w.Testing().Fatal(err)
                }
            })

            s.Test("can query", func(ctx context.Context, w samurai.W) {
                // different path, different db instance
                rows, err := db.QueryContext(ctx, "SELECT 1")
                if err != nil {
                    w.Testing().Fatal(err)
                }
                rows.Close()
            })
        })
    })
}
```

Two paths get discovered: `with database/can ping` and `with database/can query`. The builder runs fresh for each, so `db` is a new variable both times. Both paths run in parallel.

```bash
go test -v
=== RUN   TestDatabase
=== RUN   TestDatabase/with_database
=== RUN   TestDatabase/with_database/can_ping
=== RUN   TestDatabase/with_database/can_query
--- PASS: TestDatabase (0.00s)
    --- PASS: TestDatabase/with_database (0.00s)
        --- PASS: TestDatabase/with_database/can_ping (0.00s)
        --- PASS: TestDatabase/with_database/can_query (0.00s)
```

## AI-assisted development

If you use [Claude Code](https://docs.anthropic.com/en/docs/claude-code), run this once in your project root to install a samurai skill:

```bash
go run github.com/zerosixty/samurai/cmd/claude-setup@latest
```

This creates `.claude/skills/samurai/SKILL.md`. Claude Code will use it as a reference when writing or modifying samurai tests. You can also invoke `/samurai` to load the reference manually.

## How it works

### Database testing

This is where path isolation pays off. Provision a fresh database in the parent `Test` callback — every leaf gets its own isolated instance automatically:

```go
func newPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
    t.Helper()
    conf := pgtestdb.Custom(t, dbConf, migrator) // fresh DB with migrations applied
    pool, err := pgxpool.New(ctx, conf.URL())
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(pool.Close)
    return pool
}

func TestTodoRepo(t *testing.T) {
    samurai.Run(t, func(s *samurai.Scope) {
        var repo *todo.Repo

        s.Test("with fresh database", func(ctx context.Context, w samurai.W) {
            pool := newPool(ctx, w.Testing())
            repo = todo.NewRepo(pool)
        }, func(s *samurai.Scope) {

            s.Test("Add", func(ctx context.Context, w samurai.W) {
                id, err := repo.Add(ctx, "buy milk")
                assert.NoError(w.Testing(), err)
                assert.Positive(w.Testing(), id)
            })

            s.Test("Get returns the created todo", func(ctx context.Context, w samurai.W) {
                id, _ := repo.Add(ctx, "test get")
                got, err := repo.Get(ctx, id)
                assert.NoError(w.Testing(), err)
                assert.Equal(w.Testing(), "test get", got.Title)
            })
        })
    })
}
```

Each leaf (`Add`, `Get returns the created todo`) gets its own PostgreSQL database — `newPool` runs fresh for each path. Both tests execute in parallel with zero interference.

This example uses [pgtestdb](https://github.com/peterldowns/pgtestdb) for fast database provisioning via PostgreSQL template databases, but samurai has no dependency on it. The same pattern works with [testcontainers-go](https://github.com/testcontainers/testcontainers-go), custom shell scripts, or any other database provisioning approach.

See [`examples/pgtestdb`](examples/pgtestdb) for the full working example with docker-compose, Atlas migrations, and pgx.

### Test - the only method

`Test` is the only method on `*Scope`. Two forms:

Leaf test (no children):
```go
s.Test("check value", func(_ context.Context, w samurai.W) {
    assert.Equal(w.Testing(), expected, actual)
})
```

Parent test (with children):
```go
s.Test("setup db", func(ctx context.Context, w samurai.W) {
    db = setupDB(ctx)
    w.Cleanup(func() { db.Close() })
}, func(s *samurai.Scope) {
    s.Test("has tables", func(_ context.Context, w samurai.W) { /* ... */ })
    s.Test("has indexes", func(_ context.Context, w samurai.W) { /* ... */ })
})
```

The first parameter is `context.Context` (from `T.Context()`), the second is `W` (`*BaseContext`):

| Method | Returns | Purpose |
|--------|---------|---------|
| `w.Context()` | `context.Context` | The context for this scope (canceled when test completes) |
| `w.Testing()` | `*testing.T` | The test instance for this path |
| `w.Cleanup(func())` | - | Register LIFO cleanup, runs even on panic |

Callbacks only execute during the execution phase, never during discovery. Multiple `Test` calls per scope become siblings.

### Nesting

The third argument to `Test` is a builder for the child scope:

```go
samurai.Run(t, func(s *samurai.Scope) {
    var svc *UserService

    s.Test("with service", func(ctx context.Context, w samurai.W) {
        svc = NewUserService(openTestDB(ctx))
        w.Cleanup(func() { /* cleanup */ })
    }, func(s *samurai.Scope) {
        var user *User

        s.Test("create user", func(ctx context.Context, w samurai.W) {
            user, _ = svc.Create(ctx, "test@example.com")
        }, func(s *samurai.Scope) {
            s.Test("has correct email", func(_ context.Context, w samurai.W) {
                assert.Equal(w.Testing(), "test@example.com", user.Email)
            })

            s.Test("has an ID", func(_ context.Context, w samurai.W) {
                assert.NotZero(w.Testing(), user.ID)
            })

            s.Test("then deleting", func(ctx context.Context, w samurai.W) {
                svc.Delete(ctx, user.ID)
            }, func(s *samurai.Scope) {
                s.Test("no longer exists", func(ctx context.Context, w samurai.W) {
                    _, err := svc.Get(ctx, user.ID)
                    assert.ErrorIs(w.Testing(), err, ErrNotFound)
                })
            })
        })

        s.Test("list empty", func(ctx context.Context, w samurai.W) {
            users, err := svc.List(ctx)
            assert.NoError(w.Testing(), err)
            assert.Empty(w.Testing(), users)
        })
    })
})
```

Each `Test` name creates a level. The path `with service/create user/has correct email` runs: "with service" setup, then "create user" setup, then the email assertion.

### Two-phase execution

**Code inside `Test` callbacks runs once per path. Code inline in the builder runs during both discovery and execution.**

```go
// WRONG - CreateUser runs during discovery AND execution
s.Test("setup", func(_ context.Context, w samurai.W) {}, func(s *samurai.Scope) {
    user := db.CreateUser("test@example.com")  // runs twice!
    s.Test("check", func(_ context.Context, w samurai.W) { /* ... */ })
})

// CORRECT - CreateUser only runs during execution
s.Test("setup", func(_ context.Context, w samurai.W) {}, func(s *samurai.Scope) {
    var user *User
    s.Test("create user", func(_ context.Context, w samurai.W) {
        user = db.CreateUser("test@example.com")  // runs once per path
    }, func(s *samurai.Scope) {
        s.Test("check", func(_ context.Context, w samurai.W) { /* ... */ })
    })
})
```

Variable declarations (`var user *User`) are fine inline since they're zero-value allocations. Side effects (database calls, HTTP requests, file I/O) go inside `Test` callbacks.

### Execution

Given this builder:

```go
samurai.Run(t, func(s *samurai.Scope) {
    s.Test("with database", func(ctx context.Context, w samurai.W) {
        /* setup DB */
    }, func(s *samurai.Scope) {
        s.Test("users", func(_ context.Context, w samurai.W) {
            /* create user */
        }, func(s *samurai.Scope) {
            s.Test("has email", func(_ context.Context, w samurai.W) { /* assert */ })
            s.Test("has name", func(_ context.Context, w samurai.W) { /* assert */ })
        })

        s.Test("can query", func(_ context.Context, w samurai.W) { /* assert */ })
    })
})
```

Samurai produces:

```
Builder tree:                       Discovered paths:

  Root                              1. with database/users/has email
  └── Test "with database"          2. with database/users/has name
      ├── Test "users"              3. with database/can query
      │   ├── Test "has email"
      │   └── Test "has name"       Execution (each path runs fresh):
      └── Test "can query"
                                    Path 1: setup DB → create user → assert email
                                    Path 2: setup DB → create user → assert name
                                    Path 3: setup DB → assert query
```

These become nested `t.Run` calls:

```
t.Run("with database", ...)           // intermediate scope
    t.Run("users", ...)               // intermediate scope
        t.Run("has email", ...)       // leaf - executes Path 1
        t.Run("has name", ...)        // leaf - executes Path 2
    t.Run("can query", ...)           // leaf - executes Path 3
```

Each path re-executes the full chain from root to leaf.

## Skipping tests

`s.Skip()` marks all tests in the current scope as skipped. Skipped tests appear in `go test -v` output as SKIP but their callbacks never execute. Skip propagates to all nested scopes.

```go
samurai.Run(t, func(s *samurai.Scope) {
    s.Test("working feature", func(_ context.Context, w samurai.W) {
        // this runs normally
    })

    s.Test("WIP feature", func(_ context.Context, w samurai.W) {
        // setup
    }, func(s *samurai.Scope) {
        s.Skip()
        s.Test("not ready yet", func(_ context.Context, w samurai.W) {
            // never executes
        })
    })
})
```

Call order doesn't matter — `Skip()` affects the entire scope regardless of whether `Test()` was called before or after. No callbacks, cleanups, or factory calls execute for skipped paths.

## Options

### Parallel (default)

Tests run in parallel via `t.Parallel()`. Control concurrency with:

```bash
go test -parallel 4 ./...
```

### Sequential

Force sequential execution:

```go
samurai.Run(t, func(s *samurai.Scope) {
    s.Test("first", func(_ context.Context, w samurai.W) { /* runs 1st */ })
    s.Test("second", func(_ context.Context, w samurai.W) { /* runs 2nd */ })
    s.Test("third", func(_ context.Context, w samurai.W) { /* runs 3rd */ })
}, samurai.Sequential())
```

Useful when tests modify global state or hit resources that don't handle concurrent access.

## Cleanup

Register cleanup functions with `w.Cleanup()`. They run in LIFO order, even on panic:

```go
s.Test("with resources", func(ctx context.Context, w samurai.W) {
    db := openDB(ctx)
    w.Cleanup(func() { db.Close() })  // runs second

    tx, _ := db.Begin()
    w.Cleanup(func() { tx.Rollback() })  // runs first
})
```

Cleanups run even if the test panics. Each scope has its own cleanup chain, and inner scopes clean up before outer scopes. A panicking cleanup doesn't prevent the rest from running.

```go
samurai.Run(t, func(s *samurai.Scope) {
    s.Test("with outer resource", func(_ context.Context, w samurai.W) {
        w.Cleanup(func() { /* outer: runs last */ })
    }, func(s *samurai.Scope) {
        s.Test("with inner resource", func(_ context.Context, w samurai.W) {
            w.Cleanup(func() { /* inner: runs first */ })
        }, func(s *samurai.Scope) {
            s.Test("leaf", func(_ context.Context, w samurai.W) { /* ... */ })
        })
    })
})
// cleanup order: inner → outer
```

## Custom context with RunWith

### The boilerplate problem

With assertion libraries like testify you end up writing `w.Testing()` in every leaf:

```go
s.Test("has email", func(_ context.Context, w samurai.W) {
    assert.Equal(w.Testing(), "test@example.com", user.Email)
})
```

### RunWith

`RunWith` is the generic version of `Run`. You give it a factory that builds a custom context type, and that type gets passed to all callbacks instead of `W`:

```go
package service_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/zerosixty/samurai"
)

type MyCtx struct {
    *samurai.BaseContext
    *assert.Assertions
}

type S = samurai.TestScope[*MyCtx]

func TestUserService(t *testing.T) {
    samurai.RunWith(t, func(w samurai.W) *MyCtx {
        return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
    }, func(s *S) {
        var svc *UserService

        s.Test("with service", func(ctx context.Context, c *MyCtx) {
            db := NewTestDB(ctx)
            svc = NewUserService(db)
            c.Cleanup(func() { db.Close() })
        }, func(s *S) {
            var user *User

            s.Test("create user", func(ctx context.Context, c *MyCtx) {
                var err error
                user, err = svc.Create(ctx, "samurai@example.com")
                c.NoError(err)
            }, func(s *S) {
                s.Test("has the correct email", func(_ context.Context, c *MyCtx) {
                    c.Equal("samurai@example.com", user.Email)
                })

                s.Test("has a non-zero ID", func(_ context.Context, c *MyCtx) {
                    c.NotZero(user.ID)
                })
            })

            s.Test("list empty", func(ctx context.Context, c *MyCtx) {
                users, err := svc.List(ctx)
                c.NoError(err)
                c.Empty(users)
            })
        })
    })
}
```

A few things to note:

- The factory `func(W) V` runs once per scope level with that level's `*BaseContext`
- The factory can call `w.Context()` for initialization that needs a `context.Context`
- All callbacks receive `V` instead of `W`
- `BaseContext` uses `Testing()` instead of `T()` to avoid conflicts with testify's `T()` method
- `Scope` is a type alias for `TestScope[W]`, so `Run` just delegates to `RunWith[W]`
- Go 1.24+ generic type aliases let you write `type S = TestScope[*MyCtx]`

The factory can return whatever you want. Here's one that bundles assertions with a database:

```go
type testEnv struct {
    *samurai.BaseContext
    Assert *assert.Assertions
    DB     *sql.DB
}

type S = samurai.TestScope[*testEnv]

samurai.RunWith(t, func(w samurai.W) *testEnv {
    return &testEnv{
        BaseContext: w,
        Assert:     assert.New(w.Testing()),
        DB:         openTestDB(w.Testing()),
    }
}, func(s *S) {
    s.Test("db is alive", func(ctx context.Context, env *testEnv) {
        env.Assert.NoError(env.DB.PingContext(ctx))
    })
})
```

## IDE support

Samurai emits nested `t.Run` calls, so IDE test runners and `-run` flags work as expected:

```bash
go test -run "TestUserService/with_service/create_user/has_the_correct_email" -v
```

### GoLand plugin

Install the [Samurai Test Runner](https://plugins.jetbrains.com/plugin/30391-samurai-test-runner) plugin for click-to-navigate from test results to `s.Test()` source locations, and gutter run icons with pass/fail status.

<details>
<summary>Screenshots</summary>

<img src="https://plugins.jetbrains.com/files/30391/screenshot_36503e4e-f607-469e-9f7c-aa5797de1b3c" width="100%" />
<img src="https://plugins.jetbrains.com/files/30391/screenshot_14a7448f-830a-493e-8a9a-ad3a87574f57" width="100%" />

</details>

GoLand and VS Code show the green play button next to test functions. Test output shows the full path:

```
=== RUN   TestUserService
=== RUN   TestUserService/with_service
=== RUN   TestUserService/with_service/create_user
=== RUN   TestUserService/with_service/create_user/has_the_correct_email
=== RUN   TestUserService/with_service/create_user/has_a_non-zero_ID
=== RUN   TestUserService/with_service/list_empty
--- PASS: TestUserService (0.00s)
    --- PASS: TestUserService/with_service (0.00s)
        --- PASS: TestUserService/with_service/create_user (0.00s)
            --- PASS: TestUserService/with_service/create_user/has_the_correct_email (0.00s)
            --- PASS: TestUserService/with_service/create_user/has_a_non-zero_ID (0.00s)
        --- PASS: TestUserService/with_service/list_empty (0.00s)
```

## API

```go
// Entry points
func Run(t *testing.T, builder func(*Scope), opts ...Option)
func RunWith[V Context](t *testing.T, factory func(W) V, builder func(*TestScope[V]), opts ...Option)

// Scope types
type TestScope[V Context] struct{ /* unexported */ }
type Scope = TestScope[W]

// Scope methods:
func (s *TestScope[V]) Test(name string, fn func(context.Context, V))                       // leaf
func (s *TestScope[V]) Test(name string, fn func(context.Context, V), func(*TestScope[V]))  // parent
func (s *TestScope[V]) Skip()                                                                // skip all tests in scope

// Context constraint
type Context interface {
    Testing() *testing.T
    Cleanup(func())
}

// Default test context
type BaseContext struct{ /* unexported */ }
func (b *BaseContext) Context() context.Context
func (b *BaseContext) Testing() *testing.T
func (b *BaseContext) Cleanup(fn func())

// Type aliases
type W = *BaseContext

// Options
func Sequential() Option
func Parallel() Option    // default
```

## License

MIT
