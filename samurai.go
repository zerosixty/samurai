// Package samurai provides a scoped testing framework with path isolation for Go.
//
// Samurai is inspired by GoConvey but modernized with:
//   - Explicit context passing (no GLS/goroutine-local-storage)
//   - Standard context.Context support for cancellation/timeouts
//   - Assertion-library agnostic (users bring their own)
//   - Zero external dependencies
//   - Parallel execution by default
//   - No double execution - code runs exactly once per test path
//
// # Usage
//
//	func TestDatabase(t *testing.T) {
//	    samurai.Run(t, func(s *samurai.Scope) {
//	        var db *DB
//
//	        s.Test("with database", func(ctx context.Context, w samurai.W) {
//	            db = setupDB(ctx)
//	            w.Cleanup(func() { db.Close() })
//	        }, func(s *samurai.Scope) {
//	            var user *User
//
//	            s.Test("create user", func(_ context.Context, w samurai.W) {
//	                user = db.CreateUser("test@example.com")
//	            }, func(s *samurai.Scope) {
//	                s.Test("has email", func(_ context.Context, w samurai.W) {
//	                    assert.Equal(w.Testing(), "test@example.com", user.Email)
//	                })
//
//	                s.Test("has name", func(_ context.Context, w samurai.W) {
//	                    assert.NotEmpty(w.Testing(), user.Name)
//	                })
//	            })
//
//	            s.Test("can query all", func(_ context.Context, w samurai.W) {
//	                _, err := db.QueryAll()
//	                assert.NoError(w.Testing(), err)
//	            })
//	        })
//	    })
//	}
//
// # RunWith — Generic Custom Context
//
// RunWith lets you provide a factory that creates a custom context for all callbacks.
// Embed *BaseContext in your struct to get Testing() and Cleanup() for free:
//
//	type MyCtx struct {
//	    *samurai.BaseContext
//	    *assert.Assertions
//	}
//	type S = samurai.TestScope[*MyCtx]
//
//	func TestDatabase(t *testing.T) {
//	    samurai.RunWith(t, func(w samurai.W) *MyCtx {
//	        return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
//	    }, func(s *S) {
//	        var count int
//	        s.Test("setup", func(_ context.Context, c *MyCtx) {
//	            count = 42
//	        }, func(s *S) {
//	            s.Test("check", func(_ context.Context, c *MyCtx) {
//	                c.Equal(42, count)
//	            })
//	        })
//	    })
//	}
//
// # Execution Model
//
// Samurai uses a two-phase execution model:
//  1. Discovery — the builder runs to collect test structure (no test code executes)
//  2. Execution — the builder runs fresh per leaf path (test code executes)
//
// This means:
//   - w.Testing() always returns a valid *testing.T (never nil)
//   - Each path's code runs exactly once
//   - Variables declared in builders are isolated per path
//
// # Thread Safety
//
// Samurai achieves thread safety through isolation by design:
//   - Each test path gets its own execution context
//   - The testing.T instance is per-path (Go's t.Run creates subtests)
//   - context.Context is shared within a path but immutable
//
// # IDE Integration
//
// Paths are emitted as nested t.Run calls mirroring the test tree structure.
// Each Test name creates a level in the tree.
// This enables:
//   - Running individual paths via `go test -run "TestName/Parent/Child"`
//   - Clicking on specific paths in GoLand/VS Code to run or debug them
//   - Proper test reporting per path
package samurai

import (
	"testing"
)

// Option configures the behavior of Run.
type Option func(*runConfig)

type runConfig struct {
	sequential bool
}

// Sequential forces sequential execution.
// Use this when tests require deterministic execution order.
// By default, tests run in parallel using t.Parallel().
func Sequential() Option {
	return func(cfg *runConfig) {
		cfg.sequential = true
	}
}

// Parallel explicitly enables parallel execution using t.Parallel().
// This is the default behavior, so Parallel() is only needed to override
// a previous Sequential() call or for documentation purposes.
//
// To control the number of parallel tests, use the standard Go test flag:
//
//	go test -parallel N ./...
//
// The default parallelism is GOMAXPROCS.
func Parallel() Option {
	return func(cfg *runConfig) {
		cfg.sequential = false
	}
}

// Context is the constraint for custom test context types used with RunWith.
// Any type satisfying this interface can be used as a test context.
// The simplest way to satisfy it is to embed *BaseContext.
type Context interface {
	Testing() *testing.T
	Cleanup(func())
}

// W is the default test context — a pointer to BaseContext.
// It is used as the context type for the non-generic Run entry point.
type W = *BaseContext

// identityFactory returns the BaseContext as-is. Used by Run.
func identityFactory(w W) W {
	return w
}

// RunWith executes a test using the scoped builder API with a custom factory.
// The factory creates a value of type V from *BaseContext once per test path.
// This value is passed as the second argument to all Test callbacks in that path.
//
// Use RunWith when you want callbacks to receive a custom type (e.g. an assertion helper).
// Embed *BaseContext in your custom type to satisfy the Context constraint.
//
// The factory receives a *BaseContext whose Context() method returns the scope's
// context.Context, enabling initialization that requires a context:
//
//	samurai.RunWith(t, func(w samurai.W) *MyCtx {
//	    ctx := w.Context()
//	    return &MyCtx{BaseContext: w, db: connectDB(ctx)}
//	}, builder)
//
// Example:
//
//	type MyCtx struct {
//	    *samurai.BaseContext
//	    *assert.Assertions
//	}
//	type S = samurai.TestScope[*MyCtx]
//
//	func TestDatabase(t *testing.T) {
//	    samurai.RunWith(t, func(w samurai.W) *MyCtx {
//	        return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
//	    }, func(s *S) {
//	        var count int
//	        s.Test("setup", func(_ context.Context, c *MyCtx) {
//	            count = 42
//	        }, func(s *S) {
//	            s.Test("check", func(_ context.Context, c *MyCtx) {
//	                c.Equal(42, count)
//	            })
//	        })
//	    })
//	}
func RunWith[V Context](t *testing.T, factory func(W) V, builder func(*TestScope[V]), opts ...Option) {
	if builder == nil {
		panic(&samuraiErr{message: "RunWith called with nil builder"})
	}
	if factory == nil {
		panic(&samuraiErr{message: "RunWith called with nil factory"})
	}

	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Phase 1: Collect paths by dry-running the builder in discovery mode
	paths, validationErr := collectScopedPaths(builder)
	if validationErr != nil {
		t.Fatal(validationErr)
	}

	// Phase 2: Execute paths as nested subtests for proper IDE hierarchy
	tree := buildPathTree(paths)
	executeTree[V](t, tree, builder, factory, cfg)
}

// Run executes a test using the scoped builder API.
// Variables declared in the builder function are allocated fresh per path,
// making them parallel-safe without any special framework types.
//
// The only scope method is Test:
//
//	s.Test(name, func(context.Context, W))                // leaf test
//	s.Test(name, func(context.Context, W), func(*Scope))  // parent with children
//
// Example:
//
//	func TestDatabase(t *testing.T) {
//	    samurai.Run(t, func(s *samurai.Scope) {
//	        var db *DB
//
//	        s.Test("with database", func(ctx context.Context, w samurai.W) {
//	            db = setupDB(ctx)
//	            w.Cleanup(func() { db.Close() })
//	        }, func(s *samurai.Scope) {
//	            s.Test("can query", func(_ context.Context, w samurai.W) {
//	                _, err := db.QueryAll()
//	                assert.NoError(w.Testing(), err)
//	            })
//	        })
//	    })
//	}
func Run(t *testing.T, builder func(*Scope), opts ...Option) {
	RunWith[W](t, identityFactory, builder, opts...)
}
