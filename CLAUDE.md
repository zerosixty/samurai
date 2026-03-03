# CLAUDE.md

This file contains guidance for Claude Code when working on this project.

## Project Overview

Samurai is a scoped testing framework for Go with path isolation, inspired by GoConvey but modernized with:
- Explicit context passing (no GLS/goroutine-local-storage)
- Standard context.Context support for cancellation/timeouts
- Assertion-library agnostic (users bring their own)
- Zero external dependencies
- Parallel execution by default
- No double execution - code runs exactly once per test path
- Generic `RunWith` for custom test contexts via embedded `*BaseContext` (Go 1.25+ generic type aliases)

## Git Workflow

- **NEVER push directly to master.** Always work through a PR: create a branch, push, `gh pr create`, `gh pr merge`.
- **Use `gh release create` for releases.** Never use manual `git tag` + `git push --tags`. Always create releases via `gh release create vX.Y.Z` so they appear on the GitHub Releases page.

## Build & Test Commands

```bash
go test ./...           # Run all tests
go test -v ./...        # Verbose test output
go test -race ./...     # Run with race detector
```

## Code Style

- Follow standard Go conventions
- Keep the API minimal - users control behavior via their assertion library and `t.Skip()`
- Prefer composition over inheritance
- All public types and functions must have doc comments

## Architecture

- `samurai.go` - Public API: `Run`, `RunWith[V]`, `Context` interface, `W` type alias, options (`Parallel`, `Sequential`); internal path tree building (`pathNode`, `buildPathTree`, `executeTree[V]`); `identityFactory` for the non-generic `Run`
- `scope.go` - `TestScope[V]` generic struct with `Test()` and `Skip()` methods; `type Scope = TestScope[W]` alias
- `scope_runner.go` - Generic path discovery (`collectScopedPaths[V]`, `collectPathsFromChildren`, `probeBuilder`) and execution (`executeScope[V]`, `executeScopedPath[V]`)
- `base_context.go` - `BaseContext` struct implementing `Context` interface; provides `Context()`, `Testing()` and `Cleanup()`
- `runner.go` - Shared helpers: panic recovery, cleanup execution

## API Summary

```go
// Run a test suite (simple, non-generic)
Run(t, builder func(*Scope), opts ...Option)

// Run a test suite with custom context (generic)
RunWith[V Context](t, factory func(W) V, builder func(*TestScope[V]), opts ...Option)

// Context constraint — custom types must satisfy this
type Context interface {
    Testing() *testing.T
    Cleanup(func())
}

// BaseContext is the framework-provided implementation
type BaseContext struct { /* unexported fields */ }
func (b *BaseContext) Context() context.Context { ... }
func (b *BaseContext) Testing() *testing.T      { ... }
func (b *BaseContext) Cleanup(fn func())        { ... }

// Type aliases for the simple case
type W     = *BaseContext                          // Default test context
type Scope = TestScope[W]                          // Default scope

// Generic scope builder
type TestScope[V Context] struct{ /* unexported */ }

// Scope methods
s.Test("name", func(context.Context, V))                      // leaf test
s.Test("name", func(context.Context, V), func(*TestScope[V])) // parent with children
s.Skip()                                                       // skip all tests in this scope

// Options
Sequential()  // force sequential execution
Parallel()    // explicitly enable parallel (default)
```

`context.Context` is passed as the first parameter to all `Test` callbacks, following standard Go conventions. The second parameter is the test context (`W` for simple case, custom `V` for `RunWith`).

Each leaf path executes independently with fresh parent setup.

Internally, discovered paths are grouped into a trie (`buildPathTree`) and emitted as nested `t.Run` calls (`executeTree`). `Test` names create levels in the test tree. Multiple `Test()` calls per scope are allowed — they become siblings.

### Simple Usage

```go
Run(t, func(s *Scope) {
    var db *DB

    s.Test("with database", func(ctx context.Context, w W) {
        db = setupDB(ctx)
        w.Cleanup(func() { db.Close() })
    }, func(s *Scope) {
        var user *User

        s.Test("create user", func(_ context.Context, w W) {
            user = db.CreateUser("test@example.com")
        }, func(s *Scope) {
            s.Test("has email", func(_ context.Context, w W) {
                assert.Equal(w.Testing(), "test@example.com", user.Email)
            })

            s.Test("has name", func(_ context.Context, w W) {
                assert.NotEmpty(w.Testing(), user.Name)
            })
        })

        s.Test("can query all", func(_ context.Context, w W) {
            _, err := db.QueryAll()
            assert.NoError(w.Testing(), err)
        })
    })
})
```

### Skipping Tests

`s.Skip()` marks all tests in the current scope as skipped. Skipped tests appear in output as SKIP but their callbacks never execute. Skip propagates to all nested scopes.

```go
Run(t, func(s *Scope) {
    s.Test("working feature", func(_ context.Context, w W) {
        // this runs normally
    })

    s.Test("WIP feature", fn, func(s *Scope) {
        s.Skip() // all tests below are skipped
        s.Test("not ready yet", func(_ context.Context, w W) {
            // never executes
        })
    })
})
```

Key points:
- Call order doesn't matter — `Skip()` affects the entire scope regardless of whether `Test()` was called before or after
- Skip propagates: if a parent scope is skipped, all descendants are skipped too
- Skipped paths are still discovered (appear in `go test -v` output as SKIP, not invisible)
- No callbacks execute for skipped paths — no setup, no assertions, no cleanups
- `executeScope` is never entered for skipped paths, so no factory calls or cleanup registration occurs

### Generic RunWith — Custom Test Context

`RunWith` lets you provide a factory that creates a custom context for all callbacks. Embed `*BaseContext` in your struct to get `Context()`, `Testing()` and `Cleanup()` for free. The factory can use `w.Context()` for initialization that requires a `context.Context`:

```go
type MyCtx struct {
    *samurai.BaseContext     // Testing(), Cleanup()
    *assert.Assertions      // Equal(), NotNil() — no T() conflict!
}
type S = samurai.TestScope[*MyCtx]

samurai.RunWith(t, func(w samurai.W) *MyCtx {
    return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
}, func(s *S) {
    var db *DB

    s.Test("with database", func(ctx context.Context, c *MyCtx) {
        db = setupDB(ctx)
        c.Cleanup(func() { db.Close() })
    }, func(s *S) {
        var user *User

        s.Test("create user", func(_ context.Context, c *MyCtx) {
            user = db.CreateUser("test@example.com")
        }, func(s *S) {
            s.Test("has email", func(_ context.Context, c *MyCtx) {
                c.Equal("test@example.com", user.Email)  // direct from *assert.Assertions
            })
        })
    })
})
```

Key points:
- The factory `func(W) V` is called **once per test path**. The same value is reused across all scope levels in that path.
- `w.Context()` in the factory returns the scope's `context.Context` — use it for initialization that needs a context
- All callbacks receive the custom `V` — no separate W/C distinction
- `BaseContext` uses `Testing()` instead of `T()` to avoid conflicts with testify's `T()` method
- `Scope` is a type alias for `TestScope[W]`, so `Run` delegates to `RunWith[W]` internally
- Go 1.24+ generic type aliases allow `type S = TestScope[*MyCtx]`
- The factory can return any type: assertion helpers, struct with test utilities, etc.

## Why `Test` Re-executes Per Path (Not Once)

`Test` callbacks run once per leaf path, not once per scope. This is intentional — it's the mechanism that creates path isolation. Consider:

```go
Run(t, func(s *Scope) {
    var db *DB

    s.Test("with database", func(ctx context.Context, w W) {
        db = createTestDB(ctx)
        w.Cleanup(func() { db.Drop() })
    }, func(s *Scope) {
        var user *User
        s.Test("create user", func(_ context.Context, w W) {
            user = db.CreateUser("test@example.com")
        }, func(s *Scope) {
            s.Test("has email", func(_ context.Context, w W) { ... })  // gets its own db, its own user
            s.Test("has role", func(_ context.Context, w W) { ... })   // gets its own db, its own user
        })
    })
})
```

Each leaf `Test` is a separate path. The entire builder re-runs from scratch for each path, so each gets its own `db` and `user`. If parent `Test` callbacks ran only once and sibling leaves shared the same `db`, they'd operate on the same database concurrently — breaking isolation entirely.

The re-execution is a one-time learning cost. Once users understand the execution model, the API reads naturally.

## Why There Is No `BeforeAll` / `Once`

A `BeforeAll` or `Once` that runs setup once and shares the result across sibling paths would fundamentally contradict the isolation model:
- It reintroduces shared mutable state between parallel paths
- It requires synchronization (who owns the cleanup? when does it run?)
- It breaks the mental model where every path gets fresh state

For genuinely shared infrastructure (database servers, test containers), set it up outside `Run` — at `TestMain` or test function level. The framework intentionally does not provide a mechanism that encourages sharing state between paths.

## Critical Design Constraint: Two-Phase Execution

The scoped builder API uses two phases:
1. **Discovery mode** — builder runs to collect path names (no test code executes)
2. **Execution mode** — builder runs fresh per path (test code executes)

**Setup/test code MUST be inside `Test` callbacks, never inline in the builder.** Inline code in the builder runs in BOTH phases, causing double execution:

```go
// WRONG — db.CreateUser runs during discovery AND execution
s.Test("setup", func(_ context.Context, w W) {}, func(s *Scope) {
    user := db.CreateUser("test@example.com")  // Runs twice!
    s.Test("check", func(_ context.Context, w W) { ... })
})

// CORRECT — db.CreateUser only runs in execution mode
s.Test("setup", func(_ context.Context, w W) {}, func(s *Scope) {
    var user *User
    s.Test("create user", func(_ context.Context, w W) {
        user = db.CreateUser("test@example.com")  // Runs once per path
    }, func(s *Scope) {
        s.Test("check", func(_ context.Context, w W) { ... })
    })
})
```

Any API redesign must preserve this: **user code only runs in execution mode, never during discovery.**

## GoLand Plugin (`plugin-goland/`)

### Build & Install

```bash
cd plugin-goland
./gradlew clean buildPlugin    # Build plugin JAR
./install-plugin.sh            # Install to GoLand and restart
```

Check `idea.log` (Help → Show Log in Finder) for `SamuraiTestLocator` log messages.

### Plugin Architecture

See `docs/ARCHITECTURE.md` for full details. Key components:

- **SamuraiRunConfigurationProducer** — Intercepts native GoLand "Run Test" gutter actions for samurai tests. Registered with `order="first"` in plugin.xml. Detects both `samurai.Run()` and `samurai.RunWith()` calls. This is how we redirect `func TestXxx` runs into our `SamuraiRunConfiguration`.
- **SamuraiRunConfiguration** — Extends `GoTestRunConfiguration`, overrides `newRunningState()` → `SamuraiRunningState`.
- **SamuraiRunningState** — Extends `GoTestRunningState`. The critical piece: uses `super.createConsoleInner()` (preserving native tree hierarchy), then subscribes to `SMTRunnerEventsListener.TEST_STATUS` to inject `SamuraiTestLocator` onto each test proxy as it appears.
- **SamuraiTestLocator** — PSI-based locator that resolves test names to `s.Test()` source locations. Searches for `Test()` calls and recurses into builder arguments (3rd arg) for nested segments. Falls back to `GoTestLocator` for non-samurai subtests.
- **SamuraiPathResolver** — Walks Go PSI tree to resolve `GoCallExpr` to full test path (e.g., `TestFunc/Parent/Child`). Walks up the closure chain; each parent `Test("name", fn, builder)` provides a path segment. `isRootSamuraiRun()` matches both `Run`/`*.Run` and `RunWith`/`*.RunWith`. `builderArgIndex()` returns 1 for `Run`, 2 for `RunWith`.
- **SamuraiRunLineMarkerProvider** — Gutter icons on `s.Test()` calls with pass/fail status from `SamuraiTestResultCache`.
- **SamuraiTestResultCache** — Project-level `ConcurrentHashMap<String, TestResult>` service.

### Critical Constraints (Do NOT Violate)

1. **Never replace the native console** — `GoTestConsoleProperties` is `final`. Any attempt to use custom console properties breaks the hierarchical test tree. The ONLY working approach is `super.createConsoleInner()` + per-proxy locator injection via message bus. See `docs/WRONG_DECISIONS.md` for 9 failed approaches.

2. **Never call `initConsoleView` manually** — The parent `GoTestRunningState.execute()` handles `attachToProcess`. Double attachment causes "test framework quit unexpectedly".

3. **Never enable ID-based test tree** — Go test output uses name-based tree construction. Setting `isIdBasedTestTree = true` breaks everything.

4. **Never register as `GoTestFramework`** — The framework list is hardcoded in `GoTestFramework$Lazy`. No extension point exists.

5. **Always use `GlobalSearchScope.projectScope(project)`** for file searches in `SamuraiTestLocator` — framework-provided scope is too narrow and excludes test files.

6. **Always extend `GoTestRunConfiguration`/`GoTestRunningState`** — Manual command-line execution lacks Go environment setup (GOPATH, modules, build tags).

### Plugin XML Registration

Key entries in `plugin.xml`:
- `codeInsight.lineMarkerProvider` — `SamuraiRunLineMarkerProvider`
- `configurationType` — `SamuraiConfigurationType`
- `runConfigurationProducer` — `SamuraiRunConfigurationProducer` with `order="first"`
- `notificationGroup` — "Samurai" (balloon notifications for errors)

### Debugging Tips

- `SamuraiTestLocator` logs file search counts at `LOG.info()` level and detailed tracing at `LOG.debug()` level. Enable debug logging for `com.samurai.plugin.SamuraiTestLocator` in Help → Diagnostic Tools → Debug Log Settings.
- If navigation stops working, first check that `SamuraiRunConfigurationProducer` is intercepting runs (it won't if the file doesn't contain `samurai.Run` or `samurai.RunWith`).
- If tree becomes flat, something is interfering with the native console setup in `createConsoleInner()`.

## TODO

None currently.
